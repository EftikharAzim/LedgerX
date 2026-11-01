package service

import (
	"errors"
	"strconv"

	"github.com/EftikharAzim/ledgerx/internal/money"
)

// ErrMoneyRange protects exact financial values shared by all write/read paths.
var ErrMoneyRange = errors.New("the amount or combined balance exceeds the supported exact range")

func safeMinor(value int64) bool { return value >= -money.MaxSafeMinor && value <= money.MaxSafeMinor }

func addMinor(a, b int64) (int64, error) {
	// Once both operands are within the exact range, their sum cannot overflow
	// int64. Check the result as well before it can reach a JSON number.
	if !safeMinor(a) || !safeMinor(b) || !safeMinor(a+b) {
		return 0, ErrMoneyRange
	}
	return a + b, nil
}

func parseExactAggregate(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || !safeMinor(v) {
		return 0, ErrMoneyRange
	}
	return v, nil
}
