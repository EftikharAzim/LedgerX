package planning

import (
	"testing"
	"time"
)

func TestAnchoredMonthEndsAndLeapYears(t *testing.T) {
	first, _ := Date("2024-01-31")
	want := []string{"2024-01-31", "2024-02-29", "2024-03-31", "2024-04-30"}
	for i, w := range want {
		at, ok := Occurrence(first, "monthly", i)
		if !ok || at.Format("2006-01-02") != w {
			t.Fatal("month-end drift", i, at)
		}
	}
	leap, _ := Date("2024-02-29")
	for i, w := range []string{"2024-02-29", "2025-02-28", "2026-02-28", "2027-02-28", "2028-02-29"} {
		at, ok := Occurrence(leap, "yearly", i)
		if !ok || at.Format("2006-01-02") != w {
			t.Fatal("lost leap-day anchor", i, at)
		}
	}
}
func TestWeeklyWindowsAndCalendarBounds(t *testing.T) {
	first, _ := Date("2024-01-01")
	from, _ := Date("2024-01-03")
	through, _ := Date("2024-01-22")
	dates, err := Dates(first, "weekly", from, through)
	if err != nil || len(dates) != 3 || dates[0].Day() != 8 || dates[2].Day() != 22 {
		t.Fatal(dates, err)
	}
	end, _ := Date("9999-12-31")
	if _, ok := Occurrence(end, "monthly", 1); ok {
		t.Fatal("unsupported year generated")
	}
	if _, err := Date("2024-02-30"); err == nil {
		t.Fatal("invalid date accepted")
	}
	at := Today(time.Date(2026, 11, 2, 8, 45, 0, 0, time.FixedZone("BD", 6*3600)))
	if at.Hour() != 0 || at.Location() != time.UTC {
		t.Fatal("date conversion", at)
	}
}
