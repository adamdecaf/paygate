// Copyright 2019 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/moov-io/base/admin"
	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
)

func addFileTransferSyncRoute(logger log.Logger, svc *admin.Server, flushIncoming chan struct{}, flushOutging chan struct{}) {
	svc.AddHandler("/files/flush/incoming", flushFiles(logger, flushIncoming, flushOutging))
	svc.AddHandler("/files/flush/outgoing", flushFiles(logger, flushIncoming, flushOutging))
	svc.AddHandler("/files/flush", flushFiles(logger, flushIncoming, flushOutging))
}

func flushFiles(logger log.Logger, flushIncoming chan struct{}, flushOutging chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			moovhttp.Problem(w, fmt.Errorf("unsupported HTTP verb %s", r.Method))
			return
		}
		requestID := moovhttp.GetRequestID(r)

		parts := strings.Split(r.URL.Path, "/")
		switch parts[len(parts)-1] {
		case "incoming":
			if err := flushIncomingFiles(logger, flushIncoming); err != nil {
				logger.Log("flushFiles", fmt.Sprintf("problem flushing incoming files: %v", err), "requestID", requestID)
				moovhttp.Problem(w, err)
				return
			}
		case "outgoing":
			if err := flushOutgingFiles(logger, flushOutging); err != nil {
				logger.Log("flushFiles", fmt.Sprintf("problem flushing outgoing files: %v", err), "requestID", requestID)
				moovhttp.Problem(w, err)
				return
			}
		default:
			if r.URL.Path == "/files/flush" {
				inerr := flushIncomingFiles(logger, flushIncoming)
				outerr := flushOutgingFiles(logger, flushOutging)
				if inerr != nil || outerr != nil {
					err := fmt.Errorf("problem flushing incoming=%v outgoing=%v", inerr, outerr)
					logger.Log("flushFiles", err, "requestID", requestID)
					moovhttp.Problem(w, err)
				} else {
					logger.Log("flushFiles", "flushed incoming and outgoing files", "requestID", requestID)
					w.WriteHeader(http.StatusOK)
				}
			} else {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}
}

// flushIncomingFiles will download all inbound and return files to process
// them. (Updating Depositories, Transfers, micro-deposits, etc..)
func flushIncomingFiles(logger log.Logger, flushIncoming chan struct{}) error {
	logger.Log("flushIncomingFiles", "TODO(adam)")
	flushIncoming <- struct{}{}
	return nil
}

// flushOutgingFiles will initiate paygate's "merge and upload" loop of ACH
// payments to their ODFI.
func flushOutgingFiles(logger log.Logger, flushOutging chan struct{}) error {
	logger.Log("flushOutgingFiles", "TODO(adam)")
	flushOutging <- struct{}{}
	return nil
}
