// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package limiter

import (
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

func TestOverLimit(t *testing.T) {
	if err := overLimit(-1.0, nil); err == nil {
		t.Error("expected error")
	}
}

func TestIntegration(t *testing.T) {
	t.Parallel()

	limits, err := Parse("100.00", "100.00", "250.00")
	if err != nil {
		t.Fatal(err)
	}

	check := func(t *testing.T, lc *sqlChecker) {
		userID := id.User(base.ID())

		// no transfers yet, so we're allowed
		if err := lc.AllowTransfer(userID); err != nil {
			t.Fatal(err)
		}

		// To avoid cyclic deps with the 'transfers' package we are writing this insert sql
		query := `insert into transfers (amount, user_id, created_at) values (?, ?, ?);`
		stmt, err := lc.db.Prepare(query)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()

		// write a transfer
		amt, _ := model.NewAmount("USD", "2325.12")
		if _, err := stmt.Exec(amt.String(), userID, time.Now()); err != nil {
			t.Fatal(err)
		}

		// ensure it's blocked
		if err := lc.AllowTransfer(userID); err == nil {
			t.Fatal("expected error")
		}
		total, err := lc.userTransferSum(userID, time.Now().Add(-24*time.Hour))
		if err != nil {
			t.Fatal(err)
		} else {
			if int(total*100) != 232512 {
				t.Errorf("%#v: got %.2f", lc.db, total)
			}
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()

	lc := &sqlChecker{
		db:                 sqliteDB.DB,
		logger:             log.NewNopLogger(),
		limits:             limits,
		userTransferSumSQL: sqliteSumUserTransfers,
	}

	check(t, lc)

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()

	lc.db = mysqlDB.DB
	lc.userTransferSumSQL = mysqlSumUserTransfers
	check(t, lc)
}
