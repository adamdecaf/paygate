// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package gateways

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/pkg/id"
)

func TestGateways__HTTPUpdate(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLGatewayRepo{db.DB, log.NewNopLogger()}

	router := mux.NewRouter()
	AddRoutes(log.NewNopLogger(), router, repo)

	body := strings.NewReader(`{"origin": "987654320", "originName": "bank", "destination": "123456780", "destinationName": "other bank"}`)
	req := httptest.NewRequest("POST", "/gateways", body)
	req.Header.Set("x-user-id", base.ID())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status=%d: %v", w.Code, w.Body.String())
	}

	var wrapper struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&wrapper); err != nil {
		t.Fatal(err)
	}
	if wrapper.ID == "" {
		t.Errorf("missing ID: %v", w.Body.String())
	}
}

func TestGateways__HTTPUpdateNoUserID(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLGatewayRepo{db.DB, log.NewNopLogger()}

	router := mux.NewRouter()
	AddRoutes(log.NewNopLogger(), router, repo)

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest("POST", "/gateways", body)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status=%d: %v", w.Code, w.Body.String())
	}
}

func TestGateways__HTTPUpdateErr(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLGatewayRepo{db.DB, log.NewNopLogger()}

	router := mux.NewRouter()
	AddRoutes(log.NewNopLogger(), router, repo)

	// invalid JSON
	body := strings.NewReader(`{...}`)
	req := httptest.NewRequest("POST", "/gateways", body)
	req.Header.Set("x-user-id", base.ID())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP status=%d: %v", w.Code, w.Body.String())
	}
}

func TestGateways_update(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo Repository) {
		userID := id.User(base.ID())
		req := gatewayRequest{
			Origin:          "231380104",
			OriginName:      "my bank",
			Destination:     "031300012",
			DestinationName: "my other bank",
		}
		gateway, err := repo.updateUserGateway(userID, req)
		if err != nil {
			t.Fatal(err)
		}

		// read gateway
		gw, err := repo.GetUserGateway(userID)
		if err != nil {
			t.Fatal(err)
		}
		if gw.ID != gateway.ID {
			t.Errorf("gw.ID=%v gateway.ID=%v", gw.ID, gateway.ID)
		}

		// Update Origin
		req.Origin = "031300012"
		_, err = repo.updateUserGateway(userID, req)
		if err != nil {
			t.Fatal(err)
		}
		gw, err = repo.GetUserGateway(userID)
		if err != nil {
			t.Fatal(err)
		}
		if gw.ID != gateway.ID {
			t.Errorf("gw.ID=%v gateway.ID=%v", gw.ID, gateway.ID)
		}
		if gw.Origin != req.Origin {
			t.Errorf("gw.Origin=%v expected %v", gw.Origin, req.Origin)
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &SQLGatewayRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &SQLGatewayRepo{mysqlDB.DB, log.NewNopLogger()})
}
