package runtime

import (
	"fmt"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

type RealizationComparisonResult string

const (
	RealizationLeftPreferred  RealizationComparisonResult = "LEFT_PREFERRED"
	RealizationRightPreferred RealizationComparisonResult = "RIGHT_PREFERRED"
	RealizationEquivalent     RealizationComparisonResult = "EQUIVALENT"
	RealizationUnresolved     RealizationComparisonResult = "UNRESOLVED"
)

type RealizationComparison struct {
	TargetThing string
	Left        string
	Right       string
	Result      RealizationComparisonResult
	Differences []string
}

func (c RealizationComparison) Validate(st core.State) error {
	target := strings.TrimSpace(c.TargetThing)
	left := strings.TrimSpace(c.Left)
	right := strings.TrimSpace(c.Right)

	if target == "" {
		return fmt.Errorf("realization comparison requires target thing")
	}

	if left == "" || right == "" {
		return fmt.Errorf("realization comparison requires both realizations")
	}

	if left == right {
		return fmt.Errorf("realization comparison cannot self-compare %s", left)
	}

	if _, ok := st.Accepted[left]; !ok {
		return fmt.Errorf("left realization %q is not REG-accepted", left)
	}

	if _, ok := st.Accepted[right]; !ok {
		return fmt.Errorf("right realization %q is not REG-accepted", right)
	}

	switch c.Result {
	case RealizationLeftPreferred:
	case RealizationRightPreferred:
	case RealizationEquivalent:
	case RealizationUnresolved:
	default:
		return fmt.Errorf("invalid realization comparison result %q", c.Result)
	}

	if c.Result != RealizationUnresolved && len(c.Differences) == 0 {
		return fmt.Errorf("resolved realization comparison requires Difference evidence")
	}

	return nil
}
