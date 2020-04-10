// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"
)

func TestOriginators__HTTPPost(t *testing.T) {
	userID := id.User(base.ID())

	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()

	keeper := secrets.TestStringKeeper(t)
	depRepo := depository.NewDepositoryRepo(log.NewNopLogger(), sqliteDB.DB, keeper)
	origRepo := &MockRepository{}

	if err := depRepo.UpsertUserDepository(userID, &model.Depository{
		ID:            id.Depository("foo"),
		RoutingNumber: "987654320",
		Type:          model.Checking,
		BankName:      "bank name",
		Holder:        "holder",
		HolderType:    model.Individual,
		Status:        model.DepositoryUnverified,
		Created:       base.NewTime(time.Now().Add(-1 * time.Second)),
		Keeper:        keeper,
	}); err != nil {
		t.Fatal(err)
	}

	router := mux.NewRouter()
	AddOriginatorRoutes(log.NewNopLogger(), router, nil, nil, depRepo, origRepo)

	body := strings.NewReader(`{"defaultDepository": "foo", "identification": "baz", "metadata": "other"}`)
	req := httptest.NewRequest("POST", "/originators", body)
	req.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var wrapper model.Originator
	if err := json.NewDecoder(w.Body).Decode(&wrapper); err != nil {
		t.Fatal(err)
	}
	if wrapper.ID != "" {
		t.Errorf("wrapper.ID=%s", wrapper.ID)
	}
}

func TestOriginators__HTTPPostNoUserID(t *testing.T) {
	repo := &MockRepository{}

	router := mux.NewRouter()
	AddOriginatorRoutes(log.NewNopLogger(), router, nil, nil, nil, repo)

	req := httptest.NewRequest("POST", "/originators", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}

func TestOriginators_CustomersError(t *testing.T) {
	logger := log.NewNopLogger()

	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	keeper := secrets.TestStringKeeper(t)
	depRepo := depository.NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper)
	origRepo := &SQLOriginatorRepo{db.DB, log.NewNopLogger()}

	// Write Depository to repo
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
	}
	if err := depRepo.UpsertUserDepository(userID, dep); err != nil {
		t.Fatal(err)
	}

	rawBody := fmt.Sprintf(`{"defaultDepository": "%s", "identification": "test@example.com", "metadata": "Jane Doe"}`, dep.ID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/originators", strings.NewReader(rawBody))
	req.Header.Set("x-user-id", userID.String())

	// happy path
	accountsClient := &accounts.MockClient{
		Accounts: []accounts.Account{
			{
				ID:            base.ID(),
				AccountNumber: dep.EncryptedAccountNumber,
				RoutingNumber: dep.RoutingNumber,
				Type:          "Checking",
			},
		},
	}

	customersClient := &customers.TestClient{}
	createUserOriginator(logger, accountsClient, customersClient, depRepo, origRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	}

	// reset and block
	w = httptest.NewRecorder()
	customersClient = &customers.TestClient{
		Err: errors.New("bad error"),
	}
	req.Body = ioutil.NopCloser(strings.NewReader(rawBody))

	createUserOriginator(logger, accountsClient, customersClient, depRepo, origRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	}
}
