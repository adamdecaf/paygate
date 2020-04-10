// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (r *Router) getUserDepositories() http.HandlerFunc {
	return func(w http.ResponseWriter, httpReq *http.Request) {
		responder := route.NewResponder(r.logger, w, httpReq)
		if responder == nil {
			return
		}

		deposits, err := r.depositoryRepo.getUserDepositories(responder.XUserID)
		if err != nil {
			responder.Log("depositories", "problem reading user depositories")
			responder.Problem(err)
			return
		}
		for i := range deposits {
			deposits[i].Keeper = r.keeper
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(deposits)
		})
	}
}

func (r *SQLRepo) getUserDepositories(userID id.User) ([]*model.Depository, error) {
	query := `select depository_id from depositories where user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var depositoryIds []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return nil, fmt.Errorf("GetUserDepositories scan: %v", err)
		}
		if row != "" {
			depositoryIds = append(depositoryIds, row)
		}
	}

	var depositories []*model.Depository
	for i := range depositoryIds {
		dep, err := r.GetUserDepository(id.Depository(depositoryIds[i]), userID)
		if err == nil && dep != nil && dep.BankName != "" {
			depositories = append(depositories, dep)
		}
	}
	return depositories, rows.Err()
}
