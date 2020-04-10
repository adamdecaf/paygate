// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"
)

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

func TestDepositoryJSON(t *testing.T) {
	keeper := secrets.TestStringKeeper(t)
	num, _ := keeper.EncryptString("123")
	bs, err := json.MarshalIndent(Depository{
		ID:                     id.Depository(base.ID()),
		BankName:               "moov, inc",
		Holder:                 "Jane Smith",
		HolderType:             Individual,
		Type:                   Checking,
		RoutingNumber:          "987654320",
		EncryptedAccountNumber: num,
		Status:                 DepositoryVerified,
		Metadata:               "extra",
		Keeper:                 keeper,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("  %s", string(bs))
	// TODO(adam): need to check field params
}
