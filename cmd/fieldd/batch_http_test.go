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
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// Page helpers: the batch is the first min(N,cap) records by IDN matching the
// chosen filter, the typed count must equal it, and the body is what the
// server's BatchParams decodes.
func TestLivingBatchSelectionAndBody(t *testing.T) {
	src := livingUniverseHTML(t)
	out := runNode(t, block(t, src, "governed-accept")+`
const gov={"E-03":{r:{source_kind:"A",target_emergion:"E-T1"},e:{m:{f:["MEM"],h:"three"}}},
 "E-01":{r:{source_kind:"A"},e:{m:{f:["MEM","VAL"]}},m:{s:"one"}},
 "E-02":{r:{source_kind:"B",target_emergion:"E-T1"},e:{m:{f:["VAL"]}}}};
const r={
 all:batchSelect(gov,batchFilter('all','',null),500),
 tapNone:batchSelect(gov,batchFilter('tap','','ALL'),500).ids,
 tapVal:batchSelect(gov,batchFilter('tap','','VAL'),500).ids,
 cls:batchSelect(gov,{kind:'class',value:'A'},500).ids,
 tgt:batchSelect(gov,{kind:'target',value:'E-T1'},500).ids,
 cap:batchSelect(gov,{kind:'all'},2),
 opts:batchOptions(gov).class,
 ok:batchConfirmed(' 3 ',3),bad:batchConfirmed('2',3),zero:batchConfirmed('0',0),
 title:batchTitle(gov["E-01"],"E-01"),
 body:JSON.parse(batchBody(["E-01"],"why",1))};
process.stdout.write(JSON.stringify(r));`)
	var r struct {
		All struct {
			Total int      `json:"total"`
			IDs   []string `json:"ids"`
		} `json:"all"`
		TapNone, TapVal, Cls, Tgt []string
		Cap                       struct {
			Total int      `json:"total"`
			IDs   []string `json:"ids"`
		} `json:"cap"`
		Opts          [][]any `json:"opts"`
		OK, Bad, Zero bool
		Title         string
		Body          BatchParams
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if strings.Join(r.All.IDs, ",") != "E-01,E-02,E-03" || r.All.Total != 3 {
		t.Fatalf("all = %+v", r.All)
	}
	if len(r.TapNone) != 3 || strings.Join(r.TapVal, ",") != "E-01,E-02" || strings.Join(r.Cls, ",") != "E-01,E-03" || strings.Join(r.Tgt, ",") != "E-02,E-03" {
		t.Fatalf("filters: %s", out)
	}
	if r.Cap.Total != 3 || strings.Join(r.Cap.IDs, ",") != "E-01,E-02" {
		t.Fatalf("cap = %+v", r.Cap)
	}
	if len(r.Opts) != 2 || r.Opts[0][0] != "A" || r.Opts[0][1] != float64(2) {
		t.Fatalf("class options = %v", r.Opts)
	}
	if !r.OK || r.Bad || r.Zero || r.Title != "one" {
		t.Fatalf("confirm/title: %s", out)
	}
	if strings.Join(r.Body.EmergIONIDs, ",") != "E-01" || r.Body.Reason != "why" || r.Body.ConfirmCount != 1 {
		t.Fatalf("body = %+v", r.Body)
	}
}

func TestDecideBatchHumanRoute(t *testing.T) {
	rt, id, statePath := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)
	post := func(token string, p BatchParams) (int, fieldapi.BatchResult) {
		b, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/decide-batch-human", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		out, _ := io.ReadAll(rec.Result().Body)
		var res fieldapi.BatchResult
		_ = json.Unmarshal(out, &res)
		return rec.Code, res
	}
	events := func() int {
		s, err := store.Open(statePath)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		return len(ev)
	}
	good := BatchParams{EmergIONIDs: []string{id}, Reason: "batch from /living", ConfirmCount: 1}
	for _, tok := range []string{"", "wrong-token"} {
		if c, _ := post(tok, good); c != http.StatusUnauthorized {
			t.Fatalf("token %q: %d want 401", tok, c)
		}
	}
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id}, Reason: "x", ConfirmCount: 2}); c != http.StatusBadRequest || !strings.Contains(res.Error, "confirm count") {
		t.Fatalf("count mismatch: %d %+v", c, res)
	}
	old := batchCap
	batchCap = 1
	if c, res := post(testToken, BatchParams{EmergIONIDs: []string{id, "E-0000000000000001"}, Reason: "x", ConfirmCount: 2}); c != http.StatusBadRequest || !strings.Contains(res.Error, "exceeds cap 1") {
		t.Fatalf("cap: %d %+v", c, res)
	}
	batchCap = old
	if events() != 1 {
		t.Fatal("refused batches wrote events")
	}
	req := httptest.NewRequest(http.MethodGet, "/decide-batch-human", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cap":500`) {
		t.Fatalf("GET cap: %d %s", rec.Code, rec.Body.String())
	}

	c, res := post(testToken, good)
	if c != http.StatusOK || res.Accepted != 1 || res.Error != "" || !strings.HasPrefix(res.BatchID, "HFB-") {
		t.Fatalf("accept: %d %+v", c, res)
	}
	s, _ := store.Open(statePath)
	ev, _ := s.Events()
	if len(ev) != 3 || ev[1].Type != "D" || ev[2].Type != "R" || ev[1].Decision.Authority != "HUMAN_FINAL" ||
		ev[1].Decision.Reason != "batch from /living [HUMAN_FINAL batch "+res.BatchID+", 1 records]" || ev[2].REG.DecisionID != ev[1].ID {
		t.Fatalf("receipts: %+v", ev)
	}
	st, err := livefield.Rebuild(ev)
	if err != nil || len(st.Accepted) != 1 || len(st.AtGOV) != 0 {
		t.Fatalf("rebuild: %v", err)
	}
	if _, err := s.VerifyEvidence(); err != nil {
		t.Fatal(err)
	}
	if c, res := post(testToken, good); c != http.StatusOK || res.Accepted != 0 || res.Skipped != 1 || events() != 3 {
		t.Fatalf("re-run: %d %+v", c, res)
	}
}
