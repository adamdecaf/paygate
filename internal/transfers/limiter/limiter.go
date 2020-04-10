// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package limiter

import (
	"errors"
	"fmt"
	"os"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/internal/util"
	"github.com/moov-io/paygate/pkg/id"
)

var (
	ErrOverLimit = errors.New("transfers over limit")
)

// OneDay returns the maximum sum of transfers for each user over the current day.
func OneDay() string {
	return util.Or(os.Getenv("TRANSFERS_ONE_DAY_USER_LIMIT"), "5000.00")
}

// SevenDay returns the maximum sum of transfers for each user over the previous seven days.
func SevenDay() string {
	return util.Or(os.Getenv("TRANSFERS_SEVEN_DAY_USER_LIMIT"), "10000.00")
}

// ThirtyDay returns the maximum sum of transfers for each user over the previous thirty days.
func ThirtyDay() string {
	return util.Or(os.Getenv("TRANSFERS_THIRTY_DAY_USER_LIMIT"), "25000.00")
}

// Parse attempts to convert multiple strings into Amount objects.
// These need to follow the Amount format (e.g. 10000.00)
func Parse(oneDay, sevenDays, thirtyDays string) (*Limits, error) {
	one, err := model.NewAmount("USD", oneDay)
	if err != nil {
		return nil, fmt.Errorf("one day: %v", err)
	}
	seven, err := model.NewAmount("USD", sevenDays)
	if err != nil {
		return nil, fmt.Errorf("seven day: %v", err)
	}
	thirty, err := model.NewAmount("USD", thirtyDays)
	if err != nil {
		return nil, fmt.Errorf("thirty day: %v", err)
	}
	return &Limits{
		CurrentDay:        one,
		PreviousSevenDays: seven,
		PreviousThityDays: thirty,
	}, nil
}

// Limits contain the maximum Amount transfers can accumulate to over a given time period.
type Limits struct {
	CurrentDay        *model.Amount
	PreviousSevenDays *model.Amount
	PreviousThityDays *model.Amount
}

type Checker interface {
	AllowTransfer(userID id.User) error
}
