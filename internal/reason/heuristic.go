package reason

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type Heuristic struct{}

func (Heuristic) Name() string                   { return "heuristic" }
func (Heuristic) Version(context.Context) string { return "1" }
func (Heuristic) Analyze(_ context.Context, in Input) (Result, error) {
	text := strings.TrimSpace(string(in.Content))
	if text == "" {
		return Result{}, fmt.Errorf("empty input")
	}
	name := filepath.Base(in.Name)
	kind := sourceKind(name, text)
	return Result{
		Summary:       fmt.Sprintf("%s preserved as governed evidence for synthesis, validation, registry acceptance, and FIELD integration.", kind),
		Relationships: map[string]string{"source_name": name, "source_kind": kind},
		Capabilities:  []string{"OBS", "CMP", "RLT", "VLD"},
		Facts:         []string{"source_preserved", "content_addressed", "governance_required"},
		Gaps:          []string{"local_ai_reasoning_not_used", "customer_validation_required"},
		Risk:          "M",
		Facets: []string{"FIELD_COMMAND", "EMERGENCE_CAPTURE", "PROGRAM_FORGE", "PRODUCT_STORE", "CUSTOMERS_SALES", "COMMUNICATIONS", "PAYMENTS_FINANCE", "GRANT_FUNDING", "PATENT_IP", "MA_PARTNERSHIPS", "DOCS_PROJECTION", "ANALYTICS_FORECAST"},
		LogicalPrompts: []string{"What outcome does this source enable?", "Which claims require evidence before approval?", "What is the smallest test of commercial value?"},
		Monetization: &Monetization{Model: "validation required", Customer: "unresolved", Value: "governed decision asset", RevenuePath: "validate one customer outcome", NextAction: "run local AI analysis and interview a target user", Confidence: "LOW"},
	}, nil
}

func sourceKind(name, text string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go":
		return "Go runtime source"
	case ".json":
		return "structured registry data"
	case ".html", ".htm":
		return "FIELD projection"
	case ".md":
		return "architecture documentation"
	}
	if strings.Contains(strings.ToLower(text), "package main") {
		return "runtime source"
	}
	return "source artifact"
}
