package cosl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

func fullEvent() core.Event {
	captured := time.Unix(1700000000, 123456789).UTC()

	return core.Event{
		Type:     "C",
		ID:       "EV-TEST-1",
		At:       captured,
		PrevHash: "previous-event-hash",
		EmergION: &core.EmergION{
			IDN: "E-0123456789ABCDEF",
			STA: core.StateAtGOV,
			MEM: core.Memory{
				SourceHash: "0123456789abcdef",
				Codec:      "gzip",
				Bytes:      4096,
				Stored:     1024,
				Summary:    "bounded EmergION test",
				Provenance: "local_dropzone",
			},
			REL: map[string]string{
				"depends_on": "E-BASE",
				"extends":    "E-PRIOR",
			},
			CAP: []string{
				"VERIFY",
				"PROJECT",
			},
			VAL: core.Validation{
				Facts: []string{
					"source preserved",
					"identity deterministic",
				},
				Differences: []string{"TARGET_STATE!=ACCEPTED_REALITY"},
				Gaps: []string{
					"external validation pending",
				},
				Risk:        "LOW",
				Recoil:      true,
				WVC:         true,
				Reasoner:    "gemma",
				ReasonerVer: "test-version",
			},
			EVO: core.Evolution{
				Version:    2,
				Supersedes: "E-PRIOR",
				Delta: []string{
					"added verification evidence",
				},
				Metadata: &core.Metadata{
					Topology:     core.TopologyDodecahedronV1,
					CapturedAt:   captured,
					AIIntegrated: true,
					PromptSchema: "EMERGER_LOGICAL_V2",
					Archonym:     "COVERAGE PROTECTOR",
					Facets: []core.Facet{
						core.FacetEmergenceCapture,
						core.FacetProgramForge,
						core.FacetAnalyticsForecast,
					},
					BuildNodes: []core.BuildNode{
						{
							ID:     "N1",
							System: "COVERAGE",
							State:  "ACTIVE",
						},
						{
							ID:     "N2",
							System: "PROTECTOR",
							State:  "ACTIVE",
						},
					},
					BuildEdges: []core.BuildEdge{
						{
							From: "N1",
							To:   "N2",
							Kind: "TRANSITION",
						},
					},
					Monetization: &core.Monetization{
						Model:       "license",
						Customer:    "enterprise",
						Value:       "governed runtime",
						RevenuePath: "deployment",
					},
				},
			},
		},
	}
}

func TestRoundTripFullEmergION(t *testing.T) {
	want := fullEvent()

	line, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(line, EventPrefix) {
		t.Fatalf("native COSL prefix missing: %q", line)
	}

	got, err := Decode(line)
	if err != nil {
		t.Fatal(err)
	}

	sealedWant, err := Seal(want)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, sealedWant) {
		t.Fatalf(
			"roundtrip mismatch\nwant: %#v\ngot:  %#v",
			sealedWant,
			got,
		)
	}

	if err := got.EmergION.EVO.Metadata.Validate(); err != nil {
		t.Fatalf("metadata invalid after roundtrip: %v", err)
	}
}

func TestLegacyCOSL1JSONDecode(t *testing.T) {
	want := fullEvent()

	sealed, err := legacySeal(want)
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}

	legacy := Prefix + string(b)

	got, err := Decode(legacy)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, sealed) {
		t.Fatalf(
			"legacy decode mismatch\nwant: %#v\ngot:  %#v",
			sealed,
			got,
		)
	}
}

func TestTamperedNativeCOSLRejected(t *testing.T) {
	line, err := Encode(fullEvent())
	if err != nil {
		t.Fatal(err)
	}

	tampered := strings.Replace(
		line,
		"T=1:C;",
		"T=1:R;",
		1,
	)

	if tampered == line {
		t.Fatal("test could not alter COSL record")
	}

	if _, err := Decode(tampered); err == nil {
		t.Fatal("tampered COSL record was accepted")
	}
}

func TestMultilineStringsCrossCOSLPivotLineSafe(t *testing.T) {
	want := fullEvent()
	want.EmergION.MEM.Summary = "line one\nline two\r\n終\\path"
	want.EmergION.MEM.Provenance = `path\with\slashes`

	line, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}

	if strings.ContainsAny(line, "\r\n") {
		t.Fatalf("COSL event crossed physical-line boundary: %q", line)
	}

	if !strings.Contains(line, "S=B") {
		t.Fatal("multiline summary did not use line-safe COSL representation")
	}

	got, err := Decode(line)
	if err != nil {
		t.Fatal(err)
	}

	sealedWant, err := Seal(want)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, sealedWant) {
		t.Fatalf(
			"multiline roundtrip mismatch\nwant: %#v\ngot:  %#v",
			sealedWant,
			got,
		)
	}
}

func TestPlainCOSLStringRepresentationRemainsCompatible(t *testing.T) {
	line, err := Encode(fullEvent())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(line, "S=21:bounded EmergION test") {
		t.Fatalf("ordinary COSL string representation changed: %q", line)
	}

	if strings.Contains(line, "S=B") {
		t.Fatal("ordinary string unnecessarily entered line-safe representation")
	}
}
