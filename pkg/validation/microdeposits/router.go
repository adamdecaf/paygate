// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package microdeposits

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	moovcustomers "github.com/moov-io/customers/client"
	"github.com/moov-io/paygate/pkg/client"
	"github.com/moov-io/paygate/pkg/config"
	"github.com/moov-io/paygate/pkg/customers"
	"github.com/moov-io/paygate/pkg/customers/accounts"
	"github.com/moov-io/paygate/pkg/tenants"
	"github.com/moov-io/paygate/pkg/transfers"
	"github.com/moov-io/paygate/pkg/transfers/fundflow"
	"github.com/moov-io/paygate/pkg/transfers/pipeline"
	"github.com/moov-io/paygate/x/route"

	"github.com/go-kit/kit/log"
)

type Router struct {
	InitiateMicroDeposits   http.HandlerFunc
	GetMicroDeposits        http.HandlerFunc
	GetAccountMicroDeposits http.HandlerFunc
}

func NewRouter(
	cfg *config.Config,
	repo Repository,
	transferRepo transfers.Repository,
	tenantRepo tenants.Repository,
	customersClient customers.Client,
	accountDecryptor accounts.Decryptor,
	fundStrategy fundflow.Strategy,
	pub pipeline.XferPublisher,
) *Router {
	if cfg.Validation.MicroDeposits == nil {
		return &Router{
			InitiateMicroDeposits:   NotImplemented(cfg.Logger),
			GetMicroDeposits:        NotImplemented(cfg.Logger),
			GetAccountMicroDeposits: NotImplemented(cfg.Logger),
		}
	}
	config := *cfg.Validation.MicroDeposits

	// companyIdentification is the similarly named Batch Header field. It can be
	// overridden from auth on the request.
	// TODO(adam): this will also be read from auth on the request
	companyIdentification := cfg.ODFI.FileConfig.BatchHeader.CompanyIdentification

	return &Router{
		InitiateMicroDeposits:   InitiateMicroDeposits(config, cfg.Logger, companyIdentification, repo, transferRepo, customersClient, accountDecryptor, fundStrategy, pub),
		GetMicroDeposits:        GetMicroDeposits(cfg.Logger, repo),
		GetAccountMicroDeposits: GetAccountMicroDeposits(cfg.Logger, repo),
	}
}

func (c *Router) RegisterRoutes(r *mux.Router) {
	r.Methods("POST").Path("/micro-deposits").HandlerFunc(c.InitiateMicroDeposits)
	r.Methods("GET").Path("/micro-deposits/{microDepositID}").HandlerFunc(c.GetMicroDeposits)
	r.Methods("GET").Path("/accounts/{accountID}/micro-deposits").HandlerFunc(c.GetAccountMicroDeposits)
}

func InitiateMicroDeposits(
	cfg config.MicroDeposits,
	logger log.Logger,
	companyIdentification string,
	repo Repository,
	transferRepo transfers.Repository,
	customersClient customers.Client,
	accountDecryptor accounts.Decryptor,
	fundStrategy fundflow.Strategy,
	pub pipeline.XferPublisher,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		responder.Respond(func(w http.ResponseWriter) {
			var req client.CreateMicroDeposits
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				responder.Problem(err)
				return
			}

			src, err := getMicroDepositSource(cfg, customersClient, accountDecryptor)
			if err != nil {
				responder.Log("micro-deposits", fmt.Sprintf("ERROR getting micro-deposit source: %v", err))
				responder.Problem(err)
				return
			}
			dest, err := getFundflowDestination(customersClient, accountDecryptor, req.Destination)
			if err != nil {
				responder.Log("micro-deposits", fmt.Sprintf("ERROR getting micro-deposit destination: %v", err))
				responder.Problem(err)
				return
			}
			if src.Account.RoutingNumber == dest.Account.RoutingNumber {
				responder.Log("micro-deposits", "ERROR not initiating micro-deposits for account at ODFI")
				responder.Problem(err)
				return
			}

			micro, err := createMicroDeposits(cfg, responder.XUserID, companyIdentification, src, dest, transferRepo, accountDecryptor, fundStrategy, pub)
			if err != nil {
				responder.Log("micro-deposits", fmt.Sprintf("ERROR creating micro-deposits: %v", err))
				responder.Problem(err)
				return
			}
			if err := repo.writeMicroDeposits(micro); err != nil {
				responder.Log("micro-deposits", fmt.Sprintf("ERROR writing micro-deposits: %v", err))
				responder.Problem(err)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(micro)
		})
	}
}

func getMicroDepositSource(cfg config.MicroDeposits, customersClient customers.Client, accountDecryptor accounts.Decryptor) (fundflow.Source, error) {
	return transfers.GetFundflowSource(customersClient, accountDecryptor, client.Source{
		CustomerID: cfg.Source.CustomerID,
		AccountID:  cfg.Source.AccountID,
	})
}

func getFundflowDestination(client customers.Client, accountDecryptor accounts.Decryptor, dst client.Destination) (fundflow.Destination, error) {
	var destination fundflow.Destination

	// Set destination Customer
	cust, err := client.Lookup(dst.CustomerID, "requestID", "userID")
	if err != nil {
		return destination, err
	}
	if cust == nil || cust.CustomerID == "" {
		return destination, fmt.Errorf("customerID=%s is not found", dst.CustomerID)
	}
	// Check the Customer status
	if !strings.EqualFold(string(cust.Status), string(moovcustomers.UNKNOWN)) {
		return destination, fmt.Errorf("micro-deposits destination customerID=%s has unacceptable status: %s", cust.CustomerID, cust.Status)
	}
	destination.Customer = *cust

	// Get customer Account
	if acct, err := client.FindAccount(dst.CustomerID, dst.AccountID); acct == nil || acct.AccountID == "" || err != nil {
		return destination, fmt.Errorf("accountID=%s not found for customerID=%s error=%v", dst.AccountID, dst.CustomerID, err)
	} else {
		if !strings.EqualFold(string(acct.Status), string(moovcustomers.NONE)) {
			return destination, fmt.Errorf("micro-deposits destination accountID=%s has unacceptable status: %v", acct.AccountID, acct.Status)
		}
		destination.Account = *acct
	}
	if num, err := accountDecryptor.AccountNumber(dst.CustomerID, dst.AccountID); num == "" || err != nil {
		return destination, fmt.Errorf("unable to decrypt destination accountID=%s for customerID=%s error=%v", dst.AccountID, dst.CustomerID, err)
	} else {
		destination.AccountNumber = num
	}

	return destination, nil
}

func GetMicroDeposits(logger log.Logger, repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		responder.Respond(func(w http.ResponseWriter) {
			microDepositID := route.ReadPathID("microDepositID", r)
			if microDepositID == "" {
				responder.Problem(errors.New("missing microDepositID"))
				return
			}

			micro, err := repo.getMicroDeposits(microDepositID)
			if err != nil && err != sql.ErrNoRows {
				responder.Log("micro-deposits", fmt.Errorf("ERROR getting micro-deposits: %v", err))
				responder.Problem(err)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(micro)
		})
	}
}

func GetAccountMicroDeposits(logger log.Logger, repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		responder.Respond(func(w http.ResponseWriter) {
			accountID := route.ReadPathID("accountID", r)
			if accountID == "" {
				responder.Problem(errors.New("missing accountID"))
				return
			}

			micro, err := repo.getAccountMicroDeposits(accountID)
			if err != nil && err != sql.ErrNoRows {
				responder.Log("micro-deposits", fmt.Errorf("ERROR getting accountID=%s micro-deposits: %v", accountID, err))
				responder.Problem(err)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(micro)
		})
	}
}

func NotImplemented(logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		responder.Problem(errors.New("micro-deposits are disabled via config"))
	}
}
