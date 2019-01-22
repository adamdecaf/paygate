// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/moov-io/base/k8s"
	ledger "github.com/moov-io/qledger-sdk-go"
)

func getQLedgerAddress() string {
	addr := os.Getenv("QLEDGER_ENDPOINT")
	if addr != "" {
		return addr
	}
	if k8s.Inside() {
		return "http://qledger.apps.svc.cluster.local:7000"
	}
	return "http://localhost:7000"
}

// func qledgerExperiments(api *ledger.Ledger) {
// 	if api == nil {
// 		panic("uhh, ledger.Ledger is nil")
// 	}
// 	fmt.Printf("ledgerAPI: %#v\n", api)

// 	acct := &ledger.Account{
// 		ID: fmt.Sprintf("%d", time.Now().Unix()),
// 		Balance: 100,
// 	}
// 	if err := api.CreateAccount(acct); err != nil {
// 		fmt.Printf("ERROR: %v\n", err)
// 	}
// 	fmt.Printf("Account ID: %s\n", acct.ID)

// 	// Search
// 	accounts, err := api.SearchAccounts(map[string]interface{}{
// 		"balance": map[string]interface{}{
// 			"gt": 50,
// 		},
// 	})
// 	if err != nil {
// 		fmt.Printf("ERROR: %v\n", err)
// 	}
// 	fmt.Printf("Found %d accounts\n", len(accounts))
// }

func qledgerExampleA(api *ledger.Ledger, cust *Customer, custDep *Depository, orig *Originator, origDep *Depository) (string, error) {
	id := nextID()

	data := make(map[string]interface{})
	data["customer"] = cust.ID
	data["customerDepository"] = custDep.ID
	data["originator"] = orig.ID
	data["originatorDepository"] = origDep.ID

	err := api.CreateTransaction(&ledger.Transaction{
		ID: id,
		Data: data,
		Lines: []*ledger.TransactionLine{
			{
				AccountID: fmt.Sprintf("%s-%s", custDep.RoutingNumber, "gl-code1"),
				Delta: 1000,
			},
			{
				AccountID: fmt.Sprintf("%s-%s", custDep.RoutingNumber, "gl-code2"),
				Delta: -1000,
			},
		},
	})
	if err != nil {
		return id, err
	}

	// Get Account balance (GL field)
	accountId := fmt.Sprintf("%s-%s", custDep.RoutingNumber, "gl-code1")
	account, err := api.GetAccount(accountId)
	if err != nil {
		return id, err
	}
	log.Printf("Account %s has balance: %d\n", accountId, account.Balance)
	return id, nil

	// Customer initiates a $500 ACH to another bank checking account.
	// Transfer.Amount = 50000

	// Customer checking account number is used to determine the correct customer account that increases by $500.
	// Transfer.CustomerDepository.RoutingNumber
	// Transfer.CustomerDepository.AccountNumber

	// store Customer.ID and Depository.ID on tx metadata

	// ACH deposit gets put into a batch to go to the Fed clearing house to be withdrawn from other financial institution based on the routing number etc (Funds are not “available” until transaction clears through the system which takes about a day/float).
	// createACHFile (from transfers.go)

	// All the customer non-interest bearing checking accounts are totaled up in GL/Call Report Code RCON6631 as a liability on the Call Report balance sheet line 13.
	// TODO(adam): There's no 6631 GL code..

	// The $500 deposit will also be shown on Schedule RC-E Deposit Liabilities total RCON2200 and detailed in 1. Column A as an individual transaction account GL/Call Report Code RCONB549

	// The $500 deposit will also be included in Schedule RC-O Other Data for Deposit Insurance and FICO Assessment RCONF049

	// No Interest paid

	// Call Report Income Statement - Service Fees could be charged to the following - Non-Interest Income 5b - Service Changes on Deposit Accounts - RIAD4080, 5.f. Net Servicing fees - RIADB492, 5.l. Other Non-Interest Income (See Schedule RI-E Explanations - 5.l.1.f. Bank card and credit card interchange fees - RIADF555, 5.l.1.g. Income and Fees from wire transfers - RIADT047, 5.l.2.a. Other non-interest expense - Data processing expense RIADC017.
}

// POST /accounts
// {
// 	id: <routing-number>-<gl-code>,
// }

// POST /transactions
// {
// 	id: <customer-account-number>-<random>,
// 	lines: [
// 		{
// 			account: <routing-number>-<gl-code>,
// 			delta: 100,
// 			data: {
// 				customerAccount: <customer-account-number>,
// 			},
// 		},
// 		{
// 			// add -100 to liabilities
// 		}
// 	]
// }

// Get GL balance:
// GET /accounts with id: <routing-number>-<gl-code>

// Get customer balance
// GET /transactions where fields.id eq <customer-account-number>, sum for current total
