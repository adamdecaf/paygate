// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/fed"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func TestDepositoryRequest(t *testing.T) {
	keeper := secrets.TestStringKeeper(t)

	req := depositoryRequest{
		keeper: keeper,
	}
	bs := []byte(`{
  "bankName": "moov, inc",
  "holder": "john doe",
  "holderType": "business",
  "type": "savings",
  "routingNumber": "123456789",
  "accountNumber": "63531",
  "metadata": "extra"
}`)
	if err := json.NewDecoder(bytes.NewReader(bs)).Decode(&req); err != nil {
		t.Fatal(err)
	}

	t.Logf("req=%#v", req)
}

func TestDepositories__HTTPCreate(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	userID := id.User(base.ID())

	fedClient := &fed.TestClient{}

	keeper := secrets.TestStringKeeper(t)
	repo := NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper)

	router := &Router{
		logger:         log.NewNopLogger(),
		fedClient:      fedClient,
		depositoryRepo: repo,
		keeper:         keeper,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{
"bankName":    "bank",
"holder":      "holder",
"holderType":  "Individual",
"type": "model.Checking",
"metadata": "extra data",
}`)
	request := httptest.NewRequest("POST", "/depositories", body)
	request.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	// Retry with full/valid request
	body = strings.NewReader(`{
"bankName":    "bank",
"holder":      "holder",
"holderType":  "Individual",
"type": "Checking",
"metadata": "extra data",
"routingNumber": "121421212",
"accountNumber": "1321"
}`)
	request = httptest.NewRequest("POST", "/depositories", body)
	request.Header.Set("x-user-id", userID.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	w.Flush()

	if w.Code != http.StatusCreated {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	t.Logf(w.Body.String())

	var depository model.Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.Status != model.DepositoryUnverified {
		t.Errorf("unexpected status: %s", depository.Status)
	}
}

func TestDepositories__HTTPCreateFedError(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	userID := id.User(base.ID())

	fedClient := &fed.TestClient{Err: errors.New("bad error")}

	keeper := secrets.TestStringKeeper(t)
	repo := NewDepositoryRepo(log.NewNopLogger(), db.DB, keeper)

	router := &Router{
		logger:         log.NewNopLogger(),
		fedClient:      fedClient,
		depositoryRepo: repo,
		keeper:         keeper,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{
"bankName":    "bank",
"holder":      "holder",
"holderType":  "Individual",
"type": "Checking",
"metadata": "extra data",
"routingNumber": "121421212",
"accountNumber": "1321"
}`)

	request := httptest.NewRequest("POST", "/depositories", body)
	request.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "problem with FED routing number lookup") {
		t.Errorf("unexpected error: %v", w.Body.String())
	}
}

func TestDepositories__HTTPCreateNoUserID(t *testing.T) {
	repo := &MockRepository{}
	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest("POST", "/depositories", body)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}
