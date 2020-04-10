// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depository

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (r *Router) deleteUserDepository() http.HandlerFunc {
	return func(w http.ResponseWriter, httpReq *http.Request) {
		responder := route.NewResponder(r.logger, w, httpReq)
		if responder == nil {
			return
		}

		depID := GetID(httpReq)
		if depID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Currently we don't delete any pending Transfers associated to this Depository.
		// This could be done, but isn't as we're relying on the caller to delete Transfers they don't
		// want sent off to the ODFI.

		if err := r.depositoryRepo.deleteUserDepository(depID, responder.XUserID); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (r *SQLRepo) deleteUserDepository(id id.Depository, userID id.User) error {
	query := `update depositories set deleted_at = ? where depository_id = ? and user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(time.Now(), id, userID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error deleting depository_id=%q, user_id=%q: %v", id, userID, err)
	}
	return nil
}
