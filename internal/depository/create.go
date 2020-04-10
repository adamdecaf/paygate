// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/hash"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"
)

type depositoryRequest struct {
	bankName      string
	holder        string
	holderType    model.HolderType
	accountType   model.AccountType
	routingNumber string
	accountNumber string
	metadata      string

	keeper              *secrets.StringKeeper
	HashedAccountNumber string
}

func (r depositoryRequest) missingFields() error {
	if r.bankName == "" {
		return errors.New("missing depositoryRequest.BankName")
	}
	if r.holder == "" {
		return errors.New("missing depositoryRequest.Holder")
	}
	if r.holderType == "" {
		return errors.New("missing depositoryRequest.HolderType")
	}
	if r.accountType == "" {
		return errors.New("missing depositoryRequest.Type")
	}
	if r.routingNumber == "" {
		return errors.New("missing depositoryRequest.RoutingNumber")
	}
	if r.accountNumber == "" {
		return errors.New("missing depositoryRequest.AccountNumber")
	}
	return nil
}

func (r *depositoryRequest) UnmarshalJSON(data []byte) error {
	var wrapper struct {
		BankName      string            `json:"bankName,omitempty"`
		Holder        string            `json:"holder,omitempty"`
		HolderType    model.HolderType  `json:"holderType,omitempty"`
		AccountType   model.AccountType `json:"type,omitempty"`
		RoutingNumber string            `json:"routingNumber,omitempty"`
		AccountNumber string            `json:"accountNumber,omitempty"`
		Metadata      string            `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	r.bankName = wrapper.BankName
	r.holder = wrapper.Holder
	r.holderType = wrapper.HolderType
	r.accountType = wrapper.AccountType
	r.routingNumber = wrapper.RoutingNumber
	r.metadata = wrapper.Metadata

	if wrapper.AccountNumber != "" {
		if num, err := r.keeper.EncryptString(wrapper.AccountNumber); err != nil {
			return err
		} else {
			r.accountNumber = num
		}
		if hash, err := hash.AccountNumber(wrapper.AccountNumber); err != nil {
			return err
		} else {
			r.HashedAccountNumber = hash
		}
	}

	return nil
}

func readDepositoryRequest(r *http.Request, keeper *secrets.StringKeeper) (depositoryRequest, error) {
	wrapper := depositoryRequest{
		keeper: keeper,
	}
	if err := json.NewDecoder(route.Read(r.Body)).Decode(&wrapper); err != nil {
		return wrapper, err
	}
	if wrapper.accountNumber != "" {
		if num, err := keeper.DecryptString(wrapper.accountNumber); err != nil {
			return wrapper, err
		} else {
			if hash, err := hash.AccountNumber(num); err != nil {
				return wrapper, err
			} else {
				wrapper.HashedAccountNumber = hash
			}
		}
	}
	return wrapper, nil
}

func (r *Router) createUserDepository() http.HandlerFunc {
	return func(w http.ResponseWriter, httpReq *http.Request) {
		responder := route.NewResponder(r.logger, w, httpReq)
		if responder == nil {
			return
		}

		req, err := readDepositoryRequest(httpReq, r.keeper)
		if err != nil {
			responder.Log("depositories", err, "requestID")
			responder.Problem(err)
			return
		}
		if err := req.missingFields(); err != nil {
			err = fmt.Errorf("%v: %v", route.ErrMissingRequiredJson, err)
			responder.Problem(err)
			return
		}

		now := time.Now()
		depository := &model.Depository{
			ID:                     id.Depository(base.ID()),
			BankName:               req.bankName,
			Holder:                 req.holder,
			HolderType:             req.holderType,
			Type:                   req.accountType,
			RoutingNumber:          req.routingNumber,
			Status:                 model.DepositoryUnverified,
			Metadata:               req.metadata,
			Created:                base.NewTime(now),
			Updated:                base.NewTime(now),
			UserID:                 responder.XUserID,
			Keeper:                 r.keeper,
			EncryptedAccountNumber: req.accountNumber,
			HashedAccountNumber:    req.HashedAccountNumber,
		}
		if err := depository.Validate(); err != nil {
			responder.Log("depositories", err.Error())
			responder.Problem(err)
			return
		}

		// Check FED for the routing number
		if r.fedClient != nil {
			if err := r.fedClient.LookupRoutingNumber(req.routingNumber); err != nil {
				err = fmt.Errorf("problem with FED routing number lookup %q: %v", req.routingNumber, err)
				responder.Log("depositories", err)
				responder.Problem(err)
				return
			}
		}

		if err := r.depositoryRepo.UpsertUserDepository(responder.XUserID, depository); err != nil {
			responder.Log("depositories", err.Error())
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(depository)
		})
	}
}
