package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

const bdcArchonym = "BDC Innovation Funding Package"

type namedUploadReasoner struct{}

func (namedUploadReasoner) Analyze(_ context.Context, in reason.Input) (reason.Result, error) {
	return reason.Result{
		Summary:       "upload " + string(in.Content),
		Archonym:      bdcArchonym,
		Relationships: map[string]string{"source_kind": "SOURCE_UPLOAD", "source_title": bdcArchonym, "source_topic": "grants-funding"},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"bdc_upload"},
		Risk:          "L",
	}, nil
}
func (namedUploadReasoner) Name() string                   { return "bdc-test" }
func (namedUploadReasoner) Version(context.Context) string { return "1" }

// Regression for the phone outage: a ledger holding a REG-accepted sovereign
// and a second SOURCE_UPLOAD whose REG receipt names the same Archonym (as an
// older build could write) must load, and /status, /field.json and
// /projection.json must all answer 200. The owner keeps the name; the second
// record keeps its own id, stays Approved and is reported as a conflict.
func TestArchonymCollisionNeverTakesRoutesDown(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state")
	s, err := store.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	cap := fieldruntime.Runtime{Store: s, Reasoner: namedUploadReasoner{}}
	var ids []string
	for _, body := range []string{"first bdc jpg", "second bdc jpg"} {
		em, _, err := cap.CaptureBytes(context.Background(), "bdc", []byte(body), "governed_source_upload")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, em.IDN)
	}
	// Write APPROVE + REG for both, bypassing the write-time Archonym gate,
	// the way the pre-gate ledger was written.
	for _, id := range ids {
		events, _ := s.Events()
		st, err := livefield.Rebuild(events)
		if err != nil {
			t.Fatal(err)
		}
		d, rc, err := gov.Decide(st.AtGOV[id], gov.Approve, "HUMAN_FINAL", "test")
		if err != nil {
			t.Fatal(err)
		}
		did, err := s.SaveDecision(rc)
		if err != nil {
			t.Fatal(err)
		}
		_, rr, err := reg.Accept(d, did)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveAccepted(rr); err != nil {
			t.Fatal(err)
		}
	}

	rt, err := fieldapi.Open(statePath, namedUploadReasoner{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	mux := newHTTPMux(rt, testToken, true)
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		b, _ := io.ReadAll(rec.Result().Body)
		return rec.Code, string(b)
	}
	for _, p := range []string{"/status", "/field.json", "/projection.json"} {
		code, body := get(p)
		if code != http.StatusOK || strings.Contains(body, `"error"`) || strings.Contains(body, "already belongs") {
			t.Fatalf("%s = %d %s", p, code, body)
		}
	}
	_, body := get("/projection.json")
	var lp struct {
		Records           []struct{ ID string } `json:"records"`
		ArchonymConflicts []struct {
			EmergIONID string `json:"emergion_id"`
			OwnerID    string `json:"owner_id"`
		} `json:"archonym_conflicts"`
	}
	if err := json.Unmarshal([]byte(body), &lp); err != nil {
		t.Fatal(err)
	}
	if len(lp.Records) != 1 || lp.Records[0].ID != ids[0] {
		t.Fatalf("owner must stay the only accepted record: %+v", lp.Records)
	}
	if len(lp.ArchonymConflicts) != 1 || lp.ArchonymConflicts[0].EmergIONID != ids[1] || lp.ArchonymConflicts[0].OwnerID != ids[0] {
		t.Fatalf("conflict not reported with its own id: %s", body)
	}
}
