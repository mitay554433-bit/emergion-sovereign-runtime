package reason

import (
	"context"
	"fmt"
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

	summary := text
	if len(summary) > 240 {
		summary = summary[:240]
	}

	relationships := map[string]string{
		"source_name": in.Name,
	}
	facts := []string{"source_preserved"}
	gaps := []string{"ai_reasoning_not_used"}

	if strings.TrimSpace(in.GovernedState) != "" {
		relationships["governed_state"] = "accepted_context_present"
	}

	return Result{
		Summary:       summary,
		Relationships: relationships,
		Capabilities:  []string{"OBS", "CMP", "RLT", "VLD"},
		Facts:         facts,
		Gaps:          gaps,
		Risk:          "M",
	}, nil
}
