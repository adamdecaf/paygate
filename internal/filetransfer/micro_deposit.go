// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"errors"
	"strconv"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	"github.com/moov-io/paygate/internal/model"
)

func addMicroDeposit(file *ach.File, amt model.Amount) error {
	if file == nil || len(file.Batches) != 1 || len(file.Batches[0].GetEntries()) != 1 {
		return errors.New("invalid micro-deposit ACH file for deposits")
	}

	// Copy the EntryDetail and replace TransactionCode
	ed := *file.Batches[0].GetEntries()[0] // copy previous EntryDetail
	ed.ID = base.ID()[:8]

	// increment trace number
	if n, _ := strconv.Atoi(ed.TraceNumber); n > 0 {
		ed.TraceNumber = strconv.Itoa(n + 1)
	}

	// use our calculated amount to debit all micro-deposits
	ed.Amount = amt.Int()

	// append our new EntryDetail
	file.Batches[0].AddEntry(&ed)

	return nil
}

func addMicroDepositDebit(file *ach.File, debitAmount *model.Amount) error {
	// we expect two EntryDetail records (one for each micro-deposit)
	if file == nil || len(file.Batches) != 1 || len(file.Batches[0].GetEntries()) < 1 {
		return errors.New("invalid micro-deposit ACH file for debit")
	}

	// We need to adjust ServiceClassCode as this batch has a debit and credit now
	bh := file.Batches[0].GetHeader()
	bh.ServiceClassCode = ach.MixedDebitsAndCredits
	file.Batches[0].SetHeader(bh)

	// Copy the EntryDetail and replace TransactionCode
	entries := file.Batches[0].GetEntries()
	ed := *entries[len(entries)-1] // take last entry detail
	ed.ID = base.ID()[:8]
	// TransactionCodes seem to follow a simple pattern:
	//  37 SavingsDebit -> 32 SavingsCredit
	//  27 CheckingDebit -> 22 CheckingCredit
	ed.TransactionCode -= 5

	// increment trace number
	if n, _ := strconv.Atoi(ed.TraceNumber); n > 0 {
		ed.TraceNumber = strconv.Itoa(n + 1)
	}

	// use our calculated amount to debit all micro-deposits
	ed.Amount = debitAmount.Int()

	// append our new EntryDetail
	file.Batches[0].AddEntry(&ed)

	return nil
}
