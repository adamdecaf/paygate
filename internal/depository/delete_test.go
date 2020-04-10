// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func TestDepositories__delete(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo Repository) {
		userID := id.User(base.ID())
		dep := &model.Depository{
			ID:                     id.Depository(base.ID()),
			BankName:               "bank name",
			Holder:                 "holder",
			HolderType:             model.Individual,
			Type:                   model.Checking,
			RoutingNumber:          "123",
			EncryptedAccountNumber: "151",
			Status:                 model.DepositoryUnverified,
			Created:                base.NewTime(time.Now().Add(-1 * time.Second)),
		}
		if d, err := repo.GetUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		// write
		if err := repo.UpsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		// verify
		d, err := repo.GetUserDepository(dep.ID, userID)
		if err != nil || d == nil {
			t.Errorf("expected depository, d=%v, err=%v", d, err)
		}

		// delete
		if err := repo.deleteUserDepository(dep.ID, userID); err != nil {
			t.Error(err)
		}

		// verify tombstoned
		if d, err := repo.GetUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		dep, err = repo.GetUserDepository(dep.ID, userID)
		if dep != nil || err != nil {
			t.Errorf("dep=%#v expected none: error=%v", dep, err)
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

func TestDepositoriesHTTP__delete(t *testing.T) {
	repo := &MockRepository{}
	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	req := httptest.NewRequest("DELETE", "/depositories/foo", nil)
	req.Header.Set("x-user-id", "user")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	// sad path
	repo.Err = errors.New("bad error")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}

func TestDepositoriesHTTP__deleteNoUserID(t *testing.T) {
	repo := &MockRepository{}
	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	req := httptest.NewRequest("DELETE", "/depositories/foo", nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}
