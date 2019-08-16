// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/base/idempotent/lru"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/gorilla/mux"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const (
	// maxReadBytes is the number of bytes to read
	// from a request body. It's intended to be used
	// with an io.LimitReader
	maxReadBytes = 1 * 1024 * 1024
)

var (
	inmemIdempotentRecorder = lru.New()

	// Prometheus Metrics
	internalServerErrors = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "http_errors",
		Help: "Count of how many 5xx errors we send out",
	}, nil)
	routeHistogram = prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
		Name: "http_response_duration_seconds",
		Help: "Histogram representing the http response durations",
	}, []string{"route"})

	errMissingRequiredJson = errors.New("missing required JSON field(s)")
)

// read consumes an io.Reader (wrapping with io.LimitReader)
// and returns either the resulting bytes or a non-nil error.
func read(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, io.EOF
	}
	rr := io.LimitReader(r, maxReadBytes)
	return ioutil.ReadAll(rr)
}

func internalError(logger log.Logger, w http.ResponseWriter, err error) {
	internalServerErrors.Add(1)

	file := moovhttp.InternalError(w, err)
	component := strings.Split(file, ".go")[0]

	if logger != nil {
		logger.Log(component, err, "source", file)
	}
}

func addPingRoute(logger log.Logger, r *mux.Router) {
	r.Methods("GET").Path("/ping").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestID := moovhttp.GetRequestID(r); requestID != "" {
			logger.Log("route", "ping", "requestID", requestID)
		}
		moovhttp.SetAccessControlAllowHeaders(w, r.Header.Get("Origin"))
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PONG"))
	})
}

type httpRespWriter func(logger log.Logger, w http.ResponseWriter, r *http.Request) (http.ResponseWriter, error)

var wrapResponseWriter httpRespWriter = func(logger log.Logger, w http.ResponseWriter, r *http.Request) (http.ResponseWriter, error) {

	// I think we need to keep x-user-id, so how can our documentation better mention that?
	// Make sure all examples have it included.

	return defaultResponseWriter(os.Getenv("HTTP_REQUIRE_USER_ID"), logger, w, r) // TODO(adam): so... what bugs will this create? tons I assume?
}

var defaultResponseWriter = func(requireUserIdHeader string, logger log.Logger, w http.ResponseWriter, r *http.Request) (http.ResponseWriter, error) {
	route := fmt.Sprintf("%s-%s", strings.ToLower(r.Method), cleanMetricsPath(r.URL.Path))
	hist := routeHistogram.With("route", route)

	switch strings.ToLower(requireUserIdHeader) {
	case "true", "yes":
		return moovhttp.EnsureHeaders(logger, hist, inmemIdempotentRecorder, w, r)
	default:
		return moovhttp.Wrap(logger, hist, w, r), nil
	}
}

var baseIdRegex = regexp.MustCompile(`([a-f0-9]{40})`)

// cleanMetricsPath takes a URL path and formats it for Prometheus metrics
//
// This method replaces /'s with -'s and strips out moov/base.ID() values from URL path slugs.
func cleanMetricsPath(path string) string {
	parts := strings.Split(path, "/")
	var out []string
	for i := range parts {
		if parts[i] == "" || baseIdRegex.MatchString(parts[i]) {
			continue // assume it's a moov/base.ID() value
		}
		out = append(out, parts[i])
	}
	return strings.Join(out, "-")
}

func tlsHttpClient(path string) (*http.Client, error) {
	tlsConfig := &tls.Config{}
	pool, err := x509.SystemCertPool()
	if pool == nil || err != nil {
		pool = x509.NewCertPool()
	}

	// read extra CA file
	if path != "" {
		bs, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("problem reading %s: %v", path, err)
		}
		ok := pool.AppendCertsFromPEM(bs)
		if !ok {
			return nil, fmt.Errorf("couldn't parse PEM in: %s", path)
		}
	}
	tlsConfig.RootCAs = pool

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
			IdleConnTimeout:     1 * time.Minute,
		},
	}, nil
}
