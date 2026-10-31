package money

import (
	"math"
	"testing"
)

func TestDecimalPreservesMinorUnits(t *testing.T) {
	for _, tc := range []struct {
		minor      int64
		code, want string
	}{
		{101, "USD", "1.01"}, {-29, "USD", "-0.29"}, {0, "USD", "0.00"},
		{123, "JPY", "123"}, {-1, "JPY", "-1"}, {1234, "KWD", "1.234"},
		{math.MinInt64, "USD", "-92233720368547758.08"},
	} {
		if got := Decimal(tc.minor, tc.code); got != tc.want {
			t.Errorf("%d %s: %s, want %s", tc.minor, tc.code, got, tc.want)
		}
	}
}
