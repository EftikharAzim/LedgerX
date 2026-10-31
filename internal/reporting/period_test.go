package reporting

import "testing"

func TestReportPeriods(t *testing.T) {
	cases := []struct {
		name       string
		input      Selection
		start, end string
	}{
		{"legacy monthly leap year", Selection{Month: "2024-02"}, "2024-02-01", "2024-02-29"},
		{"weekly crosses year", Selection{Type: "weekly", WeekOf: "2020-12-31"}, "2020-12-28", "2021-01-03"},
		{"Sunday belongs to preceding Monday", Selection{Type: "weekly", WeekOf: "2026-11-01"}, "2026-10-26", "2026-11-01"},
		{"yearly leap year", Selection{Type: "yearly", Year: "2024"}, "2024-01-01", "2024-12-31"},
		{"custom single day", Selection{Type: "custom", StartDate: "2024-02-29", EndDate: "2024-02-29"}, "2024-02-29", "2024-02-29"},
		{"minimum supported year", Selection{Type: "yearly", Year: "0001"}, "0001-01-01", "0001-12-31"},
		{"last supported month", Selection{Month: "9999-12"}, "9999-12-01", "9999-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			period, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if period.Start.Format("2006-01-02") != tc.start || period.End.Format("2006-01-02") != tc.end {
				t.Fatalf("unexpected bounds: %+v", period)
			}
		})
	}
	p, err := Parse(Selection{Type: "all"})
	if err != nil || !p.Start.IsZero() || !p.End.IsZero() || p.Month != "" {
		t.Fatalf("all time must be unbounded: %+v, %v", p, err)
	}
}

func TestRejectInvalidOrAmbiguousPeriods(t *testing.T) {
	for _, input := range []Selection{
		{}, {Type: "unknown"}, {Month: "0000-01"}, {Month: "2026-13"},
		{Type: "yearly", Year: "10000"}, {Type: "yearly", Year: "0000"},
		{Type: "weekly", WeekOf: "2026-02-30"}, {Type: "weekly", WeekOf: "9999-12-31"},
		{Type: "custom", StartDate: "2026-10-02", EndDate: "2026-10-01"},
		{Type: "custom", StartDate: "2026-10-01"},
		{Type: "all", Month: "2026-10"}, {Type: "yearly", Year: "2026", Month: "2026-10"},
	} {
		if _, err := Parse(input); err == nil {
			t.Fatalf("accepted invalid selection %+v", input)
		}
	}
}
