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

// Insight reasoning for the nonstop worker. It proposes only: a summary, kin
// links to listed peers, combinations and gaps. The worker writes them as
// governed candidates; GOV (HUMAN_FINAL, or the A01-AUTO policy for the
// low-risk classes) decides.

// ErrUnavailable marks a reasoner that cannot run at all right now (model or
// binary missing, online endpoint not configured or unreachable). The worker
// pauses instead of counting it against a source.
var ErrUnavailable = errors.New("reasoner unavailable")

// InsightPeer is one other REG-accepted SOURCE the model may link to.
type InsightPeer struct {
	N     int
	ID    string
	Title string
	Topic string
}

// InsightInput describes one accepted SOURCE. Only manifest metadata (title,
// topic, mime, related tags) is sent, never file bytes.
type InsightInput struct {
	SourceID string
	Title    string
	Topic    string
	Mime     string
	Related  []string
	Peers    []InsightPeer
}

// InsightKin is one proposed link to a listed peer.
type InsightKin struct {
	Peer int
	Why  string
}

// Insight is the reasoner's proposal for one SOURCE.
type Insight struct {
	Summary      string
	Kin          []InsightKin
	Combinations []string
	Gaps         []string
}

// Insighter is the pluggable worker reasoner.
type Insighter interface {
	Insight(context.Context, InsightInput) (Insight, error)
	Name() string
}

// Reasoner modes.
const (
	ModeLocal     = "local"
	ModeOnline    = "online"
	ModeAuto      = "auto"
	ModeHeuristic = "heuristic" // no model: deterministic, for tests and devices without Gemma
)

// ValidMode reports whether m is a known mode.
func ValidMode(m string) bool {
	switch m {
	case ModeLocal, ModeOnline, ModeAuto, ModeHeuristic:
		return true
	}
	return false
}

const insightPromptTail = "GAP: <one thing missing to make it useful, or none>"

// InsightPrompt is the short prompt both local and online models get.
func InsightPrompt(in InsightInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Source file: %q (topic %s", oneLineText(in.Title, 160), oneLineText(in.Topic, 40))
	if in.Mime != "" {
		fmt.Fprintf(&b, ", %s", oneLineText(in.Mime, 60))
	}
	b.WriteString(").\n")
	if len(in.Related) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", oneLineText(strings.Join(in.Related, ", "), 160))
	}
	if len(in.Peers) > 0 {
		b.WriteString("Other files:\n")
		for _, p := range in.Peers {
			fmt.Fprintf(&b, "%d %s (%s)\n", p.N, oneLineText(p.Title, 100), oneLineText(p.Topic, 40))
		}
	}
	b.WriteString("Reply with exactly 4 lines:\n")
	b.WriteString("SUMMARY: <one sentence on what this file likely is>\n")
	b.WriteString("KIN: <numbers of other files it truly relates to, comma separated, or none>\n")
	b.WriteString("COMBO: <one useful combination with a listed file, or none>\n")
	b.WriteString(insightPromptTail + "\n")
	return b.String()
}

func oneLineText(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		s = truncateUTF8(s, limit)
	}
	return s
}

func noneText(s string) bool {
	s = strings.ToLower(strings.Trim(strings.TrimSpace(s), ".<>"))
	return s == "" || s == "none" || s == "n/a" || s == "no"
}

// ParseInsight reads the 4-line reply. Kin numbers must name listed peers;
// at most 3 kin, 2 combinations and 2 gaps are kept.
func ParseInsight(out string, in InsightInput) (Insight, error) {
	if i := strings.LastIndex(out, insightPromptTail); i >= 0 {
		out = out[i+len(insightPromptTail):]
	}
	var r Insight
	peers := map[int]bool{}
	for _, p := range in.Peers {
		peers[p.N] = true
	}
	seenKin := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "*-#0123456789. "))
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.Trim(strings.TrimSpace(key), "*"))
		val = strings.TrimSpace(strings.Trim(strings.TrimSpace(val), "*"))
		switch key {
		case "SUMMARY":
			if r.Summary == "" && !noneText(val) {
				r.Summary = oneLineText(val, 300)
			}
		case "KIN":
			if noneText(val) {
				continue
			}
			for _, f := range strings.FieldsFunc(val, func(c rune) bool { return c == ',' || c == ' ' || c == ';' }) {
				n, err := strconv.Atoi(strings.Trim(f, "#.()"))
				if err != nil || !peers[n] || seenKin[n] || len(r.Kin) >= 3 {
					continue
				}
				seenKin[n] = true
				r.Kin = append(r.Kin, InsightKin{Peer: n, Why: "reasoner"})
			}
		case "COMBO", "COMBINATION":
			if !noneText(val) && len(r.Combinations) < 2 {
				r.Combinations = append(r.Combinations, oneLineText(val, 240))
			}
		case "GAP":
			if !noneText(val) && len(r.Gaps) < 2 {
				r.Gaps = append(r.Gaps, oneLineText(val, 240))
			}
		}
	}
	if r.Summary == "" {
		return r, fmt.Errorf("reasoner reply has no SUMMARY line")
	}
	return r, nil
}

// HeuristicInsighter needs no model: it links peers that share a related tag
// and proposes no combinations or gaps. Deterministic.
type HeuristicInsighter struct{}

func (HeuristicInsighter) Name() string { return "heuristic-insight" }

func (HeuristicInsighter) Insight(_ context.Context, in InsightInput) (Insight, error) {
	r := Insight{Summary: oneLineText(fmt.Sprintf("SOURCE %q in topic %s.", in.Title, in.Topic), 300)}
	for _, p := range in.Peers {
		if len(r.Kin) >= 3 {
			break
		}
		if p.Topic == in.Topic {
			r.Kin = append(r.Kin, InsightKin{Peer: p.N, Why: "same topic " + in.Topic})
		}
	}
	return r, nil
}

// LocalInsighter runs the on-device model once per source with a short
// prompt, every core and no conversation mode.
type LocalInsighter struct {
	Binary    string
	Model     string
	Threads   int
	MaxTokens int
	Timeout   time.Duration
}

// LocalInsighterFromEnv prefers llama-completion (one-shot) over llama-cli.
func LocalInsighterFromEnv() LocalInsighter {
	bin := strings.TrimSpace(os.Getenv("GEMMA_BIN"))
	if bin == "" {
		if p, err := exec.LookPath("llama-completion"); err == nil {
			bin = p
		} else {
			bin = discoverBinary("")
		}
	}
	bin = preferCompletion(bin)
	return LocalInsighter{
		Binary:    bin,
		Model:     discoverModel(os.Getenv("GEMMA_MODEL")),
		Threads:   envInt("GEMMA_THREADS", runtime.NumCPU()),
		MaxTokens: envInt("UNIFUSION_WORKER_TOKENS", 120),
		Timeout:   time.Duration(envInt("UNIFUSION_WORKER_TIMEOUT_SECONDS", 600)) * time.Second,
	}
}

func (l LocalInsighter) Name() string { return "local-gemma" }

// Args is the exact llama.cpp argument list (exposed for tests).
func (l LocalInsighter) Args(prompt string) []string {
	threads := l.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	n := l.MaxTokens
	if n <= 0 {
		n = 120
	}
	args := []string{"-m", l.Model, "-p", prompt, "-n", strconv.Itoa(n), "-c", "2048",
		"-t", strconv.Itoa(threads), "--temp", "0", "--no-display-prompt"}
	// llama-completion is one-shot already and some builds reject -no-cnv;
	// only llama-cli needs it to leave chat mode.
	if filepath.Base(l.Binary) != "llama-completion" {
		args = append(args, "-no-cnv")
	}
	return args
}

func (l LocalInsighter) Insight(ctx context.Context, in InsightInput) (Insight, error) {
	if strings.TrimSpace(l.Model) == "" {
		return Insight{}, fmt.Errorf("%w: local model not found (set GEMMA_MODEL)", ErrUnavailable)
	}
	if _, err := exec.LookPath(l.Binary); err != nil {
		if st, serr := os.Stat(l.Binary); serr != nil || st.IsDir() {
			return Insight{}, fmt.Errorf("%w: local model binary not found: %s", ErrUnavailable, filepath.Base(l.Binary))
		}
	}
	timeout := l.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, l.Binary, l.Args(InsightPrompt(in))...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr, cmd.Stdin = &out, &errb, nil
	if err := cmd.Run(); err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return Insight{}, fmt.Errorf("local model timed out after %s", timeout)
		}
		return Insight{}, fmt.Errorf("local model failed: %v: %s", err, stderrTail(errb.String(), 240))
	}
	return ParseInsight(out.String(), in)
}

// OnlineInsighter calls an OpenAI-compatible chat completions endpoint. The
// key comes from UNIFUSION_REASONER_KEY or the file named by
// UNIFUSION_REASONER_KEY_FILE; it is never logged or returned.
type OnlineInsighter struct {
	URL     string
	Model   string
	key     string
	Client  *http.Client
	Timeout time.Duration
}

// OnlineInsighterFromEnv reads UNIFUSION_REASONER_URL/_MODEL/_KEY(_FILE).
func OnlineInsighterFromEnv() OnlineInsighter {
	key := strings.TrimSpace(os.Getenv("UNIFUSION_REASONER_KEY"))
	if key == "" {
		if p := strings.TrimSpace(os.Getenv("UNIFUSION_REASONER_KEY_FILE")); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				key = strings.TrimSpace(string(b))
			}
		}
	}
	return OnlineInsighter{
		URL:     strings.TrimSpace(os.Getenv("UNIFUSION_REASONER_URL")),
		Model:   strings.TrimSpace(os.Getenv("UNIFUSION_REASONER_MODEL")),
		key:     key,
		Timeout: 60 * time.Second,
	}
}

// WithKey returns a copy using key (tests).
func (o OnlineInsighter) WithKey(key string) OnlineInsighter { o.key = key; return o }

func (o OnlineInsighter) Name() string { return "online:" + o.Model }

// Configured reports whether URL and model are set (key may be optional for
// a local network server).
func (o OnlineInsighter) Configured() bool { return o.URL != "" && o.Model != "" }

// HasKey reports whether a key is configured (the key itself never leaves).
func (o OnlineInsighter) HasKey() bool { return o.key != "" }

func (o OnlineInsighter) endpoint(suffix string) string {
	u := strings.TrimRight(o.URL, "/")
	u = strings.TrimSuffix(u, "/chat/completions")
	return u + suffix
}

func (o OnlineInsighter) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{Timeout: o.Timeout}
}

// Reachable probes GET <base>/models within 4 seconds.
func (o OnlineInsighter) Reachable(ctx context.Context) bool {
	if !o.Configured() {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, o.endpoint("/models"), nil)
	if err != nil {
		return false
	}
	if o.key != "" {
		req.Header.Set("Authorization", "Bearer "+o.key)
	}
	resp, err := o.client().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode < 500
}

func (o OnlineInsighter) Insight(ctx context.Context, in InsightInput) (Insight, error) {
	if !o.Configured() {
		return Insight{}, fmt.Errorf("%w: online reasoner not configured", ErrUnavailable)
	}
	body, _ := json.Marshal(map[string]any{
		"model":       o.Model,
		"temperature": 0,
		"max_tokens":  200,
		"messages":    []map[string]string{{"role": "user", "content": InsightPrompt(in)}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return Insight{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.key != "" {
		req.Header.Set("Authorization", "Bearer "+o.key)
	}
	resp, err := o.client().Do(req)
	if err != nil {
		return Insight{}, fmt.Errorf("%w: online reasoner unreachable", ErrUnavailable)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Insight{}, fmt.Errorf("online reasoner HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return Insight{}, fmt.Errorf("online reasoner reply unreadable")
	}
	return ParseInsight(parsed.Choices[0].Message.Content, in)
}

// SwitchInsighter picks local, online or heuristic by mode. auto uses online
// when it is configured and reachable (probe cached 60s), else local; an
// online failure in auto falls back to local for that call.
type SwitchInsighter struct {
	mu        sync.Mutex
	mode      string
	Local     Insighter
	Online    *OnlineInsighter
	Heuristic Insighter
	probedAt  time.Time
	reachable bool
	last      string
}

// NewSwitchInsighter builds the switch from the environment.
func NewSwitchInsighter(mode string) *SwitchInsighter {
	on := OnlineInsighterFromEnv()
	s := &SwitchInsighter{Local: LocalInsighterFromEnv(), Online: &on, Heuristic: HeuristicInsighter{}}
	s.SetMode(mode)
	return s
}

// SetMode sets local/online/auto/heuristic; unknown values become local.
func (s *SwitchInsighter) SetMode(m string) {
	m = strings.ToLower(strings.TrimSpace(m))
	if !ValidMode(m) {
		m = ModeLocal
	}
	s.mu.Lock()
	s.mode = m
	s.probedAt = time.Time{}
	s.mu.Unlock()
}

// Mode is the selected mode.
func (s *SwitchInsighter) Mode() string { s.mu.Lock(); defer s.mu.Unlock(); return s.mode }

// LastUsed names the reasoner the last call used.
func (s *SwitchInsighter) LastUsed() string { s.mu.Lock(); defer s.mu.Unlock(); return s.last }

// OnlineConfigured reports whether an online endpoint is configured.
func (s *SwitchInsighter) OnlineConfigured() bool { return s.Online != nil && s.Online.Configured() }

func (s *SwitchInsighter) Name() string { return "switch:" + s.Mode() }

func (s *SwitchInsighter) onlineUp(ctx context.Context) bool {
	if !s.OnlineConfigured() {
		return false
	}
	s.mu.Lock()
	fresh := time.Since(s.probedAt) < time.Minute
	up := s.reachable
	s.mu.Unlock()
	if fresh {
		return up
	}
	up = s.Online.Reachable(ctx)
	s.mu.Lock()
	s.probedAt, s.reachable = time.Now(), up
	s.mu.Unlock()
	return up
}

func (s *SwitchInsighter) use(name string) { s.mu.Lock(); s.last = name; s.mu.Unlock() }

func (s *SwitchInsighter) Insight(ctx context.Context, in InsightInput) (Insight, error) {
	switch s.Mode() {
	case ModeHeuristic:
		s.use(s.Heuristic.Name())
		return s.Heuristic.Insight(ctx, in)
	case ModeOnline:
		if !s.OnlineConfigured() {
			return Insight{}, fmt.Errorf("%w: online mode: set UNIFUSION_REASONER_URL and UNIFUSION_REASONER_MODEL", ErrUnavailable)
		}
		s.use(s.Online.Name())
		return s.Online.Insight(ctx, in)
	case ModeAuto:
		if s.onlineUp(ctx) {
			s.use(s.Online.Name())
			r, err := s.Online.Insight(ctx, in)
			if err == nil {
				return r, nil
			}
			s.mu.Lock()
			s.reachable, s.probedAt = false, time.Now()
			s.mu.Unlock()
		}
	}
	s.use(s.Local.Name())
	return s.Local.Insight(ctx, in)
}

// stderrTail keeps the last non-banner lines of llama.cpp stderr (at most n
// bytes, one line) so the Auto panel shows why the local model failed.
func stderrTail(s string, n int) string {
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "llama_model_loader") || strings.HasPrefix(ln, "print_info") || strings.HasPrefix(ln, "load_tensors") {
			continue
		}
		keep = append(keep, ln)
	}
	if len(keep) > 3 {
		keep = keep[len(keep)-3:]
	}
	out := strings.Join(keep, " | ")
	if len(out) > n {
		out = out[len(out)-n:]
	}
	if out == "" {
		return "no error text"
	}
	return out
}

// preferCompletion swaps llama-cli (newer builds reject -no-cnv and open in
// chat mode) for llama-completion when one sits beside it or on PATH.
func preferCompletion(bin string) string {
	if filepath.Base(bin) != "llama-cli" {
		return bin
	}
	if dir := filepath.Dir(bin); dir != "." && dir != "" {
		side := filepath.Join(dir, "llama-completion")
		if st, err := os.Stat(side); err == nil && !st.IsDir() {
			return side
		}
	}
	if p, err := exec.LookPath("llama-completion"); err == nil {
		return p
	}
	return bin
}
