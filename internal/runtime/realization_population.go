package runtime

import (
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

type RealizationPopulation struct {
	TargetThing string
	Members     []string
	Comparisons []RealizationComparison
}

func (p RealizationPopulation) Validate(st core.State) error {
	target := strings.TrimSpace(p.TargetThing)
	if target == "" {
		return fmt.Errorf("realization population requires target thing")
	}

	if len(p.Members) < 2 {
		return fmt.Errorf("realization population requires at least two members")
	}

	members := map[string]bool{}

	for _, raw := range p.Members {
		id := strings.TrimSpace(raw)
		if id == "" {
			return fmt.Errorf("realization population contains empty member")
		}

		if members[id] {
			return fmt.Errorf("duplicate realization population member %q", id)
		}

		em, ok := st.Accepted[id]
		if !ok || em.STA != core.StateAccepted {
			return fmt.Errorf("realization population member %q is not REG-accepted", id)
		}

		members[id] = true
	}

	for i, comparison := range p.Comparisons {
		if strings.TrimSpace(comparison.TargetThing) != target {
			return fmt.Errorf(
				"comparison %d target %q does not match population target %q",
				i,
				comparison.TargetThing,
				target,
			)
		}

		if !members[strings.TrimSpace(comparison.Left)] {
			return fmt.Errorf(
				"comparison %d left realization %q is outside population",
				i,
				comparison.Left,
			)
		}

		if !members[strings.TrimSpace(comparison.Right)] {
			return fmt.Errorf(
				"comparison %d right realization %q is outside population",
				i,
				comparison.Right,
			)
		}

		if err := comparison.Validate(st); err != nil {
			return fmt.Errorf("comparison %d invalid: %w", i, err)
		}
	}

	return nil
}

func (p RealizationPopulation) OrderedMembers() []string {
	out := append([]string(nil), p.Members...)
	sort.Strings(out)
	return out
}
