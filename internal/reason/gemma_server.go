package reason

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kept-loaded local model. The worker starts one llama-server the first
// time it needs Gemma and sends every record to it, so the model loads once
// instead of once per record. A server already answering /health on the port
// is reused, so it survives fieldd restarts. If the server dies or stops
// answering it is restarted once; if that fails the record falls back to the
// one-shot llama-completion run.

// Names reported in /auto/status "last_reasoner".
const (
	NameLocalServer = "local-gemma-server"
	NameLocalExec   = "local-gemma"
)

// DefaultLlamaPort is the loopback port of the kept-loaded llama-server.
const DefaultLlamaPort = 8897

// ServerInsighter runs the local model through a kept-loaded llama-server
// and falls back to the one-shot Exec run.
type ServerInsighter struct {
	Exec      LocalInsighter
	ServerBin string // llama-server path; empty disables the server
	BaseURL   string // http://127.0.0.1:<port>
	Port      int
	Ctx       int           // context size for the server
	LoadWait  time.Duration // how long to wait for the model to load
	Cooldown  time.Duration // after a failed start, use Exec for this long
	LogPath   string
	Client    *http.Client

	// launch starts the server; nil uses startProcess. Tests replace it.
	launch func() error
	poll   time.Duration

	mu       sync.Mutex
	proc     *exec.Cmd
	exited   chan struct{}
	downAt   time.Time
	lastUsed string
}

// ServerInsighterFromEnv builds the kept-loaded backend from the same env as
// LocalInsighterFromEnv. UNIFUSION_LLAMA_SERVER overrides the server path
// (default: llama-server beside GEMMA_BIN, then PATH; "off" disables it),
// UNIFUSION_LLAMA_PORT the port (8897).
func ServerInsighterFromEnv() *ServerInsighter {
	ex := LocalInsighterFromEnv()
	port := envInt("UNIFUSION_LLAMA_PORT", DefaultLlamaPort)
	logPath := strings.TrimSpace(os.Getenv("UNIFUSION_LLAMA_LOG"))
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "llama-server.log")
	}
	return &ServerInsighter{
		Exec:      ex,
		ServerBin: resolveServerBin(os.Getenv("UNIFUSION_LLAMA_SERVER"), ex.Binary),
		BaseURL:   "http://127.0.0.1:" + strconv.Itoa(port),
		Port:      port,
		Ctx:       envInt("UNIFUSION_LLAMA_CTX", 2048),
		LoadWait:  time.Duration(envInt("UNIFUSION_LLAMA_LOAD_SECONDS", 180)) * time.Second,
		Cooldown:  10 * time.Minute,
		LogPath:   logPath,
	}
}

// resolveServerBin picks llama-server: the override, else beside the
// llama.cpp binary in use, else on PATH. "" means no server available.
func resolveServerBin(override, bin string) string {
	override = strings.TrimSpace(override)
	if strings.EqualFold(override, "off") || override == "0" {
		return ""
	}
	if override != "" {
		return override
	}
	if dir := filepath.Dir(bin); bin != "" && dir != "." && dir != "" {
		side := filepath.Join(dir, "llama-server")
		if st, err := os.Stat(side); err == nil && !st.IsDir() {
			return side
		}
	}
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p
	}
	return ""
}

func (s *ServerInsighter) Name() string { return NameLocalServer }

// LastUsed names the backend the last record used.
func (s *ServerInsighter) LastUsed() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsed
}

func (s *ServerInsighter) setUsed(n string) { s.mu.Lock(); s.lastUsed = n; s.mu.Unlock() }

// ServerArgs is the llama-server argument list (exposed for tests).
func (s *ServerInsighter) ServerArgs() []string {
	threads := s.Exec.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	c := s.Ctx
	if c <= 0 {
		c = 2048
	}
	port := s.Port
	if port <= 0 {
		port = DefaultLlamaPort
	}
	return []string{"-m", s.Exec.Model, "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"-c", strconv.Itoa(c), "-t", strconv.Itoa(threads), "--parallel", "1"}
}

func (s *ServerInsighter) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

// health reports whether the server answers GET /health with 200 (ready)
// or 503 (up, still loading the model).
func (s *ServerInsighter) health(ctx context.Context) (ready, loading bool) {
	hctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, s.BaseURL+"/health", nil)
	if err != nil {
		return false, false
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode == http.StatusOK, resp.StatusCode == http.StatusServiceUnavailable
}

func (s *ServerInsighter) healthy(ctx context.Context) bool { ok, _ := s.health(ctx); return ok }

// startProcess launches llama-server detached from the request context so it
// stays loaded across records and fieldd restarts.
func (s *ServerInsighter) startProcess() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil {
		select {
		case <-s.exited:
		default:
			return nil // ours is still loading
		}
	}
	cmd := exec.Command(s.ServerBin, s.ServerArgs()...)
	cmd.Stdin = nil
	if f, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	s.proc, s.exited = cmd, done
	return nil
}

// stopOwn kills only the llama-server this process started.
func (s *ServerInsighter) stopOwn() {
	s.mu.Lock()
	p, done := s.proc, s.exited
	s.proc = nil
	s.mu.Unlock()
	if p == nil || p.Process == nil {
		return
	}
	select {
	case <-done:
		return
	default:
	}
	_ = p.Process.Kill()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (s *ServerInsighter) ownExited() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc == nil || s.exited == nil {
		return false
	}
	select {
	case <-s.exited:
		return true
	default:
		return false
	}
}

// ensure makes the server ready: reuse a healthy one, else start it and wait
// for the model to load.
func (s *ServerInsighter) ensure(ctx context.Context) error {
	ready, loading := s.health(ctx)
	if ready {
		return nil
	}
	if !loading {
		// nothing on the port (a server still loading is left to finish)
		launch := s.launch
		if launch == nil {
			launch = s.startProcess
		}
		if err := launch(); err != nil {
			return fmt.Errorf("start llama-server: %w", err)
		}
	}
	wait := s.LoadWait
	if wait <= 0 {
		wait = 3 * time.Minute
	}
	poll := s.poll
	if poll <= 0 {
		poll = time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		if s.healthy(ctx) {
			return nil
		}
		if s.ownExited() {
			return errors.New("llama-server exited while loading (see " + filepath.Base(s.LogPath) + ")")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("llama-server not ready after %s", wait)
		}
		t := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			t.Stop()
		case <-t.C:
		}
	}
}

// Warm starts the server in the background so the first record finds Gemma
// already loaded. Errors are left for the first record to report.
func (s *ServerInsighter) Warm(ctx context.Context) {
	if !s.serverUsable() {
		return
	}
	go func() { _ = s.ensure(ctx) }()
}

func (s *ServerInsighter) serverUsable() bool {
	if strings.TrimSpace(s.ServerBin) == "" || strings.TrimSpace(s.Exec.Model) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cd := s.Cooldown
	if cd <= 0 {
		cd = 10 * time.Minute
	}
	return s.downAt.IsZero() || time.Since(s.downAt) >= cd
}

type completionReq struct {
	Prompt      string   `json:"prompt"`
	NPredict    int      `json:"n_predict"`
	Temperature float64  `json:"temperature"`
	CachePrompt bool     `json:"cache_prompt"`
	Stop        []string `json:"stop"`
}

// complete sends one prompt to POST /completion and returns .content.
func (s *ServerInsighter) complete(ctx context.Context, prompt string) (string, error) {
	n := s.Exec.MaxTokens
	if n <= 0 {
		n = 64
	}
	body, _ := json.Marshal(completionReq{Prompt: prompt, NPredict: n, Temperature: 0, CachePrompt: true, Stop: []string{"<end_of_turn>"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/completion", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama-server HTTP %d: %s", resp.StatusCode, oneLineText(string(b), 160))
	}
	var out struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("llama-server reply: %v", err)
	}
	return out.Content, nil
}

func (s *ServerInsighter) Insight(ctx context.Context, in InsightInput) (Insight, error) {
	if strings.TrimSpace(s.Exec.Model) == "" || !s.serverUsable() {
		s.setUsed(NameLocalExec)
		return s.Exec.Insight(ctx, in)
	}
	timeout := s.Exec.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	prompt := localTurn(s.Exec.Model, LocalPrompt(in))
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			// dead or unreachable: restart once
			s.stopOwn()
		}
		if err := s.ensure(cctx); err != nil {
			lastErr = err
		} else if reply, err := s.complete(cctx, prompt); err == nil {
			s.setUsed(NameLocalServer)
			if !strings.Contains(strings.ToUpper(reply), "SUMMARY") {
				reply = "SUMMARY:" + strings.TrimLeft(reply, " \t")
			}
			return ParseInsight(reply, in)
		} else {
			lastErr = err
		}
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			s.setUsed(NameLocalServer)
			return Insight{}, fmt.Errorf("%w: local model timed out after %s", ErrModelRun, timeout)
		}
		if ctx.Err() != nil {
			return Insight{}, ctx.Err()
		}
	}
	// restart failed: one-shot run for this record, server skipped for a while
	s.mu.Lock()
	s.downAt = time.Now()
	s.mu.Unlock()
	s.setUsed(NameLocalExec)
	r, err := s.Exec.Insight(ctx, in)
	if err != nil {
		return r, fmt.Errorf("%w (llama-server: %s)", err, oneLineText(lastErr.Error(), 120))
	}
	return r, nil
}
