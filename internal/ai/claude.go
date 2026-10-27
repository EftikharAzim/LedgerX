package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeCategorizer classifies a transaction into one of the user's existing
// categories using Claude via forced tool use, so the response is always a
// structured choice rather than prose. It never invents a category: the
// result is validated against the candidate set before being returned.
type ClaudeCategorizer struct {
	client anthropic.Client
	model  anthropic.Model
}

// NewClaudeCategorizer builds a categorizer. model may be empty, in which case
// a sensible default for this classification workload is used.
func NewClaudeCategorizer(apiKey, model string) *ClaudeCategorizer {
	m := anthropic.Model(model)
	if model == "" {
		m = anthropic.ModelClaudeHaiku4_5 // cheap, fast: right tier for classification
	}
	return &ClaudeCategorizer{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  m,
	}
}

// Name identifies the Claude backend and configured model.
func (c *ClaudeCategorizer) Name() string { return "claude:" + string(c.model) }

const categorizeSystem = `You categorize a single financial transaction into exactly one of the user's existing categories.
Rules:
- Choose the category_id that best fits the transaction's note and amount sign (positive = income, negative = expense).
- You MUST choose from the provided candidate ids only. If none reasonably fit, return category_id 0.
- confidence is your calibrated probability (0..1) that the choice is correct. Use a low value when unsure.
Always answer by calling the "categorize" tool.`

// Categorize asks Claude for a category and validates the returned candidate ID.
func (c *ClaudeCategorizer) Categorize(ctx context.Context, in CategorizeInput) (CategorizeResult, error) {
	if len(in.Candidates) == 0 || strings.TrimSpace(in.Note) == "" {
		return CategorizeResult{}, nil // nothing to match against
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Transaction note: %q\n", in.Note)
	fmt.Fprintf(&b, "Amount (minor units, signed): %d %s\n\n", in.AmountMinor, in.Currency)
	b.WriteString("Candidate categories (id: name):\n")
	for _, cat := range in.Candidates {
		fmt.Fprintf(&b, "%d: %s\n", cat.ID, cat.Name)
	}

	tool := anthropic.ToolParam{
		Name:        "categorize",
		Description: anthropic.String("Record the chosen category for the transaction."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"category_id": map[string]any{
					"type":        "integer",
					"description": "Chosen candidate id, or 0 if none fit.",
				},
				"confidence": map[string]any{
					"type":        "number",
					"description": "Confidence between 0 and 1.",
				},
			},
			Required: []string{"category_id", "confidence"},
		},
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:      c.model,
		MaxTokens:  256,
		System:     []anthropic.TextBlockParam{{Text: categorizeSystem}},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceParamOfTool("categorize"),
		Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(b.String()))},
	})
	if err != nil {
		return CategorizeResult{}, fmt.Errorf("claude categorize: %w", err)
	}

	for _, block := range resp.Content {
		tu, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}
		var out struct {
			CategoryID int64   `json:"category_id"`
			Confidence float64 `json:"confidence"`
		}
		if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &out); err != nil {
			return CategorizeResult{}, fmt.Errorf("parse tool input: %w", err)
		}
		// Never trust the model to name a category outside the candidate set.
		if out.CategoryID != 0 && !validCandidate(out.CategoryID, in.Candidates) {
			return CategorizeResult{}, nil
		}
		if out.Confidence < 0 {
			out.Confidence = 0
		} else if out.Confidence > 1 {
			out.Confidence = 1
		}
		return CategorizeResult{CategoryID: out.CategoryID, Confidence: out.Confidence}, nil
	}
	return CategorizeResult{}, nil // model returned no tool call
}
