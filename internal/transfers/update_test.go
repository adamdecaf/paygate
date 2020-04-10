// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

func TestTransfers__UpdateTransferStatus(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo Repository) {
		amt, _ := model.NewAmount("USD", "32.92")
		userID := id.User(base.ID())
		req := &CreateRequest{
			Type:                   model.PushTransfer,
			Amount:                 *amt,
			Originator:             model.OriginatorID("originator"),
			OriginatorDepository:   id.Depository("originator"),
			Receiver:               model.ReceiverID("receiver"),
			ReceiverDepository:     id.Depository("receiver"),
			Description:            "money",
			StandardEntryClassCode: "PPD",
			fileID:                 "test-file",
		}
		transfers, err := repo.CreateUserTransfers(userID, []*CreateRequest{req})
		if err != nil {
			t.Fatal(err)
		}

		if err := repo.UpdateTransferStatus(transfers[0].ID, model.TransferFailed); err != nil {
			t.Fatal(err)
		}

		xfer, err := repo.getUserTransfer(transfers[0].ID, userID)
		if err != nil {
			t.Error(err)
		}
		if xfer.Status != model.TransferFailed {
			t.Errorf("got status %s", xfer.Status)
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &SQLRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &SQLRepo{mysqlDB.DB, log.NewNopLogger()})
}
