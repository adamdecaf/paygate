// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package events

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func AddRoutes(logger log.Logger, r *mux.Router, eventRepo Repository) {
	r.Methods("GET").Path("/events").HandlerFunc(getUserEvents(logger, eventRepo))
	r.Methods("GET").Path("/events/{eventID}").HandlerFunc(getEventHandler(logger, eventRepo))
}

// getEventID extracts the EventID from the incoming request.
func getEventID(r *http.Request) EventID {
	v := mux.Vars(r)
	id, ok := v["eventID"]
	if !ok {
		return EventID("")
	}
	return EventID(id)
}
