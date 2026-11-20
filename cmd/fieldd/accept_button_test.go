package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/store"
)

// The /living Accept button posts exactly the body built by the page to the
// existing /decide route. Without a token it is refused and nothing changes;
// with the token it records the same HUMAN_FINAL decision + REG acceptance
// as `field decide`, and the ledger and evidence still verify.
func TestLivingAcceptButtonRecordsHumanFinal(t *testing.T) {
	rt, id, statePath := govCandidate(t)
	src := livingUniverseHTML(t)
	body := runNode(t, block(t, src, "governed-accept")+`
process.stdout.write(acceptBody(`+jsonString(id)+`,"operator accepted from /living"));`)
	mux := newHTTPMux(rt, testToken, true)
	post := func(token string) *http.Response {
		req := httptest.NewRequest(http.MethodPost, "/decide", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Result()
	}
	for _, tok := range []string{"", "wrong-token"} {
		if r := post(tok); r.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q: status %d want 401", tok, r.StatusCode)
		}
		if a, g := acceptedCount(t, rt); a != 0 || g != 1 {
			t.Fatalf("state changed without token: accepted=%d at_gov=%d", a, g)
		}
	}
	r := post(testToken)
	out, _ := io.ReadAll(r.Body)
	var resp Response
	_ = json.Unmarshal(out, &resp)
	if r.StatusCode != http.StatusOK || resp.Error != "" {
		t.Fatalf("accept: %d %s", r.StatusCode, out)
	}
	if a, g := acceptedCount(t, rt); a != 1 || g != 0 {
		t.Fatalf("after accept: accepted=%d at_gov=%d want 1,0", a, g)
	}

	s, err := store.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	if strings.Join(types, "") != "CDR" {
		t.Fatalf("ledger = %v want C,D,R (capture, HUMAN_FINAL decision, REG accept)", types)
	}
	d, reg := events[1].Decision, events[2].REG
	if d == nil || d.Authority != "HUMAN_FINAL" || d.Decision != "APPROVE" || d.EmergIONID != id || d.Reason != "operator accepted from /living" {
		t.Fatalf("decision receipt = %+v", d)
	}
	if reg == nil || reg.EmergIONID != id || reg.DecisionID != events[1].ID {
		t.Fatalf("REG receipt = %+v want decision %s", reg, events[1].ID)
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatalf("rebuild after accept: %v", err)
	}
	if _, ok := st.Accepted[id]; !ok {
		t.Fatal("record not REG-accepted after rebuild")
	}
	n := len(events)
	tip, _ := store.LedgerTip(events)
	if err := store.CheckExpectedTip(events, tip, &n); err != nil {
		t.Fatalf("tip check: %v", err)
	}
	if _, err := s.VerifyEvidence(); err != nil {
		t.Fatalf("evidence verify: %v", err)
	}
}

func jsonString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
