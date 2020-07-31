// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package inbound

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/moov-io/ach"
	"github.com/moov-io/paygate/pkg/client"
	"github.com/moov-io/paygate/pkg/model"
	"github.com/moov-io/paygate/pkg/transfers"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

var (
	returnEntriesProcessed = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "return_entries_processed",
		Help: "Counter of return EntryDetail records processed",
	}, []string{"origin", "destination", "code"})

	missingReturnTransfers = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "missing_return_transfers",
		Help: "Counter of return EntryDetail records handled without a found transfer",
	}, []string{"origin", "destination", "code"})
)

type returnProcessor struct {
	logger       log.Logger
	transferRepo transfers.Repository
}

func NewReturnProcessor(logger log.Logger, transferRepo transfers.Repository) *returnProcessor {
	return &returnProcessor{
		logger:       logger,
		transferRepo: transferRepo,
	}
}

func (pc *returnProcessor) Type() string {
	return "return"
}

func (pc *returnProcessor) Handle(event File) error {
	if !isReturnFile(event.File) {
		return nil
	}

	pc.logger.Log("inbound", "processing return file", "origin", event.File.Header.ImmediateOrigin, "destination", event.File.Header.ImmediateDestination)

	for i := range event.File.ReturnEntries {
		entries := event.File.ReturnEntries[i].GetEntries()
		for j := range entries {
			if entries[j].Addenda99 == nil {
				continue
			}

			returnEntriesProcessed.With(
				"origin", event.File.Header.ImmediateOrigin,
				"destination", event.File.Header.ImmediateDestination,
				"code", entries[j].Addenda99.ReturnCodeField().Code,
			).Add(1)

			bh := event.File.ReturnEntries[i].GetHeader()
			if err := pc.processReturnEntry(event.File.Header, bh, entries[j]); err != nil {
				return err // TODO(adam): should we just log here?
			}
		}
	}
	return nil
}

func isReturnFile(file *ach.File) bool {
	return (file != nil) && (len(file.ReturnEntries) > 0)
}

func (pc *returnProcessor) processReturnEntry(fh ach.FileHeader, bh *ach.BatchHeader, entry *ach.EntryDetail) error {
	amount, err := model.NewAmountFromInt("USD", entry.Amount)
	if err != nil {
		return fmt.Errorf("invalid amount: %v", entry.Amount)
	}
	effectiveEntryDate, err := time.Parse("060102", bh.EffectiveEntryDate) // YYMMDD
	if err != nil {
		return fmt.Errorf("invalid EffectiveEntryDate=%q: %v", bh.EffectiveEntryDate, err)
	}

	// Do we find a Transfer related to the ach.EntryDetail?
	transfer, err := pc.transferRepo.LookupTransferFromReturn(amount, entry.TraceNumber, effectiveEntryDate)
	if transfer != nil {
		pc.logger.Log("inbound", fmt.Sprintf("handling return for transferID=%s", transfer.TransferID))
		if err := SaveReturnCode(pc.transferRepo, transfer.TransferID, entry); err != nil {
			return err
		}
		if err := pc.transferRepo.UpdateTransferStatus(transfer.TransferID, client.FAILED); err != nil {
			return fmt.Errorf("problem marking transferID=%s as %s: %v", transfer.TransferID, client.FAILED, err)
		}
		// TODO(adam): We need to update the Customer/Account from return codes
		// R02 (Account Closed) -- mark account Disabled / Rejected / (new status)
		// R03 (No Account)
		// R04 (Invalid Account Number)
		// R07 (Authorization Revoked by Customer)
		// R10 (Customer Advises Not Authorized)
		// R14 (Representative payee deceased)
		// R16 (Bank account frozen)
	} else {
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("problem with returned Transfer: %v", err)
		}
		pc.logger.Log("inbound", fmt.Sprintf("transfer not found from return entry, traceNumber=%s", entry.TraceNumber))
		missingReturnTransfers.With(
			"origin", fh.ImmediateOrigin,
			"destination", fh.ImmediateDestination,
			"code", entry.Addenda99.ReturnCodeField().Code).Add(1)
	}

	// TODO(adam): lookup any micro-deposits from the transferID

	return nil
}

func SaveReturnCode(repo transfers.Repository, transferID string, ed *ach.EntryDetail) error {
	if repo == nil {
		return errors.New("nil Repository")
	}
	if ed == nil || ed.Addenda99 == nil {
		return errors.New("nil ach.EntryDetail or missing Addenda99")
	}
	returnCode := ed.Addenda99.ReturnCodeField()
	if returnCode != nil {
		if err := repo.SaveReturnCode(transferID, returnCode.Code); err != nil {
			return fmt.Errorf("problem saving transferID=%s return code: %s: %v", transferID, returnCode.Code, err)
		}
	}
	return nil
}
