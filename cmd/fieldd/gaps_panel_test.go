package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// The Gaps panel lists projection gaps by class with the source title and
// gives each AtGOV gap the existing HUMAN_FINAL Accept / Hold buttons. It
// never posts by itself, never draws canvas text and never uses cyan.
func TestLivingGapsPanelListsByClassWithHumanFinalButtons(t *testing.T) {
	src := livingUniverseHTML(t)
	if !strings.Contains(src, `id="gapP"`) || !strings.Contains(src, `id="gaps"`) || !strings.Contains(src, "renderGaps();") {
		t.Fatal("Gaps panel missing or not rendered")
	}
	b := block(t, src, "gaps-panel")
	for _, bad := range []string{"fetch(", "fillText", "strokeText", "getContext", "localStorage", "cyan", "#0ff", "#22d3ee", "#06b6d4"} {
		if strings.Contains(b, bad) {
			t.Fatalf("gaps panel must not use %q", bad)
		}
	}
	css := regexp.MustCompile(`\.gapc\{[^\n]*?\.gapi \.tok\{[^}]*\}`).FindString(src)
	if css == "" || strings.Contains(strings.ToLower(css), "cyan") || !strings.Contains(css, "var(--royal)") || !strings.Contains(css, "#05070d") {
		t.Fatalf("gaps CSS must be carbon black and royal blue: %q", css)
	}
	out := runNode(t, `
class E{constructor(t){this.tag=t;this.className='';this.textContent='';this.children=[]}append(...c){this.children.push(...c)}replaceChildren(){this.children=[]}}
const el=(t,c,x)=>{const e=new E(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
const nodes={gaps:new E('div'),gapS:new E('span')};const $=id=>nodes[id];
function setText(id,v){$(id).textContent=v}
const fmt=n=>Number(n).toLocaleString('en-US');const shortID=id=>String(id).slice(0,12);
let tok='t';function savedToken(){return tok}
function acceptButton(id){return el('button','acc','Accept:'+id)}
function holdButton(id){return el('button','acc hold','Hold:'+id)}
`+b+`
let proj={gaps:[
 {class:"continuity",pivot:"PIVOT_CONTINUITY_RECOVERY",accepted:1,at_gov:1,held:2,items:[
  {id:"E-g1",state:"AtGOV",of:"E-u1",source_title:"Helix brief 1",summary:"Gap (continuity) in Helix brief 1: no backup"},
  {id:"E-g2",state:"REG_ACCEPTED",of:"E-u2",source_title:"Helix brief 2",summary:"s2"}]},
 {class:"proof",pivot:"PIVOT_EVIDENCE_BINDING",accepted:0,at_gov:1,items:[{id:"E-g3",state:"AtGOV",of:"E-u1",source_title:"",summary:"s3"}]}]};
renderGaps();
const flat=n=>[n.textContent,...n.children.map(flat)].join('|');
const rows=nodes.gaps.children.map(flat),head=nodes.gapS.textContent;
tok='';renderGaps();const noTok=nodes.gaps.children.map(flat).join('\n');
proj={gaps:[]};renderGaps();const empty=flat(nodes.gaps);
process.stdout.write(JSON.stringify({head,rows,noTok,empty}));`)
	var r struct {
		Head  string
		Rows  []string
		NoTok string
		Empty string
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	all := strings.Join(r.Rows, "\n")
	if r.Head != "2 AtGOV · 1 accepted" || len(r.Rows) != 5 {
		t.Fatalf("head %q rows %v", r.Head, r.Rows)
	}
	if !strings.HasPrefix(r.Rows[0], "|continuity|PIVOT_CONTINUITY_RECOVERY · 1 AtGOV · 1 accepted · 2 held") || !strings.HasPrefix(r.Rows[3], "|proof|") {
		t.Fatalf("class headers %v", r.Rows)
	}
	if !strings.Contains(r.Rows[1], "Helix brief 1") || !strings.Contains(r.Rows[1], "Accept:E-g1") || !strings.Contains(r.Rows[1], "Hold:E-g1") {
		t.Fatalf("AtGOV gap row lacks title or HUMAN_FINAL buttons: %q", r.Rows[1])
	}
	if strings.Contains(r.Rows[2], "Accept:") || !strings.Contains(r.Rows[2], "ACCEPTED") {
		t.Fatalf("accepted gap must not offer a decision: %q", r.Rows[2])
	}
	if !strings.Contains(r.Rows[4], "E-u1") || !strings.Contains(all, "Accept:E-g3") {
		t.Fatalf("untitled gap falls back to the source id: %q", r.Rows[4])
	}
	if strings.Contains(r.NoTok, "Accept:") || !strings.Contains(r.NoTok, "token needed") {
		t.Fatalf("no token must show no buttons: %q", r.NoTok)
	}
	if !strings.Contains(r.Empty, "NOT_FOUND_YET") {
		t.Fatalf("empty: %q", r.Empty)
	}
}
