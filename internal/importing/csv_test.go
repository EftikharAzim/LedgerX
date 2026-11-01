package importing

import (
	"strings"
	"testing"
	"time"
)

func pos(i int) *int { return &i }
func standardMapping() Mapping {
	return Mapping{Date: pos(0), Amount: pos(1), Note: pos(2), AmountMode: "signed", DateFormat: "iso", NumberStyle: "dot", SignConvention: "income_positive"}
}
func normalize(t *testing.T, text string, m Mapping) []Row {
	t.Helper()
	doc, err := Inspect(text, "auto")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := Normalize(doc, m, "USD", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestInspectCSVFormatsAndSourceLines(t *testing.T) {
	for _, separator := range []string{",", ";", "\t"} {
		text := "\ufeffDate" + separator + "Amount" + separator + "Description\n\n2024-01-02" + separator + "-1.29" + separator + "\"Coffee\nand cake\"\n2024-01-03" + separator + "10" + separator + "Salary\n"
		doc, err := Inspect(text, "auto")
		if err != nil {
			t.Fatal(err)
		}
		if doc.RowCount != 2 || doc.Records[0].Number != 3 || doc.Records[1].Number != 5 || doc.Records[0].Cells[2] != "Coffee\nand cake" {
			t.Fatalf("bad inspection: %+v", doc)
		}
	}
}
func TestInspectCSVBoundsAndLedgerReport(t *testing.T) {
	cases := []string{"Date,Amount\n", "Date,Amount\n\"broken,2", "\xff,Amount\n1,2", strings.Repeat("x", MaxBytes+1), "transaction_id,account_id,amount_minor\n1,2,3", strings.Repeat("column,", MaxColumns) + "last\n1,2", "Date,Amount\n" + strings.Repeat("2024-01-01,1\n", MaxRows+1)}
	for i, text := range cases {
		if _, err := Inspect(text, "auto"); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
func TestNormalizeExplicitFormatsAndDates(t *testing.T) {
	m := standardMapping()
	m.DateFormat = "day_first"
	m.NumberStyle = "comma"
	m.SignConvention = "expense_positive"
	rows := normalize(t, "Date;Amount;Note\n04/03/2024;1.234,56;Rent\n31/02/2024;1,00;Bad date", m)
	if rows[0].AmountMinor != -123456 || rows[0].OccurredAt.Format("2006-01-02") != "2024-03-04" || !rows[0].DateOnly || len(rows[0].Errors) > 0 || len(rows[1].Errors) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	at, dateOnly, err := Date("2024-03-04T23:30:00-05:00", "iso")
	if err != nil || dateOnly || at.Format(time.RFC3339) != "2024-03-05T04:30:00Z" {
		t.Fatalf("timestamp %v %v %v", at, dateOnly, err)
	}
	at, _, err = Date("03/04/2024", "month_first")
	if err != nil || at.Month() != time.March {
		t.Fatal(at, err)
	}
	if _, _, err = Date("03/04/24", "day_first"); err == nil {
		t.Fatal("accepted ambiguous two-digit year")
	}
}
func TestNormalizeDebitCreditTypeAndErrors(t *testing.T) {
	m := standardMapping()
	m.Amount = nil
	m.Debit = pos(1)
	m.Credit = pos(2)
	m.Note = pos(3)
	m.AmountMode = "debit_credit"
	m.Currency = pos(4)
	m.Type = pos(5)
	rows := normalize(t, "Date,Debit,Credit,Note,Currency,Type\n2024-01-01,12,,Food,USD,expense\n2024-01-02,,5,Returned,USD,refund\n2024-01-03,1,2,Wrong,USD,expense\ninvalid,3,,Date,USD,expense\n2024-01-04,,1,Foreign,EUR,income\n2024-01-05,,1,Transfer,USD,transfer\n2024-01-06,,,Zero,USD,income", m)
	if rows[0].AmountMinor != -1200 || rows[1].AmountMinor != 500 || rows[1].EntryType != "refund" {
		t.Fatalf("rows %+v", rows)
	}
	for i := 2; i < len(rows); i++ {
		if len(rows[i].Errors) == 0 {
			t.Fatalf("accepted invalid row %d", i)
		}
	}
	if len(rows[3].Errors) != 1 || strings.Contains(rows[3].Errors[0], "Amount") {
		t.Fatalf("date error leaked into amount: %+v", rows[3])
	}
}
func TestNormalizeDuplicateReferencesAndUnsafeRows(t *testing.T) {
	m := standardMapping()
	m.Reference = pos(3)
	m.Category = pos(4)
	rows := normalize(t, "Date,Amount,Note,Reference,Category\n2024-01-01,-1,Coffee,A,Food\n2024-01-01,-1,Coffee,B,Food\n2024-01-02,-2,Changed,A,Food\n2024-01-01,-1,Coffee,,Food\n2024-01-01,-1,Coffee,,Food\n2024-01-01,0,Zero,,Food\n2024-01-01,1e3,Exponent,,Food\n2024-01-01,-1,Short", m)
	if len(rows[0].Duplicates) > 0 || len(rows[1].Duplicates) > 0 || len(rows[2].Duplicates) != 1 || rows[2].Duplicates[0].RowNumber != 2 || len(rows[3].Duplicates) != 1 || len(rows[4].Duplicates) != 1 {
		t.Fatalf("duplicates: %+v", rows)
	}
	for _, r := range rows[5:] {
		if len(r.Errors) == 0 {
			t.Fatalf("accepted unsafe row: %+v", r)
		}
	}
	bad := m
	bad.Note = pos(0)
	if err := bad.Validate(5); err == nil {
		t.Fatal("accepted two fields on same column")
	}
	bad = m
	bad.DateFormat = ""
	if err := bad.Validate(5); err == nil {
		t.Fatal("guessed ambiguous date format")
	}
}
