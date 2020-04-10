// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/events"
	"github.com/moov-io/paygate/internal/gateways"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/originators"
	"github.com/moov-io/paygate/internal/receivers"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"
)

func TestTransfers__CreateRequest(t *testing.T) {
	req := CreateRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestTransfers__asTransferJSON(t *testing.T) {
	body := strings.NewReader(`{
  "transferType": "push",
  "amount": "USD 99.99",
  "originator": "32c95f289e18fb31a9a355c24ffa4ffc00a481e6",
  "originatorDepository": "ccac06454d87b6621bc62e07708ba9c342cd87ef",
  "receiver": "47c2c9e090a3417d9951eb8f0469a0d3fe7b3610",
  "receiverDepository": "8b6aadaddb25b961afd8cebbce7af306104a667c",
  "description": "Loan Pay",
  "standardEntryClassCode": "WEB",
  "sameDay": false,
  "WEBDetail": {
    "paymentInformation": "test payment",
    "paymentType": "single"
  }
}`)
	var req CreateRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	xfer := req.asTransfer(base.ID())

	// marshal the Transfer back to JSON and verify only WEBDetail was written
	bs, err := json.MarshalIndent(xfer, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bs, []byte("CCDDetail")) {
		t.Errorf("Transfer contains CCDDetail: %v", string(bs))
	}
	if bytes.Contains(bs, []byte("IATDetail")) {
		t.Errorf("Transfer contains IATDetail: %v", string(bs))
	}
	if bytes.Contains(bs, []byte("TELDetail")) {
		t.Errorf("Transfer contains TELDetail: %v", string(bs))
	}
	if !bytes.Contains(bs, []byte("WEBDetail")) {
		t.Errorf("Transfer contains WEBDetail: %v", string(bs))
	}
}

func TestTransfers__asTransfer(t *testing.T) {
	body := strings.NewReader(`{
  "transferType": "push",
  "amount": "USD 99.99",
  "originator": "32c95f289e18fb31a9a355c24ffa4ffc00a481e6",
  "originatorDepository": "ccac06454d87b6621bc62e07708ba9c342cd87ef",
  "receiver": "47c2c9e090a3417d9951eb8f0469a0d3fe7b3610",
  "receiverDepository": "8b6aadaddb25b961afd8cebbce7af306104a667c",
  "description": "Loan Pay",
  "standardEntryClassCode": "WEB",
  "sameDay": false,
  "WEBDetail": {
    "paymentInformation": "test payment",
    "paymentType": "single"
  }
}`)
	var req CreateRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	xfer := req.asTransfer(base.ID())
	if xfer.StandardEntryClassCode != "WEB" {
		t.Errorf("xfer.StandardEntryClassCode=%s", xfer.StandardEntryClassCode)
	}
	if xfer.CCDDetail != nil && xfer.CCDDetail.PaymentInformation != "" {
		t.Errorf("xfer.CCDDetail.PaymentInformation=%s", xfer.CCDDetail.PaymentInformation)
	}
	if xfer.IATDetail != nil && xfer.IATDetail.OriginatorName != "" {
		t.Errorf("xfer.IATDetail.OriginatorName=%s", xfer.IATDetail.OriginatorName)
	}
	if xfer.TELDetail != nil && xfer.TELDetail.PhoneNumber != "" {
		t.Errorf("xfer.TELDetail.PhoneNumber=%s", xfer.TELDetail.PhoneNumber)
	}
	if xfer.WEBDetail == nil || xfer.WEBDetail.PaymentInformation != "test payment" || xfer.WEBDetail.PaymentType != model.WEBSingle {
		t.Errorf("xfer.WEBDetail.PaymentInformation=%s xfer.WEBDetail.PaymentType=%s", xfer.WEBDetail.PaymentInformation, xfer.WEBDetail.PaymentType)
	}
}

// TestTransferRequest__asTransfer is a test to ensure we copy YYYDetail sub-objects properly in (CreateRequest).asTransfer(..)
func TestTransferRequest__asTransfer(t *testing.T) {
	// CCD
	req := CreateRequest{
		StandardEntryClassCode: "CCD",
		CCDDetail: &model.CCDDetail{
			PaymentInformation: "foo",
		},
	}
	xfer := req.asTransfer(base.ID())
	if xfer.CCDDetail == nil || xfer.CCDDetail.PaymentInformation != "foo" {
		t.Errorf("xfer.CCDDetail=%#v", xfer.CCDDetail)
	}

	// IAT
	req = CreateRequest{
		StandardEntryClassCode: "IAT",
		IATDetail: &model.IATDetail{
			ODFIName: "moov bank",
		},
	}
	xfer = req.asTransfer(base.ID())
	if xfer.CCDDetail != nil { // check previous case
		t.Fatal("xfer.CCDDetail=%#V", xfer.CCDDetail)
	}
	if xfer.IATDetail == nil || xfer.IATDetail.ODFIName != "moov bank" {
		t.Errorf("xfer.IATDetail=%#v", xfer.IATDetail)
	}

	// TEL
	req = CreateRequest{
		StandardEntryClassCode: "TEL",
		TELDetail: &model.TELDetail{
			PhoneNumber: "1",
			PaymentType: model.TELSingle,
		},
	}
	xfer = req.asTransfer(base.ID())
	if xfer.IATDetail != nil { // check previous case
		t.Fatal("xfer.IATDetail=%#V", xfer.IATDetail)
	}
	if xfer.TELDetail == nil || xfer.TELDetail.PhoneNumber != "1" || xfer.TELDetail.PaymentType != model.TELSingle {
		t.Errorf("xfer.TELDetail=%#v", xfer.TELDetail)
	}

	// WEB
	req = CreateRequest{
		StandardEntryClassCode: "WEB",
		WEBDetail: &model.WEBDetail{
			PaymentInformation: "bar",
			PaymentType:        model.WEBSingle,
		},
	}
	xfer = req.asTransfer(base.ID())
	if xfer.TELDetail != nil { // check previous case
		t.Fatal("xfer.TELDetail=%#V", xfer.TELDetail)
	}
	if xfer.WEBDetail == nil || xfer.WEBDetail.PaymentInformation != "bar" || xfer.WEBDetail.PaymentType != model.WEBSingle {
		t.Errorf("xfer.WEBDetail=%#v", xfer.WEBDetail)
	}
}

func TestTransfers__read(t *testing.T) {
	amt, _ := model.NewAmount("USD", "27.12")
	request := CreateRequest{
		Type:                   model.PushTransfer,
		Amount:                 *amt,
		Originator:             model.OriginatorID("originator"),
		OriginatorDepository:   id.Depository("originator"),
		Receiver:               model.ReceiverID("receiver"),
		ReceiverDepository:     id.Depository("receiver"),
		Description:            "paycheck",
		StandardEntryClassCode: "PPD",
	}
	check := func(t *testing.T, req *CreateRequest) {
		if req.Type != model.PushTransfer {
			t.Error(req.Type)
		}
		if v := req.Amount.String(); v != "USD 27.12" {
			t.Error(v)
		}
		if req.Originator != "originator" {
			t.Error(req.Originator)
		}
		if req.OriginatorDepository != "originator" {
			t.Error(req.OriginatorDepository)
		}
		if req.Receiver != "receiver" {
			t.Error(req.Receiver)
		}
		if req.ReceiverDepository != "receiver" {
			t.Error(req.ReceiverDepository)
		}
		if req.Description != "paycheck" {
			t.Error(req.Description)
		}
		if req.StandardEntryClassCode != "PPD" {
			t.Error(req.StandardEntryClassCode)
		}
	}

	// Read a single CreateRequest object
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		t.Fatal(err)
	}
	requests, err := readTransferRequests(&http.Request{
		Body: ioutil.NopCloser(&buf),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Error(requests)
	}
	check(t, requests[0])

	// Read an array of CreateRequest objects
	if err := json.NewEncoder(&buf).Encode([]CreateRequest{request}); err != nil {
		t.Fatal(err)
	}
	requests, err = readTransferRequests(&http.Request{
		Body: ioutil.NopCloser(&buf),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Error(requests)
	}
	check(t, requests[0])
}

func TestTransfers__create(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	logger := log.NewNopLogger()
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

	eventRepo := events.NewRepo(logger, db.DB)
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
	repo := &SQLRepo{db.DB, log.NewNopLogger()}

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
	router := CreateTestTransferRouter(depRepo, eventRepo, gatewayRepo, recRepo, origRepo, repo)

	router.accountsClient = nil
	router.TransferRouter.accountsClient = nil

	req, _ := http.NewRequest("POST", "/transfers", &body)
	req.Header.Set("x-user-id", "test")
	router.createUserTransfers()(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP statu codes: %d: %s", w.Code, w.Body.String())
	}

	var xfer model.Transfer
	if err := json.NewDecoder(w.Body).Decode(&xfer); err != nil {
		t.Fatal(err)
	}
	if tt, err := repo.getUserTransfer(xfer.ID, id.User("test")); tt == nil || tt.ID == "" || err != nil {
		t.Fatalf("missing Transfer=%#v error=%v", tt, err)
	}
}

func TestTransfers__createWithCustomerError(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	logger := log.NewNopLogger()
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

	eventRepo := events.NewRepo(logger, db.DB)
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
	repo := &SQLRepo{db.DB, log.NewNopLogger()}

	amt, _ := model.NewAmount("USD", "18.61")
	request := &CreateRequest{
		Type:                   model.PullTransfer,
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
	router := CreateTestTransferRouter(depRepo, eventRepo, gatewayRepo, recRepo, origRepo, repo)

	router.accountsClient = nil
	router.TransferRouter.accountsClient = nil
	router.customersClient = &customers.TestClient{
		Err: errors.New("createWithCustomerError"),
	}

	req, _ := http.NewRequest("POST", "/transfers", &body)
	req.Header.Set("x-user-id", "test")
	router.createUserTransfers()(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP statu codes: %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "verifyCustomerStatuses: originator: createWithCustomerError") {
		t.Errorf("unexpected error: %v", w.Body.String())
	}
}

func TestTransfers__HTTPCreateNoUserID(t *testing.T) {
	xfer := CreateTestTransferRouter(nil, nil, nil, nil, nil, nil)

	router := mux.NewRouter()

	xfer.RegisterRoutes(router)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/transfers", nil)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestTransfers__HTTPCreateBatchNoUserID(t *testing.T) {
	xfer := CreateTestTransferRouter(nil, nil, nil, nil, nil, nil)

	router := mux.NewRouter()

	xfer.RegisterRoutes(router)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/transfers/batch", nil)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}
