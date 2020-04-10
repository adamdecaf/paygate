// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

type receiverRequest struct {
	Email             string         `json:"email,omitempty"`
	DefaultDepository id.Depository  `json:"defaultDepository,omitempty"`
	BirthDate         *time.Time     `json:"birthDate,omitempty"`
	Address           *model.Address `json:"address,omitempty"`
	Metadata          string         `json:"metadata,omitempty"`
}

func (r receiverRequest) missingFields() error {
	if r.Email == "" {
		return errors.New("missing receiverRequest.Email")
	}
	if r.DefaultDepository.String() == "" {
		return errors.New("missing receiverRequest.DefaultDepository")
	}
	return nil
}

func readReceiverRequest(r *http.Request) (receiverRequest, error) {
	var wrapper receiverRequest
	if err := json.NewDecoder(route.Read(r.Body)).Decode(&wrapper); err != nil {
		return wrapper, err
	}
	if err := wrapper.missingFields(); err != nil {
		return wrapper, fmt.Errorf("%v: %v", route.ErrMissingRequiredJson, err)
	}
	return wrapper, nil
}

// parseAndValidateEmail attempts to parse an email address and validate the domain name.
func parseAndValidateEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("error parsing '%s': %v", raw, err)
	}
	return addr.Address, nil
}

func createUserReceiver(logger log.Logger, customersClient customers.Client, depositoryRepo depository.Repository, receiverRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		req, err := readReceiverRequest(r)
		if err != nil {
			responder.Log("receivers", fmt.Errorf("error reading receiverRequest: %v", err))
			responder.Problem(err)
			return
		}

		dep, err := depositoryRepo.GetUserDepository(req.DefaultDepository, responder.XUserID)
		if err != nil || dep == nil {
			responder.Log("receivers", "depository not found")
			responder.Problem(errors.New("depository not found"))
			return
		}

		email, err := parseAndValidateEmail(req.Email)
		if err != nil {
			responder.Log("receivers", fmt.Sprintf("unable to validate receiver email: %v", err))
			responder.Problem(err)
			return
		}

		// Create our receiver
		receiver := &model.Receiver{
			ID:                model.ReceiverID(base.ID()),
			Email:             email,
			DefaultDepository: req.DefaultDepository,
			Status:            model.ReceiverUnverified,
			Metadata:          req.Metadata,
			Created:           base.NewTime(time.Now()),
		}
		if err := receiver.Validate(); err != nil {
			responder.Log("receivers", fmt.Errorf("error validating Receiver: %v", err))
			responder.Problem(err)
			return
		}

		// Add the Receiver into our Customers service
		if customersClient != nil {
			opts := &customers.Request{
				Name:      dep.Holder,
				Addresses: model.ConvertAddress(req.Address),
				Email:     email,
				RequestID: responder.XRequestID,
				UserID:    responder.XUserID,
			}
			if req.BirthDate != nil {
				opts.BirthDate = *req.BirthDate
			}
			customer, err := customersClient.Create(opts)
			if err != nil || customer == nil {
				responder.Log("receivers", "error creating customer", "error", err)
				responder.Problem(err)
				return
			}
			responder.Log("receivers", fmt.Sprintf("created customer=%s", customer.ID))
			receiver.CustomerID = customer.ID
		} else {
			responder.Log("receivers", "skipped adding receiver into Customers")
		}

		if err := receiverRepo.UpsertUserReceiver(responder.XUserID, receiver); err != nil {
			err = fmt.Errorf("creating receiver=%s, user_id=%s: %v", receiver.ID, responder.XUserID, err)
			responder.Log("receivers", fmt.Errorf("error inserting Receiver: %v", err))
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(receiver)
		})
	}
}
