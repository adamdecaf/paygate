// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/route"
)

func updateUserReceiver(logger log.Logger, depRepo depository.Repository, receiverRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		var wrapper receiverRequest
		if err := json.NewDecoder(route.Read(r.Body)).Decode(&wrapper); err != nil {
			responder.Problem(err)
			return
		}

		receiverID := getReceiverID(r)
		if receiverID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		receiver, err := receiverRepo.GetUserReceiver(receiverID, responder.XUserID)
		if receiver == nil || err != nil {
			responder.Log("receivers", fmt.Sprintf("problem getting receiver='%s': %v", receiverID, err))
			responder.Problem(err)
			return
		}
		if wrapper.DefaultDepository != "" {
			// Verify the user controls the requested Depository
			dep, err := depRepo.GetUserDepository(wrapper.DefaultDepository, responder.XUserID)
			if err != nil || dep == nil {
				responder.Log("receivers", "depository doesn't belong to user")
				responder.Problem(errors.New("depository not found"))
				return
			}
			receiver.DefaultDepository = wrapper.DefaultDepository
		}
		if wrapper.Metadata != "" {
			receiver.Metadata = wrapper.Metadata
		}
		receiver.Updated = base.NewTime(time.Now())

		if err := receiver.Validate(); err != nil {
			responder.Log("receivers", fmt.Sprintf("problem validating updatable receiver=%s: %v", receiver.ID, err))
			responder.Problem(err)
			return
		}

		// Perform update
		if err := receiverRepo.UpsertUserReceiver(responder.XUserID, receiver); err != nil {
			responder.Log("receivers", fmt.Sprintf("problem upserting receiver=%s: %v", receiver.ID, err))
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(receiver)
		})
	}
}
