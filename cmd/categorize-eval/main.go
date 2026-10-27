// Command categorize-eval scores the configured Categorizer against a labelled
// golden set and prints an accuracy report. This is the evals harness that
// makes the AI feature engineerable: run it before/after a prompt or model
// change to catch regressions.
//
// Usage:
//
//	go run ./cmd/categorize-eval                         # default golden set, env-selected categorizer
//	go run ./cmd/categorize-eval -data path/to/set.jsonl
//	CATEGORIZER=keyword go run ./cmd/categorize-eval     # force the deterministic baseline
//	ANTHROPIC_API_KEY=... go run ./cmd/categorize-eval   # score the Claude categorizer
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	data := flag.String("data", "internal/ai/testdata/categorize_golden.jsonl", "path to JSONL golden set")
	verbose := flag.Bool("v", false, "print each mismatch")
	flag.Parse()

	f, err := os.Open(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open golden set:", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	cases, err := ai.LoadEvalCases(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse golden set:", err)
		os.Exit(1)
	}

	c := ai.FromEnv()
	res, err := ai.Evaluate(context.Background(), c, cases)
	if err != nil {
		fmt.Fprintln(os.Stderr, "evaluate:", err)
		os.Exit(1)
	}

	fmt.Printf("categorizer: %s\n", c.Name())
	fmt.Printf("accuracy:    %d/%d  (%.1f%%)\n", res.Correct, res.Total, res.Accuracy()*100)
	if *verbose && len(res.Mismatches) > 0 {
		fmt.Println("\nmismatches:")
		for _, m := range res.Mismatches {
			fmt.Printf("  want=%d got=%d  %q\n", m.WantID, m.GotID, m.Note)
		}
	}
}
