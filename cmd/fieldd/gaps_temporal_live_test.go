package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /projection.json carries the live temporal view and the real gap totals
// (additive keys); the temporal tip time is the newest ledger event.
func TestProjectionJSONHasLiveTemporalAndGapSummary(t *testing.T) {
	rt, id, _ := govCandidate(t)
	params, _ := json.Marshal(DecideParams{EmergIONID: id, Decision: "APPROVE", Reason: "temporal proof"})
	if resp := dispatch(rt, Request{Method: "decide", Params: params}); resp.Error != "" {
		t.Fatalf("decide: %s", resp.Error)
	}
	mux := newHTTPMux(rt, testToken, true)
	get := func() map[string]json.RawMessage {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projection.json", nil))
		body, _ := io.ReadAll(rec.Result().Body)
		var m map[string]json.RawMessage
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("%v: %s", err, body)
		}
		return m
	}
	m := get()
	var tm struct {
		Source  string         `json:"source"`
		Events  int            `json:"events"`
		States  int            `json:"states"`
		TipAt   string         `json:"tip_at"`
		TipKind string         `json:"tip_kind"`
		ByKind  map[string]int `json:"by_kind"`
	}
	if err := json.Unmarshal(m["temporal"], &tm); err != nil {
		t.Fatalf("temporal: %v %s", err, m["temporal"])
	}
	var events int
	_ = json.Unmarshal(m["events"], &events)
	if tm.Source != "ledger" || tm.Events != events || tm.States < 3 || tm.TipAt == "" || tm.TipKind != "REG_ACCEPTED" {
		t.Fatalf("temporal %+v (events %d)", tm, events)
	}
	var gs struct {
		Records          *int `json:"records"`
		InsightsAccepted *int `json:"insights_accepted"`
	}
	if err := json.Unmarshal(m["gap_summary"], &gs); err != nil || gs.Records == nil || gs.InsightsAccepted == nil {
		t.Fatalf("gap_summary %s", m["gap_summary"])
	}
	if _, ok := m["gaps"]; !ok {
		t.Fatal("gaps key lost")
	}
}

// The Gaps panel sits right after MAXIOM status (before Auto) on every
// layout, the temporal cell is live with the wave file as a secondary label,
// and "last update" is the ledger tip, not the latest capture.
func TestLivingGapsNearTopAndTemporalLive(t *testing.T) {
	src := livingUniverseHTML(t)
	iS, iG, iA := strings.Index(src, `id="statusP"`), strings.Index(src, `id="gapP"`), strings.Index(src, `id="autoP"`)
	if iS < 0 || iG < iS || iA < iG {
		t.Fatalf("gaps panel must follow MAXIOM status and precede Auto: %d %d %d", iS, iG, iA)
	}
	if strings.Count(src, `id="gapP"`) != 1 || !strings.Contains(src, "#statusP{order:3}#gapP{order:3}") {
		t.Fatal("phone layout must keep the gaps panel right after status")
	}
	if !strings.Contains(src, "Last update · ledger tip") || strings.Contains(src, "Last update · latest capture") {
		t.Fatal("last update must be the ledger tip")
	}
	b := block(t, src, "temporal-live")
	for _, bad := range []string{"fetch(", "fillText", "strokeText", "getContext", "cyan", "#0ff", "#22d3ee", "#06b6d4"} {
		if strings.Contains(b, bad) {
			t.Fatalf("temporal block must not use %q", bad)
		}
	}
	g := block(t, src, "gaps-panel")
	out := runNode(t, `const fmt=n=>Number(n).toLocaleString('en-US');`+b+"\n"+strings.Replace(g, "function renderGaps", "function _unused", 1)+`
const live=temporalView({states:1234,records:900,revisions:12,events:13300,last_24h:79,days:40},{compressed_wave:new Array(13)});
const rec=temporalView(null,{compressed_wave:new Array(13),state:'RECORDED'});
const none=temporalView(null,null);
const lu=lastUpdate({tip_at:'2026-11-21T05:38:00Z',tip_kind:'REG_ACCEPTED'},'2026-11-21T04:12:04Z');
const luC=lastUpdate(null,'2026-11-21T04:12:04Z');
const gl=gapSummaryLine({records:0,at_gov:0,accepted:0,held:0,other:0,sources:0,classes:0,insights_accepted:58});
process.stdout.write(JSON.stringify({live,rec,none,lu,luC,gl}));`)
	var r struct {
		Live, Rec, None struct{ Main, Sub, Badge, Detail string }
		Lu, LuC         struct{ Main, Sub string }
		Gl              string
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Live.Main != "1,234" || r.Live.Badge != "live ledger" || !strings.Contains(r.Live.Sub, "wave file 13") || !strings.Contains(r.Live.Detail, "79 in the last 24h") {
		t.Fatalf("live %+v", r.Live)
	}
	if r.Rec.Main != "13" || r.Rec.Badge != "recorded" || r.None.Main != "NOT_FOUND_YET" {
		t.Fatalf("fallbacks %+v %+v", r.Rec, r.None)
	}
	if !strings.Contains(r.Lu.Sub, "reg accepted") || !strings.Contains(r.Lu.Sub, "latest capture") || r.Lu.Main == "" {
		t.Fatalf("last update %+v", r.Lu)
	}
	if !strings.Contains(r.LuC.Sub, "ledger tip time unavailable") {
		t.Fatalf("capture fallback %+v", r.LuC)
	}
	if !strings.Contains(r.Gl, "58 accepted insights read by the gap pass") || !strings.Contains(r.Gl, "0 sources") {
		t.Fatalf("gap summary line %q", r.Gl)
	}
}

func TestPWAShellCacheBumpedForThisDeploy(t *testing.T) {
	if pwaCacheName != "unifusion-shell-v3" || !strings.Contains(pwaSWSource, "'unifusion-shell-v3'") {
		t.Fatalf("cache name %q", pwaCacheName)
	}
	src := livingUniverseHTML(t)
	if !strings.Contains(src, `updateViaCache:"none"`) || !strings.Contains(src, "r.update()") {
		t.Fatal("sw.js must bypass the HTTP cache and check for updates on load")
	}
}
