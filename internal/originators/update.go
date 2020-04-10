// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func updateUserOriginator(logger log.Logger, originatorRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		req, err := readOriginatorRequest(r)
		if err != nil {
			responder.Log("originators", err.Error())
			responder.Problem(err)
			return
		}

		origID := getOriginatorId(r)
		if origID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		orig, err := originatorRepo.GetUserOriginator(origID, responder.XUserID)
		if err != nil {
			responder.Problem(err)
			return
		}
		if orig == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if req.DefaultDepository != "" {
			orig.DefaultDepository = req.DefaultDepository
		}
		if req.Identification != "" {
			orig.Identification = req.Identification
		}
		if req.Metadata != "" {
			orig.Metadata = req.Metadata
		}
		if err := orig.Validate(); err != nil {
			responder.Problem(err)
			return
		}

		if err := originatorRepo.updateUserOriginator(responder.XUserID, orig); err != nil {
			responder.Problem(err)
			return
		}

		responder.Respond(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
		})
	}
}

func (r *SQLOriginatorRepo) updateUserOriginator(userID id.User, orig *model.Originator) error {
	query := `update originators set default_depository = ?, identification = ?, metadata = ?
where originator_id = ? and user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(orig.DefaultDepository, orig.Identification, orig.Metadata, orig.ID, userID)
	return err
}
