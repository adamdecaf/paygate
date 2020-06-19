// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package tenants

import (
	"net/http"
)

type MockLookup struct {
	CompanyID string
	Err       error
}

func (l *MockLookup) GetCompanyID(_ *http.Request) (string, error) {
	if l.Err != nil {
		return "", l.Err
	}
	return l.CompanyID, nil
}
