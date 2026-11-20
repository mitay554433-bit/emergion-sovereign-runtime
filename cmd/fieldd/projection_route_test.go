package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// /projection.json exposes the projection engine's linked view read-only:
// GET only, open on loopback, bearer token off loopback, and reads never
// change the event ledger or create projection files.
func TestProjectionRouteReadOnly(t *testing.T) {
	rt, id, statePath := govCandidate(t)
	params, _ := json.Marshal(DecideParams{EmergIONID: id, Decision: "APPROVE", Reason: "projection route proof"})
	if resp := dispatch(rt, Request{Method: "decide", Params: params}); resp.Error != "" {
		t.Fatalf("decide: %s", resp.Error)
	}
	before, err := rt.Status()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	do := func(h http.Handler, method, token string) *http.Response {
		req := httptest.NewRequest(method, "/projection.json", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}

	loop := newHTTPMux(rt, testToken, true)
	resp := do(loop, http.MethodGet, "")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d %s", resp.StatusCode, body)
	}
	var got struct {
		Authority   string `json:"authority"`
		FacetGroups []struct {
			Facet   string   `json:"facet"`
			Members []string `json:"members"`
		} `json:"facet_groups"`
		Records []struct {
			ID      string `json:"id"`
			KinRoot string `json:"kin_root"`
		} `json:"records"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Authority != "COSL + REG remain authoritative · visualization only" || len(got.FacetGroups) != 12 {
		t.Fatalf("unexpected projection: %s", body)
	}
	if len(got.Records) != 1 || got.Records[0].ID != id || got.Records[0].KinRoot != id {
		t.Fatalf("records = %+v want accepted %s with kin root itself", got.Records, id)
	}
	seen := map[string]bool{}
	for _, r := range got.Records {
		if seen[r.ID] {
			t.Fatalf("/projection.json repeats IDN %s", r.ID)
		}
		seen[r.ID] = true
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if r := do(loop, m, testToken); r.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d want 405", m, r.StatusCode)
		}
	}
	remote := newHTTPMux(rt, testToken, false)
	if r := do(remote, http.MethodGet, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("off-loopback without token = %d want 401", r.StatusCode)
	}
	if r := do(remote, http.MethodGet, testToken); r.StatusCode != http.StatusOK {
		t.Fatalf("off-loopback with token = %d want 200", r.StatusCode)
	}
	after, err := rt.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Events != before.Events || after.TipHash != before.TipHash || after.Accepted != before.Accepted || after.AtGOV != before.AtGOV {
		t.Fatalf("status changed by reads: %+v -> %+v", before, after)
	}
	if _, err := os.Stat(filepath.Join(dir, "outputs")); !os.IsNotExist(err) {
		t.Fatalf("projection route wrote outputs: %v", err)
	}
	_ = statePath
}
