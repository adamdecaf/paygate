// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package database

import (
	"testing"
)

func TestMSSQL(t *testing.T) {
	db := CreateTestMSSQLDB(t)
	defer db.Close()

	if err := db.DB.Ping(); err != nil {
		t.Fatal(err)
	}
}
