// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/internal/util"
	"github.com/moov-io/paygate/pkg/id"
)

type transferFilterParams struct {
	Status    model.TransferStatus
	StartDate time.Time
	EndDate   time.Time
	Limit     int64
	Offset    int64
}

func readTransferFilterParams(r *http.Request) transferFilterParams {
	params := transferFilterParams{
		StartDate: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Now().Add(24 * time.Hour),
		Limit:     100,
		Offset:    0,
	}
	if r == nil {
		return params
	}
	q := r.URL.Query()
	if v := q.Get("startDate"); v != "" {
		params.StartDate = util.FirstParsedTime(v, base.ISO8601Format, util.YYMMDDTimeFormat)
	}
	if v := q.Get("endDate"); v != "" {
		params.EndDate, _ = time.Parse(base.ISO8601Format, v)
	}
	if status := model.TransferStatus(q.Get("status")); status.Validate() == nil {
		params.Status = status
	}
	if limit := route.ReadLimit(r); limit != 0 {
		params.Limit = limit
	}
	if offset := route.ReadOffset(r); offset != 0 {
		params.Offset = offset
	}
	return params
}

func (c *TransferRouter) getUserTransfers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(c.logger, w, r)
		if responder == nil {
			return
		}

		params := readTransferFilterParams(r)
		transfers, err := c.transferRepo.getUserTransfers(responder.XUserID, params)
		if err != nil {
			responder.Log("transfers", fmt.Sprintf("error getting user transfers: %v", err))
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(transfers)
		})
	}
}

func (r *SQLRepo) getUserTransfers(userID id.User, params transferFilterParams) ([]*model.Transfer, error) {
	var statusQuery string
	if string(params.Status) != "" {
		statusQuery = "and status = ?"
	}
	query := fmt.Sprintf(`select transfer_id from transfers
where user_id = ? and created_at >= ? and created_at <= ? and deleted_at is null %s
order by created_at desc limit ? offset ?;`, statusQuery)
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	args := []interface{}{userID, params.StartDate, params.EndDate, params.Limit, params.Offset}
	if statusQuery != "" {
		args = append(args, params.Status)
	}
	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transferIDs []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return nil, fmt.Errorf("getUserTransfers scan: %v", err)
		}
		if row != "" {
			transferIDs = append(transferIDs, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getUserTransfers: rows.Err=%v", err)
	}

	var transfers []*model.Transfer
	for i := range transferIDs {
		t, err := r.getUserTransfer(id.Transfer(transferIDs[i]), userID)
		if err == nil && t.ID != "" {
			transfers = append(transfers, t)
		}
	}
	return transfers, rows.Err()
}
