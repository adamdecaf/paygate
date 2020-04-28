// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"database/sql"
	"fmt"

	"github.com/moov-io/ach"
	"github.com/moov-io/paygate/pkg/client"
)

type Repository interface {
	getUserTransfers(userID string, params transferFilterParams) ([]*client.Transfer, error)
	GetTransfer(id string) (*client.Transfer, error)
	UpdateTransferStatus(id string, status client.TransferStatus) error
}

func NewRepo(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

type sqlRepo struct {
	db *sql.DB
}

func (r *sqlRepo) getUserTransfers(userID string, params transferFilterParams) ([]*client.Transfer, error) {
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

	var transfers []*client.Transfer
	for i := range transferIDs {
		t, err := r.getUserTransfer(transferIDs[i], userID)
		if err == nil && t.TransferID != "" {
			transfers = append(transfers, t)
		}
	}
	return transfers, rows.Err()
}

func (r *sqlRepo) getUserTransfer(id string, userID string) (*client.Transfer, error) {
	query := `select transfer_id, amount, source_customer_id, source_account_id, destination_customer_id, destination_account_id, description, status, same_day, return_code, created_at
from transfers
where transfer_id = ? and user_id = ? and deleted_at is null
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(id, userID)

	transfer := &client.Transfer{}
	var (
		// amt        string
		returnCode *string
	)
	err = row.Scan(
		&transfer.TransferID,
		&transfer.Source.CustomerID,
		&transfer.Source.AccountID,
		&transfer.Destination.CustomerID,
		&transfer.Destination.AccountID,
		&transfer.Amount, // &amt,
		&transfer.Description,
		&transfer.Status,
		&transfer.SameDay,
		&returnCode,
		&transfer.Created,
	)
	if transfer.TransferID == "" || err != nil {
		return nil, err
	}
	if returnCode != nil {
		rc := ach.LookupReturnCode(*returnCode)
		transfer.ReturnCode = client.ReturnCode{
			Code:        rc.Code,
			Reason:      rc.Reason,
			Description: rc.Description,
		}
	}
	return transfer, nil
}

func (r *sqlRepo) GetTransfer(transferID string) (*client.Transfer, error) {
	query := `select user_id from transfers where transfer_id = ? and deleted_at is null limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	userID := ""
	if err := stmt.QueryRow(transferID).Scan(&userID); err != nil {
		return nil, err
	}
	return r.getUserTransfer(transferID, userID)
}

func (r *sqlRepo) UpdateTransferStatus(id string, status client.TransferStatus) error {
	query := `update transfers set status = ? where transfer_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(status, id)
	return err
}