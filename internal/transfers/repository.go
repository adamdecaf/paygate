// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/moov-io/paygate/internal"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

type Repository interface {
	getUserTransfers(userID id.User, params transferFilterParams) ([]*model.Transfer, error)
	GetTransfer(id id.Transfer) (*model.Transfer, error)
	getUserTransfer(id id.Transfer, userID id.User) (*model.Transfer, error)
	UpdateTransferStatus(id id.Transfer, status model.TransferStatus) error

	GetFileIDForTransfer(id id.Transfer, userID id.User) (string, error)
	GetTraceNumber(id id.Transfer) (string, error)

	LookupTransferFromReturn(sec string, amount *model.Amount, traceNumber string, effectiveEntryDate time.Time) (*model.Transfer, error)
	SetReturnCode(id id.Transfer, returnCode string) error

	// GetCursor returns a database cursor for Transfer objects that need to be
	// posted today.
	//
	// We currently default EffectiveEntryDate to tomorrow for any transfer and thus a
	// transfer created today needs to be posted.
	GetCursor(batchSize int, depRepo depository.Repository) *Cursor
	MarkTransferAsMerged(id id.Transfer, filename string, traceNumber string) error

	// MarkTransfersAsProcessed updates Transfers to Processed to signify they have been
	// uploaded to the ODFI. This needs to be done in one blocking operation to the caller.
	MarkTransfersAsProcessed(filename string, traceNumbers []string) (int64, error)

	CreateUserTransfers(userID id.User, requests []*CreateRequest) ([]*model.Transfer, error)
	deleteUserTransfer(id id.Transfer, userID id.User) error
}

func NewTransferRepo(logger log.Logger, db *sql.DB) *SQLRepo {
	return &SQLRepo{log: logger, db: db}
}

type SQLRepo struct {
	db  *sql.DB
	log log.Logger
}

func (r *SQLRepo) Close() error {
	return r.db.Close()
}

func (r *SQLRepo) GetFileIDForTransfer(id id.Transfer, userID id.User) (string, error) {
	query := `select file_id from transfers where transfer_id = ? and user_id = ? limit 1;`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	row := stmt.QueryRow(id, userID)

	var fileID string
	if err := row.Scan(&fileID); err != nil {
		return "", err
	}
	return fileID, nil
}

func (r *SQLRepo) GetTraceNumber(id id.Transfer) (string, error) {
	query := `select trace_number from transfers where transfer_id = ? limit 1;`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	row := stmt.QueryRow(id)

	var traceNumber *string
	if err := row.Scan(&traceNumber); err != nil {
		return "", err
	}
	if traceNumber == nil {
		return "", nil
	}
	return *traceNumber, nil
}

func (r *SQLRepo) LookupTransferFromReturn(sec string, amount *model.Amount, traceNumber string, effectiveEntryDate time.Time) (*model.Transfer, error) {
	// To match returned files we take a few values which are assumed to uniquely identify a Transfer.
	// traceNumber, per NACHA guidelines, should be globally unique (routing number + random value),
	// but we are going to filter to only select Transfers created within a few days of the EffectiveEntryDate
	// to avoid updating really old (or future, I suppose) objects.
	query := `select transfer_id, user_id, transaction_id from transfers
where standard_entry_class_code = ? and amount = ? and trace_number = ? and status = ? and (created_at > ? and created_at < ?) and deleted_at is null limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	transferId, userID, transactionID := "", "", "" // holders for 'select ..'
	min, max := internal.StartOfDayAndTomorrow(effectiveEntryDate)
	// Only include Transfer objects within 5 calendar days of the EffectiveEntryDate
	min = min.Add(-5 * 24 * time.Hour)
	max = max.Add(5 * 24 * time.Hour)

	row := stmt.QueryRow(sec, amount.String(), traceNumber, model.TransferProcessed, min, max)
	if err := row.Scan(&transferId, &userID, &transactionID); err != nil {
		return nil, err
	}

	xfer, err := r.getUserTransfer(id.Transfer(transferId), id.User(userID))
	xfer.TransactionID = transactionID
	xfer.UserID = userID
	return xfer, err
}

func (r *SQLRepo) SetReturnCode(id id.Transfer, returnCode string) error {
	query := `update transfers set return_code = ? where transfer_id = ? and return_code is null and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(returnCode, id)
	return err
}

func (r *SQLRepo) MarkTransfersAsProcessed(filename string, traceNumbers []string) (int64, error) {
	query := fmt.Sprintf(`update transfers set status = ?
where status = ? and merged_filename = ? and trace_number in (%s?) and deleted_at is null`, strings.Repeat("?, ", len(traceNumbers)-1))

	stmt, err := r.db.Prepare(query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	args := []interface{}{model.TransferProcessed, model.TransferPending, filename}
	for i := range traceNumbers {
		args = append(args, traceNumbers[i])
	}

	res, err := stmt.Exec(args...)
	if res != nil {
		n, _ := res.RowsAffected()
		return n, err
	}
	return 0, err
}
