package ai

import "os"

// FromEnv selects a Categorizer from the environment:
//
//	CATEGORIZER=claude  + ANTHROPIC_API_KEY set  -> ClaudeCategorizer
//	                       (CATEGORIZER_MODEL overrides the default model)
//	CATEGORIZER=keyword                          -> KeywordCategorizer
//	(unset)             + ANTHROPIC_API_KEY set  -> ClaudeCategorizer
//	(unset)             + no key                 -> KeywordCategorizer
//
// The keyword fallback means the app runs and the feature degrades gracefully
// with no API key — useful for local dev, offline, and tests.
func FromEnv() Categorizer {
	mode := os.Getenv("CATEGORIZER")
	key := os.Getenv("ANTHROPIC_API_KEY")

	switch mode {
	case "keyword":
		return KeywordCategorizer{}
	case "claude":
		if key != "" {
			return NewClaudeCategorizer(key, os.Getenv("CATEGORIZER_MODEL"))
		}
		return KeywordCategorizer{}
	default:
		if key != "" {
			return NewClaudeCategorizer(key, os.Getenv("CATEGORIZER_MODEL"))
		}
		return KeywordCategorizer{}
	}
}
