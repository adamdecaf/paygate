// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"database/sql"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

type Repository interface {
	GetDepository(id id.Depository) (*model.Depository, error) // admin endpoint
	getUserDepositories(userID id.User) ([]*model.Depository, error)
	GetUserDepository(id id.Depository, userID id.User) (*model.Depository, error)

	UpsertUserDepository(userID id.User, dep *model.Depository) error
	UpdateDepositoryStatus(id id.Depository, status model.DepositoryStatus) error
	deleteUserDepository(id id.Depository, userID id.User) error

	LookupDepositoryFromReturn(routingNumber string, accountNumber string) (*model.Depository, error)
}

func NewDepositoryRepo(logger log.Logger, db *sql.DB, keeper *secrets.StringKeeper) *SQLRepo {
	return &SQLRepo{logger: logger, db: db, keeper: keeper}
}

type SQLRepo struct {
	db     *sql.DB
	logger log.Logger
	keeper *secrets.StringKeeper
}

func (r *SQLRepo) Close() error {
	return r.db.Close()
}
