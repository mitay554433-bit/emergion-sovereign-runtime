package runtime

import (
	"reflect"
	"testing"
)

func TestRealizationSelectionSelectsUniqueStrongestAndRetainsRunnersUp(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B", "E-C"},
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

	got, ok, err := SelectStrongestRealization(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("unique strongest realization was not selected")
	}
	if got.TargetThing != "KNIFE" || got.Selected != "E-A" {
		t.Fatalf("selection = %#v", got)
	}
	if want := []string{"E-B", "E-C"}; !reflect.DeepEqual(got.RunnersUp, want) {
		t.Fatalf("runners-up = %#v want %#v", got.RunnersUp, want)
	}
}

func TestRealizationSelectionRefusesMultipleStrongest(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B", "E-C"},
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

	got, ok, err := SelectStrongestRealization(p)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("ambiguous strongest front unexpectedly selected %#v", got)
	}
	if got.Selected != "" {
		t.Fatalf("selected = %q want empty", got.Selected)
	}
}

func TestRealizationSelectionEquivalentDoesNotInventWinner(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B"},
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

	got, ok, err := SelectStrongestRealization(p)
	if err != nil {
		t.Fatal(err)
	}
	if ok || got.Selected != "" {
		t.Fatalf("equivalent realizations unexpectedly selected %#v", got)
	}
}

func TestRealizationSelectionPropagatesPreferenceCycle(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B", "E-C"},
		Comparisons: []RealizationComparison{
			{TargetThing: "KNIFE", Left: "E-A", Right: "E-B", Result: RealizationLeftPreferred, Differences: []string{"A_BETTER_THAN_B"}},
			{TargetThing: "KNIFE", Left: "E-B", Right: "E-C", Result: RealizationLeftPreferred, Differences: []string{"B_BETTER_THAN_C"}},
			{TargetThing: "KNIFE", Left: "E-C", Right: "E-A", Result: RealizationLeftPreferred, Differences: []string{"C_BETTER_THAN_A"}},
		},
	}

	if _, ok, err := SelectStrongestRealization(p); err == nil || ok {
		t.Fatalf("cycle selection = ok:%v err:%v", ok, err)
	}
}
