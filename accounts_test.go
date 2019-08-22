// Copyright 2019 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	accounts "github.com/moov-io/accounts/client"
	"github.com/moov-io/base"
	"github.com/moov-io/base/docker"

	"github.com/go-kit/kit/log"
	"github.com/ory/dockertest"
)

type testAccountsClient struct {
	accounts    []accounts.Account
	transaction *accounts.Transaction

	postedTransactions []accountsTransaction

	err error
}

type accountsTransaction struct {
	Lines []transactionLine
}

func (c *testAccountsClient) Ping() error {
	return c.err
}

func (c *testAccountsClient) PostTransaction(_, _ string, lines []transactionLine) (*accounts.Transaction, error) {
	if len(lines) == 0 {
		return nil, errors.New("no transactionLine's")
	}
	if c.err != nil {
		return nil, c.err
	}
	c.postedTransactions = append(c.postedTransactions, accountsTransaction{
		Lines: lines,
	})
	return c.transaction, nil // yea, this doesn't match, but callers are expected to override testAccountsClient properties
}

func (c *testAccountsClient) SearchAccounts(_, _ string, _ searchRequest) (*accounts.Account, error) {
	if c.err != nil {
		return nil, c.err
	}
	if len(c.accounts) > 0 {
		return &c.accounts[0], nil
	}
	return nil, nil
}

func (c *testAccountsClient) ReverseTransaction(requestID, userID string, transactionID string) error {
	return c.err
}

type accountsDeployment struct {
	res    *dockertest.Resource
	client AccountsClient
}

func (d *accountsDeployment) close(t *testing.T) {
	if err := d.res.Close(); err != nil {
		t.Error(err)
	}
}

func spawnAccounts(t *testing.T) *accountsDeployment {
	// no t.Helper() call so we know where it failed

	if testing.Short() {
		t.Skip("-short flag enabled")
	}
	if !docker.Enabled() {
		t.Skip("Docker not enabled")
	}

	// Spawn Accounts docker image
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatal(err)
	}
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "moov/accounts",
		Tag:        "v0.4.0",
		Cmd:        []string{"-http.addr=:8080"},
		Env: []string{
			"DEFAULT_ROUTING_NUMBER=121042882",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	addr := fmt.Sprintf("http://localhost:%s", resource.GetPort("8080/tcp"))
	client := createAccountsClient(log.NewNopLogger(), addr, nil)
	err = pool.Retry(func() error {
		return client.Ping()
	})
	if err != nil {
		t.Fatal(err)
	}
	return &accountsDeployment{resource, client}
}

func TestAccounts__client(t *testing.T) {
	endpoint := ""
	if client := createAccountsClient(log.NewNopLogger(), endpoint, nil); client == nil {
		t.Fatal("expected non-nil client")
	}

	// Spawn Accounts Docker image and ping against it
	deployment := spawnAccounts(t)
	if err := deployment.client.Ping(); err != nil {
		t.Fatal(err)
	}
	deployment.close(t) // close only if successful
}

func TestAccounts(t *testing.T) {
	deployment := spawnAccounts(t)
	client, ok := deployment.client.(*moovAccountsClient)
	if !ok {
		t.Fatalf("got %T", deployment.client)
	}

	userID := base.ID()

	// Create accounts behind the scenes
	fromAccount, err := createAccount(client, "from account", "Savings", userID)
	if err != nil {
		t.Fatal(err)
	}
	toAccount, err := createAccount(client, "to account", "Savings", userID)
	if err != nil {
		t.Fatal(err)
	}

	// Setup our Transaction
	lines := []transactionLine{
		{AccountID: toAccount.ID, Purpose: "achcredit", Amount: 10000},
		{AccountID: fromAccount.ID, Purpose: "achdebit", Amount: 10000},
	}
	tx, err := deployment.client.PostTransaction(base.ID(), userID, lines)
	if err != nil || tx == nil {
		t.Fatalf("transaction=%v error=%v", tx, err)
	}

	// Verify From Account
	account, err := deployment.client.SearchAccounts(base.ID(), userID, searchRequest{
		accountNumber: fromAccount.AccountNumber,
		routingNumber: fromAccount.RoutingNumber,
		accountType:   "Savings",
	})
	if err != nil {
		t.Fatal(err)
	}
	if account.Balance != 90000 { // $900
		t.Errorf("fromAccount balance: %d", account.Balance)
	}

	// Verify To Account
	account, err = deployment.client.SearchAccounts(base.ID(), userID, searchRequest{
		accountNumber: toAccount.AccountNumber,
		routingNumber: toAccount.RoutingNumber,
		accountType:   "Savings",
	})
	if err != nil {
		t.Fatal(err)
	}
	if account.Balance != 110000 { // $1100
		t.Errorf("fromAccount balance: %d", account.Balance)
	}

	deployment.close(t) // close only if successful
}

func createAccount(api *moovAccountsClient, name, tpe string, userID string) (*accounts.Account, error) {
	ctx := context.TODO()
	req := accounts.CreateAccount{CustomerID: userID, Name: name, Type: tpe, Balance: 1000 * 100}

	account, resp, err := api.underlying.AccountsApi.CreateAccount(ctx, userID, req, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("problem creating account %s: %v", name, err)
	}
	return &account, nil
}

func TestAccounts__ReverseTransaction(t *testing.T) {
	deployment := spawnAccounts(t)
	client, ok := deployment.client.(*moovAccountsClient)
	if !ok {
		t.Fatalf("got %T", deployment.client)
	}

	userID := base.ID()

	// Create accounts behind the scenes
	fromAccount, err := createAccount(client, "from account", "Savings", userID)
	if err != nil {
		t.Fatal(err)
	}
	toAccount, err := createAccount(client, "to account", "Savings", userID)
	if err != nil {
		t.Fatal(err)
	}

	// Setup our Transaction
	lines := []transactionLine{
		{AccountID: toAccount.ID, Purpose: "achcredit", Amount: 10000},
		{AccountID: fromAccount.ID, Purpose: "achdebit", Amount: 10000},
	}
	tx, err := deployment.client.PostTransaction(base.ID(), userID, lines)
	if err != nil || tx == nil {
		t.Fatalf("transaction=%v error=%v", tx, err)
	}

	// Reverse the posted Transaction
	if err := client.ReverseTransaction("", userID, tx.ID); err != nil {
		t.Fatal(err)
	}

	deployment.close(t) // close only if successful
}
