package runtime

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func realizationComparisonState() core.State {
	st := core.EmptyState()

	st.Accepted["E-LEFT"] = core.EmergION{
		IDN: "E-LEFT",
		STA: core.StateAccepted,
		CAP: []string{"OBS"},
	}

	st.Accepted["E-RIGHT"] = core.EmergION{
		IDN: "E-RIGHT",
		STA: core.StateAccepted,
		CAP: []string{"OBS"},
	}

	return st
}

func TestRealizationComparisonAllowsUnresolvedWithoutInventedPreference(t *testing.T) {
	c := RealizationComparison{
		TargetThing: "KNIFE",
		Left:        "E-LEFT",
		Right:       "E-RIGHT",
		Result:      RealizationUnresolved,
	}

	if err := c.Validate(realizationComparisonState()); err != nil {
		t.Fatal(err)
	}
}

func TestRealizationComparisonResolvedRequiresDifferenceEvidence(t *testing.T) {
	c := RealizationComparison{
		TargetThing: "KNIFE",
		Left:        "E-LEFT",
		Right:       "E-RIGHT",
		Result:      RealizationLeftPreferred,
	}

	if err := c.Validate(realizationComparisonState()); err == nil {
		t.Fatal("resolved comparison admitted without Difference evidence")
	}
}

func TestRealizationComparisonAcceptsEvidenceBoundPreference(t *testing.T) {
	c := RealizationComparison{
		TargetThing: "KNIFE",
		Left:        "E-LEFT",
		Right:       "E-RIGHT",
		Result:      RealizationLeftPreferred,
		Differences: []string{
			"LEFT_CLOSES_TARGET_DIFFERENCE_BETTER_THAN_RIGHT",
		},
	}

	if err := c.Validate(realizationComparisonState()); err != nil {
		t.Fatal(err)
	}
}

func TestRealizationComparisonRejectsNonAcceptedRealization(t *testing.T) {
	c := RealizationComparison{
		TargetThing: "KNIFE",
		Left:        "E-LEFT",
		Right:       "E-NOT-ACCEPTED",
		Result:      RealizationUnresolved,
	}

	if err := c.Validate(realizationComparisonState()); err == nil {
		t.Fatal("non-REG-accepted realization entered comparison")
	}
}

func TestRealizationComparisonRejectsSelfComparison(t *testing.T) {
	c := RealizationComparison{
		TargetThing: "KNIFE",
		Left:        "E-LEFT",
		Right:       "E-LEFT",
		Result:      RealizationUnresolved,
	}

	if err := c.Validate(realizationComparisonState()); err == nil {
		t.Fatal("self comparison unexpectedly valid")
	}
}
