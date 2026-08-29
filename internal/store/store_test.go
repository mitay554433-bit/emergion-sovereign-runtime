package store

import (
	"emergion-sovereign-runtime/internal/core"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ev, err := s.Preserve([]byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	em := core.EmergION{IDN: "E-1", STA: core.StateAtGOV, MEM: core.Memory{SourceHash: ev.Hash, Codec: ev.Codec, Bytes: ev.Bytes, Stored: ev.Stored}, VAL: core.Validation{Recoil: true, WVC: true}, EVO: core.Evolution{Version: 1}}
	if _, err = s.SaveCandidate(em); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d", len(events))
	}
	b, err := s.ReadEvidence(ev.Hash)
	if err != nil || string(b) != "abc" {
		t.Fatalf("bad evidence %q %v", b, err)
	}
}

func TestSaveCandidateRejectsInvalidMetadata(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	em := core.EmergION{
		IDN: "E-1", STA: core.StateAtGOV,
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1, Metadata: &core.Metadata{
			CapturedAt: time.Now().UTC(),
			BuildEdges: []core.BuildEdge{{From: "missing", To: "also-missing"}},
		}},
	}
	if _, err = s.SaveCandidate(em); err == nil {
		t.Fatal("expected invalid metadata to fail")
	}
}

func TestInspectEvidenceReportsPreservedObject(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("inspect this preserved evidence")

	preserved, err := s.Preserve(content)
	if err != nil {
		t.Fatal(err)
	}

	observed, err := s.InspectEvidence(preserved.Hash)
	if err != nil {
		t.Fatal(err)
	}

	if observed.Hash != preserved.Hash {
		t.Fatalf("hash = %q want %q", observed.Hash, preserved.Hash)
	}

	if observed.Bytes != int64(len(content)) {
		t.Fatalf(
			"bytes = %d want %d",
			observed.Bytes,
			len(content),
		)
	}

	if observed.Stored != preserved.Stored {
		t.Fatalf(
			"stored = %d want %d",
			observed.Stored,
			preserved.Stored,
		)
	}

	if observed.Codec != "gzip" {
		t.Fatalf("codec = %q", observed.Codec)
	}
}

func TestSaveInterpretationRevisionPreservesOriginalCandidate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := s.Preserve([]byte("same immutable evidence"))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	original := core.EmergION{
		IDN: "E-INTERPRETATION-REVISION",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec:      evidence.Codec,
			Bytes:      evidence.Bytes,
			Stored:     evidence.Stored,
			Summary:    "malformed interpretation",
			Provenance: "saw_projection",
		},
		VAL: core.Validation{
			Facts:  []string{"malformed fact"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	if _, err = s.SaveCandidate(original); err != nil {
		t.Fatal(err)
	}

	revised := original
	revised.MEM.Summary = "corrected interpretation"
	revised.VAL.Facts = []string{"corrected fact"}

	if _, err = s.SaveInterpretationRevision(revised); err != nil {
		t.Fatal(err)
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d want 2", len(events))
	}
	if events[0].Type != "C" || events[1].Type != "I" {
		t.Fatalf("event types = %q %q", events[0].Type, events[1].Type)
	}
	if events[0].EmergION == nil || events[1].EmergION == nil {
		t.Fatal("candidate or interpretation revision payload missing")
	}
	if events[0].EmergION.MEM.Summary != "malformed interpretation" {
		t.Fatalf("original C changed: %q", events[0].EmergION.MEM.Summary)
	}
	if events[1].EmergION.MEM.Summary != "corrected interpretation" {
		t.Fatalf("revision summary = %q", events[1].EmergION.MEM.Summary)
	}
	if events[0].EmergION.IDN != events[1].EmergION.IDN {
		t.Fatal("interpretation revision changed IDN")
	}
	if events[0].EmergION.MEM.SourceHash != events[1].EmergION.MEM.SourceHash {
		t.Fatal("interpretation revision changed source hash")
	}
}

func TestSaveInterpretationRevisionRejectsEvidenceIdentityChange(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := s.Preserve([]byte("same immutable evidence"))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	original := core.EmergION{
		IDN: "E-INTERPRETATION-EVIDENCE",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec:      evidence.Codec,
			Bytes:      evidence.Bytes,
			Stored:     evidence.Stored,
			Provenance: "saw_projection",
		},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	if _, err = s.SaveCandidate(original); err != nil {
		t.Fatal(err)
	}

	revised := original
	revised.MEM.Codec = "changed-codec"

	if _, err = s.SaveInterpretationRevision(revised); err == nil {
		t.Fatal("expected evidence identity change to fail")
	}
}

func TestSaveInterpretationRevisionRejectsNonGOVReadyRevision(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := s.Preserve([]byte("same immutable evidence"))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	original := core.EmergION{
		IDN: "E-INTERPRETATION-WVC",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec:      evidence.Codec,
			Bytes:      evidence.Bytes,
			Stored:     evidence.Stored,
			Provenance: "saw_projection",
		},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	if _, err = s.SaveCandidate(original); err != nil {
		t.Fatal(err)
	}

	revised := original
	revised.VAL.WVC = false

	if _, err = s.SaveInterpretationRevision(revised); err == nil {
		t.Fatal("expected non-GOV-ready revision to fail")
	}
}
