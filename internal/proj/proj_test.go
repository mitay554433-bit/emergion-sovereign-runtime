package proj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func TestCurrentPublishesFreshnessReceipt(t *testing.T) {
	root := t.TempDir()
	st := core.State{
		AtGOV: map[string]core.EmergION{}, Approved: map[string]core.EmergION{},
		Accepted: map[string]core.EmergION{}, Held: map[string]core.EmergION{},
		Rejected: map[string]core.EmergION{}, Returned: map[string]core.EmergION{},
		Events: 7, TipHash: "tip-7",
	}

	receipt, err := Current(root, st)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.TipHash != st.TipHash || receipt.Events != st.Events {
		t.Fatalf("receipt = %#v", receipt)
	}
	for _, name := range []string{"field.json", "field.html", "projection.current.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s not published: %v", name, err)
		}
	}

	b, err := os.ReadFile(filepath.Join(root, "projection.current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published Receipt
	if err := json.Unmarshal(b, &published); err != nil {
		t.Fatal(err)
	}
	if published != receipt {
		t.Fatalf("published receipt = %#v, want %#v", published, receipt)
	}
	jsonHash, err := fileHash(filepath.Join(root, "field.json"))
	if err != nil {
		t.Fatal(err)
	}
	htmlHash, err := fileHash(filepath.Join(root, "field.html"))
	if err != nil {
		t.Fatal(err)
	}
	if jsonHash != receipt.JSONHash || htmlHash != receipt.HTMLHash {
		t.Fatal("projection hashes do not match freshness receipt")
	}
}

func TestSpatialConvergenceZoneContainsAcceptedOnly(t *testing.T) {
	accepted := core.EmergION{
		IDN: "E-ACCEPTED",
		STA: core.StateAccepted,
		MEM: core.Memory{
			Summary: "accepted structure",
		},
	}

	atGOV := core.EmergION{
		IDN: "E-GOV",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			Summary: "not yet accepted",
		},
	}

	rejected := core.EmergION{
		IDN: "E-REJECTED",
		STA: core.StateRejected,
		MEM: core.Memory{
			Summary: "rejected structure",
		},
	}

	st := core.EmptyState()
	st.Accepted[accepted.IDN] = accepted
	st.AtGOV[atGOV.IDN] = atGOV
	st.Rejected[rejected.IDN] = rejected

	rows, err := convergenceRows(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("SPATIAL CONVERGENCE ZONE rows = %d want 1", len(rows))
	}

	if rows[0].ID != accepted.IDN {
		t.Fatalf(
			"SPATIAL CONVERGENCE ZONE contains %q want %q",
			rows[0].ID,
			accepted.IDN,
		)
	}
}

func TestSpatialEmergIONProjectionContainsAcceptedOnlyWithoutMutation(t *testing.T) {
	accepted := core.EmergION{
		IDN: "E-SPATIAL-ACCEPTED",
		STA: core.StateAccepted,
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				Archonym: "SPATIAL ACCEPTED",
				Topology: core.TopologyDodecahedronV1,
				Facets: []core.Facet{
					core.FacetFIELDCommand,
				},
			},
		},
	}

	atGOV := core.EmergION{
		IDN: "E-SPATIAL-GOV",
		STA: core.StateAtGOV,
	}

	rejected := core.EmergION{
		IDN: "E-SPATIAL-REJECTED",
		STA: core.StateRejected,
	}

	st := core.EmptyState()
	st.Accepted[accepted.IDN] = accepted
	st.AtGOV[atGOV.IDN] = atGOV
	st.Rejected[rejected.IDN] = rejected

	projected := spatialEmergIONs(st)

	if len(projected) != 1 {
		t.Fatalf(
			"spatial EmergION projection count = %d want 1",
			len(projected),
		)
	}

	if projected[0].ID != accepted.IDN {
		t.Fatalf(
			"spatial EmergION = %q want %q",
			projected[0].ID,
			accepted.IDN,
		)
	}

	if projected[0].Topology != string(core.TopologyDodecahedronV1) {
		t.Fatalf(
			"spatial topology = %q want %q",
			projected[0].Topology,
			core.TopologyDodecahedronV1,
		)
	}

	projected[0].Facets[0] = core.FacetProgramForge

	if st.Accepted[accepted.IDN].EVO.Metadata.Facets[0] != core.FacetFIELDCommand {
		t.Fatal("spatial projection mutated accepted state")
	}
}

func TestSpatialFacetOrderIsCompleteDeterministicProjectionOnly(t *testing.T) {
	got := spatialFacetOrder()

	want := []core.Facet{
		core.FacetFIELDCommand,
		core.FacetEmergenceCapture,
		core.FacetProgramForge,
		core.FacetProductStore,
		core.FacetCustomersSales,
		core.FacetCommunications,
		core.FacetPaymentsFinance,
		core.FacetGrantFunding,
		core.FacetPatentIP,
		core.FacetMAPartnerships,
		core.FacetDocsProjection,
		core.FacetAnalyticsForecast,
	}

	if len(got) != 12 {
		t.Fatalf("spatial facet count = %d want 12", len(got))
	}

	seen := map[core.Facet]bool{}
	for i, facet := range got {
		if seen[facet] {
			t.Fatalf("duplicate spatial facet %q", facet)
		}
		seen[facet] = true
		if facet != want[i] {
			t.Fatalf("spatial facet[%d] = %q want %q", i, facet, want[i])
		}
	}

	got[0] = core.FacetProgramForge
	if spatialFacetOrder()[0] != core.FacetFIELDCommand {
		t.Fatal("spatial facet order retained caller mutation")
	}
}

func TestSpatialConvergenceZoneDerivesAcceptedKinWithoutMerging(t *testing.T) {
	predecessor := core.EmergION{
		IDN: "E-PREDECESSOR",
		STA: core.StateAccepted,
		MEM: core.Memory{
			Summary: "accepted predecessor",
		},
	}

	successor := core.EmergION{
		IDN: "E-SUCCESSOR",
		STA: core.StateAccepted,
		MEM: core.Memory{
			Summary: "accepted successor",
		},
		EVO: core.Evolution{
			Supersedes: predecessor.IDN,
		},
	}

	st := core.EmptyState()
	st.Accepted[predecessor.IDN] = predecessor
	st.Accepted[successor.IDN] = successor

	rows, err := convergenceRows(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("Kin projection merged sovereign EmergIONs: rows = %d want 2", len(rows))
	}

	byID := map[string]convergenceRow{}
	for _, row := range rows {
		byID[row.ID] = row
	}

	if byID[predecessor.IDN].Kin != "root → E-PREDECESSOR" {
		t.Fatalf("predecessor Kin = %q", byID[predecessor.IDN].Kin)
	}

	if byID[successor.IDN].Kin != "root → E-PREDECESSOR" {
		t.Fatalf("successor Kin = %q", byID[successor.IDN].Kin)
	}

	if _, ok := st.Accepted[predecessor.IDN]; !ok {
		t.Fatal("predecessor lost from sovereign accepted state")
	}
	if _, ok := st.Accepted[successor.IDN]; !ok {
		t.Fatal("successor lost from sovereign accepted state")
	}
}

func TestSpatialConvergenceZoneDoesNotInventDanglingKin(t *testing.T) {
	em := core.EmergION{
		IDN: "E-ACCEPTED",
		STA: core.StateAccepted,
		MEM: core.Memory{
			Summary: "accepted structure",
		},
		EVO: core.Evolution{
			Supersedes: "E-NOT-ACCEPTED",
		},
	}

	st := core.EmptyState()
	st.Accepted[em.IDN] = em

	if _, err := convergenceRows(st); err == nil {
		t.Fatal("dangling accepted Kin ancestry unexpectedly projected")
	}
}

func TestSpatialConvergenceZoneProjectsGovernedArchonym(t *testing.T) {
	em := core.EmergION{
		IDN: "E-ARCHONYM",
		STA: core.StateAccepted,
		MEM: core.Memory{
			Summary: "accepted semantic identity",
		},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	st := core.EmptyState()
	st.Accepted[em.IDN] = em

	rows, err := convergenceRows(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("rows = %d want 1", len(rows))
	}

	if rows[0].Archonym != "VERITEX CORE" {
		t.Fatalf(
			"projected Archonym = %q want VERITEX CORE",
			rows[0].Archonym,
		)
	}
}

func TestPRMCrystallizesAcceptedGovernedPrimitive(t *testing.T) {
	predecessor := core.EmergION{
		IDN: "E-PRM-ROOT",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "source-root",
			Summary:    "accepted root",
		},
	}

	em := core.EmergION{
		IDN: "E-PRM-CHILD",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "source-child",
			Summary:    "accepted reusable mechanism",
		},
		REL: map[string]string{
			"uses":     "bounded-input",
			"produces": "bounded-output",
		},
		CAP: []string{
			"ANALYZE",
			"SIMULATE",
		},
		VAL: core.Validation{
			Facts: []string{
				"deterministic transform",
				"bounded invariant",
			},
			Gaps: []string{
				"BRIDGEGAP:external_dependency",
			},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Supersedes: predecessor.IDN,
			Delta: []string{
				"DC:+:SIMULATE",
			},
			Metadata: &core.Metadata{
				Archonym: "VERITEX PRIMITIVE",
				Facets: []core.Facet{
					core.FacetProgramForge,
					core.FacetAnalyticsForecast,
				},
				BuildNodes: []core.BuildNode{
					{
						ID:     "input",
						System: "INPUT",
						State:  "BOUND",
					},
					{
						ID:     "transform",
						System: "TRANSFORM",
						State:  "ACTIVE",
					},
				},
				BuildEdges: []core.BuildEdge{
					{
						From: "input",
						To:   "transform",
						Kind: "FLOW",
					},
				},
			},
		},
	}

	st := core.EmptyState()
	st.Accepted[predecessor.IDN] = predecessor
	st.Accepted[em.IDN] = em

	prms, err := crystallizePRMs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(prms) != 2 {
		t.Fatalf("PRMs = %d want 2", len(prms))
	}

	byID := map[string]prm{}
	for _, item := range prms {
		byID[item.SourceEmergIONID] = item
	}

	got := byID[em.IDN]

	if got.SourceEmergIONID != em.IDN {
		t.Fatalf(
			"source EmergION = %q want %q",
			got.SourceEmergIONID,
			em.IDN,
		)
	}

	if len(got.BuildNodes) != 2 {
		t.Fatalf("build nodes = %#v", got.BuildNodes)
	}

	if len(got.BuildEdges) != 1 {
		t.Fatalf("build edges = %#v", got.BuildEdges)
	}

	// PRM deliberately has no Gaps field. BRIDGEGAP remains unresolved
	// validation state and is not crystallized as reusable mechanism.
}

func TestPRMCrystallizationUsesAcceptedStateOnly(t *testing.T) {
	accepted := core.EmergION{
		IDN: "E-PRM-ACCEPTED",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "accepted-source",
		},
	}

	atGOV := core.EmergION{
		IDN: "E-PRM-GOV",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "candidate-source",
		},
	}

	st := core.EmptyState()
	st.Accepted[accepted.IDN] = accepted
	st.AtGOV[atGOV.IDN] = atGOV

	prms, err := crystallizePRMs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(prms) != 1 {
		t.Fatalf("PRMs = %d want 1 accepted-only primitive", len(prms))
	}

	if prms[0].SourceEmergIONID != accepted.IDN {
		t.Fatalf(
			"PRM source = %q want %q",
			prms[0].SourceEmergIONID,
			accepted.IDN,
		)
	}
}

func TestPRMCrystallizationDoesNotMergeSovereignKin(t *testing.T) {
	root := core.EmergION{
		IDN: "E-PRM-KIN-ROOT",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "kin-root-source",
		},
		CAP: []string{"ANALYZE"},
	}

	child := core.EmergION{
		IDN: "E-PRM-KIN-CHILD",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "kin-child-source",
		},
		CAP: []string{"SIMULATE"},
		EVO: core.Evolution{
			Supersedes: root.IDN,
		},
	}

	st := core.EmptyState()
	st.Accepted[root.IDN] = root
	st.Accepted[child.IDN] = child

	prms, err := crystallizePRMs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(prms) != 2 {
		t.Fatalf(
			"sovereign Kin merged during PRM crystallization: %d PRMs",
			len(prms),
		)
	}

	if prms[0].SourceEmergIONID == prms[1].SourceEmergIONID {
		t.Fatal("distinct sovereign Kin produced duplicate PRM identity")
	}

	if st.Accepted[root.IDN].CAP[0] != "ANALYZE" {
		t.Fatal("PRM crystallization mutated root EmergION")
	}

	if st.Accepted[child.IDN].CAP[0] != "SIMULATE" {
		t.Fatal("PRM crystallization mutated child EmergION")
	}
}

func TestSAABDerivesOnlyExplicitCompositionKin(t *testing.T) {
	a := core.EmergION{
		IDN: "E-SAAB-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saab-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-SAAB-B",
		},
		CAP: []string{"ANALYZE"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				BuildNodes: []core.BuildNode{
					{
						ID:     "engine",
						System: "ANALYZER",
						State:  "READY",
					},
				},
			},
		},
	}

	b := core.EmergION{
		IDN: "E-SAAB-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saab-b",
		},
		CAP: []string{"SIMULATE"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				BuildNodes: []core.BuildNode{
					{
						ID:     "engine",
						System: "SIMULATOR",
						State:  "READY",
					},
				},
			},
		},
	}

	unrelated := core.EmergION{
		IDN: "E-SAAB-UNRELATED",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saab-unrelated",
		},
		CAP: []string{"OBS"},
	}

	st := core.EmptyState()
	st.Accepted[a.IDN] = a
	st.Accepted[b.IDN] = b
	st.Accepted[unrelated.IDN] = unrelated

	assemblies, err := deriveSAABs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(assemblies) != 1 {
		t.Fatalf("SAAB count = %d want 1", len(assemblies))
	}

	got := assemblies[0]

	if len(got.MemberPRMIDs) != 2 {
		t.Fatalf("SAAB members = %#v", got.MemberPRMIDs)
	}

	if got.MemberPRMIDs[0] != a.IDN ||
		got.MemberPRMIDs[1] != b.IDN {
		t.Fatalf("SAAB members = %#v", got.MemberPRMIDs)
	}

	for _, id := range got.MemberPRMIDs {
		if id == unrelated.IDN {
			t.Fatal("unrelated accepted PRM entered SAAB")
		}
	}

	if len(got.CompositionLinks) != 1 {
		t.Fatalf(
			"composition links = %#v",
			got.CompositionLinks,
		)
	}

	link := got.CompositionLinks[0]
	if link.FromPRM != a.IDN ||
		link.ToPRM != b.IDN ||
		link.Kind != "COMPOSITION_KIN" {
		t.Fatalf("composition link = %#v", link)
	}

	if len(got.BuildNodes) != 2 {
		t.Fatalf("build nodes = %#v", got.BuildNodes)
	}

	if got.BuildNodes[0].ID == got.BuildNodes[1].ID {
		t.Fatal("member build node identities collided")
	}

	if st.Accepted[a.IDN].IDN != a.IDN ||
		st.Accepted[b.IDN].IDN != b.IDN {
		t.Fatal("SAAB derivation mutated sovereign PRMs")
	}
}

func TestSAABRejectsDanglingCompositionTarget(t *testing.T) {
	em := core.EmergION{
		IDN: "E-SAAB-DANGLING",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saab-dangling",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-NOT-ACCEPTED",
		},
	}

	st := core.EmptyState()
	st.Accepted[em.IDN] = em

	if _, err := deriveSAABs(st); err == nil {
		t.Fatal("dangling COMPOSITION_KIN unexpectedly formed SAAB")
	}
}

func TestCPSLCompilesDeterministicSAABStructure(t *testing.T) {
	a := core.EmergION{
		IDN: "E-CPSL-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "cpsl-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-CPSL-B",
		},
		CAP: []string{"PROGRAM"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				BuildNodes: []core.BuildNode{
					{
						ID:     "source",
						System: "SOURCE",
						State:  "READY",
					},
					{
						ID:     "transform",
						System: "TRANSFORM",
						State:  "READY",
					},
				},
				BuildEdges: []core.BuildEdge{
					{
						From: "source",
						To:   "transform",
						Kind: "FLOW",
					},
				},
			},
		},
	}

	b := core.EmergION{
		IDN: "E-CPSL-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "cpsl-b",
		},
		CAP: []string{"ANALYZE"},
	}

	st := core.EmptyState()
	st.Accepted[a.IDN] = a
	st.Accepted[b.IDN] = b

	first, err := compileCPSL(st)
	if err != nil {
		t.Fatal(err)
	}

	second, err := compileCPSL(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf(
			"CPSL counts = %d %d want 1 1",
			len(first),
			len(second),
		)
	}

	if first[0].Program != second[0].Program {
		t.Fatal("CPSL compilation is not deterministic")
	}

	program := first[0].Program

	required := []string{
		"CPSL/1\n",
		`P|"E-CPSL-A"`,
		`P|"E-CPSL-B"`,
		`L|"E-CPSL-A"|"E-CPSL-B"|"COMPOSITION_KIN"`,
		`N|"E-CPSL-A::source"|"SOURCE"|"READY"`,
		`N|"E-CPSL-A::transform"|"TRANSFORM"|"READY"`,
		`E|"E-CPSL-A::source"|"E-CPSL-A::transform"|"FLOW"`,
		"Z\n",
	}

	for _, want := range required {
		if !strings.Contains(program, want) {
			t.Fatalf(
				"CPSL missing %q\n%s",
				want,
				program,
			)
		}
	}

	if st.Accepted[a.IDN].EVO.Metadata.BuildNodes[0].ID != "source" {
		t.Fatal("CPSL compilation mutated source PRM build graph")
	}
}

func TestSAWExtractsDeterministicGovernedArtifact(t *testing.T) {
	a := core.EmergION{
		IDN: "E-SAW-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saw-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-SAW-B",
		},
		CAP: []string{
			"PROGRAM",
			"ANALYZE",
		},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				BuildNodes: []core.BuildNode{
					{
						ID:     "source",
						System: "SOURCE",
						State:  "READY",
					},
				},
			},
		},
	}

	b := core.EmergION{
		IDN: "E-SAW-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saw-b",
		},
		CAP: []string{
			"SIMULATE",
		},
	}

	st := core.EmptyState()
	st.Accepted[a.IDN] = a
	st.Accepted[b.IDN] = b

	first, err := extractSAWs(st)
	if err != nil {
		t.Fatal(err)
	}

	second, err := extractSAWs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf(
			"SAW counts = %d %d want 1 1",
			len(first),
			len(second),
		)
	}

	if first[0].ID != second[0].ID {
		t.Fatalf(
			"SAW identity not deterministic: %q %q",
			first[0].ID,
			second[0].ID,
		)
	}

	if first[0].CPSL != second[0].CPSL {
		t.Fatal("SAW CPSL changed across deterministic rebuild")
	}

	if first[0].SAABID == "" {
		t.Fatal("SAW lost source SAAB identity")
	}

	if len(first[0].MemberPRMIDs) != 2 {
		t.Fatalf(
			"SAW member PRMs = %#v",
			first[0].MemberPRMIDs,
		)
	}

	if first[0].MemberPRMIDs[0] != a.IDN ||
		first[0].MemberPRMIDs[1] != b.IDN {
		t.Fatalf(
			"SAW member identities = %#v",
			first[0].MemberPRMIDs,
		)
	}

	if !strings.Contains(first[0].CPSL, "CPSL/1\n") {
		t.Fatalf("SAW did not preserve CPSL:\n%s", first[0].CPSL)
	}

	if st.Accepted[a.IDN].IDN != a.IDN ||
		st.Accepted[b.IDN].IDN != b.IDN {
		t.Fatal("SAW extraction mutated accepted EmergIONs")
	}
}

func TestSAWDoesNotExistWithoutGovernedComposition(t *testing.T) {
	em := core.EmergION{
		IDN: "E-SAW-SINGLE",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saw-single",
		},
		CAP: []string{"PROGRAM"},
	}

	st := core.EmptyState()
	st.Accepted[em.IDN] = em

	artifacts, err := extractSAWs(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(artifacts) != 0 {
		t.Fatalf(
			"SAW fabricated without governed composition: %#v",
			artifacts,
		)
	}
}

func TestCommercialMetadataSurvivesThroughSAW(t *testing.T) {
	a := core.EmergION{
		IDN: "E-COMMERCIAL-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "commercial-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-COMMERCIAL-B",
		},
		CAP: []string{"PROGRAM"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				Monetization: &core.Monetization{
					Model:       "license",
					Customer:    "enterprise",
					Value:       "governed composition",
					RevenuePath: "approved deployment",
				},
			},
		},
	}

	b := core.EmergION{
		IDN: "E-COMMERCIAL-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "commercial-b",
		},
		CAP: []string{"ANALYZE"},
	}

	st := core.EmptyState()
	st.Accepted[a.IDN] = a
	st.Accepted[b.IDN] = b

	assemblies, err := deriveSAABs(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(assemblies) != 1 || len(assemblies[0].Commercial) != 1 {
		t.Fatalf("SAAB commercial projection = %#v", assemblies)
	}

	artifacts, err := extractSAWs(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || len(artifacts[0].Commercial) != 1 {
		t.Fatalf("SAW commercial projection = %#v", artifacts)
	}

	if st.Accepted[a.IDN].EVO.Metadata.Monetization.Model != "license" {
		t.Fatal("commercial projection mutated accepted state")
	}
}

func TestSAWSourceIsDeterministicAndNonAuthoritative(t *testing.T) {
	a := core.EmergION{
		IDN: "E-SOURCE-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "source-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-SOURCE-B",
		},
		CAP: []string{"ANALYZE"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				Monetization: &core.Monetization{
					Model:       "license",
					Customer:    "customer",
					Value:       "value",
					RevenuePath: "delivery",
				},
			},
		},
	}

	b := core.EmergION{
		IDN: "E-SOURCE-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "source-b",
		},
	}

	st := core.EmptyState()
	st.Accepted[a.IDN] = a
	st.Accepted[b.IDN] = b

	first, err := SAWSources(st)
	if err != nil {
		t.Fatal(err)
	}

	second, err := SAWSources(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("SAW source counts = %d %d", len(first), len(second))
	}

	if first[0].ID != second[0].ID ||
		string(first[0].Content) != string(second[0].Content) {
		t.Fatal("SAW source representation is not deterministic")
	}

	content := string(first[0].Content)

	required := []string{
		"SAW/1\n",
		`P|"E-SOURCE-A"`,
		`P|"E-SOURCE-B"`,
		`M|"E-SOURCE-A"|"license"|"customer"|"value"|"delivery"`,
		"X|",
		"Z\n",
	}

	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Fatalf("SAW source missing %q\n%s", want, content)
		}
	}

	if st.Accepted[a.IDN].STA != core.StateAccepted ||
		st.Accepted[b.IDN].STA != core.StateAccepted {
		t.Fatal("SAW source projection mutated authoritative state")
	}
}


func TestHTMLProjectsVerifiedDeliveriesWithoutChangingFieldState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "field.html")
	st := core.EmptyState()
	st.Events = 3
	st.TipHash = "tip-delivery-ui"

	if err := HTML(path, st); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)

	for _, want := range []string{
		"VERIFIED DELIVERIES",
		"deliverables/index.json",
		"deliverables/",
		"Operator projection only.",
		"canonical authority remains COSL + REG",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("field.html missing %q", want)
		}
	}

	if st.Events != 3 || st.TipHash != "tip-delivery-ui" {
		t.Fatalf("HTML projection mutated FIELD state: %#v", st)
	}
}
