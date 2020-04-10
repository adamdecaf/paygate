// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	"github.com/moov-io/base/idempotent"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/internal/transfers/limiter"
	"github.com/moov-io/paygate/pkg/id"
)

type CreateRequest struct {
	Type                   model.TransferType `json:"transferType"`
	Amount                 model.Amount       `json:"amount"`
	Originator             model.OriginatorID `json:"originator"`
	OriginatorDepository   id.Depository      `json:"originatorDepository"`
	Receiver               model.ReceiverID   `json:"receiver"`
	ReceiverDepository     id.Depository      `json:"receiverDepository"`
	Description            string             `json:"description,omitempty"`
	StandardEntryClassCode string             `json:"standardEntryClassCode"`
	SameDay                bool               `json:"sameDay,omitempty"`

	CCDDetail *model.CCDDetail `json:"CCDDetail,omitempty"`
	IATDetail *model.IATDetail `json:"IATDetail,omitempty"`
	TELDetail *model.TELDetail `json:"TELDetail,omitempty"`
	WEBDetail *model.WEBDetail `json:"WEBDetail,omitempty"`

	// Internal fields for auditing and tracing
	fileID          string
	transactionID   string
	remoteAddr      string
	UserID          id.User
	overAmountLimit bool
}

func (r CreateRequest) missingFields() error {
	var missing []string
	check := func(name, s string) {
		if s == "" {
			missing = append(missing, name)
		}
	}

	check("transferType", string(r.Type))
	check("originator", string(r.Originator))
	check("originatorDepository", string(r.OriginatorDepository))
	check("receiver", string(r.Receiver))
	check("receiverDepository", string(r.ReceiverDepository))
	check("standardEntryClassCode", string(r.StandardEntryClassCode))

	if len(missing) > 0 {
		return fmt.Errorf("missing %s JSON field(s)", strings.Join(missing, ", "))
	}
	return nil
}

func (r CreateRequest) asTransfer(transferID string) *model.Transfer {
	xfer := &model.Transfer{
		ID:                     id.Transfer(transferID),
		Type:                   r.Type,
		Amount:                 r.Amount,
		Originator:             r.Originator,
		OriginatorDepository:   r.OriginatorDepository,
		Receiver:               r.Receiver,
		ReceiverDepository:     r.ReceiverDepository,
		Description:            r.Description,
		StandardEntryClassCode: r.StandardEntryClassCode,
		Status:                 model.TransferPending,
		SameDay:                r.SameDay,
		Created:                base.Now(),
		UserID:                 r.UserID.String(),
	}
	if r.overAmountLimit {
		xfer.Status = model.TransferReviewable
	}
	// Copy along the YYYDetail sub-object for specific SEC codes
	// where we expect one in the JSON request body.
	switch xfer.StandardEntryClassCode {
	case ach.CCD:
		xfer.CCDDetail = r.CCDDetail
	case ach.IAT:
		xfer.IATDetail = r.IATDetail
	case ach.TEL:
		xfer.TELDetail = r.TELDetail
	case ach.WEB:
		xfer.WEBDetail = r.WEBDetail
	}
	return xfer
}

// readTransferRequests will attempt to parse the incoming body as either a CreateRequest or []CreateRequest.
// If no requests were read a non-nil error is returned.
func readTransferRequests(r *http.Request) ([]*CreateRequest, error) {
	bs, err := ioutil.ReadAll(route.Read(r.Body))
	if err != nil {
		return nil, err
	}

	var req CreateRequest
	var requests []*CreateRequest
	if err := json.Unmarshal(bs, &req); err != nil {
		// failed, but try []CreateRequest
		if err := json.Unmarshal(bs, &requests); err != nil {
			return nil, err
		}
	} else {
		if err := req.missingFields(); err != nil {
			return nil, err
		}
		requests = append(requests, &req)
	}
	if len(requests) == 0 {
		return nil, errors.New("no Transfer request objects found")
	}
	return requests, nil
}

func (c *TransferRouter) createUserTransfers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(c.logger, w, r)
		if responder == nil {
			return
		}

		requests, err := readTransferRequests(r)
		if err != nil {
			responder.Problem(err)
			return
		}

		// Carry over any incoming idempotency key and set one otherwise
		idempotencyKey := idempotent.Header(r)
		if idempotencyKey == "" {
			idempotencyKey = base.ID()
		}
		remoteIP := route.RemoteAddr(r.Header)

		gateway, err := c.gatewayRepo.GetUserGateway(responder.XUserID)
		if gateway == nil || err != nil {
			responder.Problem(fmt.Errorf("missing Gateway: %v", err))
			return
		}

		for i := range requests {
			transferID, req := base.ID(), requests[i]
			if err := req.missingFields(); err != nil {
				responder.Problem(err)
				return
			}
			req.remoteAddr = remoteIP
			req.UserID = responder.XUserID

			// Grab and validate objects required for this transfer.
			receiver, receiverDep, orig, origDep, err := c.getTransferObjects(responder.XUserID, req.Originator, req.OriginatorDepository, req.Receiver, req.ReceiverDepository)
			if err != nil {
				objects := fmt.Sprintf("receiver=%v, receiverDep=%v, orig=%v, origDep=%v, err: %v", receiver, receiverDep, orig, origDep, err)
				responder.Log("transfers", fmt.Sprintf("Unable to find all objects during transfer create for user_id=%s, %s", responder.XUserID, objects))

				// Respond back to user
				responder.Problem(fmt.Errorf("missing data to create transfer: %s", err))
				return
			}

			// Check limits for this userID and destination
			// TODO(adam): We'll need user level limit overrides
			if err := c.transferLimitChecker.AllowTransfer(responder.XUserID); err != nil {
				if strings.Contains(err.Error(), limiter.ErrOverLimit.Error()) {
					// Mark the transfer as needed manual approval for being over the limit(s).
					req.overAmountLimit = true
				} else {
					responder.Log("transfers", fmt.Sprintf("rejecting transfers: %v", err))
					responder.Problem(err)
					return
				}
			}

			// Post the Transfer's transaction against the Accounts
			if c.accountsClient != nil {
				tx, err := c.postAccountTransaction(responder.XUserID, origDep, receiverDep, req.Amount, req.Type, responder.XRequestID)
				if err != nil {
					responder.Log("transfers", err.Error())
					responder.Problem(err)
					return
				}
				req.transactionID = tx.ID
			}

			// Verify Customer statuses related to this transfer
			if c.customersClient != nil {
				// Pulling from a Receiver requires we've verified it already. Also, it can't be "credit only".
				if req.Type == model.PullTransfer {
					// TODO(adam): if receiver.Status == model.ReceiverCreditOnly
					if receiver.Status != model.ReceiverVerified {
						err = fmt.Errorf("receiver_id=%s is not Verified user_id=%s", receiver.ID, responder.XUserID)
						responder.Log("transfers", "problem with Receiver", "error", err.Error())
						responder.Problem(err)
						return
					}
				}
				// Check the related Customer objects for the Originator and Receiver
				if err := verifyCustomerStatuses(orig, receiver, c.customersClient, responder.XRequestID, responder.XUserID); err != nil {
					responder.Log("transfers", "problem with Customer checks", "error", err.Error())
					responder.Problem(err)
					return
				} else {
					responder.Log("transfers", "Customer check passed")
				}
				// Check any disclaimers for related Originator and Receiver
				if err := verifyDisclaimersAreAccepted(orig, receiver, c.customersClient, responder.XRequestID, responder.XUserID); err != nil {
					responder.Log("transfers", "problem with disclaimers", "error", err.Error())
					responder.Problem(err)
					return
				} else {
					responder.Log("transfers", "Disclaimer checks passed")
				}
			}

			// Save Transfer object
			transfer := req.asTransfer(transferID)

			// Verify the Transfer isn't pushed into "reviewable"
			if transfer.Status != model.TransferPending {
				err = fmt.Errorf("transfer_id=%s is not Pending (status=%s)", transfer.ID, transfer.Status)
				responder.Log("transfers", "can't process transfer", "error", err)
				responder.Problem(err)
				return
			}

			// Write events for our audit/history log
			if err := writeTransferEvent(responder.XUserID, req, c.eventRepo); err != nil {
				responder.Log("transfers", fmt.Sprintf("error writing transfer=%s event: %v", transferID, err))
				responder.Problem(err)
				return
			}
		}

		// TODO(adam): We still create Transfers if the micro-deposits have been confirmed, but not merged (and uploaded)
		// into an ACH file. Should we check that case in this method and reject Transfers whose Depositories micro-deposts
		// haven't even been merged yet?

		transfers, err := c.transferRepo.CreateUserTransfers(responder.XUserID, requests)
		if err != nil {
			responder.Log("transfers", fmt.Sprintf("error creating transfers: %v", err))
			responder.Problem(err)
			return
		}

		writeResponse(c.logger, w, len(requests), transfers)
		responder.Log("transfers", fmt.Sprintf("Created transfers for user_id=%s request=%s", responder.XUserID, responder.XRequestID))
	}
}

func (r *SQLRepo) CreateUserTransfers(userID id.User, requests []*CreateRequest) ([]*model.Transfer, error) {
	query := `insert into transfers (
  transfer_id, user_id, type, amount, originator_id, originator_depository, receiver, receiver_depository,
  description, standard_entry_class_code, status, same_day, file_id, transaction_id, remote_address, created_at)
values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var transfers []*model.Transfer

	now := time.Now()
	var status model.TransferStatus = model.TransferPending
	for i := range requests {
		req, transferId := requests[i], base.ID()
		xfer := &model.Transfer{
			ID:                     id.Transfer(transferId),
			Type:                   req.Type,
			Amount:                 req.Amount,
			Originator:             req.Originator,
			OriginatorDepository:   req.OriginatorDepository,
			Receiver:               req.Receiver,
			ReceiverDepository:     req.ReceiverDepository,
			Description:            req.Description,
			StandardEntryClassCode: req.StandardEntryClassCode,
			Status:                 status,
			SameDay:                req.SameDay,
			Created:                base.NewTime(now),
		}
		if err := xfer.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed for transfer Originator=%s, Receiver=%s, Description=%s %v", xfer.Originator, xfer.Receiver, xfer.Description, err)
		}

		// write transfer
		_, err := stmt.Exec(
			transferId, userID, req.Type, req.Amount.String(), req.Originator, req.OriginatorDepository, req.Receiver, req.ReceiverDepository,
			req.Description, req.StandardEntryClassCode, status, req.SameDay, req.fileID, req.transactionID, req.remoteAddr, now,
		)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, xfer)
	}
	return transfers, nil
}
