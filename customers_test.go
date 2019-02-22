// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
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

	"github.com/go-kit/kit/log"
)

func TestCustomerStatus__json(t *testing.T) {
	cs := CustomerStatus("invalid")
	valid := map[string]CustomerStatus{
		"unverified":  CustomerUnverified,
		"verIFIed":    CustomerVerified,
		"SUSPENDED":   CustomerSuspended,
		"deactivated": CustomerDeactivated,
	}
	for k, v := range valid {
		in := []byte(fmt.Sprintf(`"%v"`, k))
		if err := json.Unmarshal(in, &cs); err != nil {
			t.Error(err.Error())
		}
		if cs != v {
			t.Errorf("got cs=%#v, v=%#v", cs, v)
		}
	}

	// make sure other values fail
	in := []byte(fmt.Sprintf(`"%v"`, base.ID()))
	if err := json.Unmarshal(in, &cs); err == nil {
		t.Error("expected error")
	}
}

func TestCustomers__read(t *testing.T) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(customerRequest{
		Email:             "test@moov.io",
		DefaultDepository: DepositoryID("test"),
		Metadata:          "extra",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := readCustomerRequest(&http.Request{
		Body: ioutil.NopCloser(&buf),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Email != "test@moov.io" {
		t.Errorf("got %s", req.Email)
	}
	if req.DefaultDepository != "test" {
		t.Errorf("got %s", req.DefaultDepository)
	}
	if req.Metadata != "extra" {
		t.Errorf("got %s", req.Metadata)
	}
}
func TestCustomers__customerRequest(t *testing.T) {
	req := customerRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestCustomers__emptyDB(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	r := &sqliteCustomerRepo{
		db:  db.db,
		log: log.NewNopLogger(),
	}

	userId := base.ID()
	if err := r.deleteUserCustomer(CustomerID(base.ID()), userId); err != nil {
		t.Errorf("expected no error, but got %v", err)
	}

	// all customers for a user
	customers, err := r.getUserCustomers(userId)
	if err != nil {
		t.Error(err)
	}
	if len(customers) != 0 {
		t.Errorf("expected empty, got %v", customers)
	}

	// specific customer
	cust, err := r.getUserCustomer(CustomerID(base.ID()), userId)
	if err != nil {
		t.Error(err)
	}
	if cust != nil {
		t.Errorf("expected empty, got %v", cust)
	}
}

func TestCustomers__upsert(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	r := &sqliteCustomerRepo{db.db, log.NewNopLogger()}
	userId := base.ID()

	cust := &Customer{
		ID:                CustomerID(base.ID()),
		Email:             "test@moov.io",
		DefaultDepository: DepositoryID(base.ID()),
		Status:            CustomerVerified,
		Metadata:          "extra data",
		Created:           base.NewTime(time.Now()),
	}
	if c, err := r.getUserCustomer(cust.ID, userId); err != nil || c != nil {
		t.Errorf("expected empty, c=%v | err=%v", c, err)
	}

	// write, then verify
	if err := r.upsertUserCustomer(userId, cust); err != nil {
		t.Error(err)
	}

	c, err := r.getUserCustomer(cust.ID, userId)
	if err != nil {
		t.Error(err)
	}
	if c.ID != cust.ID {
		t.Errorf("c.ID=%q, cust.ID=%q", c.ID, cust.ID)
	}
	if c.Email != cust.Email {
		t.Errorf("c.Email=%q, cust.Email=%q", c.Email, cust.Email)
	}
	if c.DefaultDepository != cust.DefaultDepository {
		t.Errorf("c.DefaultDepository=%q, cust.DefaultDepository=%q", c.DefaultDepository, cust.DefaultDepository)
	}
	if c.Status != cust.Status {
		t.Errorf("c.Status=%q, cust.Status=%q", c.Status, cust.Status)
	}
	if c.Metadata != cust.Metadata {
		t.Errorf("c.Metadata=%q, cust.Metadata=%q", c.Metadata, cust.Metadata)
	}
	if !c.Created.Equal(cust.Created) {
		t.Errorf("c.Created=%q, cust.Created=%q", c.Created, cust.Created)
	}

	// get all for our user
	customers, err := r.getUserCustomers(userId)
	if err != nil {
		t.Error(err)
	}
	if len(customers) != 1 {
		t.Errorf("expected one, got %v", customers)
	}
	if customers[0].ID != cust.ID {
		t.Errorf("customers[0].ID=%q, cust.ID=%q", customers[0].ID, cust.ID)
	}

	// update, verify default depository changed
	depositoryId := DepositoryID(base.ID())
	cust.DefaultDepository = depositoryId
	if err := r.upsertUserCustomer(userId, cust); err != nil {
		t.Error(err)
	}
	if cust.DefaultDepository != depositoryId {
		t.Errorf("got %q", cust.DefaultDepository)
	}
}

// TestCustomers__upsert2 uperts a Customer twice, which
// will evaluate the whole method.
func TestCustomers__upsert2(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	r := &sqliteCustomerRepo{db.db, log.NewNopLogger()}
	userId := base.ID()

	cust := &Customer{
		ID:                CustomerID(base.ID()),
		Email:             "test@moov.io",
		DefaultDepository: DepositoryID(base.ID()),
		Status:            CustomerUnverified,
		Metadata:          "extra data",
		Created:           base.NewTime(time.Now()),
	}
	if c, err := r.getUserCustomer(cust.ID, userId); err != nil || c != nil {
		t.Errorf("expected empty, c=%v | err=%v", c, err)
	}

	// initial create, then update
	if err := r.upsertUserCustomer(userId, cust); err != nil {
		t.Error(err)
	}

	cust.DefaultDepository = DepositoryID(base.ID())
	cust.Status = CustomerVerified
	if err := r.upsertUserCustomer(userId, cust); err != nil {
		t.Error(err)
	}

	c, err := r.getUserCustomer(cust.ID, userId)
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultDepository == cust.DefaultDepository {
		t.Error("DefaultDepository should have been updated")
	}
	if c.Status == cust.Status {
		t.Error("Status should have been updated")
	}
}

func TestCustomers__delete(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	r := &sqliteCustomerRepo{db.db, log.NewNopLogger()}
	userId := base.ID()

	cust := &Customer{
		ID:                CustomerID(base.ID()),
		Email:             "test@moov.io",
		DefaultDepository: DepositoryID(base.ID()),
		Status:            CustomerVerified,
		Metadata:          "extra data",
		Created:           base.NewTime(time.Now()),
	}
	if c, err := r.getUserCustomer(cust.ID, userId); err != nil || c != nil {
		t.Errorf("expected empty, c=%v | err=%v", c, err)
	}

	// write
	if err := r.upsertUserCustomer(userId, cust); err != nil {
		t.Error(err)
	}

	// verify
	c, err := r.getUserCustomer(cust.ID, userId)
	if err != nil || c == nil {
		t.Errorf("expected customer, c=%v, err=%v", c, err)
	}

	// delete
	if err := r.deleteUserCustomer(cust.ID, userId); err != nil {
		t.Error(err)
	}

	// verify tombstoned
	if c, err := r.getUserCustomer(cust.ID, userId); err != nil || c != nil {
		t.Errorf("expected empty, c=%v | err=%v", c, err)
	}
}

func TestCustomers_OFACMatch(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	custRepo := &sqliteCustomerRepo{db.db, log.NewNopLogger()}
	depRepo := &sqliteDepositoryRepo{db.db, log.NewNopLogger()}

	// Write Depository to repo
	userId := base.ID()
	dep := &Depository{
		ID:            DepositoryID(base.ID()),
		BankName:      "bank name",
		Holder:        "holder",
		HolderType:    Individual,
		Type:          Checking,
		RoutingNumber: "123",
		AccountNumber: "151",
		Status:        DepositoryUnverified,
	}
	if err := depRepo.upsertUserDepository(userId, dep); err != nil {
		t.Fatal(err)
	}

	rawBody := fmt.Sprintf(`{"defaultDepository": "%s", "email": "test@example.com", "metadata": "Jane Doe"}`, dep.ID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/customers", strings.NewReader(rawBody))
	req.Header.Set("x-user-id", userId)

	// happy path, no OFAC match
	client := &testOFACClient{}
	createUserCustomer(client, custRepo, depRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	}

	// reset and block via OFAC
	w = httptest.NewRecorder()
	client = &testOFACClient{
		err: errors.New("blocking"),
	}
	req.Body = ioutil.NopCloser(strings.NewReader(rawBody))
	createUserCustomer(client, custRepo, depRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	} else {
		if !strings.Contains(w.Body.String(), `ofac: blocking \"Jane Doe\"`) {
			t.Errorf("unknown error: %v", w.Body.String())
		}
	}
}
