package reason

import (
	"bufio"
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
	Stall     time.Duration // abandon a streamed answer after this long without a token
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
		threads = LocalThreads()
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
	Stream      bool     `json:"stream"`
	// TMaxPredictMS asks llama-server itself to stop generating after this
	// long, so an abandoned request cannot hold the single slot.
	TMaxPredictMS int64 `json:"t_max_predict_ms,omitempty"`
}

// DefaultStallSeconds is how long a streamed answer may go without a new
// token once the first one arrived before the call is abandoned.
const DefaultStallSeconds = 90

// errStall marks a streamed answer that stopped producing tokens.
var errStall = errors.New("llama-server stopped sending tokens")

// complete sends one prompt to POST /completion with stream on and returns
// the joined content. Streaming lets a stuck call be detected (no token for
// Stall) and, because the connection is closed on timeout, makes
// llama-server drop the task instead of finishing it for nobody and keeping
// its only slot busy for the next record. got reports how many content
// chunks arrived (0 means the server never started answering).
func (s *ServerInsighter) complete(ctx context.Context, prompt string, budget time.Duration) (text string, got int, err error) {
	n := s.Exec.MaxTokens
	if n <= 0 {
		n = 64
	}
	cr := completionReq{Prompt: prompt, NPredict: n, Temperature: 0, CachePrompt: true, Stop: []string{"<end_of_turn>"}, Stream: true}
	if budget > 0 {
		cr.TMaxPredictMS = budget.Milliseconds()
	}
	body, _ := json.Marshal(cr)
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, s.BaseURL+"/completion", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", 0, fmt.Errorf("llama-server HTTP %d: %s", resp.StatusCode, oneLineText(string(b), 160))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		// a server (or test double) that ignores stream: one JSON object
		b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return "", 0, err
		}
		var out struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			return "", 0, fmt.Errorf("llama-server reply: %v", err)
		}
		return out.Content, 1, nil
	}
	stall := s.Stall
	if stall <= 0 {
		stall = DefaultStallSeconds * time.Second
	}
	type line struct {
		b   []byte
		err error
		end bool
	}
	lines := make(chan line, 16)
	go func() {
		defer close(lines)
		send := func(l line) bool {
			select {
			case lines <- l:
				return true
			case <-cctx.Done():
				return false
			}
		}
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			if !send(line{b: append([]byte(nil), sc.Bytes()...)}) {
				return
			}
		}
		send(line{err: sc.Err(), end: true})
	}()
	var b strings.Builder
	var idle *time.Timer
	var idleC <-chan time.Time
	defer func() {
		if idle != nil {
			idle.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return b.String(), got, ctx.Err()
		case <-idleC:
			return b.String(), got, errStall
		case l, ok := <-lines:
			if !ok {
				return b.String(), got, nil
			}
			if l.end {
				return b.String(), got, l.err
			}
			data, isData := bytes.CutPrefix(bytes.TrimSpace(l.b), []byte("data:"))
			if !isData {
				continue
			}
			data = bytes.TrimSpace(data)
			if string(data) == "[DONE]" {
				return b.String(), got, nil
			}
			var ch struct {
				Content string `json:"content"`
				Stop    bool   `json:"stop"`
				Error   *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(data, &ch) != nil {
				continue
			}
			if ch.Error != nil {
				return b.String(), got, fmt.Errorf("llama-server: %s", oneLineText(ch.Error.Message, 160))
			}
			b.WriteString(ch.Content)
			got++
			if ch.Stop {
				return b.String(), got, nil
			}
			if idle == nil {
				idle = time.NewTimer(stall)
				idleC = idle.C
			} else {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(stall)
			}
		}
	}
}

func (s *ServerInsighter) Insight(ctx context.Context, in InsightInput) (Insight, error) {
	if strings.TrimSpace(s.Exec.Model) == "" || !s.serverUsable() {
		s.setUsed(NameLocalExec)
		return s.Exec.Insight(ctx, in)
	}
	timeout := s.Exec.Timeout
	if timeout <= 0 {
		timeout = DefaultWorkerTimeoutSeconds * time.Second
	}
	// Health first, on the caller's context: waiting for a loading model
	// does not eat the per-call budget, and a server that cannot come up is
	// reported as unavailable (the worker pauses) instead of as a timeout.
	if err := s.ensure(ctx); err != nil {
		if ctx.Err() != nil {
			return Insight{}, ctx.Err()
		}
		s.stopOwn()
		if err2 := s.ensure(ctx); err2 != nil {
			if ctx.Err() != nil {
				return Insight{}, ctx.Err()
			}
			return s.fallback(ctx, in, err2)
		}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	prompt := localTurn(s.Exec.Model, LocalPrompt(CapLocalInput(in, LocalPromptChars())))
	t0 := time.Now()
	var lastErr error
	got := 0
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			// the server failed the call (crashed slot, reset): restart our
			// own server once and re-check health before the retry
			s.stopOwn()
			if err := s.ensure(cctx); err != nil {
				lastErr = err
				break
			}
		}
		var reply string
		var err error
		reply, got, err = s.complete(cctx, prompt, time.Until(deadlineOr(cctx, t0.Add(timeout))))
		s.setUsed(NameLocalServer)
		if err == nil {
			if !strings.Contains(strings.ToUpper(reply), "SUMMARY") {
				reply = "SUMMARY:" + strings.TrimLeft(reply, " \t")
			}
			return ParseInsight(reply, in)
		}
		lastErr = err
		if errors.Is(err, errStall) || cctx.Err() != nil {
			break
		}
	}
	detail := fmt.Sprintf("llama-server %s, prompt %d chars, %d tokens received, %s", s.BaseURL, len(prompt), got, time.Since(t0).Round(time.Second))
	if errors.Is(lastErr, errStall) {
		return Insight{}, fmt.Errorf("%w: %w: %v (%s)", ErrModelRun, ErrModelTimeout, lastErr, detail)
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return Insight{}, modelTimeout(timeout, detail)
	}
	if ctx.Err() != nil {
		return Insight{}, ctx.Err()
	}
	return Insight{}, fmt.Errorf("%w: %v (%s)", ErrModelRun, lastErr, detail)
}

func deadlineOr(ctx context.Context, t time.Time) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return t
}

// fallback runs the one-shot llama-completion -no-cnv for this record when
// llama-server could not be made ready, and skips the server for Cooldown.
// LocalInsighter refuses to spawn llama-cli.
func (s *ServerInsighter) fallback(ctx context.Context, in InsightInput, serverErr error) (Insight, error) {
	s.mu.Lock()
	s.downAt = time.Now()
	s.mu.Unlock()
	s.setUsed(NameLocalExec)
	r, err := s.Exec.Insight(ctx, in)
	if err != nil {
		return r, fmt.Errorf("%w (llama-server: %s)", err, oneLineText(serverErr.Error(), 120))
	}
	return r, nil
}
