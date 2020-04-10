// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package events

import (
	"database/sql"

	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

type Repository interface {
	GetEvent(eventID EventID, userID id.User) (*Event, error)
	GetUserEvents(userID id.User) ([]*Event, error)

	GetUserEventsByMetadata(userID id.User, metadata map[string]string) ([]*Event, error)

	WriteEvent(userID id.User, event *Event) error
}

func NewRepo(logger log.Logger, db *sql.DB) *SQLRepository {
	return &SQLRepository{log: logger, db: db}
}

type SQLRepository struct {
	db  *sql.DB
	log log.Logger
}

func (r *SQLRepository) Close() error {
	return r.db.Close()
}
