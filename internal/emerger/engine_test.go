package emerger

import (
	"context"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
	"testing"
)

func TestEmerge(t *testing.T) {
	b := []byte("hello field")
	h := SourceHash(b)
	em, err := (Engine{Reasoner: reason.Heuristic{}}).Emerge(context.Background(), reason.Input{Name: "x", Content: b}, Evidence{Hash: h, Bytes: int64(len(b)), Codec: "gzip", Stored: 10})
	if err != nil {
		t.Fatal(err)
	}
	if em.STA != "G" || !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatalf("bad: %#v", em)
	}
	if em.EVO.Metadata == nil || em.EVO.Metadata.CapturedAt.IsZero() || em.EVO.Metadata.AIIntegrated {
		t.Fatalf("bad metadata: %#v", em.EVO.Metadata)
	}
	if em.EVO.Metadata.Topology != core.TopologyDodecahedronV1 {
		t.Fatalf("bad topology: %q", em.EVO.Metadata.Topology)
	}
	if len(em.EVO.Metadata.Facets) != 12 || len(em.EVO.Metadata.LogicalPrompts) != 3 || em.EVO.Metadata.Monetization == nil {
		t.Fatalf("missing visible registry metadata: %#v", em.EVO.Metadata)
	}
}
