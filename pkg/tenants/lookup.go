// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package tenants

import (
	"net/http"

	"github.com/moov-io/paygate/pkg/util"
)

type Lookup interface {
	GetCompanyID(req *http.Request) (string, error)
}

func NewLookup(repo Repository) (Lookup, error) {
	return &lookupImpl{
		repo: repo,
	}, nil
}

type lookupImpl struct {
	repo Repository
}

func (l *lookupImpl) GetCompanyID(req *http.Request) (string, error) {
	tenantID := util.Or(req.Header.Get("X-Tenant"), req.Header.Get("X-Tenant-ID"))
	return l.repo.GetCompanyIdentification(tenantID)
}
