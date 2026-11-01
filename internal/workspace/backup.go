// Package workspace defines the portable, versioned financial workspace format.
package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EftikharAzim/ledgerx/internal/money"
	"golang.org/x/text/currency"
)

// Limits bound preview memory and one atomic restore.
const (
	MaxBytes        = 32 * 1024 * 1024
	MaxTransactions = 50000
	MaxPostings     = 150000
)

// ErrLimits describes a workspace beyond the bounded portable restore format.
var ErrLimits = errors.New("workspace exceeds backup limits: 32 MiB, 50,000 transactions, or 150,000 postings")

// Appearance contains optional device account preferences, never credentials.
type Appearance struct {
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// Account retains identity within a backup; restore allocates new server IDs.
type Account struct {
	ID              int64       `json:"id"`
	Name            string      `json:"name"`
	Currency        string      `json:"currency"`
	Active          bool        `json:"active"`
	Kind            string      `json:"kind"`
	CreatedAt       time.Time   `json:"created_at"`
	TrackingStartAt *time.Time  `json:"tracking_start_at,omitempty"`
	Appearance      *Appearance `json:"appearance,omitempty"`
}

// Category includes its original creation time when known.
type Category struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// Transaction is the complete booked header, including corrections and hints.
type Transaction struct {
	ID                  int64     `json:"id"`
	Currency            string    `json:"currency"`
	CategoryID          int64     `json:"category_id,omitempty"`
	SuggestedCategoryID int64     `json:"suggested_category_id,omitempty"`
	SuggestionScore     *float64  `json:"suggestion_score,omitempty"`
	OccurredAt          time.Time `json:"occurred_at"`
	CreatedAt           time.Time `json:"created_at"`
	Note                string    `json:"note"`
	ReversalOf          int64     `json:"reversal_of,omitempty"`
	EntryType           string    `json:"entry_type"`
}

// Posting preserves insertion-time semantics as well as exact money.
type Posting struct {
	ID            int64     `json:"id"`
	TransactionID int64     `json:"transaction_id"`
	AccountID     int64     `json:"account_id"`
	AmountMinor   int64     `json:"amount_minor"`
	CreatedAt     time.Time `json:"created_at"`
}

// Import retains a completed import receipt; unsaved previews are transient.
type Import struct {
	ID          int64           `json:"id"`
	AccountID   int64           `json:"account_id"`
	Filename    string          `json:"filename"`
	FileHash    string          `json:"file_hash"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt time.Time       `json:"completed_at"`
	Receipt     json.RawMessage `json:"receipt"`
}

// Origin preserves duplicate detection, source references, and calendar dates.
type Origin struct {
	ID            int64  `json:"id"`
	AccountID     int64  `json:"account_id"`
	ImportID      int64  `json:"import_id"`
	TransactionID int64  `json:"transaction_id"`
	RowNumber     int32  `json:"row_number"`
	Fingerprint   string `json:"fingerprint"`
	SourceRef     string `json:"source_ref"`
	DateOnly      bool   `json:"date_only"`
}

// Correction preserves the links explaining a transaction edit.
type Correction struct {
	ID             int64           `json:"id"`
	OriginalID     int64           `json:"original_id"`
	ReversalID     int64           `json:"reversal_id"`
	ReplacementID  int64           `json:"replacement_id,omitempty"`
	ImportSourceID int64           `json:"import_source_id,omitempty"`
	DateOnly       bool            `json:"date_only"`
	Receipt        json.RawMessage `json:"receipt"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Check retains a comparison snapshot and its position in ledger history.
type Check struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	ActualMinor     int64     `json:"actual_minor"`
	RecordedMinor   int64     `json:"recorded_minor"`
	DifferenceMinor int64     `json:"difference_minor"`
	LedgerRevision  int64     `json:"ledger_revision"`
	Note            string    `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
}

// Data includes authoritative financial records and their explanatory metadata.
type Data struct {
	Accounts     []Account     `json:"accounts"`
	Categories   []Category    `json:"categories"`
	Transactions []Transaction `json:"transactions"`
	Postings     []Posting     `json:"postings"`
	Imports      []Import      `json:"imports"`
	Origins      []Origin      `json:"origins"`
	Corrections  []Correction  `json:"corrections"`
	Checks       []Check       `json:"balance_checks"`
}

// Backup is a portable workspace, excluding authentication and derived jobs/cache.
type Backup struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Data       Data      `json:"data"`
	SHA256     string    `json:"sha256,omitempty"`
}

// AccountSummary is calculated from all booked postings, including reversals.
type AccountSummary struct {
	SourceID     int64  `json:"source_id"`
	Name         string `json:"name"`
	Currency     string `json:"currency"`
	Active       bool   `json:"active"`
	BalanceMinor int64  `json:"balance_minor"`
}

// Summary is safe to show before restoring the financial data.
type Summary struct {
	ExportedAt       time.Time        `json:"exported_at"`
	Accounts         []AccountSummary `json:"accounts"`
	CategoryCount    int              `json:"category_count"`
	TransactionCount int              `json:"transaction_count"`
	PostingCount     int              `json:"posting_count"`
	ImportCount      int              `json:"import_count"`
	CorrectionCount  int              `json:"correction_count"`
	CheckCount       int              `json:"check_count"`
}

// Digest covers every typed field except the checksum itself.
func (b Backup) Digest() (string, error) {
	b.SHA256 = ""
	raw, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return "", err
	}
	raw, err = json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Seal produces a checksum after a consistent export has been assembled.
func (b *Backup) Seal() error { sum, err := b.Digest(); b.SHA256 = sum; return err }

func minor(n int64) bool             { return n >= -money.MaxSafeMinor && n <= money.MaxSafeMinor }
func text(s string) bool             { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func date(t time.Time) bool          { return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 }
func optionalDate(t *time.Time) bool { return t == nil || date(*t) }
func code(s string) bool {
	_, err := currency.ParseISO(s)
	return err == nil && len(s) == 3 && s == strings.ToUpper(s)
}
func id(n int64) bool { return n > 0 && n <= money.MaxSafeMinor }
func midnight(t time.Time) bool {
	t = t.UTC()
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

// Validate rejects corrupt references, non-balanced money, mismatched reversals,
// invalid historical balance snapshots, and unsupported format/size limits.
func (b Backup) Validate() (Summary, error) {
	var out Summary
	fail := func(reason string) (Summary, error) { return out, fmt.Errorf("invalid workspace backup: %s", reason) }
	if b.Format != "ledgerx.workspace" || b.Version != 1 {
		return fail("unsupported format or version; choose a LedgerX workspace JSON backup")
	}
	digest, err := b.Digest()
	if err != nil || b.SHA256 == "" || digest != b.SHA256 {
		return fail("checksum does not match; choose the original, unchanged backup")
	}
	if !date(b.ExportedAt) || b.ExportedAt.After(time.Now().UTC().Add(time.Minute)) {
		return fail("invalid export time")
	}
	d := b.Data
	if len(d.Accounts) > 1000 || len(d.Categories) > 10000 || len(d.Transactions) > MaxTransactions || len(d.Postings) > MaxPostings || len(d.Imports) > 10000 || len(d.Origins) > MaxTransactions || len(d.Corrections) > MaxTransactions || len(d.Checks) > 50000 {
		return fail("workspace exceeds supported restore limits")
	}
	accounts := map[int64]Account{}
	accountNames := map[string]bool{}
	external := 0
	for _, a := range d.Accounts {
		if !id(a.ID) || accounts[a.ID].ID != 0 || !text(a.Name) || accountNames[a.Name] || !code(a.Currency) || !date(a.CreatedAt) || !optionalDate(a.TrackingStartAt) || (a.Kind != "normal" && a.Kind != "external") {
			return fail("account identity, name, currency, or date is invalid")
		}
		if a.Kind == "external" {
			external++
		}
		if external > 1 {
			return fail("more than one external account")
		}
		if a.Appearance != nil {
			icons := map[string]bool{"wallet": true, "bank": true, "card": true, "coins": true}
			colors := map[string]bool{"sage": true, "apricot": true, "lavender": true, "sky": true}
			if !icons[a.Appearance.Icon] || !colors[a.Appearance.Color] {
				return fail("unknown account appearance")
			}
		}
		accounts[a.ID] = a
		accountNames[a.Name] = true
	}
	cats := map[int64]Category{}
	catNames := map[string]bool{}
	for _, c := range d.Categories {
		key := strings.ToLower(c.Name)
		if !id(c.ID) || cats[c.ID].ID != 0 || !text(c.Name) || catNames[key] || !optionalDate(c.CreatedAt) {
			return fail("category identity, name, or date is invalid")
		}
		cats[c.ID] = c
		catNames[key] = true
	}
	txs := map[int64]Transaction{}
	reversed := map[int64]int64{}
	for _, t := range d.Transactions {
		if !id(t.ID) || txs[t.ID].ID != 0 || !code(t.Currency) || !text(t.Note) || !date(t.OccurredAt) || !date(t.CreatedAt) || t.CreatedAt.After(b.ExportedAt.Add(time.Minute)) || (t.EntryType != "standard" && t.EntryType != "refund" && t.EntryType != "opening_balance") {
			return fail("transaction identity, type, money currency, or date is invalid")
		}
		if (t.CategoryID != 0 && cats[t.CategoryID].ID == 0) || (t.SuggestedCategoryID != 0 && cats[t.SuggestedCategoryID].ID == 0) {
			return fail("transaction refers to a missing category")
		}
		txs[t.ID] = t
	}
	postingIDs := map[int64]Posting{}
	legs := map[int64]map[int64]int64{}
	totals := map[int64]*big.Int{}
	postings := append([]Posting{}, d.Postings...)
	sort.Slice(postings, func(i, j int) bool { return postings[i].ID < postings[j].ID })
	checkIDs := map[int64]bool{}
	checks := append([]Check{}, d.Checks...)
	sort.Slice(checks, func(i, j int) bool { return checks[i].LedgerRevision < checks[j].LedgerRevision })
	checkIndex := 0
	validateCheck := func(c Check) bool {
		a := accounts[c.AccountID]
		if !id(c.ID) || checkIDs[c.ID] || a.Kind != "normal" || !minor(c.ActualMinor) || !minor(c.RecordedMinor) || !minor(c.DifferenceMinor) || c.ActualMinor-c.RecordedMinor != c.DifferenceMinor || !date(c.CreatedAt) || !text(c.Note) {
			return false
		}
		if c.LedgerRevision != 0 && postingIDs[c.LedgerRevision].AccountID != c.AccountID {
			return false
		}
		total := totals[c.AccountID]
		if total == nil {
			total = new(big.Int)
		}
		checkIDs[c.ID] = true
		return total.Cmp(big.NewInt(c.RecordedMinor)) == 0
	}
	for _, p := range postings {
		for checkIndex < len(checks) && checks[checkIndex].LedgerRevision < p.ID {
			if !validateCheck(checks[checkIndex]) {
				return fail("a balance check does not match its historical ledger revision")
			}
			checkIndex++
		}
		a, t := accounts[p.AccountID], txs[p.TransactionID]
		if !id(p.ID) || postingIDs[p.ID].ID != 0 || a.ID == 0 || t.ID == 0 || p.AmountMinor == 0 || !minor(p.AmountMinor) || !date(p.CreatedAt) || p.CreatedAt.After(b.ExportedAt.Add(time.Minute)) || (a.Kind == "normal" && a.Currency != t.Currency) {
			return fail("posting identity, account, amount, currency, or date is invalid")
		}
		postingIDs[p.ID] = p
		if legs[t.ID] == nil {
			legs[t.ID] = map[int64]int64{}
		}
		if legs[t.ID][a.ID] != 0 {
			return fail("a transaction repeats an account leg")
		}
		legs[t.ID][a.ID] = p.AmountMinor
		if totals[a.ID] == nil {
			totals[a.ID] = new(big.Int)
		}
		totals[a.ID].Add(totals[a.ID], big.NewInt(p.AmountMinor))
	}
	for ; checkIndex < len(checks); checkIndex++ {
		if !validateCheck(checks[checkIndex]) {
			return fail("a balance check does not match its historical ledger revision")
		}
	}
	for _, t := range d.Transactions {
		sum := new(big.Int)
		for _, amount := range legs[t.ID] {
			sum.Add(sum, big.NewInt(amount))
		}
		if sum.Sign() != 0 || len(legs[t.ID]) == 1 {
			return fail("transaction postings are not balanced")
		}
		if t.EntryType == "refund" && t.ReversalOf == 0 {
			normal := new(big.Int)
			for aid, amount := range legs[t.ID] {
				if accounts[aid].Kind == "normal" {
					normal.Add(normal, big.NewInt(amount))
				}
			}
			if normal.Sign() <= 0 {
				return fail("a refund must return positive money to normal accounts")
			}
		}
		if t.ReversalOf != 0 {
			original := txs[t.ReversalOf]
			if original.ID == 0 || original.ReversalOf != 0 || original.ID >= t.ID || reversed[original.ID] != 0 || original.Currency != t.Currency || original.EntryType != t.EntryType || len(legs[original.ID]) != len(legs[t.ID]) {
				return fail("invalid or repeated reversal link")
			}
			for aid, amount := range legs[original.ID] {
				if legs[t.ID][aid] != -amount {
					return fail("reversal does not offset the original transaction")
				}
			}
			reversed[original.ID] = t.ID
		}
	}
	imports := map[int64]Import{}
	for _, i := range d.Imports {
		if !id(i.ID) || imports[i.ID].ID != 0 || accounts[i.AccountID].Kind != "normal" || !date(i.CreatedAt) || !date(i.CompletedAt) || !text(i.Filename) || !text(i.FileHash) || !json.Valid(i.Receipt) {
			return fail("invalid completed import receipt")
		}
		imports[i.ID] = i
	}
	origins := map[int64]Origin{}
	originIDs := map[int64]bool{}
	sourceRows := map[string]bool{}
	for _, o := range d.Origins {
		rowKey := fmt.Sprintf("%d:%d", o.ImportID, o.RowNumber)
		t := txs[o.TransactionID]
		if !id(o.ID) || originIDs[o.ID] || origins[o.TransactionID].ID != 0 || sourceRows[rowKey] || imports[o.ImportID].ID == 0 || imports[o.ImportID].AccountID != o.AccountID || t.ID == 0 || t.ReversalOf != 0 || t.EntryType == "opening_balance" || accounts[o.AccountID].Kind != "normal" || legs[t.ID][o.AccountID] == 0 || o.RowNumber < 1 || o.RowNumber > 1024*1024 || len(o.Fingerprint) != 64 || !text(o.SourceRef) || (o.DateOnly && !midnight(t.OccurredAt)) {
			return fail("invalid import provenance")
		}
		origins[t.ID] = o
		originIDs[o.ID] = true
		sourceRows[rowKey] = true
	}
	correctionIDs := map[int64]bool{}
	corrected := map[int64]bool{}
	replacements := map[int64]bool{}
	correctionRoot := map[int64]int64{}
	corrections := append([]Correction{}, d.Corrections...)
	sort.Slice(corrections, func(i, j int) bool { return corrections[i].OriginalID < corrections[j].OriginalID })
	for _, c := range corrections {
		orig, rev := txs[c.OriginalID], txs[c.ReversalID]
		if !id(c.ID) || correctionIDs[c.ID] || corrected[c.OriginalID] || orig.ID == 0 || rev.ReversalOf != orig.ID || orig.ReversalOf != 0 || !date(c.CreatedAt) || !json.Valid(c.Receipt) {
			return fail("invalid correction history")
		}
		root := correctionRoot[orig.ID]
		if o := origins[orig.ID]; o.ID != 0 {
			root = orig.ID
		}
		if c.ImportSourceID != root {
			return fail("correction import origin is inconsistent")
		}
		if c.ReplacementID != 0 {
			r := txs[c.ReplacementID]
			if r.ID == 0 || r.ReversalOf != 0 || r.ID <= rev.ID || replacements[r.ID] || r.Currency != orig.Currency || (c.DateOnly && !midnight(r.OccurredAt)) {
				return fail("invalid correction replacement")
			}
			replacements[r.ID] = true
			correctionRoot[r.ID] = root
		} else if orig.EntryType != "opening_balance" {
			return fail("only a zero starting balance can omit its replacement")
		}
		correctionIDs[c.ID] = true
		corrected[orig.ID] = true
	}
	out = Summary{ExportedAt: b.ExportedAt, Accounts: []AccountSummary{}, CategoryCount: len(d.Categories), TransactionCount: len(d.Transactions), PostingCount: len(d.Postings), ImportCount: len(d.Imports), CorrectionCount: len(d.Corrections), CheckCount: len(d.Checks)}
	for _, a := range d.Accounts {
		if a.Kind != "normal" {
			continue
		}
		total := totals[a.ID]
		if total == nil {
			total = new(big.Int)
		}
		if !total.IsInt64() || !minor(total.Int64()) {
			return fail("an account balance exceeds the supported exact range")
		}
		out.Accounts = append(out.Accounts, AccountSummary{SourceID: a.ID, Name: a.Name, Currency: a.Currency, Active: a.Active, BalanceMinor: total.Int64()})
	}
	return out, nil
}

// Decode limits bytes and rejects fields from unknown/future formats.
func Decode(raw []byte) (Backup, error) {
	var b Backup
	if len(raw) > MaxBytes {
		return b, errors.New("workspace backup must be no larger than 32 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		return b, errors.New("choose a valid LedgerX workspace JSON backup")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return b, errors.New("backup contains trailing JSON data")
	}
	return b, nil
}
