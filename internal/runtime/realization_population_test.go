package runtime

import (
	"reflect"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func realizationPopulationState() core.State {
	st := core.EmptyState()

	for _, id := range []string{
		"E-KNIFE-A",
		"E-KNIFE-B",
		"E-KNIFE-C",
	} {
		st.Accepted[id] = core.EmergION{
			IDN: id,
			STA: core.StateAccepted,
			CAP: []string{"OBS"},
		}
	}

	return st
}

func TestRealizationPopulationAcceptsSameThingAcceptedPopulation(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-A",
			"E-KNIFE-B",
			"E-KNIFE-C",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-KNIFE-A",
				Right:       "E-KNIFE-B",
				Result:      RealizationLeftPreferred,
				Differences: []string{
					"A_CLOSES_KNIFE_DIFFERENCE_BETTER_THAN_B",
				},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-KNIFE-B",
				Right:       "E-KNIFE-C",
				Result:      RealizationUnresolved,
			},
		},
	}

	if err := p.Validate(realizationPopulationState()); err != nil {
		t.Fatal(err)
	}
}

func TestRealizationPopulationRejectsNonAcceptedMember(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-A",
			"E-NOT-ACCEPTED",
		},
	}

	if err := p.Validate(realizationPopulationState()); err == nil {
		t.Fatal("non-REG-accepted realization entered population")
	}
}

func TestRealizationPopulationRejectsDuplicateMember(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-A",
			"E-KNIFE-A",
		},
	}

	if err := p.Validate(realizationPopulationState()); err == nil {
		t.Fatal("duplicate realization entered population")
	}
}

func TestRealizationPopulationRejectsForeignThingComparison(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-A",
			"E-KNIFE-B",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "COFFEE",
				Left:        "E-KNIFE-A",
				Right:       "E-KNIFE-B",
				Result:      RealizationUnresolved,
			},
		},
	}

	if err := p.Validate(realizationPopulationState()); err == nil {
		t.Fatal("foreign target thing comparison entered population")
	}
}

func TestRealizationPopulationRejectsComparisonOutsidePopulation(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-A",
			"E-KNIFE-B",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-KNIFE-A",
				Right:       "E-KNIFE-C",
				Result:      RealizationUnresolved,
			},
		},
	}

	if err := p.Validate(realizationPopulationState()); err == nil {
		t.Fatal("outside realization entered population comparison")
	}
}

func TestRealizationPopulationMemberOrderIsDeterministicNotFitness(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-KNIFE-C",
			"E-KNIFE-A",
			"E-KNIFE-B",
		},
	}

	got := p.OrderedMembers()
	want := []string{
		"E-KNIFE-A",
		"E-KNIFE-B",
		"E-KNIFE-C",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("members = %#v want %#v", got, want)
	}
}
