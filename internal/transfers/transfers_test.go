// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/events"
	"github.com/moov-io/paygate/internal/gateways"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/originators"
	"github.com/moov-io/paygate/internal/receivers"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/internal/transfers/limiter"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type testTransferRouter struct {
	*TransferRouter

	accountsClient accounts.Client
}

func CreateTestTransferRouter(
	depRepo depository.Repository,
	eventRepo events.Repository,
	gatewayRepo gateways.Repository,
	recRepo receivers.Repository,
	origRepo originators.Repository,
	xfr Repository,
) *testTransferRouter {

	limits, _ := limiter.Parse(limiter.OneDay(), limiter.SevenDay(), limiter.ThirtyDay())
	var db *sql.DB
	if rr, ok := xfr.(*SQLRepo); ok {
		db = rr.db
	}
	limiter := limiter.New(log.NewNopLogger(), db, limits)

	accountsClient := &accounts.MockClient{}

	return &testTransferRouter{
		TransferRouter: &TransferRouter{
			logger:               log.NewNopLogger(),
			depRepo:              depRepo,
			eventRepo:            eventRepo,
			gatewayRepo:          gatewayRepo,
			receiverRepository:   recRepo,
			origRepo:             origRepo,
			transferRepo:         xfr,
			transferLimitChecker: limiter,
			accountsClient:       accountsClient,
		},
		accountsClient: accountsClient,
	}
}

func TestTransfers__rejectedViaLimits(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	now := base.NewTime(time.Now())
	keeper := secrets.TestStringKeeper(t)

	depRepo := &depository.MockRepository{
		Depositories: []*model.Depository{
			{
				ID:            id.Depository("originator"),
				BankName:      "orig bank",
				Holder:        "orig",
				HolderType:    model.Individual,
				Type:          model.Checking,
				RoutingNumber: "121421212",
				Status:        model.DepositoryVerified,
				Metadata:      "metadata",
				Created:       now,
				Updated:       now,
				Keeper:        keeper,
			},
			{
				ID:            id.Depository("receiver"),
				BankName:      "receiver bank",
				Holder:        "receiver",
				HolderType:    model.Individual,
				Type:          model.Checking,
				RoutingNumber: "121421212",
				Status:        model.DepositoryVerified,
				Metadata:      "metadata",
				Created:       now,
				Updated:       now,
				Keeper:        keeper,
			},
		},
	}
	depRepo.Depositories[0].ReplaceAccountNumber("1321")
	depRepo.Depositories[1].ReplaceAccountNumber("323431")

	eventRepo := events.NewRepo(log.NewNopLogger(), db.DB)
	gatewayRepo := &gateways.MockRepository{
		Gateway: &model.Gateway{
			ID: model.GatewayID(base.ID()),
		},
	}
	recRepo := &receivers.MockRepository{
		Receivers: []*model.Receiver{
			{
				ID:                model.ReceiverID("receiver"),
				Email:             "foo@moov.io",
				DefaultDepository: id.Depository("receiver"),
				Status:            model.ReceiverVerified,
				Metadata:          "other",
				Created:           now,
				Updated:           now,
			},
		},
	}
	origRepo := &originators.MockRepository{
		Originators: []*model.Originator{
			{
				ID:                model.OriginatorID("originator"),
				DefaultDepository: id.Depository("originator"),
				Identification:    "id",
				Metadata:          "other",
				Created:           now,
				Updated:           now,
			},
		},
	}
	xferRepo := NewTransferRepo(log.NewNopLogger(), db.DB)

	router := CreateTestTransferRouter(depRepo, eventRepo, gatewayRepo, recRepo, origRepo, xferRepo)

	router.accountsClient = nil
	router.TransferRouter.accountsClient = nil

	// fake like we are over the limit
	router.TransferRouter.transferLimitChecker = &limiter.MockChecker{
		Err: limiter.ErrOverLimit,
	}

	// Create our transfer
	amt, _ := model.NewAmount("USD", "18.61")
	request := &CreateRequest{
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
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(request); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/transfers", &body)
	req.Header.Set("x-user-id", "test")
	router.createUserTransfers()(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP statu codes: %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "status=reviewable") {
		t.Errorf("unexpected error: %v", w.Body.String())
	}
}

func TestTransfers__idempotency(t *testing.T) {
	// The repositories aren't used, aka idempotency check needs to be first.
	xferRouter := CreateTestTransferRouter(nil, nil, nil, nil, nil, nil)

	router := mux.NewRouter()
	xferRouter.RegisterRoutes(router)

	req := httptest.NewRequest("POST", "/transfers", nil)
	req.Header.Set("x-idempotency-key", "key")
	req.Header.Set("x-user-id", "user")

	// mark the key as seen
	if seen := route.IdempotentRecorder.SeenBefore("key"); seen {
		t.Errorf("shouldn't have been seen before")
	}

	// make our request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusPreconditionFailed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTransfers__writeResponse(t *testing.T) {
	w := httptest.NewRecorder()

	amt, _ := model.NewAmount("USD", "12.42")

	var transfers []*model.Transfer
	transfers = append(transfers, CreateRequest{
		Type:                   model.PushTransfer,
		Amount:                 *amt,
		Originator:             model.OriginatorID("originator"),
		OriginatorDepository:   id.Depository("originator"),
		Receiver:               model.ReceiverID("receiver"),
		ReceiverDepository:     id.Depository("receiver"),
		Description:            "money",
		StandardEntryClassCode: "PPD",
		fileID:                 "test-file",
	}.asTransfer(base.ID()))

	// Respond with one transfer, shouldn't be wrapped in an array
	writeResponse(log.NewNopLogger(), w, 1, transfers)
	w.Flush()

	var singleResponse model.Transfer
	if err := json.NewDecoder(w.Body).Decode(&singleResponse); err != nil {
		t.Fatal(err)
	}
	if singleResponse.ID == "" {
		t.Errorf("empty transfer: %#v", singleResponse)
	}

	// Multiple requests, so wrap with an array
	w = httptest.NewRecorder()
	writeResponse(log.NewNopLogger(), w, 2, transfers)
	w.Flush()

	var pluralResponse []model.Transfer
	if err := json.NewDecoder(w.Body).Decode(&pluralResponse); err != nil {
		t.Fatal(err)
	}
	if len(pluralResponse) != 1 {
		t.Errorf("got %d transfers", len(pluralResponse))
	}
	if pluralResponse[0].ID == "" {
		t.Errorf("empty transfer: %#v", pluralResponse[0])
	}
}

func TestTransferObjects(t *testing.T) {
	userID := id.User(base.ID())

	depID := id.Depository(base.ID())
	origID := model.OriginatorID(base.ID())
	recID := model.ReceiverID(base.ID())

	transferRepo := &MockRepository{}
	router := setupTestRouter(t, transferRepo)
	router.receiverRepo.Receivers = []*model.Receiver{
		{
			ID:                model.ReceiverID(base.ID()),
			Email:             "test@moov.io",
			DefaultDepository: id.Depository(base.ID()),
			Status:            model.ReceiverVerified,
		},
	}
	router.depositoryRepo.Depositories = nil

	rec, recDep, orig, origDep, err := router.getTransferObjects(userID, origID, depID, recID, depID)
	if err == nil || !strings.Contains(err.Error(), "receiver depository not found") {
		t.Errorf("expected error: %v", err)
	}
	if rec != nil || recDep != nil || orig != nil || origDep != nil {
		t.Errorf("receciver=%#v", rec)
		t.Errorf("receciver depository=%#v", recDep)
		t.Errorf("originator=%#v", orig)
		t.Errorf("originator depository=%#v", origDep)
	}
}
