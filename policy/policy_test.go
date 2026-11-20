package policy

import (
	"encoding/json"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

const sha = "08fe3633d051b935482798eef280dd6d71417019409a036b142771b60b5f6ca4"

func upload(id, h string) core.EmergION {
	return core.EmergION{
		IDN: id, STA: core.StateAtGOV,
		MEM: core.Memory{SourceHash: "src-" + id, Provenance: ProvenanceSourceUpload},
		REL: map[string]string{
			"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED", "source_kind": KindSourceUpload, "source_sha256": h,
			"source_path": "sources/x/" + id + ".pdf", "source_title": "T " + id, "source_bytes": "10", "source_topic": "x",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{Risk: "L", Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1, Metadata: &core.Metadata{}},
	}
}

func link(id, from, to string) core.EmergION {
	em := upload(id, sha)
	em.MEM.Provenance = ProvenanceKinLink
	em.REL = map[string]string{"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED", "source_kind": KindKinLink, "kin_link_from": from, "kin_link_to": to, "kin_link_basis": "reasoner", "worker_version": "worker-1"}
	return em
}

func classify(st core.State, em core.EmergION) Verdict {
	st.AtGOV[em.IDN] = em
	return Classify(st, BuildIndex(st), em)
}

func TestPolicyFileMatchesCode(t *testing.T) {
	var d Document
	if err := json.Unmarshal(Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{ClassSourceUpload: "APPROVE", ClassDuplicate: "HOLD", ClassGenerated: "HOLD", ClassKinLinkAccepted: "APPROVE"}
	if len(d.Classes) != len(want) {
		t.Fatalf("classes: %+v", d.Classes)
	}
	for _, c := range d.Classes {
		if want[c.Class] != c.Decision {
			t.Fatalf("class %s decision %s", c.Class, c.Decision)
		}
	}
	if d.Version != "A01-AUTO-1" || d.Default != "OFF" || d.Authority != Authority || d.Amends != "A01" {
		t.Fatalf("header: %+v", d)
	}
	if len(SHA256()) != 64 {
		t.Fatal("hash")
	}
}

func TestClassifyLowRiskClasses(t *testing.T) {
	st := core.EmptyState()
	if v := classify(st, upload("E-1", sha)); v.Class != ClassSourceUpload || v.Decision != "APPROVE" {
		t.Fatalf("upload: %+v", v)
	}
	// exact duplicate of an accepted upload -> HOLD
	st = core.EmptyState()
	acc := upload("E-0", sha)
	acc.STA = core.StateAccepted
	st.Accepted["E-0"] = acc
	if v := classify(st, upload("E-2", sha)); v.Class != ClassDuplicate || v.Decision != "HOLD" {
		t.Fatalf("dup: %+v", v)
	}
	// two AtGOV duplicates: the smaller IDN is kept, the larger held
	st = core.EmptyState()
	st.AtGOV["E-A"] = upload("E-A", sha)
	if v := classify(st, upload("E-B", sha)); v.Class != ClassDuplicate {
		t.Fatalf("dup B: %+v", v)
	}
	if v := Classify(st, BuildIndex(st), st.AtGOV["E-A"]); v.Class != ClassSourceUpload {
		t.Fatalf("keep A: %+v", v)
	}
	// generated provider combination -> HOLD (never APPROVE)
	st = core.EmptyState()
	gen := upload("E-G", strings.Repeat("a", 64))
	gen.REL = map[string]string{"source_name": "provider_population", "capability_providers": "X:E-1"}
	gen.MEM.Provenance = "governed_target_comparison"
	if v := classify(st, gen); v.Class != ClassGenerated || v.Decision != "HOLD" {
		t.Fatalf("generated: %+v", v)
	}
	// kin link between accepted SOURCE records -> APPROVE; with a waiting end -> human
	st = core.EmptyState()
	a, b := upload("E-a", strings.Repeat("1", 64)), upload("E-b", strings.Repeat("2", 64))
	a.STA, b.STA = core.StateAccepted, core.StateAccepted
	st.Accepted["E-a"], st.Accepted["E-b"] = a, b
	if v := classify(st, link("E-L", "E-a", "E-b")); v.Class != ClassKinLinkAccepted || v.Decision != "APPROVE" {
		t.Fatalf("link: %+v", v)
	}
	delete(st.Accepted, "E-b")
	st.AtGOV["E-b"] = upload("E-b", strings.Repeat("2", 64))
	if v := classify(st, link("E-L2", "E-a", "E-b")); v.Decision != "" {
		t.Fatalf("link with waiting end must need HUMAN_FINAL: %+v", v)
	}
}

func TestClassifyNeverAutoAcceptsHighRisk(t *testing.T) {
	cases := map[string]func(*core.EmergION){
		"archonym":     func(e *core.EmergION) { e.EVO.Metadata.Archonym = "Name" },
		"capability":   func(e *core.EmergION) { e.CAP = []string{"OBS", "SEND"} },
		"risk":         func(e *core.EmergION) { e.VAL.Risk = "M" },
		"facet":        func(e *core.EmergION) { e.EVO.Metadata.Facets = []core.Facet{core.FacetPaymentsFinance} },
		"monetization": func(e *core.EmergION) { e.EVO.Metadata.Monetization = &core.Monetization{Model: "x"} },
		"supersedes":   func(e *core.EmergION) { e.EVO.Supersedes = "E-OLD" },
		"composition":  func(e *core.EmergION) { e.REL["COMPOSITION_KIN"] = "E-X" },
		"target":       func(e *core.EmergION) { e.REL["target_emergion"] = "E-X" },
		"protector":    func(e *core.EmergION) { e.REL["protector"] = "HUMAN_FINAL_BOUND" },
		"provenance":   func(e *core.EmergION) { e.MEM.Provenance = "local_dropzone" },
		"bad sha":      func(e *core.EmergION) { e.REL["source_sha256"] = "zz" },
		"no recoil":    func(e *core.EmergION) { e.VAL.Recoil = false },
		"switch":       func(e *core.EmergION) { e.REL["source_kind"] = KindSwitch; e.MEM.Provenance = ProvenanceSwitch },
		"insight":      func(e *core.EmergION) { e.REL["source_kind"] = KindInsight; e.MEM.Provenance = ProvenanceInsight },
		"unknown kind": func(e *core.EmergION) { e.REL["source_kind"] = "CAPABILITY" },
		"required cap": func(e *core.EmergION) { e.REL["required_capability"] = "X" },
		"build graph":  func(e *core.EmergION) { e.EVO.Metadata.BuildNodes = []core.BuildNode{{ID: "a", System: "b"}} },
	}
	for name, mut := range cases {
		em := upload("E-1", sha)
		mut(&em)
		v := classify(core.EmptyState(), em)
		if v.Decision == "APPROVE" {
			t.Fatalf("%s: auto-accepted: %+v", name, v)
		}
	}
}

func TestVerifyEvidence(t *testing.T) {
	em := upload("E-1", sha)
	desc := "SOURCE_UPLOAD/1\nSHA256:" + sha + "\nPATH:sources/x/E-1.pdf\nTITLE:T E-1\nTOPIC:x\nMIME:\nBYTES:10\nCAPTURED_AT:\nRELATED:\nZ\n"
	v := Verdict{Class: ClassSourceUpload}
	if err := VerifyEvidence(v, em, []byte(desc), ""); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(v, em, []byte(desc), sha); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(v, em, []byte(desc), strings.Repeat("0", 64)); err == nil {
		t.Fatal("file hash mismatch must fail")
	}
	if err := VerifyEvidence(v, em, []byte(strings.Replace(desc, "TITLE:T E-1", "TITLE:other", 1)), ""); err == nil {
		t.Fatal("descriptor mismatch must fail")
	}
}

func TestReplayCheckNeedsActivePolicyAndHash(t *testing.T) {
	st := core.EmptyState()
	em := upload("E-1", sha)
	st.AtGOV["E-1"] = em
	x := BuildIndex(st)
	v := Classify(st, x, em)
	d := core.DecisionReceipt{EmergIONID: "E-1", Decision: "APPROVE", Authority: Authority, Reason: ReasonText(v, "E-S")}
	on := Status{On: true, Version: Version(), SHA256: SHA256(), SwitchID: "E-S"}
	if err := ReplayCheck(st, x, on, em, d); err != nil {
		t.Fatal(err)
	}
	if err := ReplayCheck(st, x, Status{}, em, d); err == nil {
		t.Fatal("off must fail")
	}
	other := on
	other.SHA256 = strings.Repeat("f", 64)
	if err := ReplayCheck(st, x, other, em, d); err == nil {
		t.Fatal("hash mismatch must fail")
	}
	d.Decision = "HOLD"
	if err := ReplayCheck(st, x, on, em, d); err == nil {
		t.Fatal("decision mismatch must fail")
	}
}
