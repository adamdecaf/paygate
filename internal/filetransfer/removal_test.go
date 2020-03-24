// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/config"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/depository/verification/microdeposit"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/transfers"
	"github.com/moov-io/paygate/pkg/achclient"
	"github.com/moov-io/paygate/pkg/id"
)

func TestController__removeTransferErr(t *testing.T) {
	dir, err := ioutil.TempDir("", "Controller")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	repo := NewRepository("", nil, "")

	cfg := config.Empty()

	depRepo := &depository.MockRepository{}
	microDepositRepo := &microdeposit.MockRepository{}
	transferRepo := &transfers.MockRepository{}

	achClient, _, achServer := achclient.MockClientServer("", achclient.AddGetFileRoutes)
	defer achServer.Close()

	controller, err := NewController(cfg, dir, repo, depRepo, microDepositRepo, transferRepo, achClient, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := transfers.RemoveTransferRequest{
		Transfer: &model.Transfer{
			ID: id.Transfer(base.ID()),
		},
	}

	// First error condition
	transferRepo.Err = errors.New("bad error")
	err = controller.removeTransfer(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing fileID for transfer") {
		t.Errorf("unexpected error: %v", err)
	}
	transferRepo.Err = nil
	transferRepo.FileID = "fileID"

	// Third error condition
	depRepo.Err = errors.New("bad error")
	err = controller.removeTransfer(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing receiver Depository for transfer") {
		t.Errorf("unexpected error: %v", err)
	}
	depRepo.Err = nil
}

func TestController__removeBatchSingle(t *testing.T) {
	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "removeBatchSingle")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	mergable := &achFile{
		File:     file,
		filepath: filepath.Join(dir, "076401251.ach"),
	}
	if err := mergable.write(); err != nil {
		t.Fatal(err)
	}
	if err := removeBatch(mergable, "076401255655291"); err != nil {
		t.Fatal(err)
	}

	fds, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fds) != 0 {
		t.Errorf("found %d fds: %#v", len(fds), fds)
	}
}

func TestController__removeBatchMulti(t *testing.T) {
	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "return-WEB.ach"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "removeBatchMulti")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	mergable := &achFile{
		File:     file,
		filepath: filepath.Join(dir, "091400606.ach"),
	}
	if err := mergable.write(); err != nil {
		t.Fatal(err)
	}
	if err := removeBatch(mergable, "021000029461242"); err != nil {
		t.Fatal(err)
	}

	fds, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fds) != 1 {
		t.Errorf("found %d fds: %#v", len(fds), fds)
	}

	file, err = parseACHFilepath(mergable.filepath)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Batches) != 1 {
		t.Errorf("%d batches: %#v", len(file.Batches), file)
	}

	// missing TraceNumber
	if err := removeBatch(mergable, "666ff60c"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}