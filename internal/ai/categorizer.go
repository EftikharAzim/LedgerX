// Package ai holds the LLM-backed features that sit OUTSIDE LedgerX's
// correctness boundary. Nothing here writes to the ledger or computes money;
// a Categorizer only proposes which existing category a transaction belongs
// to. The worker stores that as a *suggestion*, and a human confirms it.
package ai

import (
	"context"
	"strings"
)

// Category is a candidate the classifier may choose from.
type Category struct {
	ID   int64
	Name string
}

// CategorizeInput is everything the classifier is allowed to see. Deliberately
// small: a free-text note, the currency, the signed minor-unit amount, and the
// user's existing categories. No account identifiers, no other users' data.
type CategorizeInput struct {
	Note        string
	Currency    string
	AmountMinor int64
	Candidates  []Category
}

// CategorizeResult is a closed-set choice. CategoryID == 0 means "none of the
// candidates fit" — the worker then leaves the transaction uncategorized
// rather than guessing.
type CategorizeResult struct {
	CategoryID int64
	Confidence float64 // 0..1
}

// Categorizer proposes a category for a transaction. Implementations must be
// safe to call concurrently.
type Categorizer interface {
	Categorize(ctx context.Context, in CategorizeInput) (CategorizeResult, error)
	// Name identifies the implementation in logs and eval reports.
	Name() string
}

// validCandidate reports whether id is one of the offered candidates. The
// classifier output is never trusted to name an arbitrary category.
func validCandidate(id int64, candidates []Category) bool {
	for _, c := range candidates {
		if c.ID == id {
			return true
		}
	}
	return false
}

// KeywordCategorizer is a dependency-free fallback used when no Anthropic key
// is configured, and as the deterministic baseline in evals/tests. It assigns
// a transaction to a category when that category's name appears as a word in
// the note (case-insensitive). Confidence is fixed and modest — it is a
// baseline, not a model.
type KeywordCategorizer struct{}

// Name identifies the deterministic keyword backend.
func (KeywordCategorizer) Name() string { return "keyword" }

// Categorize suggests the first candidate whose name matches a whole word in the note.
func (KeywordCategorizer) Categorize(_ context.Context, in CategorizeInput) (CategorizeResult, error) {
	note := strings.ToLower(in.Note)
	for _, c := range in.Candidates {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if name == "" {
			continue
		}
		if containsWord(note, name) {
			return CategorizeResult{CategoryID: c.ID, Confidence: 0.5}, nil
		}
	}
	return CategorizeResult{}, nil
}

// containsWord reports whether needle appears in haystack on word boundaries,
// so "food" matches "food" and "fast food" but not "seafood".
func containsWord(haystack, needle string) bool {
	idx := 0
	for {
		i := strings.Index(haystack[idx:], needle)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(needle)
		leftOK := start == 0 || !isWordChar(haystack[start-1])
		rightOK := end == len(haystack) || !isWordChar(haystack[end])
		if leftOK && rightOK {
			return true
		}
		idx = start + 1
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
