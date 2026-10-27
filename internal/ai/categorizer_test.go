package ai

import (
	"context"
	"os"
	"testing"
)

func TestKeywordCategorizerWordBoundary(t *testing.T) {
	cats := []Category{{ID: 1, Name: "Food"}, {ID: 2, Name: "Rent"}}
	k := KeywordCategorizer{}

	cases := []struct {
		note string
		want int64
	}{
		{"fast food lunch", 1}, // whole-word match
		{"seafood platter", 0}, // must NOT match "food" inside "seafood"
		{"rent payment", 2},
		{"nothing relevant here", 0},
	}
	for _, c := range cases {
		got, err := k.Categorize(context.Background(), CategorizeInput{Note: c.note, Candidates: cats})
		if err != nil {
			t.Fatal(err)
		}
		if got.CategoryID != c.want {
			t.Errorf("note %q: got %d, want %d", c.note, got.CategoryID, c.want)
		}
	}
}

func TestContainsWord(t *testing.T) {
	cases := []struct {
		hay, needle string
		want        bool
	}{
		{"fast food", "food", true},
		{"seafood", "food", false},
		{"food", "food", true},
		{"foodie", "food", false},
		{"i like food.", "food", true},
	}
	for _, c := range cases {
		if got := containsWord(c.hay, c.needle); got != c.want {
			t.Errorf("containsWord(%q,%q)=%v want %v", c.hay, c.needle, got, c.want)
		}
	}
}

// TestEvalHarnessOnKeywordBaseline exercises the eval harness end to end with
// the deterministic keyword categorizer. It pins the baseline accuracy into a
// band: high enough to prove the harness scores correctly, below 1.0 to prove
// it discriminates (the gap is the headroom the LLM is meant to close).
func TestEvalHarnessOnKeywordBaseline(t *testing.T) {
	f, err := os.Open("testdata/categorize_golden.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	cases, err := LoadEvalCases(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("expected a non-trivial golden set, got %d cases", len(cases))
	}

	res, err := Evaluate(context.Background(), KeywordCategorizer{}, cases)
	if err != nil {
		t.Fatal(err)
	}
	acc := res.Accuracy()
	if acc < 0.5 {
		t.Errorf("keyword baseline accuracy %.2f below floor 0.50 — harness or data regression", acc)
	}
	if acc >= 1.0 {
		t.Errorf("keyword baseline accuracy is 1.0 — golden set has no semantic headroom to motivate the model")
	}
	t.Logf("keyword baseline: %d/%d correct (%.0f%%)", res.Correct, res.Total, acc*100)
}
