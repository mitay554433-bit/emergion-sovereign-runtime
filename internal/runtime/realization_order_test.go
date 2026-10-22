package runtime

import (
	"reflect"
	"testing"
)

func TestRealizationOrderDerivesEvidenceSupportedChain(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-A",
			"E-B",
			"E-C",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationLeftPreferred,
				Differences: []string{"A_BETTER_THAN_B"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-B",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"B_BETTER_THAN_C"},
			},
		},
	}

	got, err := DeriveRealizationOrder(p)
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"E-A"},
		{"E-B"},
		{"E-C"},
	}

	if !reflect.DeepEqual(got.Fronts, want) {
		t.Fatalf("fronts = %#v want %#v", got.Fronts, want)
	}
}

func TestRealizationOrderPreservesMultipleStrongestCandidates(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-A",
			"E-B",
			"E-C",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"A_BETTER_THAN_C"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-B",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"B_BETTER_THAN_C"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationUnresolved,
			},
		},
	}

	got, err := DeriveRealizationOrder(p)
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"E-A", "E-B"},
		{"E-C"},
	}

	if !reflect.DeepEqual(got.Fronts, want) {
		t.Fatalf("fronts = %#v want %#v", got.Fronts, want)
	}

	if !reflect.DeepEqual(
		got.Strongest(),
		[]string{"E-A", "E-B"},
	) {
		t.Fatalf("strongest = %#v", got.Strongest())
	}
}

func TestRealizationOrderDoesNotPromoteUnresolvedComparison(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-A",
			"E-B",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationUnresolved,
			},
		},
	}

	got, err := DeriveRealizationOrder(p)
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"E-A", "E-B"},
	}

	if !reflect.DeepEqual(got.Fronts, want) {
		t.Fatalf("fronts = %#v want %#v", got.Fronts, want)
	}
}

func TestRealizationOrderRejectsPreferenceCycle(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-A",
			"E-B",
			"E-C",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationLeftPreferred,
				Differences: []string{"A_BETTER_THAN_B"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-B",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"B_BETTER_THAN_C"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-C",
				Right:       "E-A",
				Result:      RealizationLeftPreferred,
				Differences: []string{"C_BETTER_THAN_A"},
			},
		},
	}

	if _, err := DeriveRealizationOrder(p); err == nil {
		t.Fatal("cyclic preference unexpectedly produced ordering")
	}
}

func TestRealizationOrderEquivalentDoesNotInventWinner(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{
			"E-A",
			"E-B",
		},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationEquivalent,
				Differences: []string{"A_AND_B_EQUIVALENT_FOR_TARGET"},
			},
		},
	}

	got, err := DeriveRealizationOrder(p)
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"E-A", "E-B"},
	}

	if !reflect.DeepEqual(got.Fronts, want) {
		t.Fatalf("fronts = %#v want %#v", got.Fronts, want)
	}
}
