// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package gateways

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

type gatewayRequest struct {
	Origin          string `json:"origin"`
	OriginName      string `json:"originName"`
	Destination     string `json:"destination"`
	DestinationName string `json:"destinationName"`
}

func (r gatewayRequest) missingFields() error {
	if r.Origin == "" {
		return errors.New("missing gatewayRequest.Origin")
	}
	if r.OriginName == "" {
		return errors.New("missing gatewayRequest.OriginName")
	}
	if r.Destination == "" {
		return errors.New("missing gatewayRequest.Destination")
	}
	if r.DestinationName == "" {
		return errors.New("missing gatewayRequest.DestinationName")
	}
	return nil
}

func updateUserGateway(logger log.Logger, gatewayRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		var wrapper gatewayRequest
		if err := json.NewDecoder(route.Read(r.Body)).Decode(&wrapper); err != nil {
			responder.Problem(err)
			return
		}
		if err := wrapper.missingFields(); err != nil {
			responder.Problem(fmt.Errorf("%v: %v", route.ErrMissingRequiredJson, err))
			return
		}

		gateway, err := gatewayRepo.updateUserGateway(responder.XUserID, wrapper)
		if err != nil {
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(gateway)
		})
	}
}

func (r *SQLGatewayRepo) updateUserGateway(userID id.User, req gatewayRequest) (*model.Gateway, error) {
	gateway := &model.Gateway{
		Origin:          req.Origin,
		OriginName:      req.OriginName,
		Destination:     req.Destination,
		DestinationName: req.DestinationName,
		Created:         base.NewTime(time.Now()),
	}
	if err := gateway.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	query := `select gateway_id from gateways where user_id = ? and deleted_at is null`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(userID)

	var gatewayID string
	err = row.Scan(&gatewayID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("createUserGateway: scan error=%v rollback=%v", err, tx.Rollback())
	}
	if gatewayID == "" {
		gatewayID = base.ID()
	}
	gateway.ID = model.GatewayID(gatewayID)

	// insert/update row
	query = `insert into gateways (gateway_id, user_id, origin, origin_name, destination, destination_name, created_at) values (?, ?, ?, ?, ?, ?, ?)`
	stmt, err = tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("createUserGateway: prepare error=%v rollback=%v", err, tx.Rollback())
	}
	defer stmt.Close()

	_, err = stmt.Exec(gatewayID, userID, gateway.Origin, gateway.OriginName, gateway.Destination, gateway.DestinationName, gateway.Created.Time)
	stmt.Close()
	if err != nil {
		// We need to update the row as it already exists.
		if database.UniqueViolation(err) {
			query = `update gateways set origin = ?, origin_name = ?, destination = ?, destination_name = ? where gateway_id = ? and user_id = ?`
			stmt, err = tx.Prepare(query)
			if err != nil {
				return nil, fmt.Errorf("createUserGateway: update: error=%v rollback=%v", err, tx.Rollback())
			}
			defer stmt.Close()
			_, err = stmt.Exec(gateway.Origin, gateway.OriginName, gateway.Destination, gateway.DestinationName, gatewayID, userID)
			stmt.Close()
			if err != nil {
				return nil, fmt.Errorf("createUserGateway: update exec: error=%v rollback=%v", err, tx.Rollback())
			}
		} else {
			return nil, fmt.Errorf("createUserGateway: exec error=%v rollback=%v", err, tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return gateway, nil
}
