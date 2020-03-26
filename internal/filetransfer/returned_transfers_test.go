// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/config"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/depository/verification/microdeposit"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/transfers"
	"github.com/moov-io/paygate/pkg/id"
)

func TestController__processReturnTransfer(t *testing.T) {
	file, err := parseACHFilepath(filepath.Join("..", "..", "testdata", "return-WEB.ach"))
	if err != nil {
		t.Fatal(err)
	}
	b := file.Batches[0]

	// Force the ReturnCode to a value we want for our tests
	b.GetEntries()[0].Addenda99.ReturnCode = "R02" // "Account Closed"

	amt, _ := model.NewAmount("USD", "52.12")
	userID, transactionID := base.ID(), base.ID()

	depRepo := &depository.MockRepository{
		Depositories: []*model.Depository{
			{
				ID:                     id.Depository(base.ID()), // Don't use either DepositoryID from below
				BankName:               "my bank",
				Holder:                 "jane doe",
				HolderType:             model.Individual,
				Type:                   model.Savings,
				RoutingNumber:          file.Header.ImmediateOrigin,
				EncryptedAccountNumber: "123121",
				Status:                 model.DepositoryVerified,
				Metadata:               "other info",
			},
			{
				ID:                     id.Depository(base.ID()), // Don't use either DepositoryID from below
				BankName:               "their bank",
				Holder:                 "john doe",
				HolderType:             model.Individual,
				Type:                   model.Savings,
				RoutingNumber:          file.Header.ImmediateDestination,
				EncryptedAccountNumber: b.GetEntries()[0].DFIAccountNumber,
				Status:                 model.DepositoryVerified,
				Metadata:               "other info",
			},
		},
	}
	microDepositRepo := &microdeposit.MockRepository{}
	transferRepo := &transfers.MockRepository{
		Xfer: &model.Transfer{
			Type:                   model.PushTransfer,
			Amount:                 *amt,
			Originator:             model.OriginatorID("originator"),
			OriginatorDepository:   id.Depository("orig-depository"),
			Receiver:               model.ReceiverID("receiver"),
			ReceiverDepository:     id.Depository("rec-depository"),
			Description:            "transfer",
			StandardEntryClassCode: "PPD",
			UserID:                 userID,
			TransactionID:          transactionID,
		},
	}

	dir, _ := ioutil.TempDir("", "processReturnEntry")
	defer os.RemoveAll(dir)

	repo := NewRepository("", nil, "")

	cfg := config.Empty()
	controller, err := NewController(cfg, dir, repo, depRepo, microDepositRepo, nil, transferRepo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// transferRepo.xfer will be returned inside processReturnEntry and the Transfer path will be executed
	if err := controller.processReturnEntry(file.Header, b.GetHeader(), b.GetEntries()[0]); err != nil {
		t.Error(err)
	}

	// Check for our updated statuses
	if depRepo.Status != model.DepositoryRejected {
		t.Errorf("Depository status wasn't updated, got %v", depRepo.Status)
	}
	if transferRepo.ReturnCode != "R02" {
		t.Errorf("unexpected return code: %s", transferRepo.ReturnCode)
	}
	if transferRepo.Status != model.TransferReclaimed {
		t.Errorf("unexpected status: %v", transferRepo.Status)
	}

	// Check quick error conditions
	depRepo.Err = errors.New("bad error")
	if err := controller.processReturnEntry(file.Header, b.GetHeader(), b.GetEntries()[0]); err == nil {
		t.Error("expected error")
	}
	depRepo.Err = nil

	transferRepo.Err = errors.New("bad error")
	if err := controller.processReturnEntry(file.Header, b.GetHeader(), b.GetEntries()[0]); err == nil {
		t.Error("expected error")
	}
	transferRepo.Err = nil
}
