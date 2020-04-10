// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"net/http"

	"github.com/moov-io/paygate/internal/events"
	"github.com/moov-io/paygate/internal/fed"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type Router struct {
	logger log.Logger

	fedClient fed.Client

	depositoryRepo Repository
	eventRepo      events.Repository

	keeper *secrets.StringKeeper
}

func NewRouter(
	logger log.Logger,
	fedClient fed.Client,
	depositoryRepo Repository,
	eventRepo events.Repository,
	keeper *secrets.StringKeeper,
) *Router {
	router := &Router{
		logger:         logger,
		fedClient:      fedClient,
		depositoryRepo: depositoryRepo,
		eventRepo:      eventRepo,
		keeper:         keeper,
	}
	return router
}

func (r *Router) RegisterRoutes(router *mux.Router) {
	router.Methods("GET").Path("/depositories").HandlerFunc(r.getUserDepositories())
	router.Methods("POST").Path("/depositories").HandlerFunc(r.createUserDepository())

	router.Methods("GET").Path("/depositories/{depositoryId}").HandlerFunc(r.getUserDepository())
	router.Methods("PATCH").Path("/depositories/{depositoryId}").HandlerFunc(r.updateUserDepository())
	router.Methods("DELETE").Path("/depositories/{depositoryId}").HandlerFunc(r.deleteUserDepository())
}

// GetID extracts the id.Depository from the incoming request.
func GetID(r *http.Request) id.Depository {
	v, ok := mux.Vars(r)["depositoryId"]
	if !ok {
		return id.Depository("")
	}
	return id.Depository(v)
}
