// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (c *TransferRouter) getUserTransfer() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(c.logger, w, r)
		if responder == nil {
			return
		}

		transferID := GetID(r)
		transfer, err := c.transferRepo.getUserTransfer(transferID, responder.XUserID)
		if err != nil {
			responder.Log("transfers", fmt.Sprintf("error reading transfer=%s: %v", transferID, err))
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(transfer)
		})
	}
}

func (r *SQLRepo) GetTransfer(xferID id.Transfer) (*model.Transfer, error) {
	query := `select transfer_id, user_id from transfers where transfer_id = ? and deleted_at is null limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	transferID, userID := "", ""
	if err := stmt.QueryRow(xferID).Scan(&transferID, &userID); err != nil {
		return nil, err
	}
	return r.getUserTransfer(id.Transfer(transferID), id.User(userID))
}

func (r *SQLRepo) getUserTransfer(id id.Transfer, userID id.User) (*model.Transfer, error) {
	query := `select transfer_id, user_id, type, amount, originator_id, originator_depository, receiver, receiver_depository, description, standard_entry_class_code, status, same_day, return_code, created_at
from transfers
where transfer_id = ? and user_id = ? and deleted_at is null
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(id, userID)

	transfer := &model.Transfer{}
	var (
		amt        string
		returnCode *string
		created    time.Time
	)
	err = row.Scan(&transfer.ID, &transfer.UserID, &transfer.Type, &amt, &transfer.Originator, &transfer.OriginatorDepository, &transfer.Receiver, &transfer.ReceiverDepository, &transfer.Description, &transfer.StandardEntryClassCode, &transfer.Status, &transfer.SameDay, &returnCode, &created)
	if err != nil {
		return nil, err
	}
	if returnCode != nil {
		transfer.ReturnCode = ach.LookupReturnCode(*returnCode)
	}
	transfer.Created = base.NewTime(created)
	// parse Amount struct
	if err := transfer.Amount.FromString(amt); err != nil {
		return nil, err
	}
	if transfer.ID == "" {
		return nil, nil // not found
	}
	return transfer, nil
}
