package workspace

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/money"
)

func validBackup(t *testing.T) Backup {
	t.Helper()
	at := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	b := Backup{Format: "ledgerx.workspace", Version: 1, ExportedAt: time.Now().UTC(), Data: Data{
		Accounts:     []Account{{ID: 1, Name: "Bank", Currency: "USD", Active: true, Kind: "normal", CreatedAt: at}, {ID: 2, Name: "External", Currency: "USD", Active: true, Kind: "external", CreatedAt: at}},
		Categories:   []Category{{ID: 1, Name: "Income", CreatedAt: &at}},
		Transactions: []Transaction{{ID: 1, Currency: "USD", CategoryID: 1, OccurredAt: at, CreatedAt: at, Note: "Salary", EntryType: "standard"}},
		Postings:     []Posting{{ID: 1, TransactionID: 1, AccountID: 1, AmountMinor: 100, CreatedAt: at}, {ID: 2, TransactionID: 1, AccountID: 2, AmountMinor: -100, CreatedAt: at}},
		Imports:      []Import{}, Origins: []Origin{}, Corrections: []Correction{}, Checks: []Check{{ID: 1, AccountID: 1, ActualMinor: 100, RecordedMinor: 100, DifferenceMinor: 0, LedgerRevision: 1, CreatedAt: at.Add(time.Hour)}},
	}}
	if err := b.Seal(); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBackupRoundTripAndCanonicalChecksum(t *testing.T) {
	b := validBackup(t)
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := decoded.Validate()
	if err != nil || summary.Accounts[0].BalanceMinor != 100 || summary.CheckCount != 1 {
		t.Fatal(summary, err)
	}
	// Receipt JSON objects can be reordered by JSONB without changing integrity.
	first := b
	first.Data.Imports = []Import{{Receipt: json.RawMessage(`{"b":2,"a":9007199254740991}`)}}
	second := b
	second.Data.Imports = []Import{{Receipt: json.RawMessage(`{"a":9007199254740991,"b":2}`)}}
	a, err := first.Digest()
	if err != nil {
		t.Fatal(err)
	}
	c, err := second.Digest()
	if err != nil || a != c {
		t.Fatal("checksum depends on JSON key order", a, c, err)
	}
	b.Data.Postings[0].AmountMinor = 101
	if _, err := b.Validate(); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatal("changed money passed checksum", err)
	}
}

func TestBackupRejectsInvalidFinancialRelationships(t *testing.T) {
	cases := map[string]func(*Backup){
		"unbalanced":        func(b *Backup) { b.Data.Postings[0].AmountMinor = 99 },
		"missing account":   func(b *Backup) { b.Data.Postings[0].AccountID = 999 },
		"currency mismatch": func(b *Backup) { b.Data.Accounts[0].Currency = "JPY" },
		"missing category":  func(b *Backup) { b.Data.Transactions[0].CategoryID = 999 },
		"duplicate posting": func(b *Backup) { b.Data.Postings = append(b.Data.Postings, b.Data.Postings[0]) },
		"zero posting":      func(b *Backup) { b.Data.Postings[0].AmountMinor = 0 },
		"unsafe amount":     func(b *Backup) { b.Data.Postings[0].AmountMinor = money.MaxSafeMinor + 1 },
		"missing original":  func(b *Backup) { b.Data.Transactions[0].ReversalOf = 999 },
		"forged historical balance": func(b *Backup) {
			b.Data.Checks[0].ActualMinor = 100
			b.Data.Checks[0].RecordedMinor = 50
			b.Data.Checks[0].DifferenceMinor = 50
		},
		"foreign revision":   func(b *Backup) { b.Data.Checks[0].LedgerRevision = 2 },
		"invalid revision":   func(b *Backup) { b.Data.Checks[0].LedgerRevision = 999 },
		"invalid appearance": func(b *Backup) { b.Data.Accounts[0].Appearance = &Appearance{Icon: "script", Color: "unknown"} },
		"unknown version":    func(b *Backup) { b.Version = 2 },
		"negative refund": func(b *Backup) {
			b.Data.Transactions[0].EntryType = "refund"
			b.Data.Postings[0].AmountMinor = -100
			b.Data.Postings[1].AmountMinor = 100
			b.Data.Checks = nil
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b := validBackup(t)
			change(&b)
			if err := b.Seal(); err != nil {
				t.Fatal(err)
			}
			if _, err := b.Validate(); err == nil {
				t.Fatal("invalid backup accepted")
			}
		})
	}
}

func TestBackupReversalsMustMatchOriginalAccountsAndAmounts(t *testing.T) {
	b := validBackup(t)
	at := b.Data.Transactions[0].CreatedAt.AddDate(0, 0, 2)
	b.Data.Transactions = append(b.Data.Transactions, Transaction{ID: 2, Currency: "USD", OccurredAt: at, CreatedAt: at, ReversalOf: 1, EntryType: "standard"})
	b.Data.Postings = append(b.Data.Postings, Posting{ID: 3, TransactionID: 2, AccountID: 1, AmountMinor: -100, CreatedAt: at}, Posting{ID: 4, TransactionID: 2, AccountID: 2, AmountMinor: 100, CreatedAt: at})
	if err := b.Seal(); err != nil {
		t.Fatal(err)
	}
	s, err := b.Validate()
	if err != nil || s.Accounts[0].BalanceMinor != 0 {
		t.Fatal(s, err)
	}
	b.Data.Postings[2].AmountMinor = -50
	b.Data.Postings[3].AmountMinor = 50
	if err := b.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Validate(); err == nil {
		t.Fatal("balanced but incorrect reversal accepted")
	}
}

func TestBackupDecoderRejectsOtherFormatsAndTrailingData(t *testing.T) {
	b := validBackup(t)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{"{}", "garbage"} {
		if _, err := Decode(append(append([]byte{}, raw...), []byte(tail)...)); err == nil {
			t.Fatalf("accepted trailing %q", tail)
		}
	}
	if _, err := Decode([]byte(`{"format":"ledgerx.workspace","unknown":true}`)); err == nil {
		t.Fatal("accepted unknown fields")
	}
	if _, err := Decode([]byte("Date,Amount\n2024-01-01,1")); err == nil {
		t.Fatal("accepted CSV as backup")
	}
	if _, err := Decode(make([]byte, MaxBytes+1)); err == nil {
		t.Fatal("accepted oversized backup")
	}
}
