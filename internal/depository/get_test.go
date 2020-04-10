// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base"
	client "github.com/moov-io/paygate/client"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func TestDepositories__HTTPGet(t *testing.T) {
	userID, now := id.User(base.ID()), time.Now()
	keeper := secrets.TestStringKeeper(t)

	depID := base.ID()
	num, _ := keeper.EncryptString("1234")
	dep := &model.Depository{
		ID:                     id.Depository(depID),
		BankName:               "bank name",
		Holder:                 "holder",
		HolderType:             model.Individual,
		Type:                   model.Checking,
		RoutingNumber:          "121421212",
		EncryptedAccountNumber: num,
		Status:                 model.DepositoryUnverified,
		Metadata:               "metadata",
		Created:                base.NewTime(now),
		Updated:                base.NewTime(now),
		Keeper:                 keeper,
	}
	repo := &MockRepository{
		Depositories: []*model.Depository{dep},
	}

	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
		keeper:         keeper,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	req := httptest.NewRequest("GET", fmt.Sprintf("/depositories/%s", dep.ID), nil)
	req.Header.Set("x-user-id", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}

	var depository client.Depository
	if err := json.NewDecoder(w.Body).Decode(&depository); err != nil {
		t.Error(err)
	}
	if depository.ID != depID {
		t.Errorf("unexpected depository: %s", depository.ID)
	}
	if depository.AccountNumber != "1234" {
		t.Errorf("AccountNumber=%s", depository.AccountNumber)
	}
	if !strings.EqualFold(string(depository.Status), "unverified") {
		t.Errorf("unexpected status: %s", depository.Status)
	}
}

func TestDepositories__HTTPGetNoUserID(t *testing.T) {
	repo := &MockRepository{}
	router := &Router{
		logger:         log.NewNopLogger(),
		depositoryRepo: repo,
	}
	r := mux.NewRouter()
	router.RegisterRoutes(r)

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest("GET", "/depositories/foo", body)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusForbidden {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}
