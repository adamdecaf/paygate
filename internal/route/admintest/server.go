// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package admintest

import (
	"testing"

	"github.com/moov-io/base/admin"
)

func Server(t *testing.T) *admin.Server {
	svc := admin.NewServer(":0")
	go svc.Listen()

	t.Cleanup(func() { svc.Shutdown() })

	return svc
}
