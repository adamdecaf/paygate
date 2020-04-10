// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

func TestOriginators__HTTPPatch(t *testing.T) {
	userID := id.User(base.ID())

	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()

	origRepo := &MockRepository{
		Originators: []*model.Originator{
			{
				ID: model.OriginatorID("foo"),
			},
		},
	}

	router := mux.NewRouter()
	AddOriginatorRoutes(log.NewNopLogger(), router, nil, nil, nil, origRepo)

	body := strings.NewReader(`{"defaultDepository": "other", "identification": "baz"}`)
	req := httptest.NewRequest("PATCH", "/originators/foo", body)
	req.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}

func TestOriginators__HTTPPatchNoUserID(t *testing.T) {
	repo := &MockRepository{}

	router := mux.NewRouter()
	AddOriginatorRoutes(log.NewNopLogger(), router, nil, nil, nil, repo)

	req := httptest.NewRequest("PATCH", "/originators/foo", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}

func TestRepository_updateUserOriginator(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo Repository) {
		userID := id.User(base.ID())
		req := originatorRequest{
			DefaultDepository: "depository",
			Identification:    "secret value",
			Metadata:          "extra data",
			customerID:        "custID",
		}
		orig, err := repo.createUserOriginator(userID, req)
		if err != nil {
			t.Fatal(err)
		}
		if orig.CustomerID != "custID" {
			t.Errorf("orig.CustomerID=%s", orig.CustomerID)
		}

		// update a field
		orig.DefaultDepository = "dep2"

		if err := repo.updateUserOriginator(userID, orig); err != nil {
			t.Fatal(err)
		}

		orig, err = repo.GetUserOriginator(orig.ID, userID)
		if err != nil {
			t.Fatal(err)
		}
		if orig.DefaultDepository != "dep2" {
			t.Errorf("got Originator: %#v", orig)
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &SQLOriginatorRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &SQLOriginatorRepo{mysqlDB.DB, log.NewNopLogger()})
}
