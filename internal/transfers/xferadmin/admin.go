// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package xferadmin

import (
	"github.com/go-kit/kit/log"
	"github.com/moov-io/base/admin"
	"github.com/moov-io/paygate/internal/transfers"
)

// RegisterRoutes will add HTTP handlers for paygate's admin HTTP server
func RegisterRoutes(logger log.Logger, svc *admin.Server, repo transfers.Repository) {
	svc.AddHandler("/transfers/{transferId}/status", updateTransferStatus(logger, repo))
}
