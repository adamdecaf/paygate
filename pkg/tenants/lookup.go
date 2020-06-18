// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package tenants

import (
	"net/http"

	"github.com/moov-io/paygate/pkg/util"
)

type Lookup interface {
	TenantID(req *http.Request) (string, error)
}

func NewLookup() (Lookup, error) {
	return nil, nil
}

type AuthLookup struct{}

func (l *AuthLookup) TenantID(req *http.Request) (string, error) {
	return util.Or(req.Header.Get("X-Tenant"), req.Header.Get("X-Tenant-ID")), nil
}
