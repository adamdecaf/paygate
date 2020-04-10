// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/paygate/internal/route"
)

func deleteUserReceiver(logger log.Logger, receiverRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		if receiverID := getReceiverID(r); receiverID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			if err := receiverRepo.deleteUserReceiver(receiverID, responder.XUserID); err != nil {
				responder.Problem(err)
				return
			}
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
		})
	}
}
