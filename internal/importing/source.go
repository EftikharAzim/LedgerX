package importing

import (
	"errors"
	"strings"
)

func sourceColumns(doc Document) (int, int, int) {
	account, currency, date := -1, -1, -1
	for i, h := range doc.Headers {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "account", "wallet":
			account = i
		case "currency":
			currency = i
		case "date", "transaction date":
			date = i
		}
	}
	return account, currency, date
}

func groupKey(row Record, account, currency int) string {
	a, c := "", ""
	if account >= 0 && account < len(row.Cells) {
		a = strings.TrimSpace(row.Cells[account])
	}
	if currency >= 0 && currency < len(row.Cells) {
		c = strings.ToUpper(strings.TrimSpace(row.Cells[currency]))
	}
	return Hash([]string{a, c})
}

func annotateSource(doc *Document) {
	a, c, d := sourceColumns(*doc)
	indices := map[string]int{}
	dayFirst, monthFirst := false, false
	for _, row := range doc.Records {
		if len(row.Cells) != len(doc.Headers) {
			continue
		}
		if a >= 0 {
			key := groupKey(row, a, c)
			i, ok := indices[key]
			if !ok {
				group := SourceGroup{Key: key}
				if a >= 0 {
					group.Account = strings.TrimSpace(row.Cells[a])
				}
				if c >= 0 {
					group.Currency = strings.ToUpper(strings.TrimSpace(row.Cells[c]))
				}
				i = len(doc.SourceGroups)
				indices[key] = i
				doc.SourceGroups = append(doc.SourceGroups, group)
			}
			doc.SourceGroups[i].Rows++
		}
		if d >= 0 {
			s := strings.TrimSpace(row.Cells[d])
			_, _, de := Date(s, "day_first")
			_, _, me := Date(s, "month_first")
			dayFirst = dayFirst || (de == nil && me != nil)
			monthFirst = monthFirst || (me == nil && de != nil)
		}
	}
	if dayFirst && !monthFirst {
		doc.DateFormat = "day_first"
	}
	if monthFirst && !dayFirst {
		doc.DateFormat = "month_first"
	}
	set := map[string]bool{}
	for _, h := range doc.Headers {
		set[strings.ToLower(h)] = true
	}
	if set["id"] && set["exclude report"] && set["account"] && set["date"] && set["amount"] {
		doc.Source = "moneylover"
	}
}

// SelectSource retains original file line numbers for stable review/retry identity.
func SelectSource(doc Document, key string) (Document, error) {
	if key == "" && len(doc.SourceGroups) <= 1 {
		return doc, nil
	}
	if key == "" {
		return Document{}, errors.New("choose the source account and currency from this file")
	}
	found := false
	for _, group := range doc.SourceGroups {
		found = found || group.Key == key
	}
	if !found {
		return Document{}, errors.New("source account was not found in this file")
	}
	a, c, _ := sourceColumns(doc)
	selected := doc
	selected.Records = nil
	selected.Samples = nil
	for _, row := range doc.Records {
		if groupKey(row, a, c) == key {
			selected.Records = append(selected.Records, row)
			if len(selected.Samples) < 5 {
				selected.Samples = append(selected.Samples, row.Cells)
			}
		}
	}
	selected.RowCount = len(selected.Records)
	return selected, nil
}
