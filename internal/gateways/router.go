// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package gateways

import (
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func AddRoutes(logger log.Logger, r *mux.Router, gatewayRepo Repository) {
	r.Methods("GET").Path("/gateways").HandlerFunc(getUserGateway(logger, gatewayRepo))
	r.Methods("POST").Path("/gateways").HandlerFunc(updateUserGateway(logger, gatewayRepo))
}
