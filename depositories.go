// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moov-io/ach"
	"github.com/moov-io/base"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/secrets"
	"github.com/moov-io/paygate/pkg/achclient"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type DepositoryID string

func (id DepositoryID) empty() bool {
	return string(id) == ""
}

type Depository struct {
	// ID is a unique string representing this Depository.
	ID DepositoryID `json:"id"`

	// BankName is the legal name of the financial institution.
	BankName string `json:"bankName"`

	// Holder is the legal holder name on the account
	Holder string `json:"holder"`

	// HolderType defines the type of entity of the account holder as an individual or company
	HolderType HolderType `json:"holderType"`

	// Type defines the account as checking or savings
	Type AccountType `json:"type"`

	// RoutingNumber is the ABA routing transit number for the depository account.
	RoutingNumber string `json:"routingNumber"`

	// Status defines the current state of the Depository
	Status DepositoryStatus `json:"status"`

	// Metadata provides additional data to be used for display and search only
	Metadata string `json:"metadata"`

	// Created a timestamp representing the initial creation date of the object in ISO 8601
	Created base.Time `json:"created"`

	// Updated is a timestamp when the object was last modified in ISO8601 format
	Updated base.Time `json:"updated"`

	// encryption related
	encryptedAccountNumber string
	maskedAccountNumber    string
}

func (d *Depository) validate() error {
	if d == nil {
		return errors.New("nil Depository")
	}
	if err := d.HolderType.validate(); err != nil {
		return err
	}
	if err := d.Type.validate(); err != nil {
		return err
	}
	if err := d.Status.validate(); err != nil {
		return err
	}
	if err := ach.CheckRoutingNumber(d.RoutingNumber); err != nil {
		return err
	}
	return nil
}

func (d *Depository) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID            DepositoryID     `json:"id"`
		BankName      string           `json:"bankName"`
		Holder        string           `json:"holder"`
		HolderType    HolderType       `json:"holderType"`
		Type          AccountType      `json:"type"`
		AccountNumber string           `json:"accountNumber"`
		RoutingNumber string           `json:"routingNumber"`
		Status        DepositoryStatus `json:"status"`
		Metadata      string           `json:"metadata"`
		Created       base.Time        `json:"created"`
		Updated       base.Time        `json:"updated"`
	}{
		d.ID,
		d.BankName,
		d.Holder,
		d.HolderType,
		d.Type,
		d.maskedAccountNumber,
		d.RoutingNumber,
		d.Status,
		d.Metadata,
		d.Created,
		d.Updated,
	})
}

func (d *Depository) decryptAccountNumber(keeperFactory secrets.SecretFunc) (string, error) {
	if d == nil || d.encryptedAccountNumber == "" {
		return "", errors.New("decryptAccountNumber: nil Depository or empty encrypted accoutn number")
	}
	keeper, err := keeperFactory(d.encryptionKey())
	if err != nil {
		return "", err
	}
	bs, err := base64.StdEncoding.DecodeString(d.encryptedAccountNumber)
	if err != nil {
		return "", err
	}
	out, err := keeper.Decrypt(context.Background(), bs)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type depositoryRequest struct {
	BankName      string      `json:"bankName,omitempty"`
	Holder        string      `json:"holder,omitempty"`
	HolderType    HolderType  `json:"holderType,omitempty"`
	Type          AccountType `json:"type,omitempty"`
	RoutingNumber string      `json:"routingNumber,omitempty"`
	AccountNumber string      `json:"accountNumber,omitempty"`
	Metadata      string      `json:"metadata,omitempty"`
}

func (r depositoryRequest) missingFields() error {
	if r.BankName == "" {
		return errors.New("missing depositoryRequest.BankName")
	}
	if r.Holder == "" {
		return errors.New("missing depositoryRequest.Holder")
	}
	if r.HolderType == "" {
		return errors.New("missing depositoryRequest.HolderType")
	}
	if r.Type == "" {
		return errors.New("missing depositoryRequest.Type")
	}
	if r.RoutingNumber == "" {
		return errors.New("missing depositoryRequest.RoutingNumber")
	}
	if r.AccountNumber == "" {
		return errors.New("missing depositoryRequest.AccountNumber")
	}
	return nil
}

type HolderType string

const (
	Individual HolderType = "individual"
	Business   HolderType = "business"
)

func (t *HolderType) empty() bool {
	return string(*t) == ""
}

func (t HolderType) validate() error {
	if t.empty() {
		return errors.New("empty HolderType")
	}
	switch t {
	case Individual, Business:
		return nil
	default:
		return fmt.Errorf("HolderType(%s) is invalid", t)
	}
}

func (t *HolderType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*t = HolderType(strings.ToLower(s))
	if err := t.validate(); err != nil {
		return err
	}
	return nil
}

type DepositoryStatus string

const (
	DepositoryUnverified DepositoryStatus = "unverified"
	DepositoryVerified   DepositoryStatus = "verified"
	DepositoryRejected   DepositoryStatus = "rejected"
)

func (ds DepositoryStatus) empty() bool {
	return string(ds) == ""
}

func (ds DepositoryStatus) validate() error {
	switch ds {
	case DepositoryUnverified, DepositoryVerified, DepositoryRejected:
		return nil
	default:
		return fmt.Errorf("DepositoryStatus(%s) is invalid", ds)
	}
}

func (ds *DepositoryStatus) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*ds = DepositoryStatus(strings.ToLower(s))
	if err := ds.validate(); err != nil {
		return err
	}
	return nil
}

// depositoryIdExists checks if a given DepositoryID belongs to the userID
func depositoryIdExists(userID string, id DepositoryID, repo depositoryRepository) bool {
	dep, err := repo.getUserDepository(id, userID)
	if err != nil || dep == nil {
		return false
	}
	return dep.ID == id
}

func addDepositoryRoutes(logger log.Logger, r *mux.Router, keeperFactory secrets.SecretFunc, odfiAccount *odfiAccount, accountsCallsDisabled bool, accountsClient AccountsClient, achClient *achclient.ACH, fedClient FEDClient, ofacClient OFACClient, depositoryRepo depositoryRepository, eventRepo eventRepository) {
	r.Methods("GET").Path("/depositories").HandlerFunc(getUserDepositories(logger, depositoryRepo))
	r.Methods("POST").Path("/depositories").HandlerFunc(createUserDepository(logger, keeperFactory, fedClient, ofacClient, depositoryRepo))

	r.Methods("GET").Path("/depositories/{depositoryId}").HandlerFunc(getUserDepository(logger, depositoryRepo))
	r.Methods("PATCH").Path("/depositories/{depositoryId}").HandlerFunc(updateUserDepository(logger, depositoryRepo))
	r.Methods("DELETE").Path("/depositories/{depositoryId}").HandlerFunc(deleteUserDepository(logger, depositoryRepo))

	if accountsCallsDisabled {
		accountsClient = nil // zero out so micro-deposit route doesn't call it
	}
	r.Methods("POST").Path("/depositories/{depositoryId}/micro-deposits").HandlerFunc(initiateMicroDeposits(logger, odfiAccount, accountsClient, achClient, depositoryRepo, eventRepo))
	r.Methods("POST").Path("/depositories/{depositoryId}/micro-deposits/confirm").HandlerFunc(confirmMicroDeposits(logger, depositoryRepo))
}

// GET /depositories
// response: [ depository ]
func getUserDepositories(logger log.Logger, depositoryRepo depositoryRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		userID := moovhttp.GetUserID(r)
		deposits, err := depositoryRepo.getUserDepositories(userID)
		if err != nil {
			internalError(logger, w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(deposits); err != nil {
			internalError(logger, w, err)
			return
		}
	}
}

func readDepositoryRequest(r *http.Request) (depositoryRequest, error) {
	var req depositoryRequest
	bs, err := read(r.Body)
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(bs, &req); err != nil {
		return req, err
	}
	return req, nil
}

func maskAccountNumber(s string) string {
	if utf8.RuneCountInString(s) <= 4 {
		return "####" // too short, we can't mask anything
	} else {
		return fmt.Sprintf("%s%s", strings.Repeat("#", len(s)-4), s[len(s)-4:])
	}
}

func (d *Depository) encryptionKey() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("depository-%s-account-number", d.ID)
}

func encryptAccountNumber(keeperFactory secrets.SecretFunc, dep *Depository, number string) (string, error) {
	keeper, err := keeperFactory(dep.encryptionKey())
	if err != nil {
		return "", err
	}
	encrypted, err := keeper.Encrypt(context.Background(), []byte(number))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// POST /depositories
// request: model w/o ID
// response: 201 w/ depository json
func createUserDepository(logger log.Logger, keeperFactory secrets.SecretFunc, fedClient FEDClient, ofacClient OFACClient, depositoryRepo depositoryRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		requestID, userID := moovhttp.GetRequestID(r), moovhttp.GetUserID(r)

		req, err := readDepositoryRequest(r)
		if err != nil {
			logger.Log("depositories", err.Error(), "requestID", requestID, "userID", userID)
			moovhttp.Problem(w, err)
			return
		}
		if err := req.missingFields(); err != nil {
			err = fmt.Errorf("%v: %v", errMissingRequiredJson, err)
			moovhttp.Problem(w, err)
			return
		}

		now := time.Now()
		depository := &Depository{
			ID:                  DepositoryID(base.ID()),
			BankName:            req.BankName,
			Holder:              req.Holder,
			HolderType:          req.HolderType,
			Type:                req.Type,
			RoutingNumber:       req.RoutingNumber,
			maskedAccountNumber: maskAccountNumber(req.AccountNumber),
			Status:              DepositoryUnverified,
			Metadata:            req.Metadata,
			Created:             base.NewTime(now),
			Updated:             base.NewTime(now),
		}
		if v, err := encryptAccountNumber(keeperFactory, depository, req.AccountNumber); err == nil {
			depository.encryptedAccountNumber = v
		} else {
			moovhttp.Problem(w, err)
			return
		}
		if err := depository.validate(); err != nil {
			logger.Log("depositories", err.Error(), "requestID", requestID, "userID", userID)
			moovhttp.Problem(w, err)
			return
		}

		// TODO(adam): We should check and reject duplicate Depositories (by ABA and AccountNumber) on creation

		// Check FED for the routing number
		if err := fedClient.LookupRoutingNumber(req.RoutingNumber); err != nil {
			logger.Log("depositories", fmt.Sprintf("problem with FED routing number lookup %q: %v", req.RoutingNumber, err.Error()), "requestID", requestID, "userID", userID)
			moovhttp.Problem(w, err)
			return
		}

		// Check OFAC for customer/company data
		if err := rejectViaOFACMatch(logger, ofacClient, depository.Holder, userID, requestID); err != nil {
			logger.Log("depositories", err.Error(), "requestID", requestID, "userID", userID)
			moovhttp.Problem(w, err)
			return
		}

		if err := depositoryRepo.upsertUserDepository(userID, depository); err != nil {
			logger.Log("depositories", err.Error(), "requestID", requestID, "userID", userID)
			internalError(logger, w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(depository)
	}
}

func getUserDepository(logger log.Logger, depositoryRepo depositoryRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		id, userID := getDepositoryID(r), moovhttp.GetUserID(r)
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		depository, err := depositoryRepo.getUserDepository(id, userID)
		if err != nil {
			logger.Log("depositories", err.Error(), "requestID", moovhttp.GetRequestID(r), "userID", userID)
			moovhttp.Problem(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(depository)
	}
}

func updateUserDepository(logger log.Logger, depositoryRepo depositoryRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		req, err := readDepositoryRequest(r)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		id, userID := getDepositoryID(r), moovhttp.GetUserID(r)
		if id == "" || userID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		depository, err := depositoryRepo.getUserDepository(id, userID)
		if err != nil {
			logger.Log("depositories", err.Error(), "requestID", moovhttp.GetRequestID(r), "userID", userID)
			internalError(logger, w, err)
			return
		}
		if depository == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Update model
		var requireValidation bool
		if req.BankName != "" {
			depository.BankName = req.BankName
		}
		if req.Holder != "" {
			depository.Holder = req.Holder
		}
		if req.HolderType != "" {
			depository.HolderType = req.HolderType
		}
		if req.Type != "" {
			depository.Type = req.Type
		}
		if req.RoutingNumber != "" {
			if err := ach.CheckRoutingNumber(req.RoutingNumber); err != nil {
				moovhttp.Problem(w, err)
				return
			}
			requireValidation = true
			depository.RoutingNumber = req.RoutingNumber
		}
		if req.AccountNumber != "" {
			requireValidation = true
			// depository.AccountNumber = req.AccountNumber // TODO(adam): we need to re-encrypt here
		}
		if req.Metadata != "" {
			depository.Metadata = req.Metadata
		}
		depository.Updated = base.NewTime(time.Now())

		if requireValidation {
			depository.Status = DepositoryUnverified
		}

		if err := depository.validate(); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		if err := depositoryRepo.upsertUserDepository(userID, depository); err != nil {
			logger.Log("depositories", err.Error(), "requestID", moovhttp.GetRequestID(r), "userID", userID)
			internalError(logger, w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(depository)
	}
}

func deleteUserDepository(logger log.Logger, depositoryRepo depositoryRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		id, userID := getDepositoryID(r), moovhttp.GetUserID(r)
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if err := depositoryRepo.deleteUserDepository(id, userID); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}
}

// getDepositoryID extracts the DepositoryID from the incoming request.
func getDepositoryID(r *http.Request) DepositoryID {
	v := mux.Vars(r)
	id, ok := v["depositoryId"]
	if !ok {
		return DepositoryID("")
	}
	return DepositoryID(id)
}

func markDepositoryVerified(repo depositoryRepository, id DepositoryID, userID string) error {
	dep, err := repo.getUserDepository(id, userID)
	if err != nil {
		return fmt.Errorf("markDepositoryVerified: depository %v (userID=%v): %v", id, userID, err)
	}
	dep.Status = DepositoryVerified
	return repo.upsertUserDepository(userID, dep)
}

type depositoryRepository interface {
	getUserDepositories(userID string) ([]*Depository, error)
	getUserDepository(id DepositoryID, userID string) (*Depository, error)

	upsertUserDepository(userID string, dep *Depository) error
	updateDepositoryStatus(id DepositoryID, status DepositoryStatus) error
	deleteUserDepository(id DepositoryID, userID string) error

	getMicroDeposits(id DepositoryID, userID string) ([]microDeposit, error)
	initiateMicroDeposits(id DepositoryID, userID string, microDeposit []microDeposit) error
	confirmMicroDeposits(id DepositoryID, userID string, amounts []Amount) error
	getMicroDepositCursor(batchSize int) *microDepositCursor
}

type sqliteDepositoryRepo struct {
	db     *sql.DB
	logger log.Logger
}

func (r *sqliteDepositoryRepo) close() error {
	return r.db.Close()
}

func (r *sqliteDepositoryRepo) getUserDepositories(userID string) ([]*Depository, error) {
	query := `select depository_id from depositories where user_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var depositoryIds []string
	for rows.Next() {
		var row string
		rows.Scan(&row)
		if row != "" {
			depositoryIds = append(depositoryIds, row)
		}
	}

	var depositories []*Depository
	for i := range depositoryIds {
		dep, err := r.getUserDepository(DepositoryID(depositoryIds[i]), userID)
		if err == nil && dep != nil && dep.BankName != "" {
			depositories = append(depositories, dep)
		}
	}
	return depositories, rows.Err()
}

func (r *sqliteDepositoryRepo) getUserDepository(id DepositoryID, userID string) (*Depository, error) {
	query := `select depository_id, bank_name, holder, holder_type, type, routing_number, account_number_encrypted, account_number_masked, status, metadata, created_at, last_updated_at
from depositories
where depository_id = ? and user_id = ? and deleted_at is null
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(id, userID)

	dep := &Depository{}
	var (
		created time.Time
		updated time.Time
	)

	err = row.Scan(&dep.ID, &dep.BankName, &dep.Holder, &dep.HolderType, &dep.Type, &dep.RoutingNumber, &dep.encryptedAccountNumber, &dep.maskedAccountNumber, &dep.Status, &dep.Metadata, &created, &updated)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, err
	}
	dep.Created = base.NewTime(created)
	dep.Updated = base.NewTime(updated)
	if dep.ID == "" || dep.BankName == "" {
		return nil, nil // no records found
	}
	return dep, nil
}

func (r *sqliteDepositoryRepo) upsertUserDepository(userID string, dep *Depository) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	now := base.NewTime(time.Now())
	if dep.Created.IsZero() {
		dep.Created = now
		dep.Updated = now
	}

	query := `insert into depositories (depository_id, user_id, bank_name, holder, holder_type, type, routing_number, account_number_encrypted, account_number_masked, status, metadata, created_at, last_updated_at)
values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}

	res, err := stmt.Exec(dep.ID, userID, dep.BankName, dep.Holder, dep.HolderType, dep.Type, dep.RoutingNumber, dep.encryptedAccountNumber, dep.maskedAccountNumber, dep.Status, dep.Metadata, dep.Created.Time, dep.Updated.Time)
	stmt.Close()
	if err != nil && !database.UniqueViolation(err) {
		return fmt.Errorf("problem upserting depository=%q, userID=%q: %v", dep.ID, userID, err)
	}
	if res == nil {
		goto update
	}
	if n, _ := res.RowsAffected(); n == 0 {
		goto update
	} else {
		return tx.Commit() // Depository was inserted, so cleanup and exit
	}
	// We should rollback in the event of an unexpected problem. It's not possible to check (res != nil) and
	// call res.RowsAffected() in the same 'if' statement, so we needed multiple.
	return fmt.Errorf("upsertUserDepository: rollback=%v", tx.Rollback())
update:
	query = `update depositories
set bank_name = ?, holder = ?, holder_type = ?, type = ?, routing_number = ?,
account_number_encrypted = ?, account_number_masked = ?, status = ?, metadata = ?, last_updated_at = ?
where depository_id = ? and user_id = ? and deleted_at is null`
	stmt, err = tx.Prepare(query)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(
		dep.BankName, dep.Holder, dep.HolderType, dep.Type, dep.RoutingNumber,
		dep.encryptedAccountNumber, dep.maskedAccountNumber, dep.Status, dep.Metadata, time.Now(), dep.ID, userID)
	stmt.Close()
	if err != nil {
		return fmt.Errorf("upsertUserDepository: exec error=%v rollback=%v", err, tx.Rollback())
	}
	return tx.Commit()
}

func (r *sqliteDepositoryRepo) updateDepositoryStatus(id DepositoryID, status DepositoryStatus) error {
	query := `update depositories set status = ?, last_updated_at = ? where depository_id = ? and deleted_at is null`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(status, time.Now(), id); err != nil {
		return fmt.Errorf("error updating status depository_id=%q: %v", id, err)
	}
	return nil
}

func (r *sqliteDepositoryRepo) deleteUserDepository(id DepositoryID, userID string) error {
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
