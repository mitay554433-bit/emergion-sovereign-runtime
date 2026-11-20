package proj

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func linkedEm(id, supersedes, comp string, facets ...core.Facet) core.EmergION {
	em := core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{}, CAP: []string{"cap-" + id}}
	if comp != "" {
		em.REL["COMPOSITION_KIN"] = comp
	}
	em.EVO.Supersedes = supersedes
	em.EVO.Metadata = &core.Metadata{Archonym: "A " + id, Facets: facets}
	return em
}

func TestLinkedUsesEngineGroupsKinRootsAndAssemblies(t *testing.T) {
	st := core.EmptyState()
	st.Accepted["E-a1"] = linkedEm("E-a1", "", "", core.FacetGrantFunding)
	st.Accepted["E-a2"] = linkedEm("E-a2", "E-a1", "E-a1", core.FacetGrantFunding, core.FacetPatentIP)
	st.Accepted["E-b1"] = linkedEm("E-b1", "", "")
	st.AtGOV["E-g1"] = core.EmergION{IDN: "E-g1", STA: core.StateAtGOV, MEM: core.Memory{Summary: "pending"}}

	lp := Linked(st)
	if len(lp.FacetGroups) != 12 || lp.FacetGroups[7].Facet != "GRANT_FUNDING" || len(lp.FacetGroups[7].Members) != 2 {
		t.Fatalf("facet groups = %+v", lp.FacetGroups)
	}
	if len(lp.Unassigned) != 0 {
		t.Fatalf("unassigned = %v, want empty", lp.Unassigned)
	}
	foundOther := false
	for _, g := range lp.DerivedFacetGroups {
		if g.Topic == "other" && len(g.Members) == 1 && g.Members[0] == "E-b1" && g.Label == "Other" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatalf("E-b1 not in derived Other: %+v", lp.DerivedFacetGroups)
	}
	roots := map[string]string{}
	for _, r := range lp.Records {
		roots[r.ID] = r.KinRoot
	}
	if roots["E-a2"] != "E-a1" || roots["E-a1"] != "E-a1" || len(lp.Records) != 3 {
		t.Fatalf("kin roots = %v", roots)
	}
	if lp.AssembliesError != "" || len(lp.Assemblies) != 1 {
		t.Fatalf("assemblies = %+v err %q", lp.Assemblies, lp.AssembliesError)
	}
	a := lp.Assemblies[0]
	if a.ID != "SAAB:E-a1+E-a2" || len(a.Links) != 1 || a.Links[0].Kind != "COMPOSITION_KIN" || a.CPSL == "" {
		t.Fatalf("assembly = %+v", a)
	}
	if len(lp.SAWs) != 1 || len(lp.HumanFinalQueue) != 1 || lp.HumanFinalQueue[0].ID != "E-g1" {
		t.Fatalf("saws %v queue %v", lp.SAWs, lp.HumanFinalQueue)
	}
	// Every IDN appears exactly once in records, and once per facet group.
	seenRec := map[string]bool{}
	for _, r := range lp.Records {
		if seenRec[r.ID] {
			t.Fatalf("duplicate record IDN %s", r.ID)
		}
		seenRec[r.ID] = true
	}
	for _, g := range lp.FacetGroups {
		seen := map[string]bool{}
		for _, id := range g.Members {
			if seen[id] {
				t.Fatalf("duplicate IDN %s in facet group %s", id, g.Facet)
			}
			seen[id] = true
		}
	}
	// Linked never mutates state.
	if len(st.Accepted) != 3 || len(st.AtGOV) != 1 {
		t.Fatal("state mutated")
	}
}
