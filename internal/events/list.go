// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

func getUserEvents(logger log.Logger, eventRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		events, err := eventRepo.GetUserEvents(responder.XUserID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}
		responder.Respond(func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(events)
		})
	}
}

func getEventHandler(logger log.Logger, eventRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		eventID := getEventID(r)
		if eventID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// grab event
		event, err := eventRepo.GetEvent(eventID, responder.XUserID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(event)
		})
	}
}

func (r *SQLRepository) GetEvent(eventID EventID, userID id.User) (*Event, error) {
	query := `select event_id, topic, message, type from events
where event_id = ? and user_id = ?
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(eventID, userID)

	var event Event
	if err := row.Scan(&event.ID, &event.Topic, &event.Message, &event.Type); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // not found
		}
		return nil, err
	}
	if event.ID == "" {
		return nil, nil // event not found
	}
	event.Metadata = r.getEventMetadata(event.ID)
	return &event, nil
}

func (r *SQLRepository) GetUserEvents(userID id.User) ([]*Event, error) {
	query := `select event_id from events where user_id = ?`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eventIDs []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return nil, fmt.Errorf("getUserEvents scan: %v", err)
		}
		if row != "" {
			eventIDs = append(eventIDs, row)
		}
	}
	var events []*Event
	for i := range eventIDs {
		event, err := r.GetEvent(EventID(eventIDs[i]), userID)
		if err == nil && event != nil {
			events = append(events, event)
		}
	}
	return events, rows.Err()
}

func (r *SQLRepository) GetUserEventsByMetadata(userID id.User, metadata map[string]string) ([]*Event, error) {
	query := `select distinct event_id from event_metadata where user_id = ?` + strings.Repeat(` and key = ? and value = ?`, len(metadata))
	var args = []interface{}{userID.String()}
	for k, v := range metadata {
		args = append(args, k, v)
	}
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("get events by metadata: prepare: %v", err)
	}
	defer stmt.Close()

	rows, err := stmt.Query(args...)
	if err != nil {
		return nil, fmt.Errorf("get events by metadata: query: %v", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		id := ""
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("get events by metadata: scan: %v", err)
		}
		if evt, err := r.GetEvent(EventID(id), userID); err != nil {
			return nil, fmt.Errorf("get events by metadata: get: %v", err)
		} else {
			events = append(events, evt)
		}
	}
	return events, nil
}

func (r *SQLRepository) getEventMetadata(eventID EventID) map[string]string {
	query := "select `key`, value from event_metadata where event_id = ?;"
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil
	}
	defer stmt.Close()

	rows, err := stmt.Query(eventID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		key, value := "", ""
		if err := rows.Scan(&key, &value); err != nil {
			return nil
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}
