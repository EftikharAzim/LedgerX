package importing

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMoneyLoverFullHistoryGroupsAndDates(t *testing.T) {
	var b strings.Builder
	b.WriteString("\ufeffID;Note;Amount;Category;Account;Currency;Date;Event;Exclude Report\n")
	for i := 0; i < 6753; i++ {
		wallet, currency := "Daily", "BDT"
		if i == 6752 {
			wallet, currency = "Travel", "USD"
		}
		fmt.Fprintf(&b, "ref-%d;Synthetic %d;-1.25;Food;%s;%s;23/05/%d;;%t\n", i, i, wallet, currency, 2020+i%7, i < 43)
	}
	doc, err := Inspect(b.String(), "auto")
	if err != nil {
		t.Fatal(err)
	}
	if doc.RowCount != 6753 || doc.Source != "moneylover" || doc.DateFormat != "day_first" || len(doc.SourceGroups) != 2 {
		t.Fatal("history detection failed")
	}
	if _, err := SelectSource(doc, ""); err == nil {
		t.Fatal("silently combined source wallets")
	}
	selected, err := SelectSource(doc, doc.SourceGroups[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	m := Mapping{Date: pos(6), Amount: pos(2), Note: pos(1), Category: pos(3), Currency: pos(5), Reference: pos(0), ExcludeReport: pos(8), AmountMode: "signed", DateFormat: doc.DateFormat, NumberStyle: "dot", SignConvention: "income_positive"}
	rows, err := Normalize(selected, m, "BDT", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	excluded := 0
	for _, r := range rows {
		if len(r.Errors) > 0 || len(r.Duplicates) > 0 || r.AmountMinor != -125 || r.Planned {
			t.Fatal("history changed or rejected")
		}
		if r.ExcludeReport {
			excluded++
		}
	}
	if len(rows) != 6752 || excluded != 43 || rows[0].Number != 2 {
		t.Fatal("source filtering lost records")
	}
	foreign, _ := SelectSource(doc, doc.SourceGroups[1].Key)
	if foreign.Records[0].Number != 6754 {
		t.Fatal("source line identity lost")
	}
	if _, err := Inspect(b.String(), "comma"); err == nil || !strings.Contains(err.Error(), "Detect automatically") {
		t.Fatal("missing delimiter recovery hint", err)
	}
}

func TestHistoryLimitsAndFuturePlanning(t *testing.T) {
	m := standardMapping()
	m.ExcludeReport = pos(3)
	rows := normalize(t, "Date,Amount,Note,Exclude Report\n2030-01-01,-12.34,Upcoming,true\n2024-01-01,-1,Past,false\n2024-01-02,-1,Bad,maybe", m)
	if len(rows[0].Errors) != 0 || !rows[0].Planned || !rows[0].ExcludeReport || rows[1].Planned || len(rows[2].Errors) == 0 {
		t.Fatalf("planning flags: %+v", rows)
	}
	var b strings.Builder
	b.WriteString("Date,Amount\n")
	for range MaxRows {
		b.WriteString("2024-01-01,1\n")
	}
	if _, err := Inspect(b.String(), "auto"); err != nil {
		t.Fatal("50,000 bounded rows rejected", err)
	}
	b.WriteString("2024-01-01,1\n")
	if _, err := Inspect(b.String(), "auto"); err == nil {
		t.Fatal("unbounded record count")
	}
	if _, err := Inspect(strings.Repeat("x", MaxBytes+1), "auto"); err == nil {
		t.Fatal("unbounded bytes")
	}
}
