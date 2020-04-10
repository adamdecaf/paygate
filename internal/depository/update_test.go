// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base"
	client "github.com/moov-io/paygate/client"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func TestDepositories__upsert(t *testing.T) {
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
			Status:                 model.DepositoryVerified,
			Created:                base.NewTime(time.Now().Add(-1 * time.Second)),
		}
		if d, err := repo.GetUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		// write, then verify
		if err := repo.UpsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		d, err := repo.GetUserDepository(dep.ID, userID)
		if err != nil {
			t.Error(err)
		}
		if d == nil {
			t.Fatal("expected Depository, got nil")
		}
		if d.ID != dep.ID {
			t.Errorf("d.ID=%q, dep.ID=%q", d.ID, dep.ID)
		}

		// get all for our user
		depositories, err := repo.getUserDepositories(userID)
		if err != nil {
			t.Error(err)
		}
		if len(depositories) != 1 {
			t.Errorf("expected one, got %v", depositories)
		}
		if depositories[0].ID != dep.ID {
			t.Errorf("depositories[0].ID=%q, dep.ID=%q", depositories[0].ID, dep.ID)
		}

		// update, verify default depository changed
		bankName := "my new bank"
		dep.BankName = bankName
		if err := repo.UpsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}
		d, err = repo.GetUserDepository(dep.ID, userID)
		if err != nil {
			t.Error(err)
		}
		if dep.BankName != d.BankName {
			t.Errorf("got %q", d.BankName)
		}
		if d.Status != model.DepositoryVerified {
			t.Errorf("status: %s", d.Status)
		}

		dep, err = repo.GetUserDepository(dep.ID, userID)
		if dep == nil || err != nil {
			t.Fatalf("DepositoryId should exist: %v", err)
		}
		dep, err = repo.GetDepository(dep.ID)
		if dep == nil || err != nil {
			t.Errorf("expected depository=%#v: %v", dep, err)
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

func TestDepositories__UpdateDepositoryStatus(t *testing.T) {
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

		// write
		if err := repo.UpsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		// upsert and read back
		if err := repo.UpdateDepositoryStatus(dep.ID, model.DepositoryVerified); err != nil {
			t.Fatal(err)
		}
		dep2, err := repo.GetUserDepository(dep.ID, userID)
		if err != nil {
			t.Fatal(err)
		}
		if dep.ID != dep2.ID {
			t.Errorf("expected=%s got=%s", dep.ID, dep2.ID)
		}
		if dep2.Status != model.DepositoryVerified {
			t.Errorf("unknown status: %s", dep2.Status)
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

func TestDepositories__HTTPUpdate(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	userID, now := id.User(base.ID()), time.Now()
	keeper := secrets.TestStringKeeper(t)

	repo := NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper)
	dep := &model.Depository{
		ID:            id.Depository(base.ID()),
		BankName:      "bank name",
		Holder:        "holder",
		HolderType:    model.Individual,
		Type:          model.Checking,
		RoutingNumber: "121421212",
		Status:        model.DepositoryUnverified,
		Metadata:      "metadata",
		Created:       base.NewTime(now),
		Updated:       base.NewTime(now),
		Keeper:        keeper,
	}
	if err := dep.ReplaceAccountNumber("1234"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertUserDepository(userID, dep); err != nil {
		t.Fatal(err)
	}
	if dep, _ := repo.GetUserDepository(dep.ID, userID); dep == nil {
		t.Fatal("nil Depository")
	}

	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
		keeper:         keeper,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{"accountNumber": "2515219", "bankName": "bar", "holder": "foo", "holderType": "business", "metadata": "updated"}`)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/depositories/%s", dep.ID), body)
	req.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var depository client.Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if !strings.EqualFold(string(depository.Status), "Unverified") {
		t.Errorf("unexpected status: %s", depository.Status)
	}
	if depository.Metadata != "updated" {
		t.Errorf("unexpected Depository metadata: %s", depository.Metadata)
	}

	// make another request
	body = strings.NewReader(`{"routingNumber": "231380104", "type": "savings"}`)
	req = httptest.NewRequest("PATCH", fmt.Sprintf("/depositories/%s", dep.ID), body)
	req.Header.Set("x-user-id", userID.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.RoutingNumber != "231380104" {
		t.Errorf("depository.RoutingNumber=%s", depository.RoutingNumber)
	}
}

func TestDepositories__HTTPUpdateNoUserID(t *testing.T) {
	repo := &MockRepository{}
	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest("PATCH", "/depositories/foo", body)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}
