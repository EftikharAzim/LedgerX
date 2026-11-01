// Package importing validates bank/spreadsheet CSV files without writing a ledger.
package importing

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EftikharAzim/ledgerx/internal/money"
)

// Limits bound memory, preview size, and the duration of an atomic batch.
const (
	MaxBytes   = 1024 * 1024
	MaxRows    = 1000
	MaxColumns = 50
)

// Record retains a source line number, including multiline quoted CSV fields.
type Record struct {
	Number int
	Cells  []string
}

// Document is an inspected UTF-8 CSV with one header row.
type Document struct {
	Headers   []string   `json:"headers"`
	Delimiter string     `json:"delimiter"`
	RowCount  int        `json:"row_count"`
	Samples   [][]string `json:"samples"`
	Records   []Record   `json:"-"`
}

func blank(cells []string) bool {
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// Inspect accepts comma, semicolon, or tab separated UTF-8 files.
func Inspect(text, delimiter string) (Document, error) {
	var out Document
	if len(text) > MaxBytes {
		return out, errors.New("CSV must be no larger than 1 MiB")
	}
	if !utf8.ValidString(text) {
		return out, errors.New("export the file as UTF-8 CSV")
	}
	text = strings.TrimPrefix(text, "\ufeff")
	delimiters := map[string]rune{"comma": ',', "semicolon": ';', "tab": '\t'}
	if delimiter == "" || delimiter == "auto" {
		best := 0
		for _, name := range []string{"comma", "semicolon", "tab"} {
			r := csv.NewReader(strings.NewReader(text))
			r.Comma = delimiters[name]
			r.FieldsPerRecord = -1
			for {
				cells, err := r.Read()
				if err != nil {
					break
				}
				if blank(cells) {
					continue
				}
				if len(cells) > best {
					best, delimiter = len(cells), name
				}
				break
			}
		}
	}
	separator, ok := delimiters[delimiter]
	if !ok {
		return out, errors.New("choose comma, semicolon, or tab as the delimiter")
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = separator
	r.FieldsPerRecord = -1
	for {
		cells, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, fmt.Errorf("could not parse CSV: %w", err)
		}
		if blank(cells) {
			continue
		}
		if len(cells) > MaxColumns {
			return out, errors.New("CSV can contain at most 50 columns")
		}
		if out.Headers == nil {
			if len(cells) < 2 {
				return out, errors.New("CSV needs a header row and at least two columns")
			}
			out.Headers = make([]string, len(cells))
			for i, c := range cells {
				out.Headers[i] = strings.TrimSpace(c)
				if out.Headers[i] == "" {
					out.Headers[i] = fmt.Sprintf("Column %d", i+1)
				}
				if len([]rune(out.Headers[i])) > 120 {
					return out, errors.New("column headers must be at most 120 characters")
				}
			}
			continue
		}
		if len(out.Records) >= MaxRows {
			return out, errors.New("import at most 1,000 records at a time")
		}
		line, _ := r.FieldPos(0)
		out.Records = append(out.Records, Record{Number: line, Cells: cells})
		if len(out.Samples) < 5 {
			out.Samples = append(out.Samples, cells)
		}
	}
	if len(out.Records) == 0 {
		return out, errors.New("CSV does not contain any data rows")
	}
	// A ledger report has two transfer legs and reversal history, not one row per
	// customer purchase. Reject it rather than silently replaying canceled money.
	headerSet := map[string]bool{}
	for _, h := range out.Headers {
		headerSet[strings.ToLower(h)] = true
	}
	if headerSet["transaction_id"] && headerSet["amount_minor"] && headerSet["account_id"] {
		return out, errors.New("this is a full ledger report with transfers and corrections; import a bank or spreadsheet CSV with one row per income, expense, or refund instead")
	}
	out.Delimiter, out.RowCount = delimiter, len(out.Records)
	return out, nil
}

// Mapping uses zero-based column positions, avoiding ambiguous duplicate names.
type Mapping struct {
	Date           *int   `json:"date"`
	Amount         *int   `json:"amount"`
	Debit          *int   `json:"debit"`
	Credit         *int   `json:"credit"`
	Note           *int   `json:"note"`
	Category       *int   `json:"category"`
	Currency       *int   `json:"currency"`
	Type           *int   `json:"type"`
	Reference      *int   `json:"reference"`
	AmountMode     string `json:"amount_mode"`
	DateFormat     string `json:"date_format"`
	NumberStyle    string `json:"number_style"`
	SignConvention string `json:"sign_convention"`
}

// Validate checks the mapping before any row is interpreted.
func (m Mapping) Validate(columns int) error {
	if m.Date == nil || (m.AmountMode != "signed" && m.AmountMode != "debit_credit") || (m.AmountMode == "signed" && m.Amount == nil) || (m.AmountMode == "debit_credit" && m.Debit == nil && m.Credit == nil) {
		return errors.New("map a date and either an amount or debit/credit columns")
	}
	if m.DateFormat != "iso" && m.DateFormat != "day_first" && m.DateFormat != "month_first" {
		return errors.New("choose the date format used by the file")
	}
	if m.NumberStyle != "dot" && m.NumberStyle != "comma" {
		return errors.New("choose the number format used by the file")
	}
	if m.SignConvention != "income_positive" && m.SignConvention != "expense_positive" {
		return errors.New("choose how positive amounts should be interpreted")
	}
	seen := map[int]bool{}
	positions := []*int{m.Date, m.Note, m.Category, m.Currency, m.Type, m.Reference}
	if m.AmountMode == "signed" {
		positions = append(positions, m.Amount)
	} else {
		positions = append(positions, m.Debit, m.Credit)
	}
	for _, p := range positions {
		if p == nil {
			continue
		}
		if *p < 0 || *p >= columns || seen[*p] {
			return errors.New("each mapped field must use a different valid column")
		}
		seen[*p] = true
	}
	return nil
}

// Duplicate is a possible existing transaction or repeated line, never a deletion.
type Duplicate struct {
	TransactionID int64     `json:"transaction_id,omitempty"`
	RowNumber     int       `json:"row_number,omitempty"`
	OccurredAt    time.Time `json:"occurred_at,omitempty"`
	AmountMinor   int64     `json:"amount_minor,omitempty"`
	Note          string    `json:"note,omitempty"`
	MatchType     string    `json:"match_type"`
}

// Row is a server-normalized preview row. Errors make a row unselectable.
type Row struct {
	Number      int         `json:"number"`
	OccurredAt  time.Time   `json:"occurred_at"`
	DateOnly    bool        `json:"date_only"`
	AmountMinor int64       `json:"amount_minor"`
	Currency    string      `json:"currency"`
	EntryType   string      `json:"entry_type"`
	Note        string      `json:"note"`
	Category    string      `json:"category"`
	SourceRef   string      `json:"source_ref"`
	Fingerprint string      `json:"fingerprint"`
	Errors      []string    `json:"errors"`
	Duplicates  []Duplicate `json:"duplicates"`
	NewCategory bool        `json:"new_category"`
}

// Hash is a stable digest for canonical input and preview identities.
func Hash(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Date parses explicit date formats. Date-only entries retain their UTC calendar
// date in history; timestamps preserve their offset/instant.
func Date(value, format string) (time.Time, bool, error) {
	s := strings.TrimSpace(value)
	if format == "iso" && strings.Contains(s, "T") {
		at, err := time.Parse(time.RFC3339Nano, s)
		if err != nil || at.Year() < 1 || at.Year() > 9999 {
			return time.Time{}, false, errors.New("use an ISO date or an RFC 3339 timestamp")
		}
		return at.UTC(), false, nil
	}
	var layouts []string
	switch format {
	case "iso":
		layouts = []string{"2006-01-02"}
	case "day_first":
		layouts = []string{"2/1/2006", "2-1-2006"}
	case "month_first":
		layouts = []string{"1/2/2006", "1-2-2006"}
	default:
		return time.Time{}, true, errors.New("choose a date format")
	}
	for _, layout := range layouts {
		if at, err := time.Parse(layout, s); err == nil && at.Year() >= 1 && at.Year() <= 9999 {
			return at, true, nil
		}
	}
	return time.Time{}, true, errors.New("date does not match the selected format")
}

// Normalize interprets rows with exact integer money and explicit classifications.
func Normalize(doc Document, mapping Mapping, currency string, now time.Time) ([]Row, error) {
	if err := mapping.Validate(len(doc.Headers)); err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(doc.Records))
	byRef, byBase := map[string]int{}, map[string]int{}
	for _, record := range doc.Records {
		row := Row{Number: record.Number, Currency: currency, EntryType: "standard", Errors: []string{}, Duplicates: []Duplicate{}}
		if len(record.Cells) != len(doc.Headers) {
			row.Errors = append(row.Errors, "Column count differs from the header.")
			rows = append(rows, row)
			continue
		}
		cell := func(p *int) string {
			if p == nil {
				return ""
			}
			return strings.TrimSpace(record.Cells[*p])
		}
		at, dateOnly, err := Date(cell(mapping.Date), mapping.DateFormat)
		row.OccurredAt, row.DateOnly = at, dateOnly
		if err != nil {
			row.Errors = append(row.Errors, err.Error())
		} else if at.After(now.Add(time.Minute)) {
			row.Errors = append(row.Errors, "Date is in the future; import posted records only.")
		}
		row.Note, row.Category, row.SourceRef = cell(mapping.Note), cell(mapping.Category), cell(mapping.Reference)
		if len([]rune(row.Note)) > 1000 {
			row.Errors = append(row.Errors, "Note must be at most 1,000 characters.")
		}
		if len([]rune(row.Category)) > 80 {
			row.Errors = append(row.Errors, "Category must be at most 80 characters.")
		}
		if len([]rune(row.SourceRef)) > 128 {
			row.Errors = append(row.Errors, "Reference must be at most 128 characters.")
		}
		if mapping.Currency != nil && strings.ToUpper(cell(mapping.Currency)) != currency {
			row.Errors = append(row.Errors, "Currency differs from the selected account; no conversion is performed.")
		}
		err = nil
		if mapping.AmountMode == "signed" {
			row.AmountMinor, err = money.ParseDecimal(cell(mapping.Amount), currency, mapping.NumberStyle)
			if mapping.SignConvention == "expense_positive" {
				row.AmountMinor = -row.AmountMinor
			}
		} else {
			var debit, credit int64
			var debitErr, creditErr error
			if cell(mapping.Debit) != "" {
				debit, debitErr = money.ParseDecimal(cell(mapping.Debit), currency, mapping.NumberStyle)
			}
			if cell(mapping.Credit) != "" {
				credit, creditErr = money.ParseDecimal(cell(mapping.Credit), currency, mapping.NumberStyle)
			}
			if debit < 0 {
				debit = -debit
			}
			if credit < 0 {
				credit = -credit
			}
			if debitErr != nil || creditErr != nil || (debit != 0 && credit != 0) {
				err = errors.New("use one debit or credit amount per row")
			} else {
				row.AmountMinor = credit - debit
			}
		}
		if err != nil {
			row.Errors = append(row.Errors, "Amount: "+err.Error())
		} else if row.AmountMinor == 0 {
			row.Errors = append(row.Errors, "Amount must be nonzero.")
		}
		if mapping.Type != nil {
			switch strings.ToLower(cell(mapping.Type)) {
			case "expense", "debit", "withdrawal":
				if row.AmountMinor > 0 {
					row.AmountMinor = -row.AmountMinor
				}
			case "income", "credit", "deposit":
				if row.AmountMinor < 0 {
					row.Errors = append(row.Errors, "Income cannot have a negative amount.")
				}
			case "refund":
				row.EntryType = "refund"
				if row.AmountMinor < 0 {
					row.Errors = append(row.Errors, "A refund must be positive.")
				}
			case "transfer", "opening_balance", "reversal":
				row.Errors = append(row.Errors, "Transfers, starting balances, and reversals must use their dedicated ledger actions.")
			default:
				row.Errors = append(row.Errors, "Type must be income, expense, or refund; unmap this column to use amount signs.")
			}
		}
		row.Fingerprint = Hash(struct {
			At                        string
			Amount                    int64
			Currency, Type, Note, Ref string
		}{at.UTC().Format(time.RFC3339Nano), row.AmountMinor, currency, row.EntryType, strings.ToLower(strings.Join(strings.Fields(row.Note), " ")), row.SourceRef})
		if len(row.Errors) == 0 {
			base := Hash(struct {
				At         string
				Amount     int64
				Type, Note string
			}{at.UTC().Format(time.RFC3339Nano), row.AmountMinor, row.EntryType, strings.ToLower(strings.Join(strings.Fields(row.Note), " "))})
			previous, exists := byBase[base]
			if row.SourceRef != "" {
				if number, ok := byRef[row.SourceRef]; ok {
					previous, exists = number, true
				}
			}
			if exists {
				var prior Row
				for _, p := range rows {
					if p.Number == previous {
						prior = p
						break
					}
				}
				if row.SourceRef == "" || prior.SourceRef == "" || row.SourceRef == prior.SourceRef {
					row.Duplicates = append(row.Duplicates, Duplicate{RowNumber: previous, MatchType: "file"})
				}
			}
			if _, ok := byBase[base]; !ok {
				byBase[base] = row.Number
			}
			if row.SourceRef != "" {
				byRef[row.SourceRef] = row.Number
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
