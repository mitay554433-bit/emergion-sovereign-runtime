package reason

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func bigInput() InsightInput {
	in := testInput()
	in.Title = strings.Repeat("Very long title ", 20)
	in.Related = []string{"TR-1", "TR-2"}
	in.Text = strings.Repeat("Measured strain gauge drift over the bench run. ", 200)
	for i := 3; i <= 8; i++ {
		in.Peers = append(in.Peers, InsightPeer{N: i, ID: fmt.Sprint("E-", i), Title: strings.Repeat("peer ", 20), Topic: "grants"})
	}
	return in
}

func TestCapLocalInputBoundsPromptAndKeepsSmallInputs(t *testing.T) {
	small := testInput()
	if got := CapLocalInput(small, DefaultLocalPromptChars); LocalPrompt(got) != LocalPrompt(small) {
		t.Fatal("a prompt under the cap must not change")
	}
	in := bigInput()
	for _, max := range []int{2400, 1200, 900, 600} {
		got := CapLocalInput(in, max)
		if n := len(LocalPrompt(got)); n > max {
			t.Fatalf("cap %d: prompt %d chars", max, n)
		}
		if got.SourceID != in.SourceID || got.Topic != in.Topic {
			t.Fatal("identity fields must stay")
		}
	}
	// text goes first: at 1200 the instructions, title and peers survive
	in.Peers = in.Peers[:LocalPeers]
	got := CapLocalInput(in, 1200)
	if len(got.Peers) != LocalPeers || got.Text == "" || len(got.Text) >= LocalTextChars {
		t.Fatalf("text should be trimmed before peers: peers %d text %d", len(got.Peers), len(got.Text))
	}
	if in.Text == "" || len(bigInput().Peers) != 8 {
		t.Fatal("the caller's input must not be modified")
	}
}

func TestLocalPromptCharsEnv(t *testing.T) {
	t.Setenv("UNIFUSION_LLAMA_PROMPT_CHARS", "")
	if LocalPromptChars() != DefaultLocalPromptChars {
		t.Fatal("default")
	}
	t.Setenv("UNIFUSION_LLAMA_PROMPT_CHARS", "100")
	if LocalPromptChars() != 600 {
		t.Fatal("floor 600")
	}
	t.Setenv("UNIFUSION_WORKER_TIMEOUT_SECONDS", "")
	if LocalInsighterFromEnv().Timeout != DefaultWorkerTimeoutSeconds*time.Second || DefaultWorkerTimeoutSeconds > 600 {
		t.Fatal("per-call timeout must default to a few minutes, not 20")
	}
}

// sseLlama streams like llama-server: data: {...} lines. stallAfter > 0
// stops sending after that many chunks and holds the connection.
type sseLlama struct {
	mu         sync.Mutex
	prompts    []completionReq
	stallAfter int
	healthHits int
	completes  int
}

func (f *sseLlama) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.healthHits++
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/completion", func(w http.ResponseWriter, r *http.Request) {
		var req completionReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.prompts = append(f.prompts, req)
		f.completes++
		stall := f.stallAfter
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		parts := []string{" A grant", " brief.", "\nKIN: 1", "\nCOMBO: none", "\nGAP: none"}
		for i, p := range parts {
			if stall > 0 && i == stall {
				<-r.Context().Done() // hold the slot until the client gives up
				return
			}
			b, _ := json.Marshal(map[string]any{"content": p, "stop": false})
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		fmt.Fprintf(w, "data: {\"content\":\"\",\"stop\":true}\n\n")
		fl.Flush()
	})
	return mux
}

func TestServerStreamsCapsPromptAndChecksHealthFirst(t *testing.T) {
	f := &sseLlama{}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	s.launch = func() error { t.Fatal("a healthy server must not be launched"); return nil }
	r, err := s.Insight(context.Background(), bigInput())
	if err != nil || r.Summary != "A grant brief." || len(r.Kin) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	p := f.prompts[0]
	if !p.Stream || p.TMaxPredictMS <= 0 || p.TMaxPredictMS > (10*time.Second).Milliseconds() {
		t.Fatalf("request must stream with a server-side time cap: %+v", p)
	}
	if n := len(p.Prompt); n > DefaultLocalPromptChars+120 { // + Gemma turn markers
		t.Fatalf("prompt %d chars exceeds the cap", n)
	}
	if f.healthHits == 0 {
		t.Fatal("no health check before the call")
	}
}

func TestServerStallIsATimeoutAndFreesTheSlot(t *testing.T) {
	f := &sseLlama{stallAfter: 2}
	ts := httptest.NewServer(f.handler())
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	s.Stall = 100 * time.Millisecond
	t0 := time.Now()
	_, err := s.Insight(context.Background(), testInput())
	if !errors.Is(err, ErrModelRun) || !errors.Is(err, ErrModelTimeout) || !strings.Contains(err.Error(), "2 tokens received") {
		t.Fatalf("stall: %v", err)
	}
	if time.Since(t0) > 5*time.Second {
		t.Fatal("stall detection waited for the whole budget")
	}
	if f.completes != 1 {
		t.Fatalf("a stalled call must not be retried in the same budget: %d", f.completes)
	}
}

func TestServerPerCallTimeoutIsBounded(t *testing.T) {
	slow := http.NewServeMux()
	slow.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	slow.HandleFunc("/completion", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	})
	ts := httptest.NewServer(slow)
	defer ts.Close()
	s := newTestServerInsighter(ts.URL)
	s.Exec.Timeout = 200 * time.Millisecond
	_, err := s.Insight(context.Background(), testInput())
	if !errors.Is(err, ErrModelTimeout) || !strings.Contains(err.Error(), "timed out after 200ms") || !strings.Contains(err.Error(), "0 tokens received") {
		t.Fatalf("timeout: %v", err)
	}
}

// llama-cli is never spawned, even as the fallback: only llama-completion
// (with -no-cnv) may run one-shot.
func TestNeverSpawnsLlamaCLI(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "llama-cli")
	marker := filepath.Join(dir, "ran")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\ntouch "+marker+"\necho SUMMARY: x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	l := LocalInsighter{Binary: cli, Model: "gemma.gguf", Timeout: 2 * time.Second}
	_, err := l.Insight(context.Background(), testInput())
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "llama-cli is never spawned") {
		t.Fatalf("err %v", err)
	}
	if _, serr := os.Stat(marker); serr == nil {
		t.Fatal("llama-cli was spawned")
	}
	if a := strings.Join(LocalInsighter{Binary: "/x/llama-completion", Model: "m"}.Args("p"), " "); !strings.Contains(a, "-no-cnv") {
		t.Fatalf("llama-completion must run with -no-cnv: %s", a)
	}
}
