// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/accounts"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

type originatorRequest struct {
	// DefaultDepository the depository account to be used by default per transaction.
	DefaultDepository id.Depository `json:"defaultDepository"`

	// Identification is a number by which the receiver is known to the originator
	Identification string `json:"identification"`

	// BirthDate is an optional value required for Know Your Customer (KYC) validation of this Originator
	BirthDate *time.Time `json:"birthDate,omitempty"`

	// Address is an optional object required for Know Your Customer (KYC) validation of this Originator
	Address *model.Address `json:"address,omitempty"`

	// Metadata provides additional data to be used for display and search only
	Metadata string `json:"metadata"`

	// customerID is a unique ID from Moov's Customers service
	customerID string
}

func (r originatorRequest) missingFields() error {
	if r.Identification == "" {
		return errors.New("missing originatorRequest.Identification")
	}
	if r.DefaultDepository.String() == "" {
		return errors.New("missing originatorRequest.DefaultDepository")
	}
	return nil
}

func readOriginatorRequest(r *http.Request) (originatorRequest, error) {
	var wrapper originatorRequest
	if err := json.NewDecoder(route.Read(r.Body)).Decode(&wrapper); err != nil {
		return wrapper, err
	}
	if err := wrapper.missingFields(); err != nil {
		return wrapper, fmt.Errorf("%v: %v", route.ErrMissingRequiredJson, err)
	}
	return wrapper, nil
}

func createUserOriginator(logger log.Logger, accountsClient accounts.Client, customersClient customers.Client, depositoryRepo depository.Repository, originatorRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		req, err := readOriginatorRequest(r)
		if err != nil {
			responder.Log("originators", err.Error())
			responder.Problem(err)
			return
		}

		userID, requestID := route.HeaderUserID(r), moovhttp.GetRequestID(r)

		// Verify depository belongs to the user
		dep, err := depositoryRepo.GetUserDepository(req.DefaultDepository, userID)
		if err != nil || dep == nil || dep.ID != req.DefaultDepository {
			responder.Problem(fmt.Errorf("depository %s does not exist", req.DefaultDepository))
			return
		}

		// Verify account exists in Accounts for receiver (userID)
		if accountsClient != nil {
			account, err := accountsClient.SearchAccounts(requestID, userID, dep)
			if err != nil || account == nil {
				responder.Log("originators", fmt.Sprintf("problem finding account depository=%s: %v", dep.ID, err))
				responder.Problem(err)
				return
			}
		}

		// Create the customer with Moov's service
		if customersClient != nil {
			opts := &customers.Request{
				Name:      dep.Holder,
				Addresses: model.ConvertAddress(req.Address),
				SSN:       req.Identification,
				RequestID: responder.XRequestID,
				UserID:    responder.XUserID,
			}
			if req.BirthDate != nil {
				opts.BirthDate = *req.BirthDate
			}
			customer, err := customersClient.Create(opts)
			if err != nil || customer == nil {
				responder.Log("originators", "error creating Customer", "error", err)
				responder.Problem(err)
				return
			}
			responder.Log("originators", fmt.Sprintf("created customer=%s", customer.ID))
			req.customerID = customer.ID
		} else {
			responder.Log("originators", "skipped adding originator into Customers")
		}

		// Write Originator to DB
		orig, err := originatorRepo.createUserOriginator(userID, req)
		if err != nil {
			responder.Log("originators", fmt.Sprintf("problem creating originator: %v", err))
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(orig)
		})
	}
}

func (r *SQLOriginatorRepo) createUserOriginator(userID id.User, req originatorRequest) (*model.Originator, error) {
	now := time.Now()
	orig := &model.Originator{
		ID:                model.OriginatorID(base.ID()),
		DefaultDepository: req.DefaultDepository,
		Identification:    req.Identification,
		CustomerID:        req.customerID,
		Metadata:          req.Metadata,
		Created:           base.NewTime(now),
		Updated:           base.NewTime(now),
	}
	if err := orig.Validate(); err != nil {
		return nil, err
	}

	query := `insert into originators (originator_id, user_id, default_depository, identification, customer_id, metadata, created_at, last_updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(orig.ID, userID, orig.DefaultDepository, orig.Identification, orig.CustomerID, orig.Metadata, now, now)
	if err != nil {
		return nil, err
	}
	return orig, nil
}
