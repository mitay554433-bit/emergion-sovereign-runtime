package main

import (
	"strings"
	"testing"
)

// SOURCE records with no REG facet cluster around a derived SOURCE topic
// attractor (from /projection.json derived_facets, or REL source_topic when
// the projection is unavailable). REG facets still win, shared manifest
// related ids only add weak springs to hidden hubs (never drawn, not KIN),
// and the layout stays deterministic.
func TestLivingUniverseGroupsSourceByTopic(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, block(t, src, "node-set")+`
const acc={},recs=[];
const add=(id,r,facets,der)=>{acc[id]={i:id,r,e:{m:{f:facets}}};recs.push({id,facets,derived_facets:der,kin_root:id})};
for(let i=0;i<6;i++)add("E-a"+i,{source_kind:"SOURCE_UPLOAD",source_topic:"inventions-ip"},[],["SOURCE_TOPIC:inventions-ip"]);
for(let i=0;i<4;i++)add("E-b"+i,{source_kind:"SOURCE_UPLOAD",source_topic:"grants-funding"},[],["SOURCE_TOPIC:grants-funding"]);
add("E-c0",{source_kind:"SOURCE_UPLOAD",source_topic:"investor"},["PATENT_IP"],[]);
add("E-d0",{},[],[]);
const f={Accepted:acc,AtGOV:{}},p={records:recs,source_ref_groups:[{ref:"TR-1",members:["E-a0","E-b0"]}]};
const a=nodeSet(f,p,[]),b=nodeSet(f,p,[]);
const same=a.nodes.every((n,i)=>n.x===b.nodes[i].x&&n.z===b.nodes[i].z);
const anchors=a.anchors.map(x=>x.facet+":"+x.members).sort().join("+");
const un=a.nodes.filter(n=>n.kind==="accepted"&&!n.facets.length).map(n=>n.id).join("+");
const d=(x,y)=>Math.hypot(x.x-y.x,x.y-y.y,x.z-y.z),A=a.anchors.find(x=>x.facet==="SOURCE_TOPIC:inventions-ip"),B=a.anchors.find(x=>x.facet==="SOURCE_TOPIC:grants-funding");
const tight=a.nodes.filter(n=>n.id.startsWith("E-a")).every(n=>d(n,A)<d(n,B));
const fb=nodeSet(f,null,[]).anchors.map(x=>x.facet).sort().join("+");
process.stdout.write([same,anchors,un,tight,a.byID.get("E-a0").derived.join(),fb].join(","));`)
	want := "true,PATENT_IP:1+SOURCE_TOPIC:grants-funding:4+SOURCE_TOPIC:inventions-ip:6,E-d0,true,SOURCE_TOPIC:inventions-ip,PATENT_IP+SOURCE_TOPIC:grants-funding+SOURCE_TOPIC:inventions-ip"
	if out != want {
		t.Fatalf("topic layout = %s\nwant          %s", out, want)
	}
}

// The facet list names derived topics readably and marks them as SOURCE
// topics; the palette stays carbon/royal blue with SOURCE silver (no cyan).
func TestLivingUniverseSourceTopicNavAndPalette(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		"FACET_LABEL[g.facet]=g.label",
		"accepted · '+derivedSub(g)",
		"derivedNote(n)+', not a REG facet)",
		"return 'SOURCE topic'",
		"[/SOURCE/,'source','#c9d1e0']",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	for _, bad := range []string{"cyan", "#0ff", "#00ffff", "teal"} {
		if strings.Contains(strings.ToLower(strings.ReplaceAll(src, "no cyan/teal", "")), bad) {
			t.Fatalf("palette uses %q", bad)
		}
	}
}
