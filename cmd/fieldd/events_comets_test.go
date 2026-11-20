package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// /events.json is a read-only summary of the ledger: GET only, open on
// loopback, bearer token off loopback, and reading never appends events.
func TestEventsRouteReadOnly(t *testing.T) {
	rt, id, _ := govCandidate(t)
	params, _ := json.Marshal(DecideParams{EmergIONID: id, Decision: "APPROVE", Reason: "events route proof"})
	if resp := dispatch(rt, Request{Method: "decide", Params: params}); resp.Error != "" {
		t.Fatalf("decide: %s", resp.Error)
	}
	before, _ := rt.Status()
	do := func(h http.Handler, method, token string) *http.Response {
		req := httptest.NewRequest(method, "/events.json?limit=10", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}
	loop := newHTTPMux(rt, testToken, true)
	resp := do(loop, http.MethodGet, "")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d %s", resp.StatusCode, body)
	}
	var got struct {
		Total  int `json:"total"`
		Events []struct {
			Seq        int    `json:"seq"`
			Kind       string `json:"kind"`
			EmergIONID string `json:"emergion_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != before.Events || len(got.Events) != before.Events {
		t.Fatalf("events = %d/%d want %d", len(got.Events), got.Total, before.Events)
	}
	kinds := []string{}
	for i, e := range got.Events {
		if e.Seq != i+1 || e.EmergIONID != id {
			t.Fatalf("event %d = %+v", i, e)
		}
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "CANDIDATE_CAPTURED,HUMAN_FINAL_DECISION,REG_ACCEPTED" {
		t.Fatalf("kinds = %v", kinds)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if r := do(loop, m, testToken); r.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d want 405", m, r.StatusCode)
		}
	}
	remote := newHTTPMux(rt, testToken, false)
	if r := do(remote, http.MethodGet, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("off-loopback without token = %d want 401", r.StatusCode)
	}
	after, _ := rt.Status()
	if after.Events != before.Events || after.TipHash != before.TipHash {
		t.Fatal("reading /events.json changed the ledger")
	}
}

func block(t *testing.T, src, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)// <` + name + `>.*?// </` + name + `>`).FindString(src)
	if m == "" {
		t.Fatalf("block %s not found", name)
	}
	return m
}

func runNode(t *testing.T, script string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v %s", err, out)
	}
	return string(out)
}

// Comets come only from ledger events: no random spawn anywhere in the page,
// one launch per new event, replay limited to the most recent events, and
// each comet's entry is derived from its event id.
func TestLivingUniverseCometsComeFromEventsOnly(t *testing.T) {
	src := livingUniverseHTML(t)
	if strings.Contains(src, "Math.random") {
		t.Fatal("page must not use Math.random: comets and layout derive from data")
	}
	if strings.Contains(src, "drops") {
		t.Fatal("decorative droplets must be removed")
	}
	if n := strings.Count(src, "launch("); n != 2 {
		t.Fatalf("launch( appears %d times; want the definition plus one call fed by cometEvents", n)
	}
	if !strings.Contains(src, "fresh=cometEvents(list,lastSeq,20)") || !strings.Contains(src, "fresh.forEach((e,i)=>launch(e,") {
		t.Fatal("comets must be launched only from cometEvents(/events.json)")
	}
	if !strings.Contains(src, "getJSON('/events.json?limit=60')") {
		t.Fatal("comets must read /events.json")
	}
	nodeSet := block(t, src, "node-set")
	comet := block(t, src, "comet-source")
	out := runNode(t, nodeSet+"\n"+comet+`
const evs=[];for(let i=1;i<=30;i++)evs.push({seq:i,id:'ev'+i,emergion_id:'E-'+i});
const first=cometEvents(evs,-1,20),next=cometEvents(evs.concat([{seq:31,id:'ev31'}]),30,20),none=cometEvents([],-1,20),same=cometEvents(evs,30,20);
const a=cometEntry({id:'ev7'}),b=cometEntry({id:'ev7'});
process.stdout.write([first.length,first[0].seq,first[19].seq,next.length,next[0].seq,none.length,same.length,a.ang===b.ang&&a.arc===b.arc].join(','));`)
	if out != "20,11,30,1,31,0,0,true" {
		t.Fatalf("cometEvents/cometEntry = %s", out)
	}
}

// Every IDN renders as exactly one node, even when the same record arrives
// twice from the engine or sits in both REG-accepted and AtGOV inputs; labels
// use the shortest prefix that keeps every IDN distinct.
func TestLivingUniverseNodesUniqueByIDN(t *testing.T) {
	src := livingUniverseHTML(t)
	nodeSet := block(t, src, "node-set")
	out := runNode(t, nodeSet+`
const field={Accepted:{"E-aaaaaaaaaaaa01":{i:"E-aaaaaaaaaaaa01"},"E-aaaaaaaaaaaa02":{i:"E-aaaaaaaaaaaa02"}},AtGOV:{"E-aaaaaaaaaaaa01":{},"E-bb":{}},Held:{"E-cc":{m:{h:"x"}}},Returned:{"E-dd":{m:{h:"x"}}}};
const proj={records:[{id:"E-aaaaaaaaaaaa01",facets:["GRANT_FUNDING"],kin_root:"E-aaaaaaaaaaaa02"},{id:"E-aaaaaaaaaaaa01",facets:["PATENT_IP"],kin_root:"E-aaaaaaaaaaaa02"},{id:"E-aaaaaaaaaaaa02",facets:[],kin_root:"E-aaaaaaaaaaaa02"}],
 assemblies:[{id:"S",links:[{from:"E-aaaaaaaaaaaa01",to:"E-aaaaaaaaaaaa02",kind:"COMPOSITION_KIN"},{from:"E-aaaaaaaaaaaa01",to:"E-aaaaaaaaaaaa02",kind:"COMPOSITION_KIN"}]}]};
const ns=nodeSet(field,proj,["GRANT_FUNDING","PATENT_IP"]),ids=ns.nodes.map(n=>n.id),one=ns.byID.get("E-aaaaaaaaaaaa01");
const n=shortLen(ids),shorts=new Set(ids.map(i=>i.slice(0,n)));
process.stdout.write([ids.length,new Set(ids).size,one.kind,one.facets.join('+'),ns.links.length,shorts.size===new Set(ids).size,n,JSON.stringify(sameSource(field))].join(','));`)
	if out != `3,3,accepted,GRANT_FUNDING+PATENT_IP,2,true,16,[["E-cc","E-dd"]]` {
		t.Fatalf("nodeSet = %s", out)
	}
}

// Labels show the IDN once: the name line never repeats the IDN, and the page
// never scrolls sideways on a phone.
func TestLivingUniverseLabelsShowIDNOnce(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{"overflow-x:hidden", "lines=wrap2(String(nameOf(n))", "renderQueue()", "govP.length<=400"} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	out := runNode(t, block(t, src, "node-set")+`
const a={id:"E-BBFF45DB435E0D39",label:"E-BBFF45DB435E0D39",kind:"accepted",em:{m:{s:"E-BBFF45DB435E0D39"}}};
const b={id:"E-1",label:"E-1",kind:"pending",em:{m:{s:"execution observation"}}};
const c={id:"E-2",label:"Skyhook Runtime",kind:"accepted",em:{}};
process.stdout.write([nameOf(a),nameOf(b),nameOf(c)].join("|"));`)
	if out != "REG-accepted record|execution observation|Skyhook Runtime" {
		t.Fatalf("nameOf = %q", out)
	}
}

// The layout emerges from the data: no preset ring or polyhedron. Positions are
// deterministic, facet attractors exist only for facets records carry, each
// AtGOV record sits in a haze around the accepted record it references, facet
// edges exist only where facets share relationships, and AtGOV points are not
// concentrated in a band (radius std/mean > 0.35).
func TestLivingUniverseLayoutEmergesFromData(t *testing.T) {
	src := livingUniverseHTML(t)
	nodeSet := block(t, src, "node-set")
	for _, bad := range []string{"facetOrder.indexOf", "/nF)*Math.PI*2", "dodeca"} {
		if strings.Contains(nodeSet, bad) {
			t.Fatalf("node-set imposes preset geometry: %q", bad)
		}
	}
	out := runNode(t, nodeSet+`
const acc={},gov={},recs=[],ids=[];
for(let i=0;i<40;i++){const id="E-"+(0xa000+i).toString(16).toUpperCase()+"00000000";ids.push(id);acc[id]={i:id,r:i===5?{kin_emergion:ids[1]}:{}};recs.push({id,facets:i<6?["PATENT_IP"]:i<9?["PATENT_IP","GRANT_FUNDING"]:[],kin_root:id})}
for(let i=0;i<2000;i++){const id="E-"+(0xb0000+i).toString(16).toUpperCase()+"0000";const t=ids[i%31];gov[id]=i%10===9?{m:{s:"observation"}}:{r:{target_emergion:t},m:{s:"comparison: TARGET_EMERGION:"+t}}}
const f={Accepted:acc,AtGOV:gov},p={records:recs};
const a=nodeSet(f,p,["FIELD_COMMAND","PATENT_IP","GRANT_FUNDING"]),b=nodeSet(f,p,["FIELD_COMMAND","PATENT_IP","GRANT_FUNDING"]);
const same=a.nodes.every((n,i)=>n.x===b.nodes[i].x&&n.y===b.nodes[i].y&&n.z===b.nodes[i].z);
const pend=a.nodes.filter(n=>n.kind==="pending"),near=pend.filter(n=>n.target).every(n=>Math.hypot(n.x-n.target.x,n.y-n.target.y,n.z-n.target.z)<1);
const rs=pend.map(n=>Math.hypot(n.x,n.z)),m=rs.reduce((x,y)=>x+y,0)/rs.length,sd=Math.sqrt(rs.reduce((x,y)=>x+(y-m)*(y-m),0)/rs.length);
const fe=a.anchorEdges.map(e=>[e.a.facet,e.b.facet].sort().join("+")+":"+e.w).join(";");
process.stdout.write([same,a.anchors.map(x=>x.facet).sort().join("+"),fe,a.stats.govLinked,a.stats.linkedAcc,near,sd/m>0.35,a.threads.length].join(","));`)
	if out != "true,GRANT_FUNDING+PATENT_IP,GRANT_FUNDING+PATENT_IP:3,1800,2,true,true,31" {
		t.Fatalf("layout = %s", out)
	}
}
