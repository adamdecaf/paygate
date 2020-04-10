// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"encoding/json"
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

func TestTransfers__getUserTransfer(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	repo := &SQLRepo{db.DB, log.NewNopLogger()}

	amt, _ := model.NewAmount("USD", "18.61")
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

	xfers, err := repo.CreateUserTransfers(userID, []*CreateRequest{req})
	if err != nil {
		t.Fatal(err)
	}
	if len(xfers) != 1 {
		t.Errorf("got %d transfers", len(xfers))
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", fmt.Sprintf("/transfers/%s", xfers[0].ID), nil)
	r.Header.Set("x-user-id", userID.String())

	xferRouter := CreateTestTransferRouter(nil, nil, nil, nil, nil, repo)

	router := mux.NewRouter()
	xferRouter.RegisterRoutes(router)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)

	}

	var transfer model.Transfer
	if err := json.Unmarshal(w.Body.Bytes(), &transfer); err != nil {
		t.Error(err)
	}
	if transfer.ID == "" {
		t.Fatal("failed to parse Transfer")
	}
	if v := transfer.Amount.String(); v != "USD 18.61" {
		t.Errorf("got %q", v)
	}

	fileID, _ := repo.GetFileIDForTransfer(transfer.ID, userID)
	if fileID != "test-file" {
		t.Error("no fileID found in transfers table")
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

func TestTransfers__HTTPGetNoUserID(t *testing.T) {
	xfer := CreateTestTransferRouter(nil, nil, nil, nil, nil, nil)

	router := mux.NewRouter()

	xfer.RegisterRoutes(router)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/transfers", nil)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}
