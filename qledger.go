// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/moov-io/base/k8s"
)

func getQLedgerAddress() string {
	addr := os.Getenv("QLEDGER_ENDPOINT")
	if addr != "" {
		return addr
	}
	if k8s.Inside() {
		return "http://qledger.apps.svc.cluster.local:7000"
	}
	return "http://localhost:7000"
}
