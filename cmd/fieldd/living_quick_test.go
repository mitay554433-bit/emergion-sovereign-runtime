package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// The restored page must keep the authority banner and the verified label.
func TestLivingUniversePageKeepsAuthorityBanner(t *testing.T) {
	b, err := os.ReadFile("living-universe.html")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{"COSL + REG remain authoritative", "VERIFIED REFERENCES", "/field.json"} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
}
