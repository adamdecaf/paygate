// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package limiter

import (
	"testing"
)

func TestParse(t *testing.T) {
	if limits, err := Parse(OneDay(), SevenDay(), ThirtyDay()); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		if limits.CurrentDay.Int() != 5000*100 {
			t.Errorf("got %v", limits.CurrentDay)
		}
		if limits.PreviousSevenDays.Int() != 10000*100 {
			t.Errorf("got %v", limits.PreviousSevenDays)
		}
		if limits.PreviousThityDays.Int() != 25000*100 {
			t.Errorf("got %v", limits.PreviousThityDays)
		}
	}

	if limits, err := Parse("100.00", "1000.00", "123456.00"); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		if limits.CurrentDay.Int() != 100*100 {
			t.Errorf("got %v", limits.CurrentDay)
		}
		if limits.PreviousSevenDays.Int() != 1000*100 {
			t.Errorf("got %v", limits.PreviousSevenDays)
		}
		if limits.PreviousThityDays.Int() != 123456*100 {
			t.Errorf("got %v", limits.PreviousThityDays)
		}
	}

	if limits, err := Parse("1.00", "10.00", "1.21"); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		if limits.CurrentDay.Int() != 1*100 {
			t.Errorf("got %v", limits.CurrentDay)
		}
		if limits.PreviousSevenDays.Int() != 10*100 {
			t.Errorf("got %v", limits.PreviousSevenDays)
		}
		if limits.PreviousThityDays.Int() != 121 {
			t.Errorf("got %v", limits.PreviousThityDays)
		}
	}
}

func TestParseErr(t *testing.T) {
	if l, err := Parse(OneDay(), SevenDay(), "invalid"); err == nil {
		t.Logf("%v", l)
		t.Error("expected error")
	}
	if l, err := Parse("invalid", SevenDay(), ThirtyDay()); err == nil {
		t.Logf("%v", l)
		t.Error("expected error")
	}
	if l, err := Parse(OneDay(), "invalid", ThirtyDay()); err == nil {
		t.Logf("%v", l)
		t.Error("expected error")
	}
}
