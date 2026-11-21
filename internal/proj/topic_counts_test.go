package proj

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

// Topic tiles count every REG-accepted record in the derived topic group, and
// source_uploads counts the SOURCE_UPLOAD records grouped by their recorded
// topic. AtGOV uploads and records with a REG facet are not counted.
func TestDerivedTopicGroupsCountAcceptedUploads(t *testing.T) {
	st := core.EmptyState()
	up := func(id, topic string) core.EmergION {
		return core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{"source_kind": "SOURCE_UPLOAD", "source_topic": topic, "source_title": "t " + id}}
	}
	for _, id := range []string{"E-a1", "E-a2", "E-a3"} {
		st.Accepted[id] = up(id, "inventions-ip")
	}
	for _, id := range []string{"E-m1", "E-m2"} {
		st.Accepted[id] = up(id, "math")
	}
	st.Accepted["E-k1"] = core.EmergION{IDN: "E-k1", STA: core.StateAccepted, REL: map[string]string{"source_kind": "PROGRAM"}}
	faceted := up("E-f1", "inventions-ip")
	faceted.EVO.Metadata = &core.Metadata{Archonym: "A", Facets: []core.Facet{core.FacetPatentIP}}
	st.Accepted["E-f1"] = faceted
	waiting := up("E-w1", "inventions-ip")
	waiting.STA = core.StateAtGOV
	st.AtGOV["E-w1"] = waiting

	got := map[string][2]int{}
	for _, g := range Linked(st).DerivedFacetGroups {
		got[g.Topic] = [2]int{len(g.Members), g.SourceUploads}
	}
	if got["inventions-ip"] != [2]int{3, 3} || got["math"] != [2]int{2, 2} || got["runtime-code-docs"] != [2]int{1, 0} {
		t.Fatalf("topic counts %v", got)
	}
}
