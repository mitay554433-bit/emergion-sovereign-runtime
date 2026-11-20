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
			"source_path": "sources/x/" + id + ".pdf", "source_title": "Helix grant brief " + id, "source_bytes": "10", "source_topic": "x",
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

func insight(id, of, kin string) core.EmergION {
	em := upload(id, sha)
	em.MEM.Provenance = ProvenanceInsight
	em.REL = map[string]string{"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED", "source_kind": KindInsight, "insight_of": of, "source_topic": "x", "insight_reasoner": "local-gemma-server", "worker_version": "worker-1"}
	if kin != "" {
		em.REL["insight_kin"] = kin
	}
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
	want := map[string]string{ClassSourceUpload: "APPROVE", ClassDuplicate: "HOLD", ClassGenerated: "HOLD", ClassKinLinkWorker: "APPROVE", ClassInsightWorker: "APPROVE", ClassWorkerNoise: "HOLD"}
	if len(d.Classes) != len(want) {
		t.Fatalf("classes: %+v", d.Classes)
	}
	for _, c := range d.Classes {
		if want[c.Class] != c.Decision {
			t.Fatalf("class %s decision %s", c.Class, c.Decision)
		}
	}
	for _, n := range d.Never {
		if strings.Contains(n, "SOURCE_INSIGHT") {
			t.Fatalf("insights are no longer in never: %q", n)
		}
	}
	if v, h, ok := Superseded(); !ok || v != "A01-AUTO-1" || h != "18d8739c7fca808fb4ad773b05f1a4299d5981f563e6588261e2e0102f7c037e" || !strings.Contains(d.Supersedes.Note, "HUMAN_FINAL") {
		t.Fatalf("supersedes: %v %v %v", v, h, ok)
	}
	if !strings.Contains(d.Approved, "auto-accept insights and links even if the uploads aren't accepted, with undo") {
		t.Fatalf("approved: %q", d.Approved)
	}
	if d.Version != "A01-AUTO-2" || d.Default != "OFF" || d.Authority != Authority || d.Amends != "A01" {
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
	// kin link between accepted SOURCE records -> APPROVE
	st = core.EmptyState()
	a, b := upload("E-a", strings.Repeat("1", 64)), upload("E-b", strings.Repeat("2", 64))
	a.STA, b.STA = core.StateAccepted, core.StateAccepted
	st.Accepted["E-a"], st.Accepted["E-b"] = a, b
	if v := classify(st, link("E-L", "E-a", "E-b")); v.Class != ClassKinLinkWorker || v.Decision != "APPROVE" {
		t.Fatalf("link: %+v", v)
	}
}

func TestA01Auto2LinksAndInsights(t *testing.T) {
	ends := func() core.State {
		st := core.EmptyState()
		a, b := upload("E-a", strings.Repeat("1", 64)), upload("E-b", strings.Repeat("2", 64))
		b.STA = core.StateApproved
		st.AtGOV["E-a"], st.Approved["E-b"] = a, b
		return st
	}
	// A01-AUTO-2: a link whose ends are not accepted (AtGOV, approved) auto-approves
	if v := classify(ends(), link("E-L", "E-a", "E-b")); v.Class != ClassKinLinkWorker || v.Decision != "APPROVE" {
		t.Fatalf("link with non-accepted ends: %+v", v)
	}
	// ...but the A01-AUTO-1 rules (legacy replay) still need accepted ends
	st := ends()
	st.AtGOV["E-L"] = link("E-L", "E-a", "E-b")
	if v := classifyLegacy(st, BuildIndex(st), st.AtGOV["E-L"]); v.Decision != "" {
		t.Fatalf("legacy rules changed: %+v", v)
	}
	// a link to a missing upload is not approved
	if v := classify(ends(), link("E-L", "E-a", "E-missing")); v.Decision == "APPROVE" {
		t.Fatalf("link to missing upload approved: %+v", v)
	}
	// a link to a rejected upload is not approved
	st = ends()
	r := st.Approved["E-b"]
	delete(st.Approved, "E-b")
	r.STA = core.StateRejected
	st.Rejected["E-b"] = r
	if v := classify(st, link("E-L", "E-a", "E-b")); v.Decision == "APPROVE" {
		t.Fatalf("link to rejected upload approved: %+v", v)
	}
	// a link to a record that is not a SOURCE_UPLOAD is not approved
	st = ends()
	st.AtGOV["E-i"] = insight("E-i", "E-a", "")
	if v := classify(st, link("E-L", "E-a", "E-i")); v.Decision == "APPROVE" {
		t.Fatalf("link to non-upload approved: %+v", v)
	}
	// an insight on an existing upload (cited kin too) auto-approves
	if v := classify(ends(), insight("E-I", "E-a", "E-b")); v.Class != ClassInsightWorker || v.Decision != "APPROVE" {
		t.Fatalf("insight: %+v", v)
	}
	// insight on a missing or rejected source, or citing a missing kin -> human
	if v := classify(ends(), insight("E-I", "E-missing", "")); v.Decision != "" {
		t.Fatalf("insight on missing source: %+v", v)
	}
	if v := classify(ends(), insight("E-I", "E-a", "E-missing")); v.Decision != "" {
		t.Fatalf("insight citing missing kin: %+v", v)
	}
	st = ends()
	st.Rejected["E-b"] = st.Approved["E-b"]
	delete(st.Approved, "E-b")
	if v := classify(st, insight("E-I", "E-b", "")); v.Decision != "" {
		t.Fatalf("insight on rejected source: %+v", v)
	}
	// unsafe insights are never approved
	unsafe := map[string]func(*core.EmergION){
		"facet":        func(e *core.EmergION) { e.EVO.Metadata.Facets = []core.Facet{core.FacetPaymentsFinance} },
		"archonym":     func(e *core.EmergION) { e.EVO.Metadata.Archonym = "Name" },
		"capability":   func(e *core.EmergION) { e.CAP = []string{"OBS", "SEND"} },
		"risk":         func(e *core.EmergION) { e.VAL.Risk = "M" },
		"supersedes":   func(e *core.EmergION) { e.EVO.Supersedes = "E-OLD" },
		"monetization": func(e *core.EmergION) { e.EVO.Metadata.Monetization = &core.Monetization{Model: "x"} },
		"build graph":  func(e *core.EmergION) { e.EVO.Metadata.BuildNodes = []core.BuildNode{{ID: "a", System: "b"}} },
		"authority":    func(e *core.EmergION) { e.REL["capability_providers"] = "X:E-a" },
		"provenance":   func(e *core.EmergION) { e.MEM.Provenance = "local_dropzone" },
		"no wvc":       func(e *core.EmergION) { e.VAL.WVC = false },
	}
	for name, mut := range unsafe {
		em := insight("E-I", "E-a", "")
		mut(&em)
		if v := classify(ends(), em); v.Decision == "APPROVE" {
			t.Fatalf("insight with %s approved: %+v", name, v)
		}
	}
	// descriptor must match
	v := Verdict{Class: ClassInsightWorker}
	em := insight("E-I", "E-a", "E-b")
	desc := "SOURCE_INSIGHT/1\nOF:E-a\nOF_SHA256:x\nREASONER:r\nWORKER:worker-1\nSUMMARY:s\nKIN:E-b\nZ\n"
	if err := VerifyEvidence(v, em, []byte(desc), ""); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(v, em, []byte(strings.Replace(desc, "OF:E-a", "OF:E-z", 1)), ""); err == nil {
		t.Fatal("insight descriptor mismatch must fail")
	}
}

func TestSwitchForA01Auto1CarriesOverOnlyViaSupersedes(t *testing.T) {
	oldV, oldH, _ := Superseded()
	if !Covers(Version(), SHA256()) || !Covers(oldV, oldH) || !CarriedOver(oldV, oldH) || CarriedOver(Version(), SHA256()) {
		t.Fatal("covers")
	}
	if Covers(oldV, strings.Repeat("f", 64)) || Covers("A01-AUTO-0", oldH) {
		t.Fatal("only the exact superseded version and hash carry over")
	}
	st := core.EmptyState()
	st.AtGOV["E-a"] = upload("E-a", strings.Repeat("1", 64))
	em := insight("E-I", "E-a", "")
	st.AtGOV["E-I"] = em
	x := BuildIndex(st)
	v := Classify(st, x, em)
	d := core.DecisionReceipt{EmergIONID: "E-I", Decision: "APPROVE", Authority: Authority, Reason: ReasonText(v, "E-S1")}
	old := Status{On: true, Version: oldV, SHA256: oldH, SwitchID: "E-S1"}
	if err := ReplayCheck(st, x, old, em, d); err != nil {
		t.Fatalf("A01-AUTO-2 receipt under the A01-AUTO-1 switch: %v", err)
	}
	if err := ReplayCheck(st, x, Status{Version: oldV, SHA256: oldH}, em, d); err == nil {
		t.Fatal("switch OFF must fail")
	}
	// an A01-AUTO-1 receipt replays under its own switch with its own rules
	lnk := link("E-L", "E-a", "E-b")
	acc := upload("E-b", strings.Repeat("2", 64))
	acc.STA = core.StateAccepted
	st.Accepted["E-b"] = acc
	a := st.AtGOV["E-a"]
	delete(st.AtGOV, "E-a")
	a.STA = core.StateAccepted
	st.Accepted["E-a"] = a
	st.AtGOV["E-L"] = lnk
	x = BuildIndex(st)
	legacy := core.DecisionReceipt{EmergIONID: "E-L", Decision: "APPROVE", Authority: Authority,
		Reason: "AUTO_POLICY " + oldV + " sha256=" + oldH + " class=" + ClassKinLinkAccepted + " switch=E-S1: link between accepted sources E-a and E-b"}
	if err := ReplayCheck(st, x, old, lnk, legacy); err != nil {
		t.Fatalf("legacy receipt: %v", err)
	}
	cur := Status{On: true, Version: Version(), SHA256: SHA256(), SwitchID: "E-S2"}
	if err := ReplayCheck(st, x, cur, lnk, legacy); err == nil {
		t.Fatal("an A01-AUTO-1 receipt needs the A01-AUTO-1 switch")
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
	desc := "SOURCE_UPLOAD/1\nSHA256:" + sha + "\nPATH:sources/x/E-1.pdf\nTITLE:Helix grant brief E-1\nTOPIC:x\nMIME:\nBYTES:10\nCAPTURED_AT:\nRELATED:\nZ\n"
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
	if err := VerifyEvidence(v, em, []byte(strings.Replace(desc, "TITLE:Helix grant brief E-1", "TITLE:other", 1)), ""); err == nil {
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

func TestWorkerNoiseIsHeld(t *testing.T) {
	mk := func(id, title, topic string) core.EmergION {
		em := upload(id, strings.Repeat(id[len(id)-1:], 64))
		em.REL["source_title"], em.REL["source_topic"] = title, topic
		return em
	}
	base := func(a, b core.EmergION) core.State {
		st := core.EmptyState()
		a.STA, b.STA = core.StateAccepted, core.StateAccepted
		st.Accepted[a.IDN], st.Accepted[b.IDN] = a, b
		return st
	}
	real := mk("E-1", "Helix bench protocol.pdf", "inventions-ip")
	cases := map[string]core.EmergION{
		"pipe path":    mk("E-2", "PHONE|Download|scan_0042.pdf", "inventions-ip"),
		"phone path":   mk("E-3", "/PHONE/Download/x.pdf", "inventions-ip"),
		"raw id":       mk("E-4", "E-4B978014FEAFC9EA", "inventions-ip"),
		"non-canon":    mk("E-5", "Fluid Registry v3 notes", "inventions-ip"),
		"sha as title": mk("E-6", strings.Repeat("ab", 32), "inventions-ip"),
	}
	for name, bad := range cases {
		st := base(real, bad)
		if v := classify(st, link("E-L", "E-1", bad.IDN)); v.Class != ClassWorkerNoise || v.Decision != "HOLD" {
			t.Fatalf("%s link: %+v", name, v)
		}
		if v := classify(st, insight("E-I", bad.IDN, "")); v.Class != ClassWorkerNoise || v.Decision != "HOLD" {
			t.Fatalf("%s insight: %+v", name, v)
		}
		if LinkNoise(st, "E-1", bad.IDN) == "" {
			t.Fatalf("%s: worker must skip", name)
		}
	}
	// a held end is noise
	st := base(real, mk("E-7", "Helix bench results", "inventions-ip"))
	h := st.Accepted["E-7"]
	delete(st.Accepted, "E-7")
	h.STA = core.StateHeld
	st.Held["E-7"] = h
	if v := classify(st, link("E-L", "E-1", "E-7")); v.Class != ClassWorkerNoise {
		t.Fatalf("held end: %+v", v)
	}
	// no shared topic: different topics and only generic title words
	st = base(real, mk("E-8", "Investor deck.pdf", "grants"))
	if v := classify(st, link("E-L", "E-1", "E-8")); v.Class != ClassWorkerNoise || !strings.Contains(v.Why, "no shared topic") {
		t.Fatalf("no shared topic: %+v", v)
	}
	// a generic shared topic does not count
	st = base(mk("E-1", "Helix bench protocol", "uploads"), mk("E-9", "Investor deck", "uploads"))
	if v := classify(st, link("E-L", "E-1", "E-9")); v.Class != ClassWorkerNoise {
		t.Fatalf("generic topic: %+v", v)
	}
	// different topics but a meaningful shared title term is a real link
	st = base(real, mk("E-9", "Helix funding plan", "grants"))
	if v := classify(st, link("E-L", "E-1", "E-9")); v.Class != ClassKinLinkWorker || v.Decision != "APPROVE" {
		t.Fatalf("shared term: %+v", v)
	}
	// unsafe shape still needs HUMAN_FINAL even when noisy (never auto-held into a pass)
	st = base(real, cases["non-canon"])
	em := insight("E-I", "E-5", "")
	em.EVO.Metadata.Facets = []core.Facet{core.FacetPaymentsFinance}
	if v := classify(st, em); v.Decision != "" {
		t.Fatalf("unsafe noisy insight: %+v", v)
	}
	// legacy A01-AUTO-1 rules are unchanged by the quality rules
	st = base(real, cases["pipe path"])
	st.AtGOV["E-L"] = link("E-L", "E-1", "E-2")
	if v := classifyLegacy(st, BuildIndex(st), st.AtGOV["E-L"]); v.Class != ClassKinLinkAccepted {
		t.Fatalf("legacy: %+v", v)
	}
}
