// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"database/sql"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"
)

type Repository interface {
	getUserOriginators(userID id.User) ([]*model.Originator, error)
	GetUserOriginator(id model.OriginatorID, userID id.User) (*model.Originator, error)

	createUserOriginator(userID id.User, req originatorRequest) (*model.Originator, error)
	updateUserOriginator(userID id.User, orig *model.Originator) error
	deleteUserOriginator(id model.OriginatorID, userID id.User) error
}

func NewOriginatorRepo(logger log.Logger, db *sql.DB) *SQLOriginatorRepo {
	return &SQLOriginatorRepo{log: logger, db: db}
}

type SQLOriginatorRepo struct {
	db  *sql.DB
	log log.Logger
}

func (r *SQLOriginatorRepo) Close() error {
	return r.db.Close()
}
