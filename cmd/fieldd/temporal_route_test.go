package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The Living Universe temporal view reads /real_temporal_wave.json (added in
// PR #30). This pins the route itself: it serves the projection file
// byte-for-byte as JSON, is GET-only, and follows the same read-only access
// rule as the other projection routes (open on loopback, bearer token off
// loopback). It reads only a temporary outputs directory.
func TestRealTemporalWaveRouteServesProjectionReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "outputs"), 0o700); err != nil {
		t.Fatal(err)
	}
	want := `{"frames":[{"t":0,"accepted":1},{"t":1,"accepted":2}]}`
	if err := os.WriteFile(filepath.Join(dir, "outputs", "real_temporal_wave.json"), []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	get := func(h http.Handler, method, token string) *http.Response {
		req := httptest.NewRequest(method, "/real_temporal_wave.json", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}

	loop := newHTTPMux(nil, testToken, true)
	resp := get(loop, http.MethodGet, "")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != want {
		t.Fatalf("loopback GET = %d %q want 200 %q", resp.StatusCode, body, want)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q want application/json", ct)
	}
	if resp := get(loop, http.MethodPost, ""); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d want 405", resp.StatusCode)
	}

	remote := newHTTPMux(nil, testToken, false)
	if resp := get(remote, http.MethodGet, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("off-loopback GET without token = %d want 401", resp.StatusCode)
	}
	if resp := get(remote, http.MethodGet, testToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("off-loopback GET with token = %d want 200", resp.StatusCode)
	}

	// The file on disk is unchanged by reads.
	after, err := os.ReadFile(filepath.Join(dir, "outputs", "real_temporal_wave.json"))
	if err != nil || string(after) != want {
		t.Fatalf("projection file changed by reads: %q %v", after, err)
	}
}
