package proj

import (
	"emergion-sovereign-runtime/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTMLRendersRegistryMetadata(t *testing.T) {
	e := core.EmergION{
		IDN: "E-1", STA: core.StateAccepted,
		MEM: core.Memory{Summary: "Useful synthesis"},
		REL: map[string]string{"source_name": "idea.md"},
		VAL: core.Validation{Risk: "L", Reasoner: "gemma"},
		EVO: core.Evolution{Metadata: &core.Metadata{
			Topology: core.TopologyDodecahedronV1, CapturedAt: time.Now(), PromptSchema: "EMERGER_LOGICAL_V2",
			LogicalPrompts: []string{"Who pays?"}, Facets: []core.Facet{core.FacetFIELDCommand},
			Monetization: &core.Monetization{Model: "subscription", Customer: "operators", Value: "faster decisions", NextAction: "interview five operators", Confidence: "MEDIUM"},
		}},
	}
	st := core.EmptyState()
	st.Accepted[e.IDN] = e
	path := filepath.Join(t.TempDir(), "field.html")
	if err := HTML(path, st); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"Useful synthesis", "subscription", "Who pays?", "Dodecahedron"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q", want)
		}
	}
}
