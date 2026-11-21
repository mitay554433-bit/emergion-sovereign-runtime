package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// chdirTemp runs the test in an empty directory so outputs/ is private.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

// gatedRender replaces renderField: it counts builds and, while gate is
// non-nil, blocks each build until gate is closed.
type gatedRender struct {
	mu      sync.Mutex
	gate    chan struct{}
	started chan struct{}
	builds  atomic.Int32
}

func installGatedRender(t *testing.T) *gatedRender {
	t.Helper()
	g := &gatedRender{started: make(chan struct{}, 16)}
	prev := renderField
	renderField = func(rt *fieldapi.Runtime, dir string) (proj.Receipt, error) {
		g.builds.Add(1)
		g.mu.Lock()
		gate := g.gate
		g.mu.Unlock()
		g.started <- struct{}{}
		if gate != nil {
			<-gate
		}
		return rt.RenderCurrent(dir)
	}
	t.Cleanup(func() { renderField = prev })
	return g
}

func (g *gatedRender) block() chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gate = make(chan struct{})
	return g.gate
}

func (g *gatedRender) unblock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gate = nil
}

func getTimed(t *testing.T, h http.Handler, path string) (*http.Response, string, time.Duration) {
	t.Helper()
	rec := httptest.NewRecorder()
	t0 := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	took := time.Since(t0)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b), took
}

// /status (and the watchdog's /healthz) must answer promptly while a slow
// /field.json build is running, and concurrent /field.json requests must
// share one build instead of piling up.
func TestStatusStaysResponsiveWhileFieldJSONBuilds(t *testing.T) {
	rt, _, _ := govCandidate(t)
	chdirTemp(t)
	g := installGatedRender(t)
	gate := g.block()
	mux := newHTTPMux(rt, testToken, true)

	var wg sync.WaitGroup
	codes := make([]int, 3)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, _, _ := getTimed(t, mux, "/field.json")
			codes[i] = res.StatusCode
		}(i)
	}
	select {
	case <-g.started:
	case <-time.After(5 * time.Second):
		t.Fatal("field build never started")
	}
	for _, p := range []string{"/status", "/healthz", "/projection.json", "/auto/status"} {
		if p == "/auto/status" && autoEng == nil {
			continue
		}
		res, body, took := getTimed(t, mux, p)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s while field.json builds = %d %s", p, res.StatusCode, body)
		}
		if took > time.Second {
			t.Fatalf("%s took %s while field.json builds; must stay responsive", p, took)
		}
	}
	close(gate)
	g.unblock()
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("field.json request %d = %d", i, c)
		}
	}
	if n := g.builds.Load(); n != 1 {
		t.Fatalf("concurrent field.json requests ran %d builds, want 1", n)
	}
}

// Repeat requests are served from the cache without rebuilding, the body is
// exactly the projection RenderCurrent writes (same JSON shape), and a
// ledger change (a HUMAN_FINAL decision) is picked up by the next request.
func TestFieldJSONCachedUntilLedgerChanges(t *testing.T) {
	rt, id, _ := govCandidate(t)
	chdirTemp(t)
	g := installGatedRender(t)
	mux := newHTTPMux(rt, testToken, true)

	res, body, _ := getTimed(t, mux, "/field.json")
	if res.StatusCode != http.StatusOK || res.Header.Get("X-Field-Stale") != "false" || res.Header.Get("X-Field-Tip") == "" || res.Header.Get("X-Field-Generated") == "" {
		t.Fatalf("first field.json: %d %v", res.StatusCode, res.Header)
	}
	onDisk, err := os.ReadFile(filepath.Join("outputs", "field.json"))
	if err != nil || string(onDisk) != body {
		t.Fatalf("served body must equal outputs/field.json (err %v)", err)
	}
	var st struct {
		AtGOV    map[string]json.RawMessage
		Accepted map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || st.AtGOV[id] == nil {
		t.Fatalf("field.json shape: %v %s", err, body[:min(200, len(body))])
	}
	for i := 0; i < 5; i++ {
		res, again, took := getTimed(t, mux, "/field.json")
		if again != body || res.Header.Get("X-Field-Stale") != "false" {
			t.Fatalf("repeat %d changed without a ledger change", i)
		}
		if took > 500*time.Millisecond {
			t.Fatalf("cached field.json took %s", took)
		}
	}
	if n := g.builds.Load(); n != 1 {
		t.Fatalf("repeat requests rebuilt %d times, want 1", n)
	}

	if r := postDecide(mux, id, testToken); r.StatusCode != http.StatusOK {
		t.Fatalf("decide: %d", r.StatusCode)
	}
	res, body, _ = getTimed(t, mux, "/field.json")
	st.AtGOV, st.Accepted = nil, nil
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("X-Field-Stale") != "false" || st.AtGOV[id] != nil || st.Accepted[id] == nil {
		t.Fatalf("field.json must follow the ledger after a decision: stale=%s", res.Header.Get("X-Field-Stale"))
	}
	if n := g.builds.Load(); n != 2 {
		t.Fatalf("builds after one ledger change = %d, want 2", n)
	}
	if res, _, _ := getTimed(t, mux, "/field.html"); res.StatusCode != http.StatusOK || res.Header.Get("X-Field-Tip") == "" {
		t.Fatalf("field.html: %d", res.StatusCode)
	}
}

// While a rebuild is slow, /field.json answers within the fresh wait with
// the last good snapshot, marked stale, instead of holding the request.
func TestFieldJSONServesLastGoodSnapshotWhileRebuilding(t *testing.T) {
	t.Setenv("FIELDD_FIELD_WAIT_MS", "50")
	rt, id, _ := govCandidate(t)
	chdirTemp(t)
	g := installGatedRender(t)
	mux := newHTTPMux(rt, testToken, true)
	_, before, _ := getTimed(t, mux, "/field.json")

	gate := g.block()
	if r := postDecide(mux, id, testToken); r.StatusCode != http.StatusOK {
		t.Fatalf("decide: %d", r.StatusCode)
	}
	res, body, took := getTimed(t, mux, "/field.json")
	if res.StatusCode != http.StatusOK || body != before || res.Header.Get("X-Field-Stale") != "true" {
		t.Fatalf("slow rebuild must serve the last snapshot marked stale: %d stale=%s", res.StatusCode, res.Header.Get("X-Field-Stale"))
	}
	if took > time.Second {
		t.Fatalf("stale field.json took %s", took)
	}
	<-g.started // the first build
	<-g.started // the rebuild
	close(gate)
	g.unblock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, body, _ = getTimed(t, mux, "/field.json")
		if res.Header.Get("X-Field-Stale") == "false" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rebuild never landed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if body == before {
		t.Fatal("fresh snapshot must include the decision")
	}
}

// After a restart the field.json left on disk answers at once (stale) while
// the first build runs, so the UI is not offline during boot.
func TestFieldJSONAnswersFromDiskSnapshotAfterRestart(t *testing.T) {
	t.Setenv("FIELDD_FIELD_WAIT_MS", "50")
	rt, _, _ := govCandidate(t)
	chdirTemp(t)
	if _, err := rt.RenderCurrent("outputs"); err != nil {
		t.Fatal(err)
	}
	onDisk, _ := os.ReadFile(filepath.Join("outputs", "field.json"))
	g := installGatedRender(t)
	gate := g.block()
	mux := newHTTPMux(rt, testToken, true)
	res, body, took := getTimed(t, mux, "/field.json")
	close(gate)
	g.unblock()
	if res.StatusCode != http.StatusOK || body != string(onDisk) || res.Header.Get("X-Field-Stale") != "true" || res.Header.Get("X-Field-Generated") == "" {
		t.Fatalf("disk snapshot: %d stale=%s", res.StatusCode, res.Header.Get("X-Field-Stale"))
	}
	if took > time.Second {
		t.Fatalf("disk snapshot took %s", took)
	}
	// Let the background build finish inside this test's directory.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if res, _, _ := getTimed(t, mux, "/field.json"); res.Header.Get("X-Field-Stale") == "false" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first build never landed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHealthzIsCheap(t *testing.T) {
	rt, _, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)
	res, body, _ := getTimed(t, mux, "/healthz")
	var h struct {
		OK  bool `json:"ok"`
		PID int  `json:"pid"`
	}
	if res.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &h) != nil || !h.OK || h.PID != os.Getpid() {
		t.Fatalf("/healthz = %d %s", res.StatusCode, body)
	}
}
