package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Trajectory list labels lead with the first provider and composition so
// distinct trajectories are distinguishable; the long "provider trajectory ·
// ESTABLIS…" prefix is reserved for title/detail.
func TestLivingTrajectoryRowLabelsAreDistinct(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, batchBlocks(t, src)+`
const a={required_capability:"ESTABLISH_CAPABILITY",capability_composition:"ANALYZE+CMP",first_provider:"E-AAAAAA01",first_provider_title:"Alpha Sensor",members:["E-T1","E-T2"],newest_id:"E-T2"};
const b={required_capability:"ESTABLISH_CAPABILITY",capability_composition:"OBS+VLD",first_provider:"E-BBBBBB02",first_provider_title:"Beta Gate",members:["E-U1"],newest_id:"E-U1"};
const c={required_capability:"ESTABLISH_CAPABILITY",capability_composition:"ANALYZE+CMP",first_provider:"E-CCCCCC03",first_provider_title:"",members:["E-V1","E-V2","E-V3"],newest_id:"E-V3"};
const labels=[trajRowLabel(a),trajRowLabel(b),trajRowLabel(c)],full=trajLabel(a);
process.stdout.write(JSON.stringify({labels,full,same:labels[0]===labels[1]||labels[0]===labels[2]||labels[1]===labels[2]}));`)
	var r struct {
		Labels []string
		Full   string
		Same   bool
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Same || len(r.Labels) != 3 {
		t.Fatalf("labels must be distinct: %v", r.Labels)
	}
	if r.Labels[0] != "via Alpha Sensor · ANALYZE×CMP · 1 held" || r.Labels[1] != "via Beta Gate · OBS×VLD" ||
		!strings.HasPrefix(r.Labels[2], "via E-CCCCCC") || !strings.Contains(r.Labels[2], "ANALYZE×CMP · 2 held") {
		t.Fatalf("row labels = %v", r.Labels)
	}
	for _, lb := range r.Labels {
		if strings.Contains(lb, "provider trajectory") || strings.Contains(lb, "ESTABLISH") {
			t.Fatalf("row label still leads with the repeated prefix: %q", lb)
		}
	}
	if !strings.HasPrefix(r.Full, "provider trajectory · ESTABLISH_CAPABILITY → Alpha Sensor · ") {
		t.Fatalf("full trajLabel for tap/detail = %q", r.Full)
	}
}

// Hold all newest holds every remaining trajectory member (including newest)
// and never a non-trajectory / real AtGOV record.
func TestLivingHoldAllNewestPlanExcludesRealAtGOV(t *testing.T) {
	src := livingUniverseHTML(t)
	if !strings.Contains(src, `hnBtn.textContent=armHN?'CONFIRM HOLD '+fmt(hn.hold.length):'Hold all newest ('+fmt(hn.hold.length)+')'`) ||
		!strings.Contains(src, "Hold all newest (") || !strings.Contains(src, "hfHoldNewest") {
		t.Fatal("Hold all newest must use two-press CONFIRM HOLD N in the batch panel and at the top of Next Governed Step")
	}
	out := runNode(t, batchBlocks(t, src)+`
const gov={"E-X00001":{r:{source_kind:"EXECUTION_OBSERVATION",target_emergion:"E-AAAAAA01"}},
 "E-T1":{r:{source_name:"provider_population"}},"E-T2":{r:{source_name:"provider_population"}},
 "E-U1":{r:{source_name:"provider_population"}}};
const trs=[{key:"k1",newest_id:"E-T2",members:["E-T1","E-T2"]},{key:"k2",newest_id:"E-U1",members:["E-U1"]}];
const hn=holdAllNewestPlan(trs),ha=holdAllPlan(trs);
const body=JSON.parse(batchBody(hn.hold,HOLD_ALL_NEWEST_REASON,"","HOLD"));
const st1=batchStep(null,'HN|'+batchKey(hn.hold),1000),st2=batchStep(st1.arm,'HN|'+batchKey(hn.hold),2000),stLate=batchStep(st1.arm,'HN|'+batchKey(hn.hold),1000+4001);
process.stdout.write(JSON.stringify({hold:hn.hold,n:hn.hold.length,trajCount:hn.trajCount,ha:ha.hold,
 body,reason:HOLD_ALL_NEWEST_REASON,send1:st1.send,send2:st2.send,sendLate:stLate.send}));`)
	var r struct {
		Hold      []string
		N         int
		TrajCount int `json:"trajCount"`
		Ha        []string
		Body      map[string]any
		Reason    string
		Send1     bool `json:"send1"`
		Send2     bool `json:"send2"`
		SendLate  bool `json:"sendLate"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if strings.Join(r.Hold, ",") != "E-T1,E-T2,E-U1" || r.N != 3 || r.TrajCount != 2 {
		t.Fatalf("holdAllNewestPlan = %v n=%d trajCount=%d", r.Hold, r.N, r.TrajCount)
	}
	if strings.Contains(strings.Join(r.Hold, ","), "E-X00001") {
		t.Fatal("hold-all-newest must not include non-trajectory AtGOV")
	}
	if strings.Join(r.Ha, ",") != "E-T1" {
		t.Fatalf("hold-all (rest) should still keep newest: %v", r.Ha)
	}
	if r.Body["decision"] != "HOLD" || r.Body["confirm_count"] != float64(3) ||
		r.Reason != "Trajectory HUMAN_FINAL hold-all-newest: generated provider combinations" || r.Body["reason"] != r.Reason {
		t.Fatalf("hold-all-newest body/reason = %+v %q", r.Body, r.Reason)
	}
	if r.Send1 || !r.Send2 || r.SendLate {
		t.Fatalf("two-press: first=%v second=%v late=%v", r.Send1, r.Send2, r.SendLate)
	}
}

// Generated vs real is classified from trajectories[] membership or a
// provider_population source, never from a title or summary string.
func TestLivingGeneratedClassificationAndRealOnlyQueue(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		"Generated · recommend Hold",
		"Generated from providers you already accepted. Not new evidence. Accepting adds a provider and grows the next round. Hold keeps it on record.",
		"Generated combinations:",
		"hfHoldNewest",
		`og.label='Generated'`,
		"real '+fmt(nReal)+' · generated ",
		"holdButton(it.id)",
		"acc sec",
		"hfReal.length",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	// Real rows keep Accept as the only/primary action; traj rows add Hold first.
	if !strings.Contains(src, "q.append(b,holdButton(it.id),acc)") || !strings.Contains(src, "q.append(b,acceptButton(it.id))") {
		t.Fatal("trajectory rows must be HOLD-primary with secondary Accept; real rows keep Accept")
	}

	out := runNode(t, batchBlocks(t, src)+`
const gov={
 "E-REAL01":{r:{source_kind:"EXECUTION_RESULT",target_emergion:"E-AAAAAA01"},m:{s:"provider trajectory · ESTABLIS decoy title"}},
 "E-T1":{r:{source_name:"provider_population"}},
 "E-T2":{r:{source_name:"provider_population"}},
 "E-P1":{r:{source_name:"provider_population/path"}}
};
const trs=[{key:"k1",newest_id:"E-T2",members:["E-T1","E-T2"],first_provider:"E-AAAAAA01",first_provider_title:"Alpha",capability_composition:"ANALYZE+CMP"}];
const q=[{id:"E-REAL01",summary:"provider trajectory · ESTABLIS decoy title"},{id:"E-T1",summary:"t1"},{id:"E-T2",summary:"t2"},{id:"E-P1",summary:"p1"}];
const split=splitGovQueue(q,trs,gov),ids=generatedIDs(gov,trs).slice().sort();
const idx=batchIndex(gov,{},trs),realG=[],genG=[];
for(const g of idx.groups){(g.traj?genG:realG).push(g.key)};
const a=1,z=Math.min(split.real.length,5),page=fmt(a)+'\u2013'+fmt(z)+' of '+fmt(split.real.length);
process.stdout.write(JSON.stringify({
 decoy:isGeneratedRecord("E-REAL01",gov["E-REAL01"],trs),
 mem:isGeneratedRecord("E-T1",gov["E-T1"],trs),
 pop:isGeneratedRecord("E-P1",gov["E-P1"],[]),
 real:split.real.map(x=>x.id),
 trajRows:split.trajRows.map(x=>[x.id,!!x.traj,x.traj?trajRowLabel(x.traj):x.summary]),
 ids,nReal:split.real.length,page,realG,genG}));`)
	var r struct {
		Decoy    bool
		Mem      bool
		Pop      bool
		Real     []string
		TrajRows [][]any  `json:"trajRows"`
		IDs      []string `json:"ids"`
		NReal    int      `json:"nReal"`
		Page     string
		RealG    []string `json:"realG"`
		GenG     []string `json:"genG"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Decoy {
		t.Fatal("a real record must not be classified generated because its title mentions a trajectory")
	}
	if !r.Mem || !r.Pop {
		t.Fatalf("membership and provider_population source must count as generated: mem=%v pop=%v", r.Mem, r.Pop)
	}
	if fmt.Sprint(r.Real) != "[E-REAL01]" || r.NReal != 1 || r.Page != "1–1 of 1" {
		t.Fatalf("real-only queue/pagination = real %v page %q", r.Real, r.Page)
	}
	if strings.Contains(strings.Join(r.Real, ","), "E-T") || strings.Contains(strings.Join(r.Real, ","), "E-P1") {
		t.Fatalf("generated records leaked into the real list: %v", r.Real)
	}
	if strings.Join(r.IDs, ",") != "E-P1,E-T1,E-T2" {
		t.Fatalf("generatedIDs = %v", r.IDs)
	}
	if len(r.RealG) == 0 || strings.Contains(strings.Join(r.RealG, ","), "traj:") {
		t.Fatalf("real groups first, no trajectory keys: %v", r.RealG)
	}
	if len(r.GenG) == 0 || !strings.HasPrefix(r.GenG[0], "traj:") {
		t.Fatalf("trajectory groups belong under Generated: %v", r.GenG)
	}
	for _, it := range r.TrajRows {
		if len(it) < 1 || fmt.Sprint(it[0]) == "E-REAL01" {
			t.Fatalf("real record leaked into generated rows: %v", r.TrajRows)
		}
	}
}
