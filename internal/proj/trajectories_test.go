package proj

import (
	"fmt"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

// The real flood: every first provider fans out into CMP x RLT paths, each
// recorded by a different run (different origin). Each AtGOV record must sit
// in exactly one trajectory, keyed by the first ID of capability_providers.
func TestTrajectoriesMembersDisjointAndCountsSumToDistinctMembers(t *testing.T) {
	st := core.EmptyState()
	var events []core.Event
	base := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	provider := 0
	firsts := []string{"E-A000000000000001", "E-A000000000000002", "E-A000000000000003", "E-C000000000000001"}
	for fi, first := range firsts {
		for c := 1; c <= 7; c++ {
			for r := 1; r <= 6; r++ {
				cmp := fmt.Sprintf("E-C%015X", c)
				if cmp == first {
					continue // self-adjacent path is never emitted
				}
				rlt := fmt.Sprintf("E-D%015X", r)
				id := fmt.Sprintf("E-F%015X", provider)
				provider++
				em := core.EmergION{IDN: id, STA: core.StateAtGOV, REL: map[string]string{
					"source_name":                       "provider_population",
					"required_capability":               "DERIVE_CAPABILITY",
					"capability_resolution":             "COMPOSABLE_CANDIDATE",
					"capability_composition":            "ANALYZE+CMP+RLT",
					"capability_providers":              "ANALYZE:" + first + ",CMP:" + cmp + ",RLT:" + rlt,
					"capability_provider_edge_proposal": first + "->" + cmp + "," + cmp + "->" + rlt,
					"origin":                            fmt.Sprintf("E-0%015X", provider%5+fi),
				}}
				st.AtGOV[id] = em
				e := em
				events = append(events, core.Event{Type: "C", At: base.Add(time.Duration(provider) * time.Second), EmergION: &e})
			}
		}
	}
	for i := 0; i < 9; i++ {
		id := fmt.Sprintf("E-B%015X", i)
		st.AtGOV[id] = core.EmergION{IDN: id, STA: core.StateAtGOV, REL: map[string]string{"source_name": "governed_target_comparison"}}
	}

	trs := Trajectories(st, events)
	if len(trs) != len(firsts) {
		t.Fatalf("trajectories = %d want %d (one per first provider)", len(trs), len(firsts))
	}
	seen := map[string]string{}
	sum := 0
	for _, tr := range trs {
		sum += tr.Count
		if tr.Count != len(tr.Members) {
			t.Fatalf("%s count %d != members %d", tr.Key, tr.Count, len(tr.Members))
		}
		for _, id := range tr.Members {
			if prev, dup := seen[id]; dup {
				t.Fatalf("%s is in trajectory %s and %s", id, prev, tr.Key)
			}
			seen[id] = tr.Key
			em, ok := st.AtGOV[id]
			if !ok || TrajectoryKey(em) != tr.Key {
				t.Fatalf("member %s key %q in trajectory %q", id, TrajectoryKey(em), tr.Key)
			}
			if FirstCapabilityProvider(em.REL["capability_providers"]) != tr.FirstProvider {
				t.Fatalf("member %s first provider differs from %s", id, tr.FirstProvider)
			}
		}
		if _, ok := seen[tr.NewestID]; !ok || seen[tr.NewestID] != tr.Key {
			t.Fatalf("newest %s not a member of %s", tr.NewestID, tr.Key)
		}
	}
	if sum != len(seen) || sum != provider {
		t.Fatalf("sum of counts %d, distinct members %d, provider records %d", sum, len(seen), provider)
	}
	if trs[0].Count != 42 || trs[len(trs)-1].Count != 36 {
		t.Fatalf("counts = %d..%d want 42..36 (self-adjacent paths skipped)", trs[0].Count, trs[len(trs)-1].Count)
	}
}
