package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// Page helpers: the batch is the first min(N,cap) records by IDN matching the
// chosen filter, the typed count must equal it, and the body is what the
// server's BatchParams decodes.
func TestLivingBatchSelectionAndBody(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, batchBlocks(t, src)+`
const gov={"E-03":{r:{source_kind:"A",target_emergion:"E-T1"},e:{m:{f:["MEM"],h:"three"}}},
 "E-01":{r:{source_kind:"A"},e:{m:{f:["MEM","VAL"]}},m:{s:"one"}},
 "E-02":{r:{source_kind:"B",target_emergion:"E-T1"},e:{m:{f:["VAL"]}}}};
const idx=batchIndex(gov,{});
const r={
 all:batchSelect(gov,batchFilter('all','',null),500),
 tapNone:batchSelect(gov,batchFilter('tap','','ALL'),500).ids,
 tapVal:batchSelect(gov,batchFilter('tap','','VAL'),500,idx).ids,
 cls:batchSelect(gov,{kind:'class',value:'A'},500).ids,
 tgt:batchSelect(gov,{kind:'target',value:'E-T1'},500).ids,
 cap:batchSelect(gov,{kind:'all'},2),
 opts:batchOptions(gov).class,
 title:batchTitle(gov["E-01"],"E-01"),
 body:JSON.parse(batchBody(["E-01","E-03"],"why"))};
process.stdout.write(JSON.stringify(r));`)
	var r struct {
		All struct {
			Total int      `json:"total"`
			IDs   []string `json:"ids"`
		} `json:"all"`
		TapNone, TapVal, Cls, Tgt []string
		Cap                       struct {
			Total int      `json:"total"`
			IDs   []string `json:"ids"`
		} `json:"cap"`
		Opts          [][]any `json:"opts"`
		OK, Bad, Zero bool
		Title         string
		Body          BatchParams
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if strings.Join(r.All.IDs, ",") != "E-01,E-02,E-03" || r.All.Total != 3 {
		t.Fatalf("all = %+v", r.All)
	}
	if len(r.TapNone) != 3 || strings.Join(r.TapVal, ",") != "E-01,E-02" || strings.Join(r.Cls, ",") != "E-01,E-03" || strings.Join(r.Tgt, ",") != "E-02,E-03" {
		t.Fatalf("filters: %s", out)
	}
	if r.Cap.Total != 3 || strings.Join(r.Cap.IDs, ",") != "E-01,E-02" {
		t.Fatalf("cap = %+v", r.Cap)
	}
	if len(r.Opts) != 2 || r.Opts[0][0] != "A" || r.Opts[0][1] != float64(2) {
		t.Fatalf("class options = %v", r.Opts)
	}
	if r.Title != "one" {
		t.Fatalf("title: %s", out)
	}
	if strings.Join(r.Body.EmergIONIDs, ",") != "E-01,E-03" || r.Body.Reason != "why" || r.Body.ConfirmCount != 2 {
		t.Fatalf("body = %+v", r.Body)
	}
}

// The reason is pre-filled from filter, count and the most common title
// prefix, and an edited reason is remembered. One press only arms; a second
// press on the same batch within 4 s sends; a late or changed press re-arms.
func TestLivingBatchPrefilledReasonAndDoublePress(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, `const _ls={};globalThis.localStorage={getItem:k=>k in _ls?_ls[k]:null,setItem:(k,v)=>{_ls[k]=String(v)},removeItem:k=>{delete _ls[k]}};
`+batchBlocks(t, src)+`
const gov={"E-01":{r:{source_kind:"COMPARISON"},e:{m:{h:"comparison: TARGET_EMERGION:E-AA"}}},
 "E-02":{r:{source_kind:"COMPARISON"},e:{m:{h:"comparison: TARGET_EMERGION:E-BB"}}},
 "E-03":{r:{source_kind:"OBS"},m:{s:"execution observation of run"}}};
const f={kind:'class',value:'COMPARISON'},ids=batchSelect(gov,f,500).ids;
const pre=autoReason(gov,f,ids),all=autoReason(gov,{kind:'all'},Object.keys(gov).sort());
const before=savedReason();storeReason('my reason');const kept=savedReason();storeReason('');
const k=batchKey(ids),t0=1000;
const p1=batchStep(null,k,t0),p2=batchStep(p1.arm,k,t0+1500),late=batchStep(p1.arm,k,t0+4100),other=batchStep(p1.arm,batchKey(['E-09']),t0+500);
process.stdout.write(JSON.stringify({pre,all,before,kept,after:savedReason(),p1:p1.send,armed:!!p1.arm,p2:p2.send,late:late.send,other:other.send}));`)
	var r struct {
		Pre, All, Before, Kept, After string
		P1, Armed, P2, Late, Other    bool
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Pre != "Batch HUMAN_FINAL: class COMPARISON · 2 records · sample comparison" {
		t.Fatalf("pre-filled reason = %q", r.Pre)
	}
	if !strings.HasPrefix(r.All, "Batch HUMAN_FINAL: all AtGOV · 3 records · sample ") {
		t.Fatalf("all reason = %q", r.All)
	}
	if r.Before != "" || r.Kept != "my reason" || r.After != "" {
		t.Fatalf("reason memory: %s", out)
	}
	if r.P1 || !r.Armed || !r.P2 || r.Late || r.Other {
		t.Fatalf("double press: single=%v armed=%v second=%v late=%v changed=%v", r.P1, r.Armed, r.P2, r.Late, r.Other)
	}
}

// batchBlocks is the batch code plus the reference resolver the haze uses.
func batchBlocks(t *testing.T, src string) string {
	return block(t, src, "ref-resolve") + "\n" + block(t, src, "ref-resolve-fn") + "\n" + block(t, src, "trajectory-rows") + "\n" + block(t, src, "governed-accept")
}

// A provider trajectory is ONE row: in the group list and in the HUMAN_FINAL
// queue. Accept takes only the newest member; Hold the rest HOLDs the others
// (decision HOLD in the batch body); the row carries the OBS warning.
func TestLivingTrajectoryOneRowAcceptNewestHoldRest(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, batchBlocks(t, src)+`
const acc={"E-AAAAAA01":{e:{m:{h:"F;C;T",f:["ANALYTICS_FORECAST"]}}}};
const gov={"E-X00001":{r:{source_kind:"EXECUTION_OBSERVATION",target_emergion:"E-AAAAAA01"}}};
const members=[];for(let i=1;i<=5;i++){const id="E-T0000"+i;members.push(id);gov[id]={r:{source_name:"provider_population",capability_providers:"ANALYZE:E-AAAAAA01,CMP:E-C"+i}}}
const proj={trajectories:[{key:"DERIVE_CAPABILITY|ANALYZE+CMP+RLT|E-AAAAAA01",required_capability:"DERIVE_CAPABILITY",first_provider:"E-AAAAAA01",first_provider_title:"F;C;T",count:6,first_at:"2026-11-01T10:00:00Z",last_at:"2026-11-19T22:00:00Z",newest_id:"E-T00004",members:members.concat(["E-GONE01"])}]};
const trs=trajList(proj,gov),idx=batchIndex(gov,acc,trs),g=idx.groups.find(g=>g.traj),f={kind:'group',value:g.key};
const s=batchSelect(gov,f,500,idx),pl=batchPlan(s,f,idx),other=batchPlan(batchSelect(gov,{kind:'group',value:idx.groups.find(g=>!g.traj).key},500,idx),{kind:'group',value:idx.groups.find(g=>!g.traj).key},idx);
const q=collapseTraj([{id:"E-X00001",summary:"x"}].concat(members.map(id=>({id,summary:"deterministic capability provider trajectory"}))),trs);
process.stdout.write(JSON.stringify({count:trs[0].count,label:g.label,groups:idx.groups.map(x=>x.count),accept:pl.accept,hold:pl.hold,otherAccept:other.accept,otherTraj:!!other.traj,
 q:q.map(x=>[x.id,!!x.traj]),hold_body:JSON.parse(batchBody(pl.hold.slice(0,2),"r","","HOLD")),appr_body:JSON.parse(batchBody(pl.accept,"r","",undefined)),warn:TRAJ_WARN}));`)
	var r struct {
		Count       int
		Label       string
		Groups      []int
		Accept      []string
		Hold        []string
		OtherAccept []string
		OtherTraj   bool
		Q           [][]any
		HoldBody    map[string]any `json:"hold_body"`
		ApprBody    map[string]any `json:"appr_body"`
		Warn        string
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Count != 5 || !strings.HasPrefix(r.Label, "provider trajectory · DERIVE_CAPABILITY → F;C;T · 5 runs · ") || !strings.Contains(r.Label, "…") {
		t.Fatalf("label/count: %d %q", r.Count, r.Label)
	}
	if fmt.Sprint(r.Groups) != "[5 1]" {
		t.Fatalf("groups = %v (trajectory members must leave the haze groups)", r.Groups)
	}
	if fmt.Sprint(r.Accept) != "[E-T00004]" || len(r.Hold) != 4 || strings.Contains(fmt.Sprint(r.Hold), "E-T00004") {
		t.Fatalf("plan accept=%v hold=%v", r.Accept, r.Hold)
	}
	if r.OtherTraj || fmt.Sprint(r.OtherAccept) != "[E-X00001]" {
		t.Fatalf("non-trajectory group plan: %v", r.OtherAccept)
	}
	if fmt.Sprint(r.Q) != "[[E-X00001 false] [E-T00004 true]]" {
		t.Fatalf("queue = %v want one trajectory row at the newest member", r.Q)
	}
	if r.HoldBody["decision"] != "HOLD" || r.HoldBody["confirm_count"] != float64(2) {
		t.Fatalf("hold body = %v", r.HoldBody)
	}
	if _, ok := r.ApprBody["decision"]; ok {
		t.Fatalf("approve body must stay wire-compatible (no decision field): %v", r.ApprBody)
	}
	if r.Warn != "accepting adds OBS providers" {
		t.Fatalf("warning = %q", r.Warn)
	}
}

// AtGOV records carry no facet of their own; for the batch facet filter and
// counts they belong to the facet(s) of the accepted record they reference,
// resolved like the haze: target_emergion, then REL IDNs, then the summary.
func TestLivingBatchFacetFromReferencedTarget(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, batchBlocks(t, src)+`
const acc={"E-AAAAAA01":{e:{m:{f:["DOCS_PROJECTION","MEM"]}}},"E-AAAAAA02":{e:{m:{f:["GRANT_FUNDING"]}}},"E-AAAAAA03":{e:{m:{}}}};
const gov={
 "E-B00001":{r:{target_emergion:"E-AAAAAA01"}},
 "E-B00002":{r:{kin:"E-AAAAAA02"}},
 "E-B00003":{r:{},m:{s:"compare with E-AAAAAA01"}},
 "E-B00004":{r:{target_emergion:"E-FFFFFF99",parent:"E-AAAAAA02"}},
 "E-B00005":{r:{target_emergion:"E-AAAAAA03"}},
 "E-B00006":{r:{source_kind:"X"}}};
const idx=batchIndex(gov,acc),sel=f=>batchSelect(gov,{kind:'facet',value:f},500,idx).ids;
process.stdout.write(JSON.stringify({docs:sel("DOCS_PROJECTION"),grant:sel("GRANT_FUNDING"),un:sel("UNASSIGNED"),none:sel("PATENT"),
 opts:batchOptions(gov,idx).facet,tap:batchSelect(gov,batchFilter('tap','','DOCS_PROJECTION'),500,idx).ids,same:batchIndex(gov,acc)===idx}));`)
	var r struct {
		Docs, Grant, Un, None, Tap []string
		Opts                       [][]any
		Same                       bool
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if strings.Join(r.Docs, ",") != "E-B00001,E-B00003" || strings.Join(r.Tap, ",") != "E-B00001,E-B00003" {
		t.Fatalf("docs facet = %v tap = %v", r.Docs, r.Tap)
	}
	// unresolved target_emergion falls through to the next REL IDN, as the haze does
	if strings.Join(r.Grant, ",") != "E-B00002,E-B00004" || strings.Join(r.Un, ",") != "E-B00005" || len(r.None) != 0 {
		t.Fatalf("grant = %v unassigned = %v none = %v", r.Grant, r.Un, r.None)
	}
	if len(r.Opts) != 4 || r.Opts[0][0] != "DOCS_PROJECTION" || r.Opts[0][1] != float64(2) {
		t.Fatalf("facet options = %v", r.Opts)
	}
	if !r.Same {
		t.Fatal("index must be cached per field snapshot")
	}
	// zero matches: hint line and Accept stays disabled
	if !strings.Contains(src, "No waiting records point at this facet") || !strings.Contains(src, "go.disabled=!pl.accept.length") {
		t.Fatal("zero-match facet must show the hint and keep Accept disabled")
	}
}

// Emergent groups are the data's own (same resolved haze target + same
// source_kind), sorted by size; a group is selected whole even over the cap
// and sent in cap-sized chunks; later chunks carry the first batch id.
func TestLivingBatchEmergentGroupsWholeAndChunked(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, batchBlocks(t, src)+`
const acc={"E-AAAAAA01":{e:{m:{f:["DOCS_PROJECTION"]}}},"E-AAAAAA02":{e:{m:{f:["GRANT_FUNDING"]}}}};
const gov={};
for(let i=0;i<7;i++)gov["E-C0000"+i]={r:{source_kind:"DETERMINISTIC_CAPABILITY_PROVIDERS",target_emergion:"E-AAAAAA01"}};
for(let i=0;i<3;i++)gov["E-D0000"+i]={r:{source_kind:"COMPARISON",kin:"E-AAAAAA02"}};
gov["E-E00001"]={r:{source_kind:"COMPARISON"}};
const idx=batchIndex(gov,acc),g=idx.groups,s=batchSelect(gov,{kind:'group',value:g[0].key},3,idx);
process.stdout.write(JSON.stringify({groups:g.map(x=>[x.label,x.count]),n:s.ids.length,whole:s.whole,
 chunks:batchChunks(s.ids,3).map(c=>c.length),b1:JSON.parse(batchBody(s.ids.slice(0,3),"r","")),b2:JSON.parse(batchBody(s.ids.slice(3,6),"r","HFB-0123456789AB")),
 reason:autoReason(gov,{kind:'group',value:g[0].key},s.ids,idx)}));`)
	var r struct {
		Groups [][]any
		N      int
		Whole  bool
		Chunks []int
		B1, B2 map[string]any
		Reason string
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := [][]any{{"deterministic capability providers → E-AAAAAA01", float64(7)}, {"comparison → E-AAAAAA02", float64(3)}, {"comparison → no accepted target", float64(1)}}
	if fmt.Sprint(r.Groups) != fmt.Sprint(want) {
		t.Fatalf("groups = %v", r.Groups)
	}
	if r.N != 7 || !r.Whole || fmt.Sprint(r.Chunks) != "[3 3 1]" {
		t.Fatalf("whole group over cap: n=%d whole=%v chunks=%v", r.N, r.Whole, r.Chunks)
	}
	if _, ok := r.B1["batch_id"]; ok || r.B2["batch_id"] != "HFB-0123456789AB" || r.B2["confirm_count"] != float64(3) {
		t.Fatalf("chunk bodies: %v %v", r.B1, r.B2)
	}
	if !strings.HasPrefix(r.Reason, "Batch HUMAN_FINAL: group deterministic capability providers → E-AAAAAA01 · 7 records") {
		t.Fatalf("reason = %q", r.Reason)
	}
}

func TestDecideBatchHumanRoute(t *testing.T) {
	rt, id, statePath := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)
	post := func(token string, p BatchParams) (int, fieldapi.BatchResult) {
		b, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/decide-batch-human", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		out, _ := io.ReadAll(rec.Result().Body)
		var res fieldapi.BatchResult
		_ = json.Unmarshal(out, &res)
		return rec.Code, res
	}
	events := func() int {
		s, err := store.Open(statePath)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		return len(ev)
	}
	good := BatchParams{EmergIONIDs: []string{id}, Reason: "batch from /living", ConfirmCount: 1}
	for _, tok := range []string{"", "wrong-token"} {
		if c, _ := post(tok, good); c != http.StatusUnauthorized {
			t.Fatalf("token %q: %d want 401", tok, c)
		}
	}
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id}, Reason: "x", ConfirmCount: 2}); c != http.StatusBadRequest || !strings.Contains(res.Error, "confirm count") {
		t.Fatalf("count mismatch: %d %+v", c, res)
	}
	old := batchCap
	batchCap = 1
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id, "E-0000000000000001"}, Reason: "x", ConfirmCount: 2}); c != http.StatusBadRequest || !strings.Contains(res.Error, "exceeds cap 1") {
		t.Fatalf("cap: %d %+v", c, res)
	}
	batchCap = old
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id}, Reason: "x", ConfirmCount: 1, BatchID: "HFB-zz"}); c != http.StatusBadRequest || !strings.Contains(res.Error, "invalid batch id") {
		t.Fatalf("bad batch id: %d %+v", c, res)
	}
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id}, Reason: "x", ConfirmCount: 1, Decision: "REJECT"}); c != http.StatusBadRequest || !strings.Contains(res.Error, "APPROVE or HOLD") {
		t.Fatalf("bad decision: %d %+v", c, res)
	}
	if events() != 1 {
		t.Fatal("refused batches wrote events")
	}
	req := httptest.NewRequest(http.MethodGet, "/decide-batch-human", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cap":500`) {
		t.Fatalf("GET cap: %d %s", rec.Code, rec.Body.String())
	}

	c, res := post(testToken, good)
	if c != http.StatusOK || res.Accepted != 1 || res.Error != "" || !strings.HasPrefix(res.BatchID, "HFB-") {
		t.Fatalf("accept: %d %+v", c, res)
	}
	s, _ := store.Open(statePath)
	ev, _ := s.Events()
	if len(ev) != 3 || ev[1].Type != "D" || ev[2].Type != "R" || ev[1].Decision.Authority != "HUMAN_FINAL" ||
		ev[1].Decision.Reason != "batch from /living [HUMAN_FINAL batch "+res.BatchID+", 1 records]" || ev[2].REG.DecisionID != ev[1].ID {
		t.Fatalf("receipts: %+v", ev)
	}
	st, err := livefield.Rebuild(ev)
	if err != nil || len(st.Accepted) != 1 || len(st.AtGOV) != 0 {
		t.Fatalf("rebuild: %v", err)
	}
	if _, err := s.VerifyEvidence(); err != nil {
		t.Fatal(err)
	}
	if c, res := post(testToken, good); c != http.StatusOK || res.Accepted != 0 || res.Skipped != 1 || events() != 3 {
		t.Fatalf("re-run: %d %+v", c, res)
	}
}

// decision HOLD over the route: token-gated, one HUMAN_FINAL HOLD receipt.
func TestDecideBatchHumanRouteHold(t *testing.T) {
	rt, id, statePath := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)
	post := func(token string) (int, fieldapi.BatchResult) {
		b, _ := json.Marshal(BatchParams{EmergIONIDs: []string{id}, Reason: "hold the rest", ConfirmCount: 1, Decision: "HOLD"})
		req := httptest.NewRequest(http.MethodPost, "/decide-batch-human", bytes.NewReader(b))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var res fieldapi.BatchResult
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		return rec.Code, res
	}
	if c, _ := post(""); c != http.StatusUnauthorized {
		t.Fatalf("no token: %d", c)
	}
	c, res := post(testToken)
	if c != http.StatusOK || res.Held != 1 || res.Accepted != 0 {
		t.Fatalf("hold: %d %+v", c, res)
	}
	s, _ := store.Open(statePath)
	ev, _ := s.Events()
	if len(ev) != 2 || ev[1].Type != "D" || ev[1].Decision.Decision != "HOLD" || ev[1].Decision.Authority != "HUMAN_FINAL" {
		t.Fatalf("events: %+v", ev)
	}
	st, err := livefield.Rebuild(ev)
	if err != nil || len(st.Held) != 1 || len(st.Accepted) != 0 {
		t.Fatalf("rebuild: %v", err)
	}
}

// Hold all trajectories holds, across every current trajectory, every member
// except that trajectory's newest (the newest stays AtGOV). N is the hold
// count; the page shows the trajectory count too. The body is decision HOLD
// with the pre-filled hold-all reason, chunked by the existing cap.
func TestLivingHoldAllTrajectoriesPlanAndConfirm(t *testing.T) {
	src := livingUniverseHTML(t)
	if !strings.Contains(src, `haBtn.textContent=armHA?'CONFIRM HOLD '+fmt(ha.hold.length):'Hold all trajectories ('+fmt(ha.hold.length)+')'`) {
		t.Fatal("Hold all trajectories must use the two-press CONFIRM HOLD N label")
	}
	if !strings.Contains(src, "batchStep(batchArm,'HA|'+batchKey(ids)") || !strings.Contains(src, "batchAccept(ids,HOLD_ALL_REASON,res,'HOLD')") {
		t.Fatal("Hold all must arm with HA| and send HOLD_ALL_REASON via the existing batch route")
	}
	if strings.Contains(src, "Accept all") {
		t.Fatal("Accept all must not be added")
	}
	out := runNode(t, batchBlocks(t, src)+`
const gov={};
const m1=[],m2=[],m3=["E-C00001"];
for(let i=1;i<=4;i++){const id="E-A0000"+i;m1.push(id);gov[id]={r:{source_name:"provider_population"}}}
for(let i=1;i<=3;i++){const id="E-B0000"+i;m2.push(id);gov[id]={r:{source_name:"provider_population"}}}
gov["E-C00001"]={r:{source_name:"provider_population"}};
const trs=trajList({trajectories:[
 {key:"k1",newest_id:"E-A00003",members:m1.concat(["E-GONE"])},
 {key:"k2",newest_id:"E-B00003",members:m2},
 {key:"k3",newest_id:"E-C00001",members:m3}
]},gov);
const ha=holdAllPlan(trs),body=JSON.parse(batchBody(ha.hold,HOLD_ALL_REASON,"","HOLD"));
const st1=batchStep(null,'HA|'+batchKey(ha.hold),1000),st2=batchStep(st1.arm,'HA|'+batchKey(ha.hold),1000+1000),stLate=batchStep(st1.arm,'HA|'+batchKey(ha.hold),1000+4001);
process.stdout.write(JSON.stringify({trajCount:ha.trajCount,hold:ha.hold,n:ha.hold.length,
 chunks:batchChunks(ha.hold,2).map(c=>c.length),body,reason:HOLD_ALL_REASON,
 send2:st2.send,sendLate:stLate.send,send1:st1.send}));`)
	var r struct {
		TrajCount int `json:"trajCount"`
		Hold      []string
		N         int
		Chunks    []int
		Body      map[string]any
		Reason    string
		Send2     bool `json:"send2"`
		SendLate  bool `json:"sendLate"`
		Send1     bool `json:"send1"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	// newest of each stays out: A00003, B00003, C00001. E-GONE is not in AtGOV.
	if r.TrajCount != 3 || r.N != 5 || strings.Join(r.Hold, ",") != "E-A00001,E-A00002,E-A00004,E-B00001,E-B00002" {
		t.Fatalf("holdAllPlan = trajCount %d hold %v", r.TrajCount, r.Hold)
	}
	if strings.Contains(strings.Join(r.Hold, ","), "E-A00003") || strings.Contains(strings.Join(r.Hold, ","), "E-C00001") || strings.Contains(strings.Join(r.Hold, ","), "E-GONE") {
		t.Fatalf("newest or absent members must not be held: %v", r.Hold)
	}
	if fmt.Sprint(r.Chunks) != "[2 2 1]" {
		t.Fatalf("chunks = %v want cap-sized [2 2 1]", r.Chunks)
	}
	if r.Body["decision"] != "HOLD" || r.Body["confirm_count"] != float64(5) || r.Body["reason"] != r.Reason ||
		r.Reason != "Trajectory HUMAN_FINAL hold-all: keep newest of each, hold rest" {
		t.Fatalf("hold-all body/reason = %+v %q", r.Body, r.Reason)
	}
	if r.Send1 || !r.Send2 || r.SendLate {
		t.Fatalf("two-press: first=%v second=%v late=%v", r.Send1, r.Send2, r.SendLate)
	}
}
