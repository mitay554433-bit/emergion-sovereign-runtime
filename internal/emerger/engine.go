package emerger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/pivot"
	"emergion-sovereign-runtime/internal/reason"
)

type Evidence struct {
	RuntimeIdentity string
	Hash            string
	Bytes           int64
	Stored          int64
	Codec           string
	Provenance      string
}

type Engine struct{ Reasoner reason.Reasoner }

func SourceHash(content []byte) string {
	s := sha256.Sum256(content)
	return hex.EncodeToString(s[:])
}

func (e Engine) Emerge(ctx context.Context, in reason.Input, ev Evidence) (core.EmergION, error) {
	if e.Reasoner == nil {
		return core.EmergION{}, fmt.Errorf("reasoner required")
	}
	if len(in.Content) == 0 {
		return core.EmergION{}, fmt.Errorf("empty source")
	}
	if ev.Hash == "" {
		ev.Hash = SourceHash(in.Content)
	}
	if ev.Bytes == 0 {
		ev.Bytes = int64(len(in.Content))
	}
	result, err := e.Reasoner.Analyze(ctx, in)
	if err != nil {
		return core.EmergION{}, err
	}
	_, err = pivot.Observe(
		"EMERGER_RECOIL",
		"REASONER_SUMMARY_CLAIM",
		"CALIBRATED_SUMMARY_OBSERVATION",
		"SUMMARY_PRESENT",
		func() error {
			if strings.TrimSpace(result.Summary) == "" {
				return fmt.Errorf("empty summary")
			}
			return nil
		},
	)
	if err != nil {
		return core.EmergION{}, err
	}
	id := strings.TrimSpace(ev.RuntimeIdentity)
	if id == "" {
		id = "E-" + strings.ToUpper(ev.Hash[:16])
	} else if id != ev.RuntimeIdentity || !strings.HasPrefix(id, "E-") {
		return core.EmergION{}, fmt.Errorf("invalid runtime EmergION identity %q", ev.RuntimeIdentity)
	}
	em := core.EmergION{
		IDN: id,
		STA: "",
		MEM: core.Memory{SourceHash: ev.Hash, Codec: ev.Codec, Bytes: ev.Bytes, Stored: ev.Stored, Summary: result.Summary, Provenance: ev.Provenance},
		REL: result.Relationships,
		CAP: result.Capabilities,
		VAL: core.Validation{Facts: result.Facts, Differences: result.Differences, Gaps: result.Gaps, Risk: result.Risk, Recoil: false, WVC: false, Reasoner: e.Reasoner.Name(), ReasonerVer: e.Reasoner.Version(ctx)},
		EVO: core.Evolution{
			Version:    1,
			Supersedes: result.Supersedes,
			Delta:      result.Delta,
			Metadata:   metadata(result, e.Reasoner.Name() != "heuristic"),
		},
	}
	return em, nil
}

func metadata(result reason.Result, aiIntegrated bool) *core.Metadata {
	m := &core.Metadata{
		Topology:     core.TopologyDodecahedronV1,
		CapturedAt:   time.Now().UTC(),
		AIIntegrated: aiIntegrated,
		PromptSchema: "MXPD/2",
		Archonym:     result.Archonym,
	}
	for _, facet := range result.Facets {
		m.Facets = append(m.Facets, core.Facet(facet))
	}
	for _, node := range result.BuildNodes {
		m.BuildNodes = append(m.BuildNodes, core.BuildNode{ID: node.ID, System: node.System, State: node.State})
	}
	for _, edge := range result.BuildEdges {
		m.BuildEdges = append(m.BuildEdges, core.BuildEdge{From: edge.From, To: edge.To, Kind: edge.Kind})
	}
	if result.Monetization != nil {
		m.Monetization = &core.Monetization{Model: result.Monetization.Model, Customer: result.Monetization.Customer, Value: result.Monetization.Value, RevenuePath: result.Monetization.RevenuePath}
	}
	return m
}
