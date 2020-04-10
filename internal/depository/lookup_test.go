// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"testing"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func TestDepositories__LookupDepositoryFromReturn(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo *SQLRepo) {
		userID := id.User(base.ID())
		routingNumber, accountNumber := "987654320", "152311"

		// lookup when nothing will be returned
		dep, err := repo.LookupDepositoryFromReturn(routingNumber, accountNumber)
		if dep != nil || err != nil {
			t.Fatalf("depository=%#v error=%v", dep, err)
		}

		depID := id.Depository(base.ID())
		dep = &model.Depository{
			ID:            depID,
			RoutingNumber: routingNumber,
			Type:          model.Checking,
			BankName:      "bank name",
			Holder:        "holder",
			HolderType:    model.Individual,
			Status:        model.DepositoryUnverified,
			Created:       base.NewTime(time.Now().Add(-1 * time.Second)),
			Keeper:        repo.keeper,
		}
		if err := dep.ReplaceAccountNumber(accountNumber); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpsertUserDepository(userID, dep); err != nil {
			t.Fatal(err)
		}

		// lookup again now after we wrote the Depository
		dep, err = repo.LookupDepositoryFromReturn(routingNumber, accountNumber)
		if dep == nil || err != nil {
			t.Fatalf("depository=%#v error=%v", dep, err)
		}
		if depID != dep.ID {
			t.Errorf("depID=%s dep.ID=%s", depID, dep.ID)
		}
	}

	keeper := secrets.TestStringKeeper(t)

	// SQLite
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, NewDepositoryRepo(log.NewNopLogger(), sqliteDB.DB, keeper))

	// MySQL
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, NewDepositoryRepo(log.NewNopLogger(), mysqlDB.DB, keeper))
}
