// Package money formats integer ledger amounts without floating-point rounding.
package money

import (
	"strconv"
	"strings"

	"golang.org/x/text/currency"
)

// Decimal returns an ungrouped decimal amount using the currency's minor units.
func Decimal(minor int64, code string) string {
	digits := 2
	if unit, err := currency.ParseISO(code); err == nil {
		digits, _ = currency.Standard.Rounding(unit)
	}
	value := strconv.FormatInt(minor, 10)
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign, value = "-", value[1:]
	}
	if digits == 0 {
		return sign + value
	}
	if len(value) <= digits {
		value = strings.Repeat("0", digits+1-len(value)) + value
	}
	return sign + value[:len(value)-digits] + "." + value[len(value)-digits:]
}
