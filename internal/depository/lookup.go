// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"database/sql"
	"fmt"

	"github.com/moov-io/paygate/internal/hash"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

func (r *SQLRepo) LookupDepositoryFromReturn(routingNumber string, accountNumber string) (*model.Depository, error) {
	hash, err := hash.AccountNumber(accountNumber)
	if err != nil {
		return nil, err
	}
	// order by created_at to ignore older rows with non-null deleted_at's
	query := `select depository_id, user_id from depositories where routing_number = ? and account_number_hashed = ? and deleted_at is null order by created_at desc limit 1;`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	depID, userID := "", ""
	if err := stmt.QueryRow(routingNumber, hash).Scan(&depID, &userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("LookupDepositoryFromReturn: %v", err)
	}
	return r.GetUserDepository(id.Depository(depID), id.User(userID))
}
