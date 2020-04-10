// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depadmin

import (
	"github.com/moov-io/base/admin"
	"github.com/moov-io/paygate/internal/depository"

	"github.com/go-kit/kit/log"
)

// RegisterRoutes will add HTTP handlers for paygate's admin HTTP server
func RegisterRoutes(logger log.Logger, svc *admin.Server, repo depository.Repository) {
	svc.AddHandler("/depositories/{depositoryId}", overrideDepositoryStatus(logger, repo))
}
