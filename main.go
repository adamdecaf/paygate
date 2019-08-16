// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/moov-io/base/admin"
	"github.com/moov-io/base/http/bind"
	"github.com/moov-io/paygate/internal/database"
	"github.com/moov-io/paygate/internal/filetransfer"
	"github.com/moov-io/paygate/internal/version"
	"github.com/moov-io/paygate/pkg/achclient"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

var (
	httpAddr  = flag.String("http.addr", bind.HTTP("paygate"), "HTTP listen address")
	adminAddr = flag.String("admin.addr", bind.Admin("paygate"), "Admin HTTP listen address")

	flagLogFormat = flag.String("log.format", "", "Format for log lines (Options: json, plain")
)

func main() {
	flag.Parse()

	var logger log.Logger
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		*flagLogFormat = v
	}
	if strings.ToLower(*flagLogFormat) == "json" {
		logger = log.NewJSONLogger(os.Stderr)
	} else {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	logger.Log("startup", fmt.Sprintf("Starting paygate server version %s", version.Version))

	// migrate database
	db, err := database.New(logger, os.Getenv("DATABASE_TYPE"))
	if err != nil {
		panic(fmt.Sprintf("error creating database: %v", err))
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Log("exit", err)
		}
	}()

	// Listen for application termination.
	errs := make(chan error)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	// Spin up admin HTTP server and optionally override -admin.addr
	if v := os.Getenv("HTTP_ADMIN_BIND_ADDRESS"); v != "" {
		*adminAddr = v
	}
	adminServer := admin.NewServer(*adminAddr)
	go func() {
		logger.Log("admin", fmt.Sprintf("listening on %s", adminServer.BindAddr()))
		if err := adminServer.Listen(); err != nil {
			err = fmt.Errorf("problem starting admin http: %v", err)
			logger.Log("admin", err)
			errs <- err
		}
	}()
	defer adminServer.Shutdown()

	// Setup repositories
	receiverRepo := &sqliteReceiverRepo{db, logger}
	defer receiverRepo.close()
	depositoryRepo := &sqliteDepositoryRepo{db, logger}
	defer depositoryRepo.close()
	eventRepo := &sqliteEventRepo{db, logger}
	defer eventRepo.close()
	gatewaysRepo := &sqliteGatewayRepo{db, logger}
	defer gatewaysRepo.close()
	originatorsRepo := &sqliteOriginatorRepo{db, logger}
	defer originatorsRepo.close()
	transferRepo := &sqliteTransferRepo{db, logger}
	defer transferRepo.close()

	httpClient, err := tlsHttpClient(os.Getenv("HTTP_CLIENT_CAFILE"))
	if err != nil {
		panic(fmt.Sprintf("problem creating TLS ready *http.Client: %v", err))
	}

	// Create ACH client
	achClient := achclient.New(logger, "ach", httpClient)
	if achClient == nil {
		panic("no ACH client created")
	}
	adminServer.AddLivenessCheck("ach", achClient.Ping)

	// Create FED client
	fedClient := createFEDClient(logger, httpClient)
	if fedClient == nil {
		panic("no FED client created")
	}
	adminServer.AddLivenessCheck("fed", fedClient.Ping)

	// Create Accounts client
	accountsClient := setupAccountsClient(logger, adminServer, httpClient, os.Getenv("ACCOUNTS_ENDPOINT"), os.Getenv("ACCOUNTS_CALLS_DISABLED"))
	accountsCallsDisabled := accountsClient == nil
	odfiAccount := &odfiAccount{
		client:        accountsClient,
		accountNumber: or(os.Getenv("ODFI_ACCOUNT_NUMBER"), "123"),
		routingNumber: or(os.Getenv("ODFI_ROUTING_NUMBER"), "121042882"),
	}
	if v := os.Getenv("ODFI_ACCOUNT_TYPE"); v != "" {
		t := AccountType(v)
		if err := t.validate(); err == nil {
			odfiAccount.accountType = t
		}
	} else {
		odfiAccount.accountType = Savings
	}

	// Create OFAC client
	ofacClient := newOFACClient(logger, os.Getenv("OFAC_ENDPOINT"), httpClient)
	if ofacClient == nil {
		panic("no OFAC client created")
	}
	adminServer.AddLivenessCheck("ofac", ofacClient.Ping)

	// Start periodic ACH file sync
	achStorageDir := filepath.Dir(os.Getenv("ACH_FILE_STORAGE_DIR"))
	if achStorageDir == "." {
		achStorageDir = "./storage/"
	}
	if err := os.MkdirAll(achStorageDir, 0777); err != nil {
		panic(fmt.Sprintf("problem creating %s: %v", achStorageDir, err))
	}
	logger.Log("storage", fmt.Sprintf("using %s for storage directory", achStorageDir))

	fileTransferRepo := filetransfer.NewRepository(db, os.Getenv("DATABASE_TYPE"))
	defer fileTransferRepo.Close()

	fileTransferController, err := newFileTransferController(logger, achStorageDir, fileTransferRepo, achClient, accountsClient, accountsCallsDisabled)
	if err != nil {
		panic(fmt.Sprintf("ERROR: creating ACH file transfer controller: %v", err))
	}
	forceFileUplaods := make(chan struct{}, 1) // buffered to allow only one force
	if fileTransferController != nil && err == nil {
		ctx, cancelFileSync := context.WithCancel(context.Background())
		defer cancelFileSync()

		// start our controller's operations in an anon goroutine
		go fileTransferController.startPeriodicFileOperations(ctx, forceFileUplaods, depositoryRepo, transferRepo)

		addFileTransferSyncRoute(logger, adminServer, forceFileUplaods)

		// side-effect register HTTP routes
		filetransfer.AddFileTransferConfigRoutes(logger, adminServer, fileTransferRepo)
	}

	// Register the micro-deposit admin route
	addMicroDepositAdminRoutes(logger, adminServer, depositoryRepo)

	// Create HTTP handler
	handler := mux.NewRouter()
	addReceiverRoutes(logger, handler, ofacClient, receiverRepo, depositoryRepo)
	addDepositoryRoutes(logger, handler, odfiAccount, accountsCallsDisabled, accountsClient, achClient, fedClient, ofacClient, depositoryRepo, eventRepo)
	addEventRoutes(logger, handler, eventRepo)
	addGatewayRoutes(logger, handler, gatewaysRepo)
	addOriginatorRoutes(logger, handler, accountsCallsDisabled, accountsClient, ofacClient, depositoryRepo, originatorsRepo)
	addPingRoute(logger, handler)

	xferRouter := &transferRouter{
		logger:             logger,
		depRepo:            depositoryRepo,
		eventRepo:          eventRepo,
		receiverRepository: receiverRepo,
		origRepo:           originatorsRepo,
		transferRepo:       transferRepo,

		achClientFactory: func(userId string) *achclient.ACH {
			return achclient.New(logger, userId, httpClient)
		},

		accountsClient:        accountsClient,
		accountsCallsDisabled: accountsCallsDisabled,

		httpRespWriter: wrapResponseWriter,
	}
	xferRouter.registerRoutes(handler)

	// Check to see if our -http.addr flag has been overridden
	if v := os.Getenv("HTTP_BIND_ADDRESS"); v != "" {
		*httpAddr = v
	}
	// Create main HTTP server
	serve := &http.Server{
		Addr:    *httpAddr,
		Handler: handler,
		TLSConfig: &tls.Config{
			InsecureSkipVerify:       false,
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS12,
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	shutdownServer := func() {
		if err := serve.Shutdown(context.TODO()); err != nil {
			logger.Log("shutdown", err)
		}
	}
	defer shutdownServer()

	// Start main HTTP server
	go func() {
		if certFile, keyFile := os.Getenv("HTTPS_CERT_FILE"), os.Getenv("HTTPS_KEY_FILE"); certFile != "" && keyFile != "" {
			logger.Log("startup", fmt.Sprintf("binding to %s for secure HTTP server", *httpAddr))
			if err := serve.ListenAndServeTLS(certFile, keyFile); err != nil {
				logger.Log("exit", err)
			}
		} else {
			logger.Log("startup", fmt.Sprintf("binding to %s for HTTP server", *httpAddr))
			if err := serve.ListenAndServe(); err != nil {
				logger.Log("exit", err)
			}
		}
	}()

	if err := <-errs; err != nil {
		logger.Log("exit", err)
	}
}

// or returns primary if non-empty and backup otherwise
func or(primary, backup string) string {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		return strings.TrimSpace(backup)
	}
	return primary
}

// yes returns true if the provided case-insensitive string matches 'yes' and is used to parse config values.
func yes(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "yes")
}

func setupAccountsClient(logger log.Logger, svc *admin.Server, httpClient *http.Client, endpoint, disabled string) AccountsClient {
	if yes(disabled) {
		return nil
	}
	accountsClient := createAccountsClient(logger, endpoint, httpClient)
	if accountsClient == nil {
		panic("no Accounts client created")
	}
	svc.AddLivenessCheck("accounts", accountsClient.Ping)
	return accountsClient
}
