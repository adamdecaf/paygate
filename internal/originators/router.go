// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"net/http"

	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func AddOriginatorRoutes(logger log.Logger, r *mux.Router, accountsClient accounts.Client, customersClient customers.Client, depositoryRepo depository.Repository, originatorRepo Repository) {
	r.Methods("GET").Path("/originators").HandlerFunc(getUserOriginators(logger, originatorRepo))
	r.Methods("POST").Path("/originators").HandlerFunc(createUserOriginator(logger, accountsClient, customersClient, depositoryRepo, originatorRepo))

	r.Methods("GET").Path("/originators/{originatorId}").HandlerFunc(getUserOriginator(logger, originatorRepo))
	r.Methods("PATCH").Path("/originators/{originatorId}").HandlerFunc(updateUserOriginator(logger, originatorRepo))
	r.Methods("DELETE").Path("/originators/{originatorId}").HandlerFunc(deleteUserOriginator(logger, originatorRepo))
}

func getOriginatorId(r *http.Request) model.OriginatorID {
	vars := mux.Vars(r)
	v, ok := vars["originatorId"]
	if ok {
		return model.OriginatorID(v)
	}
	return model.OriginatorID("")
}
