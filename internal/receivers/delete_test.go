// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package receivers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func TestReceivers__HTTPDelete(t *testing.T) {
	userID, now := base.ID(), time.Now()
	rec := &model.Receiver{
		ID:                model.ReceiverID(base.ID()),
		Email:             "foo@moov.io",
		DefaultDepository: id.Depository(base.ID()),
		Status:            model.ReceiverVerified,
		Metadata:          "other",
		Created:           base.NewTime(now),
		Updated:           base.NewTime(now),
	}
	repo := &MockRepository{
		Receivers: []*model.Receiver{rec},
	}

	router := mux.NewRouter()
	AddReceiverRoutes(log.NewNopLogger(), router, nil, nil, repo)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/receivers/%s", rec.ID), nil)
	req.Header.Set("x-user-id", userID)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("bogus HTTP status: %d: %s", w.Code, w.Body.String())
	}
}
