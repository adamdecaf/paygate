// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package limiter

import (
	"github.com/moov-io/paygate/pkg/id"
)

type MockChecker struct {
	Err error
}

func (mc *MockChecker) AllowTransfer(userID id.User) error {
	return mc.Err
}
