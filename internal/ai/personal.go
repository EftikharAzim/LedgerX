package ai

import "strings"

// PersonalRecommendation is an explainable hint, never a ledger write or a
// calibrated model-confidence estimate. History is ordered newest first.
type PersonalRecommendation struct {
	CategoryID int64
	Source     string
	Supporting int
	Samples    int
}

// RecommendPersonal uses the caller's confirmed categories before falling back
// to an unambiguous category-name match. Conflicting history abstains unless at
// least 80% supports one choice. It never invokes an external model.
func RecommendPersonal(note string, candidates []Category, history []int64) PersonalRecommendation {
	counts := make(map[int64]int)
	samples := 0
	for _, id := range history {
		if validCandidate(id, candidates) {
			counts[id]++
			samples++
		}
	}
	if samples > 0 {
		var best int64
		for id, count := range counts {
			if count > counts[best] {
				best = id
			}
		}
		if counts[best]*5 >= samples*4 {
			return PersonalRecommendation{CategoryID: best, Source: "history", Supporting: counts[best], Samples: samples}
		}
		return PersonalRecommendation{Source: "ambiguous", Samples: samples}
	}
	var match int64
	for _, candidate := range candidates {
		name := strings.ToLower(strings.TrimSpace(candidate.Name))
		if name == "" || !containsWord(strings.ToLower(note), name) {
			continue
		}
		if match != 0 && match != candidate.ID {
			return PersonalRecommendation{Source: "ambiguous"}
		}
		match = candidate.ID
	}
	if match != 0 {
		return PersonalRecommendation{CategoryID: match, Source: "keyword"}
	}
	return PersonalRecommendation{Source: "none"}
}
