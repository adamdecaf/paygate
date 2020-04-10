// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package originators

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func deleteUserOriginator(logger log.Logger, originatorRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(logger, w, r)
		if responder == nil {
			return
		}

		origID := getOriginatorId(r)
		if err := originatorRepo.deleteUserOriginator(origID, responder.XUserID); err != nil {
			responder.Log("originators", fmt.Sprintf("problem deleting originator=%s: %v", origID, err))
			responder.Problem(err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (r *SQLOriginatorRepo) deleteUserOriginator(id model.OriginatorID, userID id.User) error {
	query := `update originators set deleted_at = ? where originator_id = ? and user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), id, userID)
	return err
}
