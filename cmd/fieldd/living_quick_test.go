package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// Verified references count only links with both ends REG-accepted; any link
// touching an AtGOV record is shown separately as "Pending (AtGOV)".
func TestLivingUniverseVerifiedExcludesPending(t *testing.T) {
	src := livingUniverseHTML(t)
	fn := regexp.MustCompile(`function refCounts\(es\)\{[^\n]*\}`).FindString(src)
	if fn == "" {
		t.Fatal("refCounts(es) not found in living-universe.html")
	}
	if !strings.Contains(fn, "e.a.kind==='accepted'&&e.b.kind==='accepted')verified++;else pending++") {
		t.Fatalf("refCounts must count verified only when both ends are accepted: %s", fn)
	}
	for _, want := range []string{
		"rc=refCounts(edges),ver=rc.verified,pend=rc.pending",
		"$('nRef').textContent=field?fmt(ver):'—'",
		"$('nGovRef').textContent=fmt(pend)",
		"<span>Pending (AtGOV)</span>",
		"<span>Verified references</span>",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}

	// When node is available, execute the real function against a mixed set.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Log("node not installed; static checks only")
		return
	}
	script := fn + `
const A={kind:'accepted'},B={kind:'accepted'},G={kind:'pending'},H={kind:'pending'};
const r=refCounts([{a:A,b:B},{a:A,b:G},{a:G,b:B},{a:G,b:H},{a:B,b:A}]);
process.stdout.write(r.verified+','+r.pending);`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v %s", err, out)
	}
	if string(out) != "2,3" {
		t.Fatalf("refCounts verified,pending = %s want 2,3", out)
	}
}
