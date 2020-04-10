// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
)

func TestDepositories__depositoryRequest(t *testing.T) {
	req := depositoryRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestDepositories__read(t *testing.T) {
	body := strings.NewReader(`{
"bankName":    "test",
"holder":      "me",
"holderType":  "Individual",
"type": "Checking",
"metadata": "extra data",
"routingNumber": "123456789",
"accountNumber": "123"
}`)
	keeper := secrets.TestStringKeeper(t)
	req, err := readDepositoryRequest(&http.Request{
		Body: ioutil.NopCloser(body),
	}, keeper)
	if err != nil {
		t.Fatal(err)
	}
	if req.bankName != "test" {
		t.Error(req.bankName)
	}
	if req.holder != "me" {
		t.Error(req.holder)
	}
	if req.holderType != model.Individual {
		t.Error(req.holderType)
	}
	if req.accountType != model.Checking {
		t.Error(req.accountType)
	}
	if req.routingNumber != "123456789" {
		t.Error(req.routingNumber)
	}
	if num, err := keeper.DecryptString(req.accountNumber); err != nil {
		t.Fatal(err)
	} else {
		if num != "123" {
			t.Errorf("num=%s", req.accountNumber)
		}
	}
}

func TestDepositorStatus__json(t *testing.T) {
	ht := model.DepositoryStatus("invalid")
	valid := map[string]model.DepositoryStatus{
		"Verified":   model.DepositoryVerified,
		"unverifieD": model.DepositoryUnverified,
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
