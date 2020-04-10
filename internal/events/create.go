// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package events

import (
	"fmt"
	"time"

	"github.com/moov-io/paygate/pkg/id"
)

func (r *SQLRepository) WriteEvent(userID id.User, event *Event) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("write event: begin: error=%v rollback=%v", err, tx.Rollback())
	}

	query := `insert into events (event_id, user_id, topic, message, type, created_at) values (?, ?, ?, ?, ?, ?)`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("write event: prepare: error=%v rollback=%v", err, tx.Rollback())
	}
	defer stmt.Close()

	_, err = stmt.Exec(event.ID, userID, event.Topic, event.Message, event.Type, time.Now())
	if err != nil {
		return fmt.Errorf("write event: exec: error=%v rollback=%v", err, tx.Rollback())
	}

	query = "insert into event_metadata (event_id, user_id, `key`, value) values (?, ?, ?, ?);"
	stmt, err = tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("write event: metadata prepare: error=%v rollback=%v", err, tx.Rollback())
	}
	defer stmt.Close()
	for k, v := range event.Metadata {
		if _, err := stmt.Exec(event.ID, userID, k, v); err != nil {
			return fmt.Errorf("write event metadata: error=%v rollback=%v", err, tx.Rollback())
		}
	}
	return tx.Commit()
}
