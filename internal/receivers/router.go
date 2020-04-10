// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"net/http"

	"github.com/moov-io/paygate/internal/customers"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func AddReceiverRoutes(logger log.Logger, r *mux.Router, customersClient customers.Client, depositoryRepo depository.Repository, receiverRepo Repository) {
	r.Methods("GET").Path("/receivers").HandlerFunc(getUserReceivers(logger, receiverRepo))
	r.Methods("POST").Path("/receivers").HandlerFunc(createUserReceiver(logger, customersClient, depositoryRepo, receiverRepo))

	r.Methods("GET").Path("/receivers/{receiverId}").HandlerFunc(GetUserReceiver(logger, receiverRepo))
	r.Methods("PATCH").Path("/receivers/{receiverId}").HandlerFunc(updateUserReceiver(logger, depositoryRepo, receiverRepo))
	r.Methods("DELETE").Path("/receivers/{receiverId}").HandlerFunc(deleteUserReceiver(logger, receiverRepo))
}

// getReceiverID extracts the ReceiverID from the incoming request.
func getReceiverID(r *http.Request) model.ReceiverID {
	v := mux.Vars(r)
	id, ok := v["receiverId"]
	if !ok {
		return model.ReceiverID("")
	}
	return model.ReceiverID(id)
}
