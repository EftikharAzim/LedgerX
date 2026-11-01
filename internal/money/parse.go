package money

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/currency"
)

// MaxSafeMinor keeps API integers exact in JavaScript clients.
const MaxSafeMinor int64 = 9007199254740991

// ParseDecimal parses CSV amounts exactly, with explicit decimal/grouping rules.
// It accepts accounting parentheses and zero padding but never rounds a value.
func ParseDecimal(value, code, style string) (int64, error) {
	invalid := errors.New("use a valid amount with the selected number format and currency precision")
	if len(value) > 128 || (style != "dot" && style != "comma") {
		return 0, invalid
	}
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 0, invalid
	}
	digits, _ := currency.Standard.Rounding(unit)
	s := strings.TrimSpace(strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(value))
	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negative = true
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		if negative {
			return 0, invalid
		}
		negative = s[0] == '-'
		s = strings.TrimSpace(s[1:])
	}
	symbols := map[string][]string{"USD": {"$"}, "EUR": {"€"}, "GBP": {"£"}, "JPY": {"¥", "￥"}, "BDT": {"৳"}, "INR": {"₹"}}
	for _, token := range append([]string{code}, symbols[code]...) {
		if strings.HasPrefix(s, token) {
			s = strings.TrimSpace(strings.TrimPrefix(s, token))
			break
		}
		if strings.HasSuffix(s, token) {
			s = strings.TrimSpace(strings.TrimSuffix(s, token))
			break
		}
	}
	decimal, grouping := ".", ","
	if style == "comma" {
		decimal, grouping = ",", "."
	}
	parts := strings.Split(s, decimal)
	if len(parts) > 2 || len(parts) == 0 || parts[0] == "" {
		return 0, invalid
	}
	whole, fraction := parts[0], ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" {
			return 0, invalid
		}
	}
	if strings.Contains(whole, " ") {
		if strings.Contains(whole, grouping) {
			return 0, invalid
		}
		grouping = " "
	}
	groups := strings.Split(whole, grouping)
	if len(groups) > 1 {
		if len(groups[0]) < 1 || len(groups[0]) > 3 {
			return 0, invalid
		}
		for _, g := range groups[1:] {
			if len(g) != 3 {
				return 0, invalid
			}
		}
	}
	whole = strings.Join(groups, "")
	for _, text := range []string{whole, fraction} {
		for _, r := range text {
			if !unicode.IsDigit(r) || r > '9' {
				return 0, invalid
			}
		}
	}
	fraction = strings.TrimRight(fraction, "0")
	if len(fraction) > digits {
		return 0, invalid
	}
	minor, err := strconv.ParseInt(whole+fraction+strings.Repeat("0", digits-len(fraction)), 10, 64)
	if err != nil || minor > MaxSafeMinor {
		return 0, invalid
	}
	if negative {
		minor = -minor
	}
	return minor, nil
}
