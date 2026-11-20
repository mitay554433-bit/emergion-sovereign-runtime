package main

import (
	"strings"
	"testing"
)

// Derived kind/title groups from /projection.json cluster like SOURCE
// topics, and a record with no REG facet and a derived group is not left
// unassigned. The layout stays deterministic.
func TestLivingUniverseGroupsDerivedTopics(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, block(t, src, "node-set")+`
const acc={},recs=[];
const add=(id,r,facets,der,src)=>{acc[id]={i:id,r,e:{m:{f:facets}}};recs.push({id,facets,derived_facets:der,facet_source:src,kin_root:id})};
for(let i=0;i<5;i++)add("E-p"+i,{source_kind:"SOURCE"},[],["SOURCE_TOPIC:inventions-ip"],"derived_keyword");
for(let i=0;i<3;i++)add("E-k"+i,{source_kind:"PROGRAM"},[],["SOURCE_TOPIC:runtime-code-docs"],"derived_kind");
add("E-o0",{},[],["SOURCE_TOPIC:other"],"derived_fallback");
const f={Accepted:acc,AtGOV:{}},p={records:recs,derived_facet_groups:[
  {facet:"SOURCE_TOPIC:inventions-ip",label:"Inventions & IP",topic:"inventions-ip",facet_source:"derived_keyword",members:["E-p0","E-p1","E-p2","E-p3","E-p4"]},
  {facet:"SOURCE_TOPIC:runtime-code-docs",label:"Runtime Code & Docs",topic:"runtime-code-docs",facet_source:"derived_kind",members:["E-k0","E-k1","E-k2"]},
  {facet:"SOURCE_TOPIC:other",label:"Other",topic:"other",facet_source:"derived_fallback",members:["E-o0"]}
]};
const a=nodeSet(f,p,[]),b=nodeSet(f,p,[]);
const same=a.nodes.every((n,i)=>n.x===b.nodes[i].x&&n.z===b.nodes[i].z);
const anchors=a.anchors.map(x=>x.facet+":"+x.members).sort().join("+");
const un=a.nodes.filter(n=>n.kind==="accepted"&&!n.facets.length).map(n=>n.id).join("+");
const d=(x,y)=>Math.hypot(x.x-y.x,x.y-y.y,x.z-y.z),A=a.anchors.find(x=>x.facet==="SOURCE_TOPIC:inventions-ip"),B=a.anchors.find(x=>x.facet==="SOURCE_TOPIC:runtime-code-docs");
const tight=a.nodes.filter(n=>n.id.startsWith("E-p")).every(n=>d(n,A)<d(n,B));
process.stdout.write([same,anchors,un||"none",tight,a.byID.get("E-o0").derived.join()].join(","));`)
	want := "true,SOURCE_TOPIC:inventions-ip:5+SOURCE_TOPIC:other:1+SOURCE_TOPIC:runtime-code-docs:3,none,true,SOURCE_TOPIC:other"
	if out != want {
		t.Fatalf("derived layout = %s\nwant            %s", out, want)
	}
}

func TestLivingUniverseDerivedGroupLabels(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		"function derivedSub(g)",
		"function derivedNote(n)",
		"return 'derived kind'",
		"return 'derived title'",
		"return 'derived group'",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
}
