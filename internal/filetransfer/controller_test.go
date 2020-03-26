// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/config"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/depository/verification/microdeposit"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/originators"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/internal/transfers"
	"github.com/moov-io/paygate/pkg/achclient"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type TestController struct {
	*Controller

	dir string

	repo             *mockRepository
	depRepo          *depository.MockRepository
	microDepositRepo *microdeposit.MockRepository
	transferRepo     *transfers.MockRepository

	achClient *achclient.ACH
	achServer *httptest.Server

	accountsClient accounts.Client
}

func (c *TestController) Close() {
	if c == nil {
		return
	}
	if c.achServer != nil {
		c.achServer.Close()
	}
	os.RemoveAll(c.dir)
}

func setupTestController(t *testing.T) *TestController {
	t.Helper()

	cfg := config.Empty()
	cfg.Logger = log.NewLogfmtLogger(os.Stdout)
	dir, _ := ioutil.TempDir("", "file-transfer-controller")

	repo := &mockRepository{}
	depRepo := &depository.MockRepository{}
	microDepositRepo := &microdeposit.MockRepository{}
	origRepo := &originators.MockRepository{}
	transferRepo := &transfers.MockRepository{}

	achClient, _, achServer := achclient.MockClientServer("", func(r *mux.Router) {
		achFileContentsRoute(r)
	})

	controller, err := NewController(cfg, dir, repo, depRepo, microDepositRepo, origRepo, transferRepo, achClient, nil)
	if err != nil {
		t.Fatal(err)
	}

	out := &TestController{
		Controller:       controller,
		dir:              dir,
		repo:             repo,
		depRepo:          depRepo,
		microDepositRepo: microDepositRepo,
		transferRepo:     transferRepo,
		achClient:        achClient,
		achServer:        achServer,
	}
	t.Cleanup(func() { out.Close() })
	return out
}

func TestController__configs(t *testing.T) {
	dir, err := ioutil.TempDir("", "Controller")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	repo := NewRepository("", nil, "")

	cfg := config.Empty()
	controller, err := NewController(cfg, dir, repo, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v := fmt.Sprintf("%v", controller.interval); v != "10m0s" {
		t.Errorf("interval, got %q", v)
	}
	if controller.batchSize != 100 {
		t.Errorf("batchSize: %d", controller.batchSize)
	}

	cutoffTimes, err := controller.repo.GetCutoffTimes()
	if len(cutoffTimes) != 1 || err != nil {
		t.Errorf("local len(cutoffTimes)=%d error=%v", len(cutoffTimes), err)
	}
	ftpConfigs, err := controller.repo.GetFTPConfigs()
	if len(ftpConfigs) != 1 || err != nil {
		t.Errorf("local len(ftpConfigs)=%d error=%v", len(ftpConfigs), err)
	}

	if r, ok := controller.repo.(*staticRepository); ok {
		r.protocol = "sftp" // force into SFTP mode
		r.populate()
	} else {
		t.Fatalf("got %#v", controller.repo)
	}
	sftpConfigs, err := controller.repo.GetSFTPConfigs()
	if len(sftpConfigs) != 1 || err != nil {
		t.Errorf("local len(sftpConfigs)=%d error=%v", len(sftpConfigs), err)
	}
}

func TestController__updateDepsFromNOCs(t *testing.T) {
	cases := []struct {
		value    string
		expected bool
	}{
		{"", false},
		{" true", true},
		{"yes", true},
		{"false", false},
		{"no", false},
	}
	for i := range cases {
		if got := updateDepsFromNOCs(cases[i].value); got != cases[i].expected {
			t.Errorf("input=%s expected=%v got=%v", cases[i].value, cases[i].expected, got)
		}
	}
}

func TestController__findFileTransferConfig(t *testing.T) {
	cutoff := &CutoffTime{
		RoutingNumber: "123",
		Cutoff:        1700,
		Loc:           time.UTC,
	}
	repo := &mockRepository{
		configs: []*Config{
			{RoutingNumber: "123", InboundPath: "inbound/"},
			{RoutingNumber: "321", InboundPath: "incoming/"},
		},
		ftpConfigs: []*FTPConfig{
			{RoutingNumber: "123", Hostname: "ftp.foo.com"},
			{RoutingNumber: "321", Hostname: "ftp.bar.com"},
		},
	}
	controller := &Controller{repo: repo}

	// happy path - found
	fileTransferConf := controller.findFileTransferConfig(cutoff.RoutingNumber)
	if fileTransferConf == nil {
		t.Fatalf("fileTransferConf=%v", fileTransferConf)
	}
	if fileTransferConf.InboundPath != "inbound/" {
		t.Errorf("fileTransferConf=%#v", fileTransferConf)
	}

	// not found
	fileTransferConf = controller.findFileTransferConfig("456")
	if fileTransferConf != nil {
		t.Fatalf("fileTransferConf=%v", fileTransferConf)
	}

	// error
	repo.err = errors.New("bad errors")
	if conf := controller.findFileTransferConfig("987654320"); conf != nil {
		t.Error("expected nil config")
	}
}

func TestController__findTransferType(t *testing.T) {
	controller := &Controller{
		repo: &mockRepository{},
	}

	if v := controller.findTransferType(""); v != "unknown" {
		t.Errorf("got %s", v)
	}
	if v := controller.findTransferType("987654320"); v != "unknown" {
		t.Errorf("got %s", v)
	}

	// Get 'sftp' as type
	controller = &Controller{
		repo: &mockRepository{
			sftpConfigs: []*SFTPConfig{
				{RoutingNumber: "987654320"},
			},
		},
	}
	if v := controller.findTransferType("987654320"); v != "sftp" {
		t.Errorf("got %s", v)
	}

	// 'ftp' is checked first, so let's override that now
	controller = &Controller{
		repo: &mockRepository{
			ftpConfigs: []*FTPConfig{
				{RoutingNumber: "987654320"},
			},
		},
	}
	if v := controller.findTransferType("987654320"); v != "ftp" {
		t.Errorf("got %s", v)
	}

	// error
	controller = &Controller{
		repo: &mockRepository{
			err: errors.New("bad error"),
		},
	}
	if v := controller.findTransferType("ftp"); !strings.Contains(v, "unknown: error") {
		t.Errorf("got %s", v)
	}
}

func TestController__startPeriodicFileOperations(t *testing.T) {
	// FYI, this test is more about bumping up code coverage than testing anything.
	// How the polling loop is implemented currently prevents us from inspecting much
	// about what it does.

	dir, _ := ioutil.TempDir("", "startPeriodicFileOperations")
	defer os.RemoveAll(dir)

	repo := NewRepository("", nil, "")

	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	keeper := secrets.TestStringKeeper(t)
	innerDepRepo := depository.NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper)
	microDepositRepo := &microdeposit.MockRepository{}
	microDepositRepo.Cur = &microdeposit.Cursor{
		BatchSize: 5,
		Repo:      microdeposit.NewRepository(log.NewNopLogger(), db.DB),
	}
	transferRepo := &transfers.MockRepository{
		Cur: &transfers.Cursor{
			BatchSize:    5,
			TransferRepo: transfers.NewTransferRepo(log.NewNopLogger(), db.DB),
		},
	}

	// write a micro-deposit
	amt, _ := model.NewAmount("USD", "0.22")
	if err := microDepositRepo.InitiateMicroDeposits(id.Depository("depositoryID"), "userID", []*microdeposit.Credit{{Amount: *amt, FileID: "fileID"}}); err != nil {
		t.Fatal(err)
	}

	achClient, _, achServer := achclient.MockClientServer("mergeGroupableTransfer", func(r *mux.Router) {
		achFileContentsRoute(r)
	})
	defer achServer.Close()

	// setup transfer controller to start a manual merge and upload
	cfg := config.Empty()
	controller, err := NewController(cfg, dir, repo, innerDepRepo, microDepositRepo, nil, transferRepo, achClient, nil)
	if err != nil {
		t.Fatal(err)
	}

	flushIncoming, flushOutgoing := make(FlushChan, 1), make(FlushChan, 1)
	removal := make(RemovalChan, 1)
	ctx, cancelFileSync := context.WithCancel(context.Background())

	go controller.StartPeriodicFileOperations(ctx, flushIncoming, flushOutgoing, removal) // async call to register the polling loop
	// trigger the calls
	flushIncoming <- &periodicFileOperationsRequest{}
	flushOutgoing <- &periodicFileOperationsRequest{}

	time.Sleep(250 * time.Millisecond)

	cancelFileSync()
}

func readFileAsCloser(path string) io.ReadCloser {
	fd, err := os.Open(path)
	if err != nil {
		return nil
	}
	bs, _ := ioutil.ReadAll(fd)
	return ioutil.NopCloser(bytes.NewReader(bs))
}

type mockFileTransferAgent struct {
	inboundFiles []File
	returnFiles  []File
	uploadedFile *File        // non-nil on file upload
	deletedFile  string       // filepath of last deleted file
	mu           sync.RWMutex // protects all fields
}

func (a *mockFileTransferAgent) GetInboundFiles() ([]File, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.inboundFiles, nil
}

func (a *mockFileTransferAgent) GetReturnFiles() ([]File, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.returnFiles, nil
}

func (a *mockFileTransferAgent) UploadFile(f File) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// read f.contents before callers close the underlying os.Open file descriptor
	bs, _ := ioutil.ReadAll(f.Contents)
	a.uploadedFile = &f
	a.uploadedFile.Contents = ioutil.NopCloser(bytes.NewReader(bs))
	return nil
}

func (a *mockFileTransferAgent) Delete(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.deletedFile = path
	return nil
}

func (a *mockFileTransferAgent) hostname() string {
	return "moov.io"
}

func (a *mockFileTransferAgent) InboundPath() string  { return "inbound/" }
func (a *mockFileTransferAgent) OutboundPath() string { return "outbound/" }
func (a *mockFileTransferAgent) ReturnPath() string   { return "return/" }

func (a *mockFileTransferAgent) Close() error { return nil }

func TestController__ACHFile(t *testing.T) {
	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	if file == nil {
		t.Error("nil ach.File")
	}

	dir, err := ioutil.TempDir("", "paygate")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// test writing the file
	f := &achFile{
		File:     file,
		filepath: filepath.Join(dir, "out.ach"),
	}
	if err := f.write(); err != nil {
		t.Fatal(err)
	}
	if fd, err := os.Stat(f.filepath); err != nil || fd.Size() == 0 {
		t.Fatalf("fd=%v err=%v", fd, err)
	}
	if n := f.lineCount(); n != 10 {
		t.Errorf("got %d for line count", n)
	}
}

func writeACHFile(path string) error {
	fd, err := os.Open(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		return err
	}
	defer fd.Close()
	f, err := parseACHFile(fd)
	if err != nil {
		return err
	}
	return (&achFile{
		File:     f,
		filepath: path,
	}).write()
}
