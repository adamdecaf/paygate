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

	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func TestGateways__HTTPGetNoUserID(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLGatewayRepo{db.DB, log.NewNopLogger()}

	router := mux.NewRouter()
	AddRoutes(log.NewNopLogger(), router, repo)

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest("GET", "/gateways", body)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status=%d: %v", w.Code, w.Body.String())
	}
}

func TestGateways_getUserGateway(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo *SQLGatewayRepo) {
		defer repo.Close()

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

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/gateways", nil)
		r.Header.Set("x-user-id", userID.String())

		getUserGateway(log.NewNopLogger(), repo)(w, r)
		w.Flush()

		if w.Code != http.StatusOK {
			t.Errorf("got %d", w.Code)
		}

		var gw *model.Gateway
		if err := json.Unmarshal(w.Body.Bytes(), &gw); err != nil {
			t.Error(err)
		}
		if gw.ID != gateway.ID {
			t.Errorf("gw.ID=%v, gateway.ID=%v", gw.ID, gateway.ID)
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, NewRepo(log.NewNopLogger(), sqliteDB.DB))

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, NewRepo(log.NewNopLogger(), mysqlDB.DB))
}
