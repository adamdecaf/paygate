// Copyright 2019 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

// paytest is a cli tool for testing Moov's Paygate API endpoints.
//
// paytest is not a stable tool. Please contact Moov developers if you intend to use this tool,
// otherwise we might change the tool (or remove it) without notice.
package main

import (
	"flag"
	"io"
	"os"
	// "fmt"
	"log"
	"path/filepath"

	"github.com/moov-io/paygate/internal/version"
)

var (
	flagLocal = flag.Bool("local", false, "Use local HTTP addresses (e.g. 'go run')")

	flagMergedDir = flag.String("merged-dir", filepath.Join("storage", "merged"), "Directory for merged ACH files to upload")
)

func main() {
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds | log.Lshortfile)
	log.Printf("Starting paytest %s", version.Version)

	if v := *flagMergedDir; v == "" || !dirIsEmpty(*flagMergedDir) {
		log.Fatalf("%s is not empty", v)
	}
}

// TODO(adam): write an integration test verifying the storage/merged/*.ach file
//  Should some code be moved from apitest and into here?

func dirIsEmpty(dir string) bool {
	s, err := os.Stat(dir)
	if err != nil || !s.IsDir() {
		if os.IsNotExist(err) {
			return true // dir doesn't exist
		}
		return false // dir isn't a directory
	}
	fd, err := os.Open(dir)
	if err != nil {
		return false
	}
	names, err := fd.Readdirnames(1)
	if (err != nil && err != io.EOF) || len(names) > 0 {
		return false // found a file, so not empty
	}
	return true
}
