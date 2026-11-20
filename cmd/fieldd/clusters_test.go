package main

import (
	"regexp"
	"strings"
	"testing"
)

// Status tiles must not be confusable: labels are unique, and every label
// that says "pending" or "waiting" also names what it counts (records or
// references), so 19,323 pending references cannot be read as 19,323
// records waiting for HUMAN_FINAL.
func TestLivingUniverseStatusLabelsUnambiguous(t *testing.T) {
	src := livingUniverseHTML(t)
	labels := regexp.MustCompile(`<div class="st"[^>]*><small>([^<]+)</small>`).FindAllStringSubmatch(src, -1)
	if len(labels) < 10 {
		t.Fatalf("found %d status tiles", len(labels))
	}
	seen := map[string]bool{}
	pendingLike := 0
	for _, m := range labels {
		l := strings.ToLower(strings.TrimSpace(m[1]))
		if seen[l] {
			t.Fatalf("duplicate status label %q", m[1])
		}
		seen[l] = true
		if strings.Contains(l, "pending") || strings.Contains(l, "waiting") {
			pendingLike++
			if !strings.Contains(l, "records") && !strings.Contains(l, "references") {
				t.Fatalf("status label %q says pending/waiting without saying records or references", m[1])
			}
		}
	}
	if pendingLike != 2 || !seen["atgov waiting (records)"] || !seen["pending references (atgov)"] {
		t.Fatalf("want exactly the two disambiguated tiles; got %d pending-like", pendingLike)
	}
}

// Hulls come only from emergent groups with >= 3 accepted members, and the
// camera frame for a cluster is its centroid with a bounded zoom. Facet taps,
// nav tiles, empty taps and RESET drive the flight; nothing is labelled.
func TestLivingUniverseClustersAndFlight(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, block(t, src, "node-set")+`
const mk=(id,f,root,x,y,z)=>({id,kind:'accepted',facets:f,root,x,y,z});
const ns=[mk('a',['PATENT_IP'],'a',1,0,0),mk('b',['PATENT_IP'],'a',1.2,0,0),mk('c',['PATENT_IP'],'c',1,0.2,0),mk('d',['GRANT_FUNDING'],'d',-1,0,0),mk('e',['GRANT_FUNDING'],'e',-1,0,0),{id:'p',kind:'pending',facets:['GRANT_FUNDING'],root:'p',x:0,y:0,z:0}];
const cl=clusterGroups(ns),fr=groupFrame(ns.slice(0,3)),none=groupFrame([]);
process.stdout.write([cl.map(c=>c.key+':'+c.members.length).join('|'),fr.tx.toFixed(3),fr.ty.toFixed(3),fr.zoom>=1.2&&fr.zoom<=2.4,none===null].join(','));`)
	if out != "facet:PATENT_IP:3,1.067,0.067,true,true" {
		t.Fatalf("clusters/frame = %s", out)
	}
	for _, want := range []string{
		"else flyToFacet(it.key)",
		"if(filter===A.facet){filter='ALL';renderNav();flyHome()}else flyToFacet(A.facet)",
		"if(!best&&!hev&&!hitFacet&&(filter!=='ALL'||cam.tx||cam.ty||cam.tz)){filter='ALL';renderNav();flyHome()}",
		"$('bFit').onclick=()=>{cam.yaw=-.5;cam.pitch=.42;flyHome();",
		"if(reduced.matches){Object.assign(cam,to);fly=null}",
		"function P(x,y,z){x-=cam.tx||0;",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	hull := src[strings.Index(src, "// soft hulls behind emergent clusters"):strings.Index(src, "// links: thin faint curves")]
	if strings.Contains(hull, "fillText") || strings.Contains(hull, "strokeText") {
		t.Fatal("hulls must not add labels")
	}
}
