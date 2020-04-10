// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"testing"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func TestDepositories__emptyDB(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo Repository) {
		userID := id.User(base.ID())
		if err := repo.deleteUserDepository(id.Depository(base.ID()), userID); err != nil {
			t.Errorf("expected no error, but got %v", err)
		}

		// all depositories for a user
		deps, err := repo.getUserDepositories(userID)
		if err != nil {
			t.Error(err)
		}
		if len(deps) != 0 {
			t.Errorf("expected empty, got %v", deps)
		}

		// specific Depository
		dep, err := repo.GetUserDepository(id.Depository(base.ID()), userID)
		if err != nil {
			t.Error(err)
		}
		if dep != nil {
			t.Errorf("expected empty, got %v", dep)
		}

		// depository check
		dep, err = repo.GetUserDepository(id.Depository(base.ID()), userID)
		if dep != nil {
			t.Errorf("dep=%#v expected no depository", dep)
		}
		if err != nil {
			t.Error(err)
		}

		dep, err = repo.GetDepository(id.Depository(base.ID()))
		if dep != nil || err != nil {
			t.Errorf("expected no depository: %#v: %v", dep, err)
		}
	}

	keeper := secrets.TestStringKeeper(t)

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, NewDepositoryRepo(log.NewNopLogger(), sqliteDB.DB, keeper))

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, NewDepositoryRepo(log.NewNopLogger(), mysqlDB.DB, keeper))
}
