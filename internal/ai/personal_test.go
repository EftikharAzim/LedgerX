package ai

import "testing"

func TestPersonalHintsPreferConfirmedHistoryAndAbstainWhenUnclear(t *testing.T) {
	candidates := []Category{{ID: 1, Name: "Food"}, {ID: 2, Name: "Transport"}}
	for _, example := range []struct {
		name, note          string
		history             []int64
		wantID              int64
		source              string
		supporting, samples int
	}{
		{"personal history beats name matching", "Transport", []int64{1, 1}, 1, "history", 2, 2},
		{"one prior choice is explained as one", "Coffee", []int64{1}, 1, "history", 1, 1},
		{"clear majority", "Coffee", []int64{1, 1, 2, 1, 1}, 1, "history", 4, 5},
		{"mixed choices do not fall back to a keyword", "Food", []int64{1, 2, 1}, 0, "ambiguous", 0, 3},
		{"equal choices abstain", "Food", []int64{1, 2}, 0, "ambiguous", 0, 2},
		{"unambiguous name fallback", "weekly food", nil, 1, "keyword", 0, 0},
		{"several category names", "food and transport", nil, 0, "ambiguous", 0, 0},
		{"no semantic guessing", "Uber to airport", nil, 0, "none", 0, 0},
		{"ignore IDs outside candidates", "Coffee", []int64{99, 99}, 0, "none", 0, 0},
	} {
		t.Run(example.name, func(t *testing.T) {
			got := RecommendPersonal(example.note, candidates, example.history)
			if got.CategoryID != example.wantID || got.Source != example.source || got.Supporting != example.supporting || got.Samples != example.samples {
				t.Fatalf("unexpected hint: %+v", got)
			}
		})
	}
}

func TestPersonalHintRespectsUnicodeWordBoundaries(t *testing.T) {
	candidates := []Category{{ID: 1, Name: "খাবার"}}
	if got := RecommendPersonal("আজকের খাবার", candidates, nil); got.CategoryID != 1 {
		t.Fatalf("missed category: %+v", got)
	}
	if got := RecommendPersonal("খাবারের খরচ", candidates, nil); got.CategoryID != 0 {
		t.Fatalf("matched part of another word: %+v", got)
	}
	if got := RecommendPersonal("Food店", []Category{{ID: 1, Name: "Food"}}, nil); got.CategoryID != 0 {
		t.Fatalf("non-ASCII letters were treated as punctuation: %+v", got)
	}
}
