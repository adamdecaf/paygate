// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transform

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/moov-io/ach"
	"github.com/moov-io/paygate/internal/gpgx"
	"github.com/moov-io/paygate/pkg/config"
)

var (
	password = []byte("password")
)

func TestGPGEncryptor(t *testing.T) {
	cfg := config.Empty()
	cfg.Pipeline.PreUpload = &config.PreUpload{
		GPG: &config.GPG{
			KeyFile: filepath.Join("..", "..", "..", "..", "internal", "gpgx", "testdata", "moov.pub"),
		},
	}
	gpg, err := NewGPGEncryptor(cfg.Logger, cfg.Pipeline.PreUpload.GPG)
	if err != nil {
		t.Fatal(err)
	}

	// Read file and encrypt it
	orig, err := ach.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := gpg.Transform(&Result{File: orig})
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt file and compare to original
	privKey, err := gpgx.ReadPrivateKeyFile(filepath.Join("..", "..", "..", "..", "internal", "gpgx", "testdata", "moov.key"), password)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := gpgx.Decrypt(res.Encrypted, privKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := compareKeys(orig, decrypted); err != nil {
		t.Error(err)
	}
}

func TestGPGEncryptorSSHKey(t *testing.T) {
	cfg := config.Empty()
	cfg.Pipeline.PreUpload = &config.PreUpload{
		GPG: &config.GPG{
			KeyFile: filepath.Join("..", "..", "..", "..", "internal", "sshx", "testdata", "rsa-2048.pub"),
		},
	}
	gpg, err := NewGPGEncryptor(cfg.Logger, cfg.Pipeline.PreUpload.GPG)
	if err != nil {
		t.Fatal(err)
	}

	// Read file and encrypt it
	orig, err := ach.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "ppd-debit.ach"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := gpg.Transform(&Result{File: orig})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(res.Encrypted, []byte("BEGIN PGP MESSAGE")) {
		t.Errorf("unexpected encrypted message:\n%v", string(res.Encrypted))
	}
}

func compareKeys(orig *ach.File, decrypted []byte) error {
	if orig == nil {
		return errors.New("missing Original")
	}
	if len(decrypted) == 0 {
		return errors.New("missing decrypted File")
	}

	// marshal the original to bytes so we can compare
	var origBuf bytes.Buffer
	if err := ach.NewWriter(&origBuf).Write(orig); err != nil {
		return err
	}
	origBS := origBuf.Bytes()

	// byte-by-byte compare
	if len(origBS) != len(decrypted) {
		return fmt.Errorf("orig=%d decrypted=%d", len(origBS), len(decrypted))
	}
	for i := range origBS {
		if origBS[i] != decrypted[i] {
			return fmt.Errorf("byte #%d '%v' vs '%v'", i, origBS[i], decrypted[i])
		}
	}

	return nil
}
