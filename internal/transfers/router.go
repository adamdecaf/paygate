// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/events"
	"github.com/moov-io/paygate/internal/gateways"
	"github.com/moov-io/paygate/internal/originators"
	"github.com/moov-io/paygate/internal/receivers"
	"github.com/moov-io/paygate/internal/transfers/limiter"
	"github.com/moov-io/paygate/pkg/id"
)

type TransferRouter struct {
	logger log.Logger

	depRepo            depository.Repository
	eventRepo          events.Repository
	gatewayRepo        gateways.Repository
	receiverRepository receivers.Repository
	origRepo           originators.Repository
	transferRepo       Repository

	transferLimitChecker limiter.Checker

	accountsClient  accounts.Client
	customersClient customers.Client
}

func NewTransferRouter(
	logger log.Logger,
	depositoryRepo depository.Repository,
	eventRepo events.Repository,
	gatewayRepo gateways.Repository,
	receiverRepo receivers.Repository,
	originatorsRepo originators.Repository,
	transferRepo Repository,
	transferLimitChecker limiter.Checker,
	accountsClient accounts.Client,
	customersClient customers.Client,
) *TransferRouter {
	return &TransferRouter{
		logger:               logger,
		depRepo:              depositoryRepo,
		eventRepo:            eventRepo,
		gatewayRepo:          gatewayRepo,
		receiverRepository:   receiverRepo,
		origRepo:             originatorsRepo,
		transferRepo:         transferRepo,
		transferLimitChecker: transferLimitChecker,
		accountsClient:       accountsClient,
		customersClient:      customersClient,
	}
}

func (c *TransferRouter) RegisterRoutes(router *mux.Router) {
	router.Methods("GET").Path("/transfers").HandlerFunc(c.getUserTransfers())
	router.Methods("GET").Path("/transfers/{transferId}").HandlerFunc(c.getUserTransfer())

	router.Methods("POST").Path("/transfers").HandlerFunc(c.createUserTransfers())
	router.Methods("POST").Path("/transfers/batch").HandlerFunc(c.createUserTransfers())

	router.Methods("DELETE").Path("/transfers/{transferId}").HandlerFunc(c.deleteUserTransfer())

	router.Methods("GET").Path("/transfers/{transferId}/events").HandlerFunc(c.getUserTransferEvents())
	router.Methods("POST").Path("/transfers/{transferId}/failed").HandlerFunc(c.validateUserTransfer())
	router.Methods("POST").Path("/transfers/{transferId}/files").HandlerFunc(c.getUserTransferFiles())
}

func GetID(r *http.Request) id.Transfer {
	vars := mux.Vars(r)
	v, ok := vars["transferId"]
	if ok {
		return id.Transfer(v)
	}
	return id.Transfer("")
}
