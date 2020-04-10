// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"
	"github.com/moov-io/paygate/pkg/id"
)

func (c *TransferRouter) deleteUserTransfer() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responder := route.NewResponder(c.logger, w, r)
		if responder == nil {
			return
		}

		transferID := GetID(r)
		transfer, err := c.transferRepo.getUserTransfer(transferID, responder.XUserID)
		if err != nil {
			responder.Log("transfers", fmt.Sprintf("error reading transfer=%s for deletion: %v", transferID, err))
			responder.Problem(err)
			return
		}
		if transfer.Status != model.TransferPending {
			responder.Problem(fmt.Errorf("a %s transfer can't be deleted", transfer.Status))
			return
		}

		// cancel and delete the transfer
		if err := c.transferRepo.UpdateTransferStatus(transferID, model.TransferCanceled); err != nil {
			responder.Problem(err)
			return
		}
		if err := c.transferRepo.deleteUserTransfer(transferID, responder.XUserID); err != nil {
			responder.Problem(err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (r *SQLRepo) deleteUserTransfer(id id.Transfer, userID id.User) error {
	query := `update transfers set deleted_at = ? where transfer_id = ? and user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), id, userID)
	return err
}
