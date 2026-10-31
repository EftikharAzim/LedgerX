// Package reporting defines the UTC calendar periods used by report requests.
package reporting

import (
	"errors"
	"time"
)

// Selection contains the mutually exclusive inputs for one report period.
type Selection struct{ Type, Month, WeekOf, Year, StartDate, EndDate string }

// Period is a validated inclusive UTC date range. All-time has zero bounds.
type Period struct {
	Type, Month string
	Start, End  time.Time
}

func parseDate(value string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Year() < 1 || date.Year() > 9999 {
		return time.Time{}, errors.New("dates must use YYYY-MM-DD with years 0001–9999")
	}
	return date, nil
}

// Parse validates a selection without silently accepting unrelated filters.
func Parse(s Selection) (Period, error) {
	if s.Type == "" {
		s.Type = "monthly"
	}
	p := Period{Type: s.Type}
	fields := []string{s.Month, s.WeekOf, s.Year, s.StartDate, s.EndDate}
	allowed := map[string][]int{"monthly": {0}, "weekly": {1}, "yearly": {2}, "custom": {3, 4}, "all": {}}
	indices, ok := allowed[s.Type]
	if !ok {
		return p, errors.New("type must be monthly, weekly, yearly, custom, or all")
	}
	for i, field := range fields {
		permitted := false
		for _, index := range indices {
			if i == index {
				permitted = true
			}
		}
		if !permitted && field != "" {
			return p, errors.New("choose only the date fields for the selected report type")
		}
	}
	var err error
	switch s.Type {
	case "monthly":
		p.Start, err = time.Parse("2006-01", s.Month)
		if err != nil || p.Start.Year() < 1 {
			return p, errors.New("month must be YYYY-MM with years 0001–9999")
		}
		p.Month = s.Month
		p.End = p.Start.AddDate(0, 1, -1)
	case "weekly":
		var day time.Time
		day, err = parseDate(s.WeekOf)
		if err != nil {
			return p, err
		}
		p.Start = day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
		p.End = p.Start.AddDate(0, 0, 6)
		if p.Start.Year() < 1 || p.End.Year() > 9999 {
			return p, errors.New("the full week must fall within years 0001–9999")
		}
	case "yearly":
		p.Start, err = time.Parse("2006", s.Year)
		if err != nil || p.Start.Year() < 1 {
			return p, errors.New("year must be four digits, from 0001 to 9999")
		}
		p.End = p.Start.AddDate(1, 0, -1)
	case "custom":
		p.Start, err = parseDate(s.StartDate)
		if err != nil {
			return p, err
		}
		p.End, err = parseDate(s.EndDate)
		if err != nil {
			return p, err
		}
		if p.End.Before(p.Start) {
			return p, errors.New("end_date must be on or after start_date")
		}
	}
	return p, nil
}
