package proj

import (
	"reflect"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func bareEm(id string) core.EmergION {
	return core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{}}
}

func TestDerivedDisplayGroupPriority(t *testing.T) {
	// SOURCE_UPLOAD topic wins over a keyword in the title.
	s := sourceEm("E-s", "investor", "")
	s.REL["source_title"] = "CIPO patent filing"
	topic, facet, src := derivedDisplayGroup(s, nil)
	if topic != "investor" || facet != "SOURCE_TOPIC:investor" || src != FacetSourceTopic {
		t.Fatalf("source topic lost: %s %s %s", topic, facet, src)
	}

	// A REG facet is never given a derived group.
	reg := bareEm("E-r")
	reg.MEM.Summary = "CIPO patent filing"
	if topic, facet, src = derivedDisplayGroup(reg, []core.Facet{core.FacetPatentIP}); topic != "" || facet != "" || src != "" {
		t.Fatalf("REG facet got derived group: %s %s %s", topic, facet, src)
	}

	// Keywords beat kind: a PROGRAM named as a patent is inventions-ip.
	p := bareEm("E-p")
	p.REL["source_kind"] = "PROGRAM"
	p.EVO.Metadata = &core.Metadata{Archonym: "CIPO patent filing"}
	topic, facet, src = derivedDisplayGroup(p, nil)
	if topic != "inventions-ip" || src != FacetSourceKeyword || facet != "SOURCE_TOPIC:inventions-ip" {
		t.Fatalf("keyword should win over kind: %s %s %s", topic, facet, src)
	}

	// Kind is used when title-like fields have nothing to go on.
	k := bareEm("E-k")
	k.REL["source_kind"] = "PROGRAM"
	topic, facet, src = derivedDisplayGroup(k, nil)
	if topic != "runtime-code-docs" || src != FacetSourceKind || facet != "SOURCE_TOPIC:runtime-code-docs" {
		t.Fatalf("kind mapping: %s %s %s", topic, facet, src)
	}

	// Nothing to go on -> Other, not left ungrouped.
	o := bareEm("E-o")
	topic, facet, src = derivedDisplayGroup(o, nil)
	if topic != "other" || src != FacetSourceFallback || facet != "SOURCE_TOPIC:other" {
		t.Fatalf("fallback: %s %s %s", topic, facet, src)
	}
}

func TestDerivedKeywordAndKindTables(t *testing.T) {
	cases := []struct {
		name  string
		setup func(core.EmergION) core.EmergION
		topic string
		src   string
	}{
		{"archonym patent", func(em core.EmergION) core.EmergION {
			em.EVO.Metadata = &core.Metadata{Archonym: "HarvestSkin voluntary amendment"}
			return em
		}, "inventions-ip", FacetSourceKeyword},
		{"summary mitacs", func(em core.EmergION) core.EmergION {
			em.MEM.Summary = "Mitacs nanocomposite proposal"
			return em
		}, "grants-funding", FacetSourceKeyword},
		{"title investor deck", func(em core.EmergION) core.EmergION {
			em.REL["source_title"] = "Investor deck one-page overview"
			return em
		}, "investor", FacetSourceKeyword},
		{"target thing", func(em core.EmergION) core.EmergION {
			em.REL["source_kind"] = "TARGET"
			em.REL["target_thing"] = "term sheet for incorporation"
			return em
		}, "business-legal", FacetSourceKeyword},
		{"formula summary", func(em core.EmergION) core.EmergION {
			em.MEM.Summary = "field equation simulation"
			return em
		}, "math-formulas", FacetSourceKeyword},
		{"living universe title", func(em core.EmergION) core.EmergION {
			em.REL["source_title"] = "Living Universe view"
			return em
		}, "design-visuals", FacetSourceKeyword},
		{"fieldd summary", func(em core.EmergION) core.EmergION {
			em.MEM.Summary = "fieldd watchdog notes"
			return em
		}, "runtime-code-docs", FacetSourceKeyword},
		{"zip path", func(em core.EmergION) core.EmergION {
			em.REL["source_path"] = "bundles/old-pack.zip"
			return em
		}, "archives", FacetSourceKeyword},
		{"csv path", func(em core.EmergION) core.EmergION {
			em.REL["source_path"] = "tables/inventory.csv"
			return em
		}, "data", FacetSourceKeyword},
		{"execution kind", func(em core.EmergION) core.EmergION {
			em.REL["source_kind"] = "EXECUTION_RESULT"
			return em
		}, "runtime-code-docs", FacetSourceKind},
		{"class alias", func(em core.EmergION) core.EmergION {
			em.REL["class"] = "PROGRAM_PATCH"
			return em
		}, "runtime-code-docs", FacetSourceKind},
		{"ambiguous SOURCE kind", func(em core.EmergION) core.EmergION {
			em.REL["source_kind"] = "SOURCE"
			return em
		}, "other", FacetSourceFallback},
		{"stray source_topic ignored", func(em core.EmergION) core.EmergION {
			em.REL["source_topic"] = "investor"
			return em
		}, "other", FacetSourceFallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			em := tc.setup(bareEm("E-x"))
			topic, _, src := derivedDisplayGroup(em, nil)
			if topic != tc.topic || src != tc.src {
				t.Fatalf("got %s/%s want %s/%s", topic, src, tc.topic, tc.src)
			}
		})
	}
}

func TestLinkedDerivesGroupsForUnassignedRecords(t *testing.T) {
	st := core.EmptyState()
	st.Accepted["E-a1"] = linkedEm("E-a1", "", "", core.FacetGrantFunding)
	patent := bareEm("E-p1")
	patent.EVO.Metadata = &core.Metadata{Archonym: "CIPO patent filing"}
	st.Accepted["E-p1"] = patent
	prog := bareEm("E-k1")
	prog.REL["source_kind"] = "PROGRAM"
	st.Accepted["E-k1"] = prog
	st.Accepted["E-o1"] = bareEm("E-o1")
	st.Accepted["E-s1"] = sourceEm("E-s1", "inventions-ip", "")
	// AtGOV is not grouped as accepted.
	g := sourceEm("E-g1", "data", "")
	g.STA = core.StateAtGOV
	st.AtGOV["E-g1"] = g

	lp := Linked(st)
	if len(lp.Unassigned) != 0 {
		t.Fatalf("unassigned = %v, want empty", lp.Unassigned)
	}
	if !reflect.DeepEqual(lp.RegUnassigned, []string{"E-k1", "E-o1", "E-p1", "E-s1"}) {
		t.Fatalf("reg_unassigned = %v", lp.RegUnassigned)
	}

	got := map[string]LinkedDerivedFacetGroup{}
	for _, g := range lp.DerivedFacetGroups {
		got[g.Topic] = g
	}
	if g := got["inventions-ip"]; !reflect.DeepEqual(g.Members, []string{"E-p1", "E-s1"}) || g.Facet != "SOURCE_TOPIC:inventions-ip" || g.Label != "Inventions & IP" || g.FacetSource != FacetSourceMixed {
		t.Fatalf("inventions-ip group = %+v", g)
	}
	if g := got["runtime-code-docs"]; !reflect.DeepEqual(g.Members, []string{"E-k1"}) || g.FacetSource != FacetSourceKind {
		t.Fatalf("runtime group = %+v", g)
	}
	if g := got["other"]; !reflect.DeepEqual(g.Members, []string{"E-o1"}) || g.Label != "Other" || g.FacetSource != FacetSourceFallback {
		t.Fatalf("other group = %+v", g)
	}

	byID := map[string]LinkedRecord{}
	for _, r := range lp.Records {
		byID[r.ID] = r
	}
	if r := byID["E-s1"]; !reflect.DeepEqual(r.DerivedFacets, []string{"SOURCE_TOPIC:inventions-ip"}) || r.FacetSource != FacetSourceTopic || len(r.Facets) != 0 {
		t.Fatalf("E-s1 = %+v", r)
	}
	if r := byID["E-p1"]; !reflect.DeepEqual(r.DerivedFacets, []string{"SOURCE_TOPIC:inventions-ip"}) || r.FacetSource != FacetSourceKeyword || len(r.Facets) != 0 {
		t.Fatalf("E-p1 = %+v", r)
	}
	if r := byID["E-k1"]; r.FacetSource != FacetSourceKind || !reflect.DeepEqual(r.DerivedFacets, []string{"SOURCE_TOPIC:runtime-code-docs"}) {
		t.Fatalf("E-k1 = %+v", r)
	}
	if r := byID["E-o1"]; r.FacetSource != FacetSourceFallback || !reflect.DeepEqual(r.DerivedFacets, []string{"SOURCE_TOPIC:other"}) {
		t.Fatalf("E-o1 = %+v", r)
	}
	if r := byID["E-a1"]; len(r.DerivedFacets) != 0 || r.FacetSource != "" || !reflect.DeepEqual(r.Facets, []string{"GRANT_FUNDING"}) {
		t.Fatalf("REG record derived: %+v", r)
	}
	for _, g := range lp.FacetGroups {
		for _, id := range g.Members {
			if id == "E-p1" || id == "E-k1" || id == "E-o1" || id == "E-s1" {
				t.Fatalf("derived member %s leaked into REG facet group %s", id, g.Facet)
			}
		}
	}
	if len(st.Accepted) != 5 || st.Accepted["E-p1"].EVO.Metadata.Facets != nil {
		t.Fatal("state mutated")
	}
}

func TestDerivedTopicGroupOrderIsDeterministic(t *testing.T) {
	by := map[string][]string{"data": {"E-b", "E-a"}, "investor": {"E-c"}}
	src := map[string]string{"E-a": FacetSourceKeyword, "E-b": FacetSourceKeyword, "E-c": FacetSourceKeyword}
	a := derivedTopicGroups(by, src)
	b := derivedTopicGroups(by, src)
	if !reflect.DeepEqual(a, b) || a[0].Topic != "data" || !reflect.DeepEqual(a[0].Members, []string{"E-a", "E-b"}) {
		t.Fatalf("groups = %+v", a)
	}
}
