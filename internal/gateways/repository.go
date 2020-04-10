// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package gateways

import (
	"database/sql"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

type Repository interface {
	GetUserGateway(userID id.User) (*model.Gateway, error)
	updateUserGateway(userID id.User, req gatewayRequest) (*model.Gateway, error)
}

func NewRepo(logger log.Logger, db *sql.DB) *SQLGatewayRepo {
	return &SQLGatewayRepo{log: logger, db: db}
}

type SQLGatewayRepo struct {
	db  *sql.DB
	log log.Logger
}

func (r *SQLGatewayRepo) Close() error {
	return r.db.Close()
}
