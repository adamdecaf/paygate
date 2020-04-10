// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package depadmin

import (
	"encoding/json"
	"fmt"
	"net/http"

	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/depository"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/route"

	"github.com/go-kit/kit/log"
)

type request struct {
	Status model.DepositoryStatus `json:"status"`
}

func overrideDepositoryStatus(logger log.Logger, repo depository.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = route.Wrap(logger, w, r)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if r.Method != "PUT" {
			moovhttp.Problem(w, fmt.Errorf("unsupported HTTP verb: %s", r.Method))
			return
		}

		depID, requestID := depository.GetID(r), moovhttp.GetRequestID(r)

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		// read the depository so we know it exists
		dep, err := repo.GetDepository(depID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}
		if err := repo.UpdateDepositoryStatus(depID, req.Status); err != nil {
			moovhttp.Problem(w, err)
			return
		}
		// re-read for marshaling
		dep, err = repo.GetUserDepository(depID, dep.UserID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		logger.Log("depositories", fmt.Sprintf("updated depository=%s to %s", depID, req.Status), "requestID", requestID)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(dep)
	}
}
