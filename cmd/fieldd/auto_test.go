package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
)

func autoReq(t *testing.T, mux http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestAutoRoutesDefaultOffAndNeedToken(t *testing.T) {
	rt, _, statePath := govCandidate(t)
	sw := reason.NewSwitchInsighter(reason.ModeLocal)
	autoEng = newAutoEngine(rt, filepath.Join(filepath.Dir(statePath), "work"), sw)
	defer func() { autoEng = nil }()
	mux := newHTTPMux(rt, testToken, true)

	code, st := autoReq(t, mux, http.MethodGet, "/auto/status", "", nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	pol := st["policy"].(map[string]any)
	wk := st["worker"].(map[string]any)
	if pol["on"] != false || wk["on"] != false || wk["mode"] != "local" {
		t.Fatalf("defaults must be OFF/local: %v", st)
	}
	for _, p := range []string{"/auto/policy", "/auto/worker", "/auto/undo"} {
		if code, _ := autoReq(t, mux, http.MethodPost, p, "", map[string]any{"state": "on", "reason": "x"}); code != http.StatusUnauthorized {
			t.Fatalf("%s without token: %d", p, code)
		}
	}
	if code, _ := autoReq(t, mux, http.MethodPost, "/auto/policy", testToken, map[string]any{"state": "on"}); code != http.StatusBadRequest {
		t.Fatalf("policy on without reason must be refused: %d", code)
	}
	code, out := autoReq(t, mux, http.MethodPost, "/auto/policy", testToken, map[string]any{"state": "on", "reason": "Michael approved A01-AUTO-1"})
	if code != 200 || out["switch_id"] == "" {
		t.Fatalf("policy on: %d %v", code, out)
	}
	on := true
	if code, out := autoReq(t, mux, http.MethodPost, "/auto/worker", testToken, map[string]any{"on": on, "mode": "auto"}); code != 200 || out["mode"] != "auto" {
		t.Fatalf("worker: %d %v", code, out)
	}
	if code, _ := autoReq(t, mux, http.MethodPost, "/auto/worker", testToken, map[string]any{"mode": "cloud"}); code != http.StatusBadRequest {
		t.Fatalf("bad mode accepted: %d", code)
	}
	_, st = autoReq(t, mux, http.MethodGet, "/auto/status", "", nil)
	pol = st["policy"].(map[string]any)
	wk = st["worker"].(map[string]any)
	if pol["on"] != true || pol["effective"] != true || wk["on"] != true || wk["mode"] != "auto" {
		t.Fatalf("after switches: %v", st)
	}
	if sw.Mode() != reason.ModeAuto {
		t.Fatalf("reasoner mode not applied: %s", sw.Mode())
	}
	// The HUMAN_FINAL test candidate is not low-risk: one pass leaves it AtGOV.
	autoEng.once(context.Background())
	ps, err := rt.PolicyStatus()
	if err != nil {
		t.Fatal(err)
	}
	if ps.AutoAccepted != 0 || ps.Waiting != 1 {
		t.Fatalf("non-low-risk record was decided: %+v", ps)
	}
}
