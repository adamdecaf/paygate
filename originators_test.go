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

	"github.com/moov-io/base"

	"github.com/go-kit/kit/log"
)

func TestOriginators__read(t *testing.T) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(originatorRequest{
		DefaultDepository: DepositoryID("test"),
		Identification:    "secret",
		Metadata:          "extra",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := readOriginatorRequest(&http.Request{
		Body: ioutil.NopCloser(&buf),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.DefaultDepository != "test" {
		t.Error(req.DefaultDepository)
	}
	if req.Identification != "secret" {
		t.Error(req.Identification)
	}
	if req.Metadata != "extra" {
		t.Error(req.Metadata)
	}
}

func TestOriginators__originatorRequest(t *testing.T) {
	req := originatorRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestOriginators_getUserOriginators(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	repo := &sqliteOriginatorRepo{
		db:  db.db,
		log: log.NewNopLogger(),
	}

	userId := base.ID()
	req := originatorRequest{
		DefaultDepository: "depository",
		Identification:    "secret value",
		Metadata:          "extra data",
	}
	_, err = repo.createUserOriginator(userId, req)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/originators", nil)
	r.Header.Set("x-user-id", userId)

	getUserOriginators(repo)(w, r)
	w.Flush()

	if w.Code != 200 {
		t.Errorf("got %d", w.Code)
	}

	var originators []*Originator
	if err := json.Unmarshal(w.Body.Bytes(), &originators); err != nil {
		t.Error(err)
	}
	if len(originators) != 1 {
		t.Errorf("got %d originators=%v", len(originators), originators)
	}
	if originators[0].ID == "" {
		t.Errorf("originators[0]=%v", originators[0])
	}
}

func TestOriginators_OFACMatch(t *testing.T) {
	db, err := createTestSqliteDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	depRepo := &sqliteDepositoryRepo{db.db, log.NewNopLogger()}
	origRepo := &sqliteOriginatorRepo{db.db, log.NewNopLogger()}

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

	rawBody := fmt.Sprintf(`{"defaultDepository": "%s", "identification": "test@example.com", "metadata": "Jane Doe"}`, dep.ID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/originators", strings.NewReader(rawBody))
	req.Header.Set("x-user-id", userId)

	// happy path, no OFAC match
	client := &testOFACClient{}
	createUserOriginator(client, origRepo, depRepo)(w, req)
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
	createUserOriginator(client, origRepo, depRepo)(w, req)
	w.Flush()

	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus status code: %d: %v", w.Code, w.Body.String())
	} else {
		if !strings.Contains(w.Body.String(), `ofac: blocking \"Jane Doe\"`) {
			t.Errorf("unknown error: %v", w.Body.String())
		}
	}
}
