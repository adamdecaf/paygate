// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type mockDepositoryRepository struct {
	depositories  []*Depository
	microDeposits []microDeposit
	err           error

	cur *microDepositCursor

	// Updated fields
	status DepositoryStatus
}

func (r *mockDepositoryRepository) getUserDepositories(userID string) ([]*Depository, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.depositories, nil
}

func (r *mockDepositoryRepository) getUserDepository(id DepositoryID, userID string) (*Depository, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.depositories) > 0 {
		return r.depositories[0], nil
	}
	return nil, nil
}

func (r *mockDepositoryRepository) upsertUserDepository(userID string, dep *Depository) error {
	return r.err
}

func (r *mockDepositoryRepository) updateDepositoryStatus(id DepositoryID, status DepositoryStatus) error {
	r.status = status
	return r.err
}

func (r *mockDepositoryRepository) deleteUserDepository(id DepositoryID, userID string) error {
	return r.err
}

func (r *mockDepositoryRepository) getMicroDeposits(id DepositoryID, userID string) ([]microDeposit, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.microDeposits, nil
}

func (r *mockDepositoryRepository) initiateMicroDeposits(id DepositoryID, userID string, microDeposit []microDeposit) error {
	return r.err
}

func (r *mockDepositoryRepository) confirmMicroDeposits(id DepositoryID, userID string, amounts []Amount) error {
	return r.err
}

func (r *mockDepositoryRepository) getMicroDepositCursor(batchSize int) *microDepositCursor {
	return r.cur
}

func TestDepositories__depositoryRequest(t *testing.T) {
	req := depositoryRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestDepository__types(t *testing.T) {
	if !DepositoryStatus("").empty() {
		t.Error("expected empty")
	}
}

func TestDepositoriesHolderType__json(t *testing.T) {
	ht := HolderType("invalid")
	valid := map[string]HolderType{
		"indIVIdual": Individual,
		"Business":   Business,
	}
	for k, v := range valid {
		in := []byte(fmt.Sprintf(`"%v"`, k))
		if err := json.Unmarshal(in, &ht); err != nil {
			t.Error(err.Error())
		}
		if ht != v {
			t.Errorf("got ht=%#v, v=%#v", ht, v)
		}
	}

	// make sure other values fail
	in := []byte(fmt.Sprintf(`"%v"`, base.ID()))
	if err := json.Unmarshal(in, &ht); err == nil {
		t.Error("expected error")
	}
}

func TestDepositories__mask(t *testing.T) {
	if v := maskAccountNumber(""); v != "####" {
		t.Errorf("got %s", v)
	}
	if v := maskAccountNumber("##"); v != "####" {
		t.Errorf("got %s", v)
	}
	if v := maskAccountNumber("####"); v != "####" {
		t.Errorf("got %s", v)
	}
	if v := maskAccountNumber("12345"); v != "#2345" {
		t.Errorf("got %s", v)
	}
	if v := maskAccountNumber("123456789"); v != "#####6789" {
		t.Errorf("got %s", v)
	}
	if v := maskAccountNumber("1234567"); v != "###4567" {
		t.Errorf("got %s", v)
	}
}

func TestDepositories__read(t *testing.T) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(depositoryRequest{
		BankName:      "test",
		Holder:        "me",
		HolderType:    Individual,
		Type:          Checking,
		RoutingNumber: "123456789",
		AccountNumber: "123",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := readDepositoryRequest(&http.Request{
		Body: ioutil.NopCloser(&buf),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.BankName != "test" {
		t.Error(req.BankName)
	}
	if req.Holder != "me" {
		t.Error(req.Holder)
	}
	if req.HolderType != Individual {
		t.Error(req.HolderType)
	}
	if req.Type != Checking {
		t.Error(req.Type)
	}
	if req.RoutingNumber != "123456789" {
		t.Error(req.RoutingNumber)
	}
	if req.AccountNumber != "123" {
		t.Error(req.AccountNumber)
	}
}

func TestDepositorStatus__json(t *testing.T) {
	ht := DepositoryStatus("invalid")
	valid := map[string]DepositoryStatus{
		"Verified":   DepositoryVerified,
		"unverifieD": DepositoryUnverified,
	}
	for k, v := range valid {
		in := []byte(fmt.Sprintf(`"%v"`, k))
		if err := json.Unmarshal(in, &ht); err != nil {
			t.Error(err.Error())
		}
		if ht != v {
			t.Errorf("got ht=%#v, v=%#v", ht, v)
		}
	}

	// make sure other values fail
	in := []byte(fmt.Sprintf(`"%v"`, base.ID()))
	if err := json.Unmarshal(in, &ht); err == nil {
		t.Error("expected error")
	}
}

func TestDepositories__emptyDB(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo depositoryRepository) {
		userID := base.ID()
		if err := repo.deleteUserDepository(DepositoryID(base.ID()), userID); err != nil {
			t.Errorf("expected no error, but got %v", err)
		}

		// all depositories for a user
		deps, err := repo.getUserDepositories(userID)
		if err != nil {
			t.Error(err)
		}
		if len(deps) != 0 {
			t.Errorf("expected empty, got %v", deps)
		}

		// specific Depository
		dep, err := repo.getUserDepository(DepositoryID(base.ID()), userID)
		if err != nil {
			t.Error(err)
		}
		if dep != nil {
			t.Errorf("expected empty, got %v", dep)
		}

		// depository check
		if depositoryIdExists(userID, DepositoryID(base.ID()), repo) {
			t.Error("DepositoryId shouldn't exist")
		}
	}

	// SQLite tests
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &sqliteDepositoryRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL tests
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &sqliteDepositoryRepo{mysqlDB.DB, log.NewNopLogger()})
}

func TestDepositories__upsert(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo depositoryRepository) {
		userID := base.ID()
		keeper := testSecretKeeper(testSecretKey)

		dep := &Depository{
			ID:                  DepositoryID(base.ID()),
			BankName:            "bank name",
			Holder:              "holder",
			HolderType:          Individual,
			Type:                Checking,
			RoutingNumber:       "123",
			Status:              DepositoryVerified,
			Created:             base.NewTime(time.Now().Add(-1 * time.Second)),
			maskedAccountNumber: "#1151",
		}
		if enc, err := encryptAccountNumber(keeper, dep, "21151"); err != nil {
			t.Fatal(err)
		} else {
			dep.encryptedAccountNumber = enc
		}
		if d, err := repo.getUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		// write, then verify
		if err := repo.upsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		d, err := repo.getUserDepository(dep.ID, userID)
		if err != nil {
			t.Error(err)
		}
		if d == nil {
			t.Fatal("expected Depository, got nil")
		}
		if d.ID != dep.ID {
			t.Errorf("d.ID=%q, dep.ID=%q", d.ID, dep.ID)
		}
		if d.maskedAccountNumber != "#1151" {
			t.Errorf("bad account number: %#v", d)
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
		if err := repo.upsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}
		d, err = repo.getUserDepository(dep.ID, userID)
		if err != nil {
			t.Error(err)
		}
		if dep.BankName != d.BankName {
			t.Errorf("got %q", d.BankName)
		}
		if d.Status != DepositoryVerified {
			t.Errorf("status: %s", d.Status)
		}
		if !depositoryIdExists(userID, dep.ID, repo) {
			t.Error("DepositoryId should exist")
		}
	}

	// SQLite
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &sqliteDepositoryRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &sqliteDepositoryRepo{mysqlDB.DB, log.NewNopLogger()})
}

func TestDepositories__delete(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo depositoryRepository) {
		userID := base.ID()
		dep := &Depository{
			ID:                     DepositoryID(base.ID()),
			BankName:               "bank name",
			Holder:                 "holder",
			HolderType:             Individual,
			Type:                   Checking,
			RoutingNumber:          "123",
			Status:                 DepositoryUnverified,
			Created:                base.NewTime(time.Now().Add(-1 * time.Second)),
			encryptedAccountNumber: "foo",
			maskedAccountNumber:    base64.StdEncoding.EncodeToString([]byte("#1151")),
		}
		if d, err := repo.getUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		// write
		if err := repo.upsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		// verify
		d, err := repo.getUserDepository(dep.ID, userID)
		if err != nil || d == nil {
			t.Errorf("expected depository, d=%v, err=%v", d, err)
		}

		// delete
		if err := repo.deleteUserDepository(dep.ID, userID); err != nil {
			t.Error(err)
		}

		// verify tombstoned
		if d, err := repo.getUserDepository(dep.ID, userID); err != nil || d != nil {
			t.Errorf("expected empty, d=%v | err=%v", d, err)
		}

		if depositoryIdExists(userID, dep.ID, repo) {
			t.Error("DepositoryId shouldn't exist")
		}
	}

	// SQLite
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &sqliteDepositoryRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &sqliteDepositoryRepo{mysqlDB.DB, log.NewNopLogger()})
}

func TestDepositories__updateDepositoryStatus(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo depositoryRepository) {
		userID := base.ID()
		dep := &Depository{
			ID:                     DepositoryID(base.ID()),
			BankName:               "bank name",
			Holder:                 "holder",
			HolderType:             Individual,
			Type:                   Checking,
			RoutingNumber:          "123",
			Status:                 DepositoryUnverified,
			Created:                base.NewTime(time.Now().Add(-1 * time.Second)),
			encryptedAccountNumber: "foo",
			maskedAccountNumber:    base64.StdEncoding.EncodeToString([]byte("#1151")),
		}

		// write
		if err := repo.upsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		// upsert and read back
		if err := repo.updateDepositoryStatus(dep.ID, DepositoryVerified); err != nil {
			t.Fatal(err)
		}
		dep2, err := repo.getUserDepository(dep.ID, userID)
		if err != nil {
			t.Fatal(err)
		}
		if dep.ID != dep2.ID {
			t.Errorf("expected=%s got=%s", dep.ID, dep2.ID)
		}
		if dep2.Status != DepositoryVerified {
			t.Errorf("unknown status: %s", dep2.Status)
		}
	}

	// SQLite
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &sqliteDepositoryRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &sqliteDepositoryRepo{mysqlDB.DB, log.NewNopLogger()})
}

func TestDepositories__markApproved(t *testing.T) {
	t.Parallel()

	check := func(t *testing.T, repo depositoryRepository) {
		userID := base.ID()
		dep := &Depository{
			ID:                     DepositoryID(base.ID()),
			BankName:               "bank name",
			Holder:                 "holder",
			HolderType:             Individual,
			Type:                   Checking,
			RoutingNumber:          "123",
			Status:                 DepositoryUnverified,
			Created:                base.NewTime(time.Now().Add(-1 * time.Second)),
			encryptedAccountNumber: "foo",
			maskedAccountNumber:    base64.StdEncoding.EncodeToString([]byte("#1151")),
		}

		// write
		if err := repo.upsertUserDepository(userID, dep); err != nil {
			t.Error(err)
		}

		// read
		d, err := repo.getUserDepository(dep.ID, userID)
		if err != nil || d == nil {
			t.Errorf("expected depository, d=%v, err=%v", d, err)
		}
		if d.Status != DepositoryUnverified {
			t.Errorf("got %v", d.Status)
		}

		// Verify, then re-check
		if err := markDepositoryVerified(repo, dep.ID, userID); err != nil {
			t.Fatal(err)
		}

		d, err = repo.getUserDepository(dep.ID, userID)
		if err != nil || d == nil {
			t.Errorf("expected depository, d=%v, err=%v", d, err)
		}
		if d.Status != DepositoryVerified {
			t.Errorf("got %v", d.Status)
		}
	}

	// SQLite
	sqliteDB := database.CreateTestSqliteDB(t)
	defer sqliteDB.Close()
	check(t, &sqliteDepositoryRepo{sqliteDB.DB, log.NewNopLogger()})

	// MySQL
	mysqlDB := database.CreateTestMySQLDB(t)
	defer mysqlDB.Close()
	check(t, &sqliteDepositoryRepo{mysqlDB.DB, log.NewNopLogger()})
}

func TestDepositories_OFACMatch(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	depRepo := &sqliteDepositoryRepo{db.DB, log.NewNopLogger()}

	userID := "userID"
	request := depositoryRequest{
		BankName:      "my bank",
		Holder:        "john smith",
		HolderType:    Individual,
		Type:          Checking,
		RoutingNumber: "121042882", // real routing number
		AccountNumber: "1151",
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(request); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/depositories", &body)
	req.Header.Set("x-user-id", userID)

	// happy path, no OFAC match
	fedClient := &testFEDClient{}
	ofacClient := &testOFACClient{}
	createUserDepository(log.NewNopLogger(), testSecretKeeper(testSecretKey), fedClient, ofacClient, depRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusCreated {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	}

	// reset and block via OFAC
	w = httptest.NewRecorder()
	ofacClient = &testOFACClient{
		err: errors.New("blocking"),
	}

	// refill HTTP body
	if err := json.NewEncoder(&body).Encode(request); err != nil {
		t.Fatal(err)
	}
	req.Body = ioutil.NopCloser(&body)

	createUserDepository(log.NewNopLogger(), testSecretKeeper(testSecretKey), fedClient, ofacClient, depRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	} else {
		if !strings.Contains(w.Body.String(), `ofac: blocking \"john smith\"`) {
			t.Errorf("unknown error: %v", w.Body.String())
		}
	}
}

func TestDepositories__HTTPCreate(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	userID := base.ID()

	accountsClient := &testAccountsClient{}

	fedClient, ofacClient := &testFEDClient{}, &testOFACClient{}
	repo := &sqliteDepositoryRepo{db.DB, log.NewNopLogger()}

	testODFIAccount := makeTestODFIAccount()

	router := mux.NewRouter()
	addDepositoryRoutes(log.NewNopLogger(), router, testSecretKeeper(testSecretKey), testODFIAccount, false, accountsClient, nil, fedClient, ofacClient, repo, nil)

	req := depositoryRequest{
		BankName:   "bank",
		Holder:     "holder",
		HolderType: Individual,
		Type:       Checking,
		// Leave off to test failure
		// RoutingNumber: "121421212",
		// AccountNumber: "1321",
		Metadata: "extra data",
	}

	var body bytes.Buffer
	json.NewEncoder(&body).Encode(req)

	r := httptest.NewRequest("POST", "/depositories", &body)
	r.Header.Set("x-user-id", userID)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	// Retry with full/valid request
	req.RoutingNumber = "121421212"
	req.AccountNumber = "1321"
	json.NewEncoder(&body).Encode(req) // re-encode to bytes.Buffer

	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusCreated {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var depository Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.Status != DepositoryUnverified {
		t.Errorf("unexpected status: %s", depository.Status)
	}
}

func TestDepositories__HTTPUpdate(t *testing.T) {
	db := database.CreateTestSqliteDB(t)
	defer db.Close()

	userID, now := base.ID(), time.Now()

	repo := &sqliteDepositoryRepo{db.DB, log.NewNopLogger()}
	dep := &Depository{
		ID:                     DepositoryID(base.ID()),
		BankName:               "bank name",
		Holder:                 "holder",
		HolderType:             Individual,
		Type:                   Checking,
		RoutingNumber:          "121421212",
		Status:                 DepositoryUnverified,
		Metadata:               "metadata",
		Created:                base.NewTime(now),
		Updated:                base.NewTime(now),
		encryptedAccountNumber: "foo",
		maskedAccountNumber:    base64.StdEncoding.EncodeToString([]byte("#1321")),
	}
	if err := repo.upsertUserDepository(userID, dep); err != nil {
		t.Fatal(err)
	}
	if dep, _ := repo.getUserDepository(dep.ID, userID); dep == nil {
		t.Fatal("nil Depository")
	}

	accountsClient := &testAccountsClient{}
	testODFIAccount := makeTestODFIAccount()

	router := mux.NewRouter()
	addDepositoryRoutes(log.NewNopLogger(), router, testSecretKeeper(testSecretKey), testODFIAccount, false, accountsClient, nil, nil, nil, repo, nil)

	body := strings.NewReader(`{"accountNumber": "251i5219", "bankName": "bar", "holder": "foo", "holderType": "business", "metadata": "updated"}`)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/depositories/%s", dep.ID), body)
	req.Header.Set("x-user-id", userID)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var depository Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.Status != DepositoryUnverified {
		t.Errorf("unexpected status: %s", depository.Status)
	}
	if depository.Metadata != "updated" {
		t.Errorf("unexpected Depository metadata: %s", depository.Metadata)
	}

	// make another request
	body = strings.NewReader(`{"routingNumber": "231380104", "type": "savings"}`)
	req = httptest.NewRequest("PATCH", fmt.Sprintf("/depositories/%s", dep.ID), body)
	req.Header.Set("x-user-id", userID)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
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

func TestDepositories__HTTPGet(t *testing.T) {
	userID, now := base.ID(), time.Now()
	dep := &Depository{
		ID:                     DepositoryID(base.ID()),
		BankName:               "bank name",
		Holder:                 "holder",
		HolderType:             Individual,
		Type:                   Checking,
		RoutingNumber:          "121421212",
		Status:                 DepositoryUnverified,
		Metadata:               "metadata",
		Created:                base.NewTime(now),
		Updated:                base.NewTime(now),
		encryptedAccountNumber: "foo",
		maskedAccountNumber:    base64.StdEncoding.EncodeToString([]byte("#1321")),
	}
	repo := &mockDepositoryRepository{
		depositories: []*Depository{dep},
	}

	accountsClient := &testAccountsClient{}
	testODFIAccount := makeTestODFIAccount()

	router := mux.NewRouter()
	addDepositoryRoutes(log.NewNopLogger(), router, testSecretKeeper(testSecretKey), testODFIAccount, false, accountsClient, nil, nil, nil, repo, nil)

	req := httptest.NewRequest("GET", fmt.Sprintf("/depositories/%s", dep.ID), nil)
	req.Header.Set("x-user-id", userID)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var depository Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.ID != dep.ID {
		t.Errorf("unexpected depository: %s", depository.ID)
	}
	if depository.Status != DepositoryUnverified {
		t.Errorf("unexpected status: %s", depository.Status)
	}
}
