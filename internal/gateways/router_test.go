// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package gateways

import (
	"testing"
)

func TestGateways__gatewayRequest(t *testing.T) {
	req := gatewayRequest{}
	if err := req.missingFields(); err == nil {
		t.Error("expected error")
	}
}
