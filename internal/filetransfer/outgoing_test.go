// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/depository/verification/microdeposit"
	"github.com/moov-io/paygate/internal/filetransfer/config"
	"github.com/moov-io/paygate/internal/filetransfer/upload"
	"github.com/moov-io/paygate/internal/gateways"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/originators"
	"github.com/moov-io/paygate/internal/receivers"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/internal/transfers"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func TestController__grabAllFiles(t *testing.T) {
	// grab ACH files from our testdata directory
	files, err := grabAllFiles(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Error("no ACH files found")
	}
	for i := range files {
		if files[i].File == nil {
			t.Errorf("files[%d].filepath=%s has nil ach.File", i, files[i].filepath)
		}
	}

	// dir with an invalid ACH file
	dir, err := ioutil.TempDir("", "grabAllFilesErr")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := ioutil.WriteFile(filepath.Join(dir, "invalid.ach"), []byte("invalid ACH file contents"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err = grabAllFiles(dir)
	if len(files) != 0 || err == nil {
		t.Errorf("error=%v files=%#v", err, files)
	}
}

func TestController__filesNearTheirCutoff(t *testing.T) {
	nyc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(nyc)

	dir, err := ioutil.TempDir("", "filesNearTheirCutoff")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// use a valid file
	src, err := os.Open(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	filename, err := config.RenderACHFilename(config.DefaultFilenameTemplate, config.FilenameData{
		RoutingNumber: "987654320",
		N:             "1",
	})
	if err != nil {
		t.Fatal(err)
	}

	dst, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}

	// Setup our cutoff time to be "just head" in time
	cutoffTimes := []*config.CutoffTime{
		{
			RoutingNumber: "987654320",
			Cutoff:        (now.Hour() * 100) + now.Minute() + 1, // 1 minute in the future in HHmm
			Loc:           nyc,
		},
	}

	outFiles, err := filesNearTheirCutoff(cutoffTimes, dir)
	if err != nil {
		t.Error(err)
	}
	if len(outFiles) != 1 {
		t.Fatalf("got %d files, expected one file for upload", len(outFiles))
	}
	if outFiles[0] == nil || outFiles[0].File == nil {
		t.Fatalf("outFiles[0]=%#v", outFiles[0])
	}

	// verify the file exists (for the sad path test)
	fds, _ := ioutil.ReadDir(dir)
	if len(fds) != 1 {
		t.Errorf("got %d files", len(fds))
	}

	// bump out time ahead
	cutoffTimes[0].Cutoff += 100 // add one hour
	outFiles, err = filesNearTheirCutoff(cutoffTimes, dir)
	if err != nil {
		t.Error(err)
	}
	if len(outFiles) != 0 {
		t.Fatal("expected no files (as cutoff is far enough forward in time)")
	}
}

func TestController__mergeTransfer(t *testing.T) {
	// build a mergableFile from an example WEB entry
	webFile, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "return-WEB.ach"))
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := ioutil.TempDir("", "mergeTransfer")
	defer os.RemoveAll(dir)

	controller := &Controller{
		logger: log.NewNopLogger(),
		repo: &config.StaticRepository{
			Configs: []*config.Config{
				{
					RoutingNumber:            "091400606",
					OutboundFilenameTemplate: config.DefaultFilenameTemplate,
				},
			},
		},
		rootDir: dir,
	}

	filename, err := config.RenderACHFilename(config.DefaultFilenameTemplate, config.FilenameData{
		RoutingNumber: webFile.Header.ImmediateDestination,
		N:             "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	mergableFile := &achFile{
		File:     ach.NewFile(),
		filepath: filepath.Join(controller.mergeDir(), filename),
	}
	mergableFile.Header = ach.NewFileHeader()
	mergableFile.Header.ImmediateDestination = webFile.Header.ImmediateDestination
	mergableFile.Header.ImmediateOrigin = webFile.Header.ImmediateOrigin
	mergableFile.Header.FileCreationDate = time.Now().Format("060102")
	mergableFile.Header.ImmediateDestinationName = webFile.Header.ImmediateDestinationName
	mergableFile.Header.ImmediateOriginName = webFile.Header.ImmediateOriginName
	// Add 10000 batches to mergableFile (so it's over the LoC limit)
	for i := 0; i < 10000; i++ {
		mergableFile.AddBatch(webFile.Batches[0]) // AddBatch doesn't do unique-ness checks
	}
	if err := mergableFile.Create(); err != nil {
		t.Fatal(err)
	}
	if err := mergableFile.write(); err != nil {
		t.Fatal(err)
	}

	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	file.Header.ImmediateDestination = webFile.Header.ImmediateDestination
	file.Header.ImmediateOrigin = webFile.Header.ImmediateOrigin

	// call .mergeTransfer
	fileToUpload, err := controller.mergeTransfer(file, mergableFile)
	if err != nil {
		t.Fatal(err)
	}

	expected := fmt.Sprintf("%s-091400606-1.ach", time.Now().Format("20060102"))
	if v := filepath.Base(fileToUpload.filepath); v != expected {
		t.Errorf("got=%q expected=%q", v, expected)
	}

	// grab the latest mergable file and verify it's '*-2.ach'
	mergableFile, err = controller.grabLatestMergedACHFile(webFile.Header.ImmediateDestination, file)
	if err != nil {
		t.Fatal(err)
	}
	expected = fmt.Sprintf("%s-091400606-2.ach", time.Now().Format("20060102"))
	if v := filepath.Base(mergableFile.filepath); v != expected {
		t.Errorf("got=%q expected=%q", v, expected)
	}
}

func TestController__mergeGroupableTransfer(t *testing.T) {
	dir, err := ioutil.TempDir("", "mergeGroupableTransfer")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	controller := setupTestController(t)
	controller.makeTransfer(id.Transfer(base.ID()))
	// controller := &Controller{
	// 	logger: log.NewNopLogger(),
	// 	repo: &config.StaticRepository{
	// 		Configs: []*config.Config{
	// 			{
	// 				RoutingNumber:            "076401251",
	// 				OutboundFilenameTemplate: config.DefaultFilenameTemplate,
	// 			},
	// 		},
	// 	},
	// 	rootDir: dir,
	// 	transferRepo: &transfers.MockRepository{
	// 		FileID: "foo", // some non-empty value, our test ACH server doesn't care
	// 	},
	// }

	xfer := &transfers.GroupableTransfer{
		Transfer: &model.Transfer{
			ID: id.Transfer(base.ID()),
		},
		Destination: "076401251", // from testdata/ppd-debit.ach
	}

	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	if fileToUpload := controller.mergeGroupableTransfer(xfer); fileToUpload != nil {
		t.Errorf("didn't expect fileToUpload=%v", fileToUpload)
	}

	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}

	// check our mergable files
	mergableFile, err := controller.grabLatestMergedACHFile(xfer.Destination, file)
	if err != nil {
		t.Fatal(err)
	}

	// verify the file exists
	if len(mergableFile.Batches) != 1 {
		t.Errorf("len(mergableFile.Batches)=%d", len(mergableFile.Batches))
	}
	if !mergableFile.Batches[0].Equal(file.Batches[0]) {
		t.Errorf("Batches aren't equal!")
	}
}

func TestController__mergeMicroDeposit(t *testing.T) {
	dir, err := ioutil.TempDir("", "mergeMicroDeposit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	keeper := secrets.TestStringKeeper(t)
	microDepositRepo := microdeposit.NewRepository(log.NewNopLogger(), db.DB)
	controller := &Controller{
		logger: log.NewNopLogger(),
		repo: &config.StaticRepository{
			Configs: []*config.Config{
				{
					RoutingNumber:            "987654320",
					OutboundFilenameTemplate: config.DefaultFilenameTemplate,
				},
			},
		},
		rootDir: dir,
		depRepo: depository.NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper),
		gatewayRepo: &gateways.MockRepository{
			Gateway: &model.Gateway{
				ID:              model.GatewayID(base.ID()),
				Origin:          "987654320",
				OriginName:      "My Bank",
				Destination:     "123456780", // TODO(adam): use valid routing number
				DestinationName: "Their Bank",
			},
		},
		microDepositRepo:   microDepositRepo,
		origRepo:           &originators.MockRepository{},
		receiverRepository: &receivers.MockRepository{},
		transferRepo:       &transfers.MockRepository{},
	}

	// Setup our micro-deposit
	amt, _ := model.NewAmount("USD", "0.22")
	mc := microdeposit.UploadableCredit{
		DepositoryID: "depositoryID",
		UserID:       "userID",
		FileID:       "fileID",
		Amount:       amt,
	}
	if err := controller.microDepositRepo.InitiateMicroDeposits(id.Depository("depositoryID"), "userID", []*microdeposit.Credit{{Amount: *amt, FileID: "fileID"}}); err != nil {
		t.Fatal(err)
	}

	// write a Depository
	if err := controller.depRepo.UpsertUserDepository("userID", &model.Depository{
		ID:            "depositoryID",
		BankName:      "Mooc, Inc",
		RoutingNumber: "987654320",
	}); err != nil {
		t.Fatal(err)
	}

	if fileToUpload := controller.mergeMicroDeposit(mc); fileToUpload != nil {
		t.Errorf("didn't expect an ACH file to upload: %#v", fileToUpload)
	}

	mergedFilename, err := microdeposit.ReadMergedFilename(microDepositRepo, amt, id.Depository(mc.DepositoryID))
	if err != nil {
		t.Fatal(err)
	}
	if v := fmt.Sprintf("%s-076401251-1.ach", time.Now().Format("20060102")); mergedFilename != v {
		t.Errorf("got mergedFilename=%s expected=%s", mergedFilename, v)
	}
}

func TestController__startUploadError(t *testing.T) {
	dir, err := ioutil.TempDir("", "startUploadError")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	nyc, _ := time.LoadLocation("America/New_York")
	controller := &Controller{
		logger: log.NewNopLogger(),
		repo: &config.StaticRepository{
			CutoffTimes: []*config.CutoffTime{
				{
					RoutingNumber: "987654320",
					Cutoff:        1700,
					Loc:           nyc,
				},
			},
			Configs: []*config.Config{
				{
					RoutingNumber: "987654320",
					OutboundPath:  "outbound/",
				},
			},
		},
		rootDir:      dir,
		transferRepo: &transfers.MockRepository{},
	}

	// Setup our test file for upload
	file := ach.NewFile()
	file.Header = ach.NewFileHeader()
	file.Header.ImmediateOrigin = "987654320"

	var filesToUpload = []*achFile{
		{File: file, filepath: "/dev/null"}, // invalid filepath
	}

	if err := controller.startUpload(filesToUpload); err == nil {
		t.Error("expected error")
	}
}

func TestController__uploadFile(t *testing.T) {
	agent := &upload.MockAgent{}
	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "uploadFile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	controller := &Controller{
		logger:  log.NewNopLogger(),
		rootDir: dir,
	}
	if err := controller.uploadFile(agent, &achFile{File: file, filepath: filepath.Join("..", "..", "testdata", "ppd-debit.ach")}); err != nil {
		t.Error(err)
	}

	if agent.UploadedFile == nil {
		t.Fatal("nil agent.uploadedFile")
	}
	if v := agent.UploadedFile.Filename; v != "ppd-debit.ach" {
		t.Errorf("got %v", v)
	}
	if bs, err := ioutil.ReadAll(agent.UploadedFile.Contents); len(bs) == 0 || err != nil {
		t.Errorf("copied empty file: %v", err)
	}
}

func TestController__groupTransfers(t *testing.T) {
	transfers := []*transfers.GroupableTransfer{
		{
			Transfer: &model.Transfer{
				ID: "1",
			},
			Destination: "123456789",
		},
		{
			Transfer: &model.Transfer{
				ID: "2",
			},
			Destination: "123456789",
		},
		{
			Transfer: &model.Transfer{
				ID: "3",
			},
			Destination: "987654321",
		},
	}
	grouped, err := groupTransfers(transfers, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped) != 2 {
		t.Fatalf("len(grouped)=%d", len(grouped))
	}
	first, second := grouped[0], grouped[1]
	if first[0].ID != "1" && first[1].ID != "2" {
		t.Errorf("first[0].ID=%s first[1].ID=%s", first[0].ID, first[1].ID)
	}
	if second[0].ID != "3" {
		t.Errorf("second[0].ID=%s", second[0].ID)
	}

	// ensure we error if err != nil
	grouped, err = groupTransfers(transfers, errors.New("test error"))
	if err == nil || len(grouped) != 0 {
		t.Errorf("expected error but got none, len(grouped)=%d", len(grouped))
	}
}

func TestController__grabLatestMergedACHFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "grabLatestMergedACHFile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	mergeDir := filepath.Join(dir, "merged")
	if err := os.Mkdir(mergeDir, 0777); err != nil {
		t.Fatal(err)
	}
	origin, destination := "076401251", "076401251" // yea, these are the same in ppd-debit.ach

	// write two files under achFilename (same routingNumber, diff seq)
	filename, _ := config.RenderACHFilename(config.DefaultFilenameTemplate, config.FilenameData{RoutingNumber: destination, N: "1"})
	if err := writeACHFile(filepath.Join(mergeDir, filename)); err != nil { // writes ppd-debit.ach as a new name
		t.Fatal(err)
	}
	filename, _ = config.RenderACHFilename(config.DefaultFilenameTemplate, config.FilenameData{RoutingNumber: destination, N: "2"})
	if err := writeACHFile(filepath.Join(mergeDir, filename)); err != nil {
		t.Fatal(err)
	}
	controller := &Controller{
		logger: log.NewNopLogger(),
		repo: &config.StaticRepository{
			Configs: []*config.Config{
				{
					RoutingNumber:            origin,
					OutboundFilenameTemplate: config.DefaultFilenameTemplate,
				},
			},
		},
		rootDir: dir,
	}

	incoming, _ := parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	file, err := controller.grabLatestMergedACHFile(origin, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil {
		t.Fatal("nil achFile")
	}
	if expected := filepath.Join(mergeDir, filename); file.filepath != expected {
		t.Errorf("got %q expected %q", file.filepath, expected)
	}

	// Then look for a new ABA and ensure we get a new achFile created
	incoming, _ = parseACHFilepath(filepath.Join("..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	incoming.Header.ImmediateOrigin = "432156784" // random routing number
	incoming.Header.ImmediateOriginName = "origin bank"
	incoming.Header.ImmediateDestination = "987654320"
	incoming.Header.ImmediateDestinationName = "destination bank"
	incoming.Header.FileCreationDate = time.Now().Format("060102") // YYMMDD
	incoming.Header.FileCreationTime = time.Now().Format("1504")   // HHMM
	if err := incoming.Create(); err != nil {
		t.Fatal(err)
	}

	// Add a new file_transfer_config
	controller = &Controller{
		logger: log.NewNopLogger(),
		repo: &config.StaticRepository{
			Configs: []*config.Config{
				{
					RoutingNumber: incoming.Header.ImmediateDestination,
				},
			},
		},
		rootDir: dir,
	}
	file, err = controller.grabLatestMergedACHFile(incoming.Header.ImmediateDestination, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil {
		t.Fatal("nil achFile")
	}
	filename, _ = config.RenderACHFilename(config.DefaultFilenameTemplate, config.FilenameData{
		RoutingNumber: "987654320",
		N:             "1",
	})
	if expected := filepath.Join(mergeDir, filename); file.filepath != expected {
		t.Errorf("got %q expected %q", file.filepath, expected)
	}
}

func TestController__mergeDir(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "mergeDir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	controller := &Controller{
		rootDir: tempDir,
	}

	dir := controller.mergeDir()
	if dir == "" {
		t.Fatal("empty dir")
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
