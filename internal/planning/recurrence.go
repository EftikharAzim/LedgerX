// Package planning defines calendar recurrence without moving recorded money.
package planning

import (
	"errors"
	"time"
)

// Schedule is portable financial metadata; retry credentials are excluded.
type Schedule struct {
	ID               int64     `json:"id"`
	AccountID        int64     `json:"account_id"`
	Currency         string    `json:"currency"`
	AmountMinor      int64     `json:"amount_minor"`
	CategoryID       int64     `json:"category_id,omitempty"`
	Note             string    `json:"note"`
	Cadence          string    `json:"cadence"`
	FirstDueDate     string    `json:"first_due_date"`
	EndDate          string    `json:"end_date,omitempty"`
	Status           string    `json:"status"`
	Revision         int64     `json:"revision"`
	GenerateFrom     string    `json:"generate_from"`
	GeneratedThrough string    `json:"generated_through,omitempty"`
	RefreshedOn      string    `json:"refreshed_on,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Date parses a supported UTC calendar date.
func Date(s string) (time.Time, error) {
	at, err := time.Parse("2006-01-02", s)
	if err != nil || at.Year() < 1 || at.Year() > 9999 {
		return time.Time{}, errors.New("use a calendar date from years 0001–9999")
	}
	return at, nil
}

// Today removes clock time without converting date-only obligations.
func Today(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
}

// Occurrence uses the original anchor, clamping short months without drifting.
func Occurrence(first time.Time, cadence string, index int) (time.Time, bool) {
	if index < 0 {
		return time.Time{}, false
	}
	var at time.Time
	switch cadence {
	case "weekly":
		at = first.AddDate(0, 0, index*7)
	case "monthly", "yearly":
		months := index
		if cadence == "yearly" {
			months *= 12
		}
		start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, months, 0)
		day := first.Day()
		last := start.AddDate(0, 1, -1).Day()
		if day > last {
			day = last
		}
		at = start.AddDate(0, 0, day-1)
	default:
		return time.Time{}, false
	}
	return at, at.Year() >= 1 && at.Year() <= 9999
}

// Dates enumerates a bounded date window, retaining the original month/day rule.
func Dates(first time.Time, cadence string, from, through time.Time) ([]time.Time, error) {
	if cadence != "weekly" && cadence != "monthly" && cadence != "yearly" {
		return nil, errors.New("choose weekly, monthly or yearly")
	}
	index := 0
	if from.After(first) {
		switch cadence {
		case "weekly":
			index = int((from.Unix() - first.Unix()) / 86400 / 7)
		case "monthly":
			index = (from.Year()-first.Year())*12 + int(from.Month()-first.Month())
		case "yearly":
			index = from.Year() - first.Year()
		}
	}
	out := []time.Time{}
	for {
		at, ok := Occurrence(first, cadence, index)
		if !ok || at.After(through) {
			break
		}
		if !at.Before(from) {
			out = append(out, at)
			if len(out) > 500 {
				return nil, errors.New("choose a smaller recurrence window")
			}
		}
		index++
	}
	return out, nil
}
