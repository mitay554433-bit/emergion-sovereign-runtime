package main

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// /living is a read-only alias of /living-universe: same bytes, GET only,
// open on loopback, bearer token required off loopback.
func TestLivingAliasServesLivingUniverse(t *testing.T) {
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	get := func(h http.Handler, method, path, token string) (*http.Response, string) {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		b, _ := io.ReadAll(rec.Result().Body)
		return rec.Result(), string(b)
	}

	loop := newHTTPMux(nil, testToken, true)
	r1, page := get(loop, http.MethodGet, "/living-universe", "")
	r2, alias := get(loop, http.MethodGet, "/living", "")
	if r1.StatusCode != http.StatusOK || r2.StatusCode != http.StatusOK {
		t.Fatalf("GET /living-universe=%d /living=%d want 200/200", r1.StatusCode, r2.StatusCode)
	}
	if alias != page || !strings.Contains(alias, "COSL + REG remain authoritative") {
		t.Fatal("/living does not serve the Living Universe page")
	}
	if ct := r2.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("/living Content-Type = %q", ct)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if r, _ := get(loop, m, "/living", ""); r.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s /living = %d want 405", m, r.StatusCode)
		}
	}
	remote := newHTTPMux(nil, testToken, false)
	if r, _ := get(remote, http.MethodGet, "/living", ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("off-loopback /living without token = %d want 401", r.StatusCode)
	}
	if r, _ := get(remote, http.MethodGet, "/living", testToken); r.StatusCode != http.StatusOK {
		t.Fatalf("off-loopback /living with token = %d want 200", r.StatusCode)
	}
}

func livingUniverseHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("living-universe.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The page keeps the authority banner, reads only the read-only projection
// routes, and never posts to a mutating route.
func TestLivingUniversePageIsReadOnlyProjection(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{"COSL + REG remain authoritative", "visualization only", "/field.json", "/projection.json", "/real_temporal_wave.json", "Verified references", "Pending references (AtGOV)", "NOT_FOUND_YET"} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	for _, banned := range []string{"/decide", "/authorize", "method:'POST'", "method:\"POST\"", "localStorage", "parent_child", "hierarchy"} {
		if strings.Contains(src, banned) {
			t.Fatalf("living-universe.html must not contain %q", banned)
		}
	}
}

// Verified references count only links with both ends REG-accepted; links
// touching an AtGOV record are counted separately as Pending references (AtGOV).
func TestLivingUniverseVerifiedExcludesPending(t *testing.T) {
	src := livingUniverseHTML(t)
	fn := regexp.MustCompile(`function refCounts\(es\)\{[^\n]*\}`).FindString(src)
	if fn == "" || !strings.Contains(fn, "e.a.kind==='accepted'&&e.b.kind==='accepted')verified++;else pending++") {
		t.Fatalf("refCounts must count verified only when both ends are accepted: %q", fn)
	}
	for _, want := range []string{"const rc=refCounts(relRefs(field))", "setText('sRef',field?fmt(rc.verified):'—')", "setText('sPend',field?fmt(rc.pending):'—')"} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Log("node not installed; static checks only")
		return
	}
	rel := regexp.MustCompile(`function relRefs\(f\)\{[\s\S]*?\n return out\}`).FindString(src)
	if rel == "" {
		t.Fatal("relRefs(f) not found")
	}
	script := rel + "\n" + fn + `
const f={Accepted:{"E-a1":{r:{}},"E-a2":{r:{kin_emergion:"E-a1",origin:"E-a1"}},"E-a3":{r:{capability_providers:"p:E-a1,q:E-c1",kin_emergion:"E-ff"}}},
 AtGOV:{"E-c1":{r:{kin_emergion:"E-a1"}},"E-c2":{r:{transition_emergion:"E-c1"}}}};
const r=refCounts(relRefs(f));process.stdout.write(r.verified+","+r.pending);`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v %s", err, out)
	}
	// verified: a2->a1 KIN, a2->a1 ORIGIN, a3->a1 CAPABILITY = 3
	// pending: a3->c1 CAPABILITY, c1->a1 KIN, c2->c1 SUCCESSOR = 3 (a3->ff is outside the FIELD)
	if string(out) != "3,3" {
		t.Fatalf("verified,pending = %s want 3,3", out)
	}
}

// Owner rule: no cyan/teal anywhere. Every literal colour in the page must sit
// outside hue 165-200 unless it is nearly grey (saturation <= 0.3).
func TestLivingUniverseHasNoCyan(t *testing.T) {
	src := livingUniverseHTML(t)
	check := func(lit string, r, g, b float64) {
		mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
		if mx == 0 || mx == mn {
			return
		}
		sat := (mx - mn) / mx
		var h float64
		switch mx {
		case r:
			h = math.Mod((g-b)/(mx-mn), 6)
		case g:
			h = (b-r)/(mx-mn) + 2
		default:
			h = (r-g)/(mx-mn) + 4
		}
		h *= 60
		if h < 0 {
			h += 360
		}
		if h >= 165 && h <= 200 && sat > 0.3 {
			t.Errorf("cyan colour %s (hue %.0f, sat %.2f)", lit, h, sat)
		}
	}
	for _, m := range regexp.MustCompile(`rgba?\((\d+),\s*(\d+),\s*(\d+)`).FindAllStringSubmatch(src, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		check(m[0], float64(r), float64(g), float64(b))
	}
	for _, m := range regexp.MustCompile(`#([0-9a-fA-F]{6})\b`).FindAllStringSubmatch(src, -1) {
		v, _ := strconv.ParseUint(m[1], 16, 32)
		check(m[0], float64(v>>16&255), float64(v>>8&255), float64(v&255))
	}
	for _, m := range regexp.MustCompile(`'(\d+),(\d+),(\d+)'`).FindAllStringSubmatch(src, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		check(m[0], float64(r), float64(g), float64(b))
	}
	// The kind palette as rendered: every kind colour (and the unknown-kind
	// fallback) stays out of the cyan band and inside the owner's family.
	out := runNode(t, block(t, src, "node-set")+`
const ks=["SOURCE_FILE","CAPABILITY_PROVIDER","PROJECTION","EXECUTION_OBSERVATION","GOVERNED_EVIDENCE","IMPLEMENTATION","COMPARISON","","ATGOV"];
process.stdout.write(ks.map(k=>kindColor(k)).join(";"));`)
	want := "#c9d1e0;#2f5bff;#7f9cff;#5b7bd6;#4b4fd8;#4b4fd8;#ffffff;#ffffff;rgba(140,155,190,0.35)"
	if out != want {
		t.Fatalf("kind palette = %s want %s", out, want)
	}
	for _, c := range strings.Split(out, ";") {
		if m := regexp.MustCompile(`^#([0-9a-f]{6})$`).FindStringSubmatch(c); m != nil {
			v, _ := strconv.ParseUint(m[1], 16, 32)
			check(c, float64(v>>16&255), float64(v>>8&255), float64(v&255))
		} else if m := regexp.MustCompile(`rgba\((\d+),(\d+),(\d+)`).FindStringSubmatch(c); m != nil {
			r, _ := strconv.Atoi(m[1])
			g, _ := strconv.Atoi(m[2])
			b, _ := strconv.Atoi(m[3])
			check(c, float64(r), float64(g), float64(b))
		} else {
			t.Fatalf("unexpected kind colour %q", c)
		}
	}
}
