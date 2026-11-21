package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Auto-accept policy cell and the Policy button read the same
// /auto/status object. A failed poll keeps the last good status (so the cell
// never shows "—" next to a "Policy ON" button) and says so in the auto line.
func TestLivingAutoPolicyCellMatchesButtonAfterFailedPoll(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, `
const els={};const mk=()=>({textContent:'',dataset:{},classList:{toggle(){},add(){},remove(){}},value:''});
const $=id=>els[id]||(els[id]=mk());const document={activeElement:null};
function setText(id,v){$(id).textContent=v}
const fmt=n=>Number(n).toLocaleString('en-US');
let next=null;async function getJSON(u){if(next instanceof Error)throw next;return next}
async function autoPost(){return null}function refresh(){}function refreshAfterDecision(){return Promise.resolve()}function select(){}
function el(){return mk()}function savedToken(){return ''}
`+block(t, src, "governed-auto")+`
(async()=>{
 const r={};
 next=new Error('/auto/status HTTP 500');await pollAuto();r.never=[$('auPol').textContent,$('auLine').textContent];
 next={policy:{on:true,effective:true,auto_accepted:404,auto_held:6,waiting:0},worker:{on:true,mode:'local',insight_records:58}};
 await pollAuto();r.ok=[$('auPol').textContent,$('auPolB').textContent,$('auAcc').textContent];
 next=new Error('Failed to fetch');await pollAuto();r.fail=[$('auPol').textContent,$('auPolB').textContent,$('auLine').textContent];
 next={policy:{on:false,effective:false},worker:{on:false}};await pollAuto();r.off=[$('auPol').textContent,$('auPolB').textContent,$('auLine').textContent];
 process.stdout.write(JSON.stringify(r));
})();`)
	var r struct{ Never, Ok, Fail, Off []string }
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Never[0] != "—" || !strings.Contains(r.Never[1], "auto status unavailable") {
		t.Fatalf("never loaded: %v", r.Never)
	}
	if r.Ok[0] != "ON" || r.Ok[1] != "Policy ON · turn off" || r.Ok[2] != "404" {
		t.Fatalf("loaded: %v", r.Ok)
	}
	if r.Fail[0] != "ON" || r.Fail[1] != "Policy ON · turn off" || !strings.Contains(r.Fail[2], "last refresh failed") {
		t.Fatalf("failed poll must keep cell and button in step: %v", r.Fail)
	}
	if r.Off[0] != "OFF" || r.Off[1] != "Policy OFF · turn on" || strings.Contains(r.Off[2], "failed") {
		t.Fatalf("recovered: %v", r.Off)
	}
}

// Topic tiles come before the REG facet tiles in the facet row, and the REG
// facet tiles are labelled as REG facets.
func TestLivingNavPutsTopicTilesBeforeRegFacets(t *testing.T) {
	src := livingUniverseHTML(t)
	b := block(t, src, "nav-tiles")
	for _, want := range []string{"regItems.push(", "accepted · REG facet", "sub:topicSub(g)", "items.push(...regItems)"} {
		if !strings.Contains(b, want) {
			t.Fatalf("nav-tiles block missing %q", want)
		}
	}
	if strings.Index(b, "sub:topicSub(g)") > strings.Index(b, "items.push(...regItems)") {
		t.Fatal("topic tiles must be pushed before the REG facet tiles")
	}
	if !strings.Contains(src, "g.source_uploads||0") || !strings.Contains(src, "Temporal states · wave file") {
		t.Fatal("uploads count or temporal label missing")
	}
}
