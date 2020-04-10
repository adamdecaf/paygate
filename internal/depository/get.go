// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/depository/verification/microdeposit/returns"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (r *Router) getUserDepository() http.HandlerFunc {
	return func(w http.ResponseWriter, httpReq *http.Request) {
		responder := route.NewResponder(r.logger, w, httpReq)
		if responder == nil {
			return
		}

		depID := GetID(httpReq)
		if depID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		depository, err := r.depositoryRepo.GetUserDepository(depID, responder.XUserID)
		if err != nil {
			responder.Log("depositories", err.Error())
			responder.Problem(err)
			return
		}
		if depository != nil {
			depository.Keeper = r.keeper
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(depository)
		})
	}
}

func (r *SQLRepo) GetDepository(depID id.Depository) (*model.Depository, error) {
	query := `select user_id from depositories where depository_id = ? and deleted_at is null limit 1;`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var userID string
	if err := stmt.QueryRow(depID).Scan(&userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if userID == "" {
		return nil, nil // not found
	}

	dep, err := r.GetUserDepository(depID, id.User(userID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return dep, err
}

func (r *SQLRepo) GetUserDepository(id id.Depository, userID id.User) (*model.Depository, error) {
	query := `select depository_id, bank_name, holder, holder_type, type, routing_number, account_number_encrypted, account_number_hashed, status, metadata, created_at, last_updated_at
from depositories
where depository_id = ? and user_id = ? and deleted_at is null
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("GetUserDepository: prepare: %v", err)
	}
	defer stmt.Close()

	row := stmt.QueryRow(id, userID)

	dep := &model.Depository{UserID: userID}
	var (
		created time.Time
		updated time.Time
	)
	err = row.Scan(&dep.ID, &dep.BankName, &dep.Holder, &dep.HolderType, &dep.Type, &dep.RoutingNumber, &dep.EncryptedAccountNumber, &dep.HashedAccountNumber, &dep.Status, &dep.Metadata, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetUserDepository: scan: %v", err)
	}
	dep.ReturnCodes = returns.FromMicroDeposits(r.db, dep.ID)
	dep.Created = base.NewTime(created)
	dep.Updated = base.NewTime(updated)
	if dep.ID == "" || dep.BankName == "" {
		return nil, nil // no records found
	}
	dep.Keeper = r.keeper
	return dep, nil
}
