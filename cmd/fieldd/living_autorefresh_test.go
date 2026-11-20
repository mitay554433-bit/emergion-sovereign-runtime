package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// /field.json and /projection.json must not be stored by the browser, or a
// successful accept leaves the Living Universe panel on a stale snapshot.
func TestFieldAndProjectionJSONHaveNoStore(t *testing.T) {
	rt, _, _ := govCandidate(t)
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.MkdirAll("outputs", 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.RenderCurrent("outputs"); err != nil {
		t.Fatal(err)
	}

	mux := newHTTPMux(rt, testToken, true)
	for _, path := range []string{"/field.json", "/projection.json"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		resp := rec.Result()
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d %s", path, resp.StatusCode, body)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("%s Cache-Control = %q want no-store", path, cc)
		}
	}
}

// After a successful decide, decided IDs leave the group picker and the
// HUMAN_FINAL / AtGOV list immediately, and the selection moves to the
// largest remaining group. The page also force-refreshes with a cache-bust.
func TestLivingAfterDecisionDropsIDsAndAdvancesGroup(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		"function dropAtGOVIDs(f,ids)",
		"function dropDecidedFromProj(p,ids)",
		"function nextBatchValue(",
		"async function refreshAfterDecision(ids)",
		"await refreshAfterDecision([id])",
		"await refreshAfterDecision(done)",
		"'_='+Date.now()",
		"cache:'no-store'",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}

	out := runNode(t, batchBlocks(t, src)+`
const decided=["E-60092640F2E1BB98"];
const field={AtGOV:{
  "E-60092640F2E1BB98":{r:{source_kind:"EXECUTION_RESULT",target_emergion:"E-0678AAAA"},m:{s:"execution result"}},
  "E-B00001":{r:{source_kind:"COMPARISON",target_emergion:"E-AAAAAA01"}},
  "E-B00002":{r:{source_kind:"COMPARISON",target_emergion:"E-AAAAAA01"}},
  "E-B00003":{r:{source_kind:"COMPARISON",target_emergion:"E-AAAAAA01"}}
},Accepted:{"E-0678AAAA":{e:{m:{f:["DOCS_PROJECTION"]}}},"E-AAAAAA01":{e:{m:{f:["ANALYTICS_FORECAST"]}}}}};
const proj={human_final_queue:[
  {id:"E-60092640F2E1BB98",summary:"execution result"},
  {id:"E-B00001",summary:"b1"},{id:"E-B00002",summary:"b2"},{id:"E-B00003",summary:"b3"}
],trajectories:[]};
const emptyKey=batchIndex(field.AtGOV,field.Accepted,trajNow(proj,field.AtGOV)).groups.find(g=>g.count===1).key;
const f2=dropAtGOVIDs(field,decided),p2=dropDecidedFromProj(proj,decided);
const idx=batchIndex(f2.AtGOV,f2.Accepted,trajNow(p2,f2.AtGOV));
const opts=batchOptions(f2.AtGOV,idx);
const next=nextBatchValue('group',emptyKey,idx,opts);
const kept=nextBatchValue('group',idx.groups[0].key,idx,opts);
const q=(p2.human_final_queue||[]).map(it=>it.id);
process.stdout.write(JSON.stringify({gone:!f2.AtGOV[decided[0]],atgov:Object.keys(f2.AtGOV).sort(),q,next,kept,largest:idx.groups[0].key,groups:idx.groups.map(g=>[g.key,g.count]),emptyGone:!idx.groups.some(g=>g.key===emptyKey)}));`)

	var r struct {
		Gone      bool
		AtGOV     []string `json:"atgov"`
		Q         []string
		Next      string
		Kept      string
		Largest   string
		Groups    [][]any
		EmptyGone bool
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !r.Gone || strings.Contains(strings.Join(r.AtGOV, ","), "E-60092640F2E1BB98") {
		t.Fatalf("decided ID must leave AtGOV: %v", r.AtGOV)
	}
	if fmt.Sprint(r.Q) != "[E-B00001 E-B00002 E-B00003]" {
		t.Fatalf("HUMAN_FINAL / AtGOV list still has decided ID: %v", r.Q)
	}
	if !r.EmptyGone || r.Next == "" || r.Next != r.Largest || r.Kept != r.Largest {
		t.Fatalf("selection should move to the largest remaining group: next=%q largest=%q kept=%q groups=%v", r.Next, r.Largest, r.Kept, r.Groups)
	}
	if len(r.AtGOV) != 3 {
		t.Fatalf("remaining AtGOV = %v", r.AtGOV)
	}
}
