// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/moov-io/paygate/pkg/id"
)

func TestReceivers__read(t *testing.T) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(receiverRequest{
		Email:             "test@moov.io",
		DefaultDepository: id.Depository("test"),
		Metadata:          "extra",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := readReceiverRequest(&http.Request{
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
func TestReceivers__receiverRequest(t *testing.T) {
	req := receiverRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}

func TestReceivers__parseAndValidateEmail(t *testing.T) {
	if addr, err := parseAndValidateEmail("a@foo.com"); addr != "a@foo.com" || err != nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
	if addr, err := parseAndValidateEmail("a+bar@foo.com"); addr != "a+bar@foo.com" || err != nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
	if addr, err := parseAndValidateEmail(`"a b"@foo.com`); addr != `a b@foo.com` || err != nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
	if addr, err := parseAndValidateEmail("Barry Gibbs <bg@example.com>"); addr != "bg@example.com" || err != nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}

	// sad path
	if addr, err := parseAndValidateEmail(""); addr != "" || err == nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
	if addr, err := parseAndValidateEmail("@"); addr != "" || err == nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
	if addr, err := parseAndValidateEmail("example.com"); addr != "" || err == nil {
		t.Errorf("addr=%s error=%v", addr, err)
	}
}
