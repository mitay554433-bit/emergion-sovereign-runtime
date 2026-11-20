package proj

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func sourceEm(id, topic, related string) core.EmergION {
	em := core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{
		"source_kind":  "SOURCE_UPLOAD",
		"source_title": "T " + id,
	}}
	if topic != "" {
		em.REL["source_topic"] = topic
	}
	if related != "" {
		em.REL["source_related"] = related
	}
	return em
}

func topicState() core.State {
	st := core.EmptyState()
	st.Accepted["E-a1"] = linkedEm("E-a1", "", "", core.FacetGrantFunding)
	st.Accepted["E-b1"] = linkedEm("E-b1", "", "")
	st.Accepted["E-s1"] = sourceEm("E-s1", "inventions-ip", "TR-078,TR-079")
	st.Accepted["E-s2"] = sourceEm("E-s2", "inventions-ip", "TR-078")
	st.Accepted["E-s3"] = sourceEm("E-s3", "grants-funding", "TR-012")
	st.Accepted["E-s4"] = sourceEm("E-s4", "", "")           // no topic: genuinely unassigned
	st.Accepted["E-s5"] = sourceEm("E-s5", "bad topic!", "") // unsafe topic: unassigned
	// A SOURCE record that already has a REG facet keeps it and gets no derived facet.
	s6 := sourceEm("E-s6", "investor", "")
	s6.EVO.Metadata = &core.Metadata{Facets: []core.Facet{core.FacetPatentIP}}
	st.Accepted["E-s6"] = s6
	// A non-SOURCE record with a stray source_topic is not grouped.
	b2 := linkedEm("E-b2", "", "")
	b2.REL["source_topic"] = "investor"
	st.Accepted["E-b2"] = b2
	// AtGOV SOURCE records are never grouped as accepted.
	g := sourceEm("E-g1", "data", "")
	g.STA = core.StateAtGOV
	st.AtGOV["E-g1"] = g
	return st
}

func TestLinkedDerivesSourceTopicFacets(t *testing.T) {
	st := topicState()
	lp := Linked(st)

	got := map[string][]string{}
	for _, g := range lp.DerivedFacetGroups {
		if g.FacetSource != "source_topic" || g.Facet != "SOURCE_TOPIC:"+g.Topic {
			t.Fatalf("group not marked derived: %+v", g)
		}
		got[g.Topic] = g.Members
	}
	want := map[string][]string{"inventions-ip": {"E-s1", "E-s2"}, "grants-funding": {"E-s3"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("derived groups = %v", got)
	}
	if lp.DerivedFacetGroups[0].Topic != "inventions-ip" || lp.DerivedFacetGroups[0].Label != "Inventions & IP" {
		t.Fatalf("order/label = %+v", lp.DerivedFacetGroups[0])
	}
	if !reflect.DeepEqual(lp.Unassigned, []string{"E-b1", "E-b2", "E-s4", "E-s5"}) {
		t.Fatalf("unassigned = %v", lp.Unassigned)
	}
	if !reflect.DeepEqual(lp.RegUnassigned, []string{"E-b1", "E-b2", "E-s1", "E-s2", "E-s3", "E-s4", "E-s5"}) {
		t.Fatalf("reg_unassigned = %v", lp.RegUnassigned)
	}
	for _, r := range lp.Records {
		switch r.ID {
		case "E-s1", "E-s2":
			if !reflect.DeepEqual(r.DerivedFacets, []string{"SOURCE_TOPIC:inventions-ip"}) || r.FacetSource != "source_topic" || len(r.Facets) != 0 {
				t.Fatalf("record %s = %+v", r.ID, r)
			}
		case "E-s6":
			if len(r.DerivedFacets) != 0 || r.FacetSource != "" || !reflect.DeepEqual(r.Facets, []string{"PATENT_IP"}) {
				t.Fatalf("REG facet overridden: %+v", r)
			}
		case "E-a1", "E-b1", "E-b2", "E-s4", "E-s5":
			if len(r.DerivedFacets) != 0 || r.FacetSource != "" {
				t.Fatalf("record %s got derived facet: %+v", r.ID, r)
			}
		}
	}
	// REG facet groups are untouched by derivation.
	for _, g := range lp.FacetGroups {
		for _, id := range g.Members {
			if strings.HasPrefix(id, "E-s") && id != "E-s6" {
				t.Fatalf("SOURCE record %s in REG facet group %s", id, g.Facet)
			}
		}
	}
	refs := map[string][]string{}
	for _, g := range lp.SourceRefGroups {
		refs[g.Ref] = g.Members
	}
	if !reflect.DeepEqual(refs, map[string][]string{"TR-078": {"E-s1", "E-s2"}}) {
		t.Fatalf("source ref groups = %v", refs)
	}
	if len(st.Accepted) != 9 || st.Accepted["E-s1"].EVO.Metadata != nil {
		t.Fatal("state mutated")
	}
}

func TestLinkedNonSourceRecordsUnchanged(t *testing.T) {
	st := core.EmptyState()
	st.Accepted["E-a1"] = linkedEm("E-a1", "", "", core.FacetGrantFunding)
	st.Accepted["E-b1"] = linkedEm("E-b1", "", "")
	before := Linked(st)
	st.Accepted["E-s1"] = sourceEm("E-s1", "data", "")
	after := Linked(st)
	if !reflect.DeepEqual(before.FacetGroups, after.FacetGroups) || !reflect.DeepEqual(before.Unassigned, after.Unassigned) {
		t.Fatalf("non-SOURCE grouping changed: %v -> %v", before.Unassigned, after.Unassigned)
	}
	for i, r := range before.Records {
		if !reflect.DeepEqual(r, after.Records[i]) {
			t.Fatalf("record changed: %+v -> %+v", r, after.Records[i])
		}
	}
}

func TestLinkedSourceTopicJSONIsAdditive(t *testing.T) {
	b, err := json.Marshal(Linked(topicState()))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"authority", "tip_hash", "events", "facet_order", "facet_groups", "unassigned", "records", "assemblies", "saws", "realizations", "human_final_queue", "trajectories", "archonym_conflicts", "archonym_waiting_conflicts", "derived_facet_groups", "reg_unassigned", "source_ref_groups"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %s", k)
		}
	}
	var order []string
	_ = json.Unmarshal(m["facet_order"], &order)
	for _, f := range order {
		if strings.HasPrefix(f, "SOURCE_TOPIC:") {
			t.Fatal("derived facet leaked into canonical facet_order")
		}
	}
	// A record without derivation serializes exactly as before (no new keys).
	b2, _ := json.Marshal(LinkedRecord{ID: "E-x"})
	if string(b2) != `{"id":"E-x"}` {
		t.Fatalf("record wire format changed: %s", b2)
	}
}

func TestSourceTopicLabels(t *testing.T) {
	for in, want := range map[string]string{"inventions-ip": "Inventions & IP", "math-formulas": "Math & Formulas", "other": "Other Sources", "lab-notes": "Lab Notes"} {
		if got := SourceTopicLabel(in); got != want {
			t.Fatalf("%s -> %q", in, got)
		}
	}
}
