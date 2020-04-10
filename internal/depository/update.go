// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (r *Router) updateUserDepository() http.HandlerFunc {
	return func(w http.ResponseWriter, httpReq *http.Request) {
		responder := route.NewResponder(r.logger, w, httpReq)
		if responder == nil {
			return
		}

		req, err := readDepositoryRequest(httpReq, r.keeper)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		depID := GetID(httpReq)
		if depID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		depository, err := r.depositoryRepo.GetUserDepository(depID, responder.XUserID)
		if err != nil {
			r.logger.Log("depositories", err.Error())
			moovhttp.Problem(w, err)
			return
		}
		if depository == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Update model
		var requireValidation bool
		if req.bankName != "" {
			depository.BankName = req.bankName
		}
		if req.holder != "" {
			depository.Holder = req.holder
		}
		if req.holderType != "" {
			depository.HolderType = req.holderType
		}
		if req.accountType != "" {
			depository.Type = req.accountType
		}
		if req.routingNumber != "" {
			if err := ach.CheckRoutingNumber(req.routingNumber); err != nil {
				responder.Problem(err)
				return
			}
			requireValidation = true
			depository.RoutingNumber = req.routingNumber
		}
		if req.accountNumber != "" {
			requireValidation = true
			// readDepositoryRequest encrypts and hashes for us
			depository.EncryptedAccountNumber = req.accountNumber
			depository.HashedAccountNumber = req.HashedAccountNumber
		}
		if req.metadata != "" {
			depository.Metadata = req.metadata
		}
		depository.Updated = base.NewTime(time.Now())

		if requireValidation {
			depository.Status = model.DepositoryUnverified
		}

		if err := depository.Validate(); err != nil {
			responder.Problem(err)
			return
		}

		if err := r.depositoryRepo.UpsertUserDepository(responder.XUserID, depository); err != nil {
			responder.Log("depositories", err.Error())
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(depository)
		})
	}
}

func (r *SQLRepo) UpsertUserDepository(userID id.User, dep *model.Depository) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	now := base.NewTime(time.Now())
	if dep.Created.IsZero() {
		dep.Created = now
		dep.Updated = now
	}

	query := `insert into depositories (depository_id, user_id, bank_name, holder, holder_type, type, routing_number, account_number_encrypted, account_number_hashed, status, metadata, created_at, last_updated_at)
values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	res, err := stmt.Exec(dep.ID, userID, dep.BankName, dep.Holder, dep.HolderType, dep.Type, dep.RoutingNumber, dep.EncryptedAccountNumber, dep.HashedAccountNumber, dep.Status, dep.Metadata, dep.Created.Time, dep.Updated.Time)
	stmt.Close()
	if err != nil && !database.UniqueViolation(err) {
		return fmt.Errorf("problem upserting depository=%q, userID=%q: %v", dep.ID, userID, err)
	}
	if res != nil {
		if n, _ := res.RowsAffected(); n != 0 {
			return tx.Commit() // Depository was inserted, so cleanup and exit
		}
	}
	query = `update depositories
set bank_name = ?, holder = ?, holder_type = ?, type = ?, routing_number = ?,
account_number_encrypted = ?, account_number_hashed = ?, status = ?, metadata = ?, last_updated_at = ?
where depository_id = ? and user_id = ? and deleted_at is null`
	stmt, err = tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		dep.BankName, dep.Holder, dep.HolderType, dep.Type, dep.RoutingNumber,
		dep.EncryptedAccountNumber, dep.HashedAccountNumber, dep.Status, dep.Metadata, time.Now(), dep.ID, userID)
	stmt.Close()
	if err != nil {
		return fmt.Errorf("UpsertUserDepository: exec error=%v rollback=%v", err, tx.Rollback())
	}
	return tx.Commit()
}

func (r *SQLRepo) UpdateDepositoryStatus(id id.Depository, status model.DepositoryStatus) error {
	query := `update depositories set status = ?, last_updated_at = ? where depository_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(status, time.Now(), id); err != nil {
		return fmt.Errorf("error updating status depository_id=%q: %v", id, err)
	}
	return nil
}
