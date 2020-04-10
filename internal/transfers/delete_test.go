// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

func TestTransfers__HTTPDeleteNoUserID(t *testing.T) {
	xfer := CreateTestTransferRouter(nil, nil, nil, nil, nil, nil)

	router := mux.NewRouter()

	xfer.RegisterRoutes(router)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/transfers/foo", nil)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestTransfers__deleteUserTransfer(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLRepo{db.DB, log.NewNopLogger()}

	amt, _ := model.NewAmount("USD", "12.42")
	userID := id.User(base.ID())
	req := &CreateRequest{
		Type:                   model.PushTransfer,
		Amount:                 *amt,
		Originator:             model.OriginatorID("originator"),
		OriginatorDepository:   id.Depository("originator"),
		Receiver:               model.ReceiverID("receiver"),
		ReceiverDepository:     id.Depository("receiver"),
		Description:            "money",
		StandardEntryClassCode: "PPD",
		fileID:                 "test-file",
	}

	transfers, err := repo.CreateUserTransfers(userID, []*CreateRequest{req})
	if err != nil {
		t.Fatal(err)
	}
	if len(transfers) != 1 {
		t.Errorf("got %d transfers", len(transfers))
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", fmt.Sprintf("/transfers/%s", transfers[0].ID), nil)
	r.Header.Set("x-user-id", userID.String())

	xferRouter := CreateTestTransferRouter(nil, nil, nil, nil, nil, repo)

	router := mux.NewRouter()
	xferRouter.RegisterRoutes(router)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("got %d: %s", w.Code, w.Body.String())
	}

	// have our repository error and verify we get non-200's
	xferRouter.transferRepo = &MockRepository{Err: errors.New("bad error")}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}
