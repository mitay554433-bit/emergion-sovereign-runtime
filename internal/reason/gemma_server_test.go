package reason

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLlama is a stand-in llama-server: /health and /completion.
type fakeLlama struct {
	mu          sync.Mutex
	ready       bool
	failFirst   int // completions that answer 500 before working
	completions int
	prompts     []completionReq
}

func (f *fakeLlama) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"Loading model"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/completion", func(w http.ResponseWriter, r *http.Request) {
		var req completionReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.completions++
		f.prompts = append(f.prompts, req)
		fail := f.failFirst > 0
		if fail {
			f.failFirst--
		}
		f.mu.Unlock()
		if fail {
			http.Error(w, "slot crashed", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": " A grant brief for the BDC pitch.\nKIN: 2\nCOMBO: none\nGAP: none", "stop": true})
	})
	return mux
}

func testInput() InsightInput {
	return InsightInput{SourceID: "E-1", Title: "bdc.pdf", Topic: "grants", Peers: []InsightPeer{{N: 1, ID: "E-2", Title: "a"}, {N: 2, ID: "E-3", Title: "b"}}}
}

func newTestServerInsighter(url string) *ServerInsighter {
	return &ServerInsighter{
		Exec:      LocalInsighter{Binary: "definitely-not-llama", Model: "gemma-2-2b-it.gguf", MaxTokens: 64, Timeout: 10 * time.Second},
		ServerBin: "llama-server",
		BaseURL:   url,
		LoadWait:  2 * time.Second,
		poll:      10 * time.Millisecond,
	}
}

func TestServerReusesRunningServerAndCachesPrefix(t *testing.T) {
	f := &fakeLlama{ready: true}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	launches := 0
	s.launch = func() error { launches++; return nil }
	for i := 0; i < 2; i++ {
		r, err := s.Insight(context.Background(), testInput())
		if err != nil || r.Summary != "A grant brief for the BDC pitch." || len(r.Kin) != 1 || r.Kin[0].Peer != 2 {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if launches != 0 || s.LastUsed() != NameLocalServer || f.completions != 2 {
		t.Fatalf("a healthy server must be reused: launches %d used %s completions %d", launches, s.LastUsed(), f.completions)
	}
	p := f.prompts[0]
	if !p.CachePrompt || p.Temperature != 0 || p.NPredict != 64 || len(p.Stop) != 1 || p.Stop[0] != "<end_of_turn>" {
		t.Fatalf("completion request: %+v", p)
	}
	if !strings.HasPrefix(p.Prompt, "<start_of_turn>user\nReply with exactly 4 lines:\n") || !strings.HasSuffix(p.Prompt, "<start_of_turn>model\nSUMMARY:") {
		t.Fatalf("prompt must start with the fixed instructions (cache_prompt) and prefill SUMMARY:\n%s", p.Prompt)
	}
	other := testInput()
	other.Title, other.Peers = "other.pdf", nil
	a, b := LocalPrompt(testInput()), LocalPrompt(other)
	if i := strings.Index(a, "Source file:"); i < 0 || a[:i] != b[:i] {
		t.Fatal("local prompts must share the instruction prefix")
	}
}

func TestServerStartsAndWaitsForLoad(t *testing.T) {
	f := &fakeLlama{}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	launches := 0
	// a 503 "loading" server is left to finish, so make the port look empty first
	down := httptest.NewServer(http.NotFoundHandler())
	s.BaseURL = down.URL
	down.Close()
	s.launch = func() error {
		launches++
		s.BaseURL = ts.URL
		go func() { time.Sleep(50 * time.Millisecond); f.mu.Lock(); f.ready = true; f.mu.Unlock() }()
		return nil
	}
	if _, err := s.Insight(context.Background(), testInput()); err != nil {
		t.Fatal(err)
	}
	if launches != 1 || s.LastUsed() != NameLocalServer {
		t.Fatalf("launches %d used %s", launches, s.LastUsed())
	}
	// loaded once: later records do not relaunch
	if _, err := s.Insight(context.Background(), testInput()); err != nil || launches != 1 {
		t.Fatalf("relaunched: %d %v", launches, err)
	}
}

func TestServerRestartsOnceAfterFailure(t *testing.T) {
	f := &fakeLlama{ready: true, failFirst: 1}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	stops := 0
	s.launch = func() error { stops++; return nil }
	r, err := s.Insight(context.Background(), testInput())
	if err != nil || r.Summary == "" || f.completions != 2 || s.LastUsed() != NameLocalServer {
		t.Fatalf("restart-then-retry: %+v %v completions %d used %s", r, err, f.completions, s.LastUsed())
	}
}

func TestServerFallsBackToOneShotRun(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "llama-completion")
	script := "#!/bin/sh\nprintf ' A fallback summary of the file.\\nKIN: 1\\n'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	s := newTestServerInsighter(url)
	s.Exec.Binary = bin
	launches := 0
	s.launch = func() error { launches++; return errors.New("no llama-server") }
	r, err := s.Insight(context.Background(), testInput())
	if err != nil || r.Summary != "A fallback summary of the file." || s.LastUsed() != NameLocalExec {
		t.Fatalf("fallback: %+v %v used %s", r, err, s.LastUsed())
	}
	if launches != 2 {
		t.Fatalf("want start + one restart before falling back, got %d", launches)
	}
	// during the cooldown the next record goes straight to the one-shot run
	if _, err := s.Insight(context.Background(), testInput()); err != nil || launches != 2 {
		t.Fatalf("cooldown: launches %d %v", launches, err)
	}
}

func TestServerTimeoutIsModelRun(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer ts.Close()
	defer close(release)
	s := newTestServerInsighter(ts.URL)
	s.Exec.Timeout = 200 * time.Millisecond
	s.launch = func() error { return nil }
	_, err := s.Insight(context.Background(), testInput())
	if !errors.Is(err, ErrModelRun) || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want model-run timeout, got %v", err)
	}
}

func TestServerArgsAndResolve(t *testing.T) {
	s := &ServerInsighter{Exec: LocalInsighter{Model: "m.gguf", Threads: 8}, Port: 8897}
	args := strings.Join(s.ServerArgs(), " ")
	for _, want := range []string{"-m m.gguf", "--host 127.0.0.1", "--port 8897", "-c 2048", "-t 8", "--parallel 1"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q lack %q", args, want)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "llama-server"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveServerBin("", filepath.Join(dir, "llama-completion")); got != filepath.Join(dir, "llama-server") {
		t.Fatalf("want llama-server beside GEMMA_BIN, got %q", got)
	}
	if resolveServerBin("off", filepath.Join(dir, "llama-completion")) != "" || resolveServerBin("/x/llama-server", "") != "/x/llama-server" {
		t.Fatal("override")
	}
}

func TestSwitchReportsServerBackend(t *testing.T) {
	f := &fakeLlama{ready: true}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	s.launch = func() error { return nil }
	sw := &SwitchInsighter{Local: s, Online: &OnlineInsighter{}, Heuristic: HeuristicInsighter{}}
	sw.SetMode(ModeLocal)
	if _, err := sw.Insight(context.Background(), testInput()); err != nil || sw.LastUsed() != NameLocalServer {
		t.Fatalf("via %q %v", sw.LastUsed(), err)
	}
}
