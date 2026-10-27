package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// EvalCase is one labelled example: an input plus the category id a human
// considers correct (0 means "should be left uncategorized").
type EvalCase struct {
	Note        string     `json:"note"`
	Currency    string     `json:"currency"`
	AmountMinor int64      `json:"amount_minor"`
	Candidates  []Category `json:"candidates"`
	WantID      int64      `json:"want_id"`
}

// EvalResult aggregates how a Categorizer did against a labelled set. This is
// the artifact that makes the AI feature engineerable: a regression in the
// prompt or model shows up as a drop in Accuracy.
type EvalResult struct {
	Total      int
	Correct    int
	Mismatches []EvalMismatch
}

// EvalMismatch records an example whose predicted category differs from its label.
type EvalMismatch struct {
	Note   string
	WantID int64
	GotID  int64
}

// Accuracy returns the fraction of exact matches, or zero for an empty dataset.
func (r EvalResult) Accuracy() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Correct) / float64(r.Total)
}

// LoadEvalCases reads JSONL (one EvalCase per line). Blank lines are skipped.
func LoadEvalCases(r io.Reader) ([]EvalCase, error) {
	var cases []EvalCase
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c EvalCase
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

// Evaluate runs the categorizer over every case and scores exact-match on the
// chosen category id.
func Evaluate(ctx context.Context, c Categorizer, cases []EvalCase) (EvalResult, error) {
	res := EvalResult{Total: len(cases)}
	for _, tc := range cases {
		got, err := c.Categorize(ctx, CategorizeInput{
			Note:        tc.Note,
			Currency:    tc.Currency,
			AmountMinor: tc.AmountMinor,
			Candidates:  tc.Candidates,
		})
		if err != nil {
			return res, err
		}
		if got.CategoryID == tc.WantID {
			res.Correct++
		} else {
			res.Mismatches = append(res.Mismatches, EvalMismatch{Note: tc.Note, WantID: tc.WantID, GotID: got.CategoryID})
		}
	}
	return res, nil
}
