// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"encoding/json"
	"net/http"

	"github.com/go-kit/kit/log"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/route"
)

func GetUserReceiver(logger log.Logger, receiverRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		receiverID := getReceiverID(r)
		if receiverID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		receiver, err := receiverRepo.GetUserReceiver(receiverID, responder.XUserID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(receiver)
		})
	}
}
