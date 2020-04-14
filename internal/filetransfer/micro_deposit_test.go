// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package filetransfer

import (
	"testing"

	"github.com/moov-io/ach"
	"github.com/moov-io/paygate/internal/model"
)

func TestMicroDeposits__addMicroDepositDebit(t *testing.T) {
	ed := ach.NewEntryDetail()
	ed.TransactionCode = ach.CheckingCredit
	ed.TraceNumber = "123"
	ed.Amount = 12 // $0.12

	bh := ach.NewBatchHeader()
	bh.StandardEntryClassCode = "PPD"
	batch, err := ach.NewBatch(bh)
	if err != nil {
		t.Fatal(err)
	}
	batch.AddEntry(ed)

	file := ach.NewFile()
	file.AddBatch(batch)

	debitAmount, _ := model.NewAmount("USD", "0.14") // not $0.12 on purpose

	// nil, so expect no changes
	if err := addMicroDepositDebit(nil, debitAmount); err == nil {
		t.Fatal("expected error")
	}
	if len(file.Batches) != 1 || len(file.Batches[0].GetEntries()) != 1 {
		t.Fatalf("file.Batches[0]=%#v", file.Batches[0])
	}

	// add reversal batch
	if err := addMicroDepositDebit(file, debitAmount); err != nil {
		t.Fatal(err)
	}

	// verify
	if len(file.Batches) != 1 {
		t.Fatalf("file.Batches=%#v", file.Batches)
	}
	entries := file.Batches[0].GetEntries()
	if len(entries) != 2 {
		t.Fatalf("entries=%#v", entries)
	}
	if entries[0].TransactionCode-5 != entries[1].TransactionCode {
		t.Errorf("entries[0].TransactionCode=%d entries[1].TransactionCode=%d", entries[0].TransactionCode, entries[1].TransactionCode)
	}
	if entries[0].Amount != 12 {
		t.Errorf("entries[0].Amount=%d", entries[0].Amount)
	}
	if entries[1].Amount != 14 {
		t.Errorf("entries[1].Amount=%d", entries[1].Amount)
	}
	if entries[1].TraceNumber != "124" {
		t.Errorf("entries[1].TraceNumber=%s", entries[1].TraceNumber)
	}
}
