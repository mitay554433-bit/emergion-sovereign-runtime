// Package worker is the nonstop UNIFUSION worker. It walks REG-accepted
// SOURCE records, asks the insight reasoner for a summary, kin links and
// combinations/gaps, and writes the results as normal governed candidates
// AtGOV. It never decides: low-risk results are decided by the A01-AUTO
// policy when that is switched on, the rest wait for one HUMAN_FINAL batch
// press. Bounded batches, a resumable checkpoint, and no Cartesian
// generation: links only go to peers the reasoner picked from a short,
// related list, at most 3 per source, each unordered pair once.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/reason"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/policy"
)

// Version is recorded on every worker proposal.
const Version = "worker-1"

// failRev bumps when old failure counts must be forgotten.
const failRev = 4

// Limits.
const (
	DefaultBatch    = 3
	DefaultInterval = 60 * time.Second
	MaxPeers        = 8
	MaxKinPerSource = 3
	MaxFailures     = 3
)

// Control is the on/off and mode file the UI toggles. It is not authority:
// the worker only proposes.
type Control struct {
	On       bool   `json:"on"`
	Mode     string `json:"mode"`
	Batch    int    `json:"batch,omitempty"`
	Interval int    `json:"interval_seconds,omitempty"`
}

// Checkpoint makes the worker resumable. The ledger is the source of truth
// for what is done; the checkpoint keeps the cursor, failures and counters.
type Checkpoint struct {
	Version  string         `json:"version"`
	Cursor   string         `json:"cursor,omitempty"`
	Failures map[string]int `json:"failures,omitempty"`
	// Timeouts counts model timeouts per source (never blamed as failures);
	// each one halves the file text that source gets on its next try.
	Timeouts  map[string]int `json:"timeouts,omitempty"`
	FailRev   int            `json:"failures_rev,omitempty"`
	Processed int            `json:"processed"`
	Insights  int            `json:"insights"`
	Links     int            `json:"links"`
	Gaps      int            `json:"gaps,omitempty"`
	LastError string         `json:"last_error,omitempty"`
	LastRun   string         `json:"last_run,omitempty"`
	LastUsed  string         `json:"last_reasoner,omitempty"`
}

// Worker runs steps over one store.
type Worker struct {
	Store     *store.Store
	Insighter reason.Insighter
	Dir       string // control + checkpoint directory (outside the ledger)
	// SourcesRoot (optional) is where source_path files live; the first
	// TextChars characters of their text go into the insight prompt.
	SourcesRoot string
	TextChars   int
	mu          sync.Mutex
}

// WorkDir is the default control/checkpoint directory for a state root:
// a sibling "<state>-work" so the ledger directory is never touched.
func WorkDir(stateRoot string) string {
	clean := filepath.Clean(stateRoot)
	return clean + "-work"
}

func (w *Worker) controlPath() string    { return filepath.Join(w.Dir, "worker.json") }
func (w *Worker) checkpointPath() string { return filepath.Join(w.Dir, "checkpoint.json") }

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadControl reads the control file; missing means OFF, mode local.
func (w *Worker) LoadControl() Control {
	c := Control{Mode: reason.ModeLocal}
	if b, err := os.ReadFile(w.controlPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if !reason.ValidMode(c.Mode) {
		c.Mode = reason.ModeLocal
	}
	return c
}

// SaveControl writes the control file.
func (w *Worker) SaveControl(c Control) error {
	if !reason.ValidMode(c.Mode) {
		return fmt.Errorf("unknown reasoner mode %q", c.Mode)
	}
	return writeJSON(w.controlPath(), c)
}

// LoadCheckpoint reads the checkpoint (empty when missing).
func (w *Worker) LoadCheckpoint() Checkpoint {
	cp := Checkpoint{Version: Version, Failures: map[string]int{}}
	if b, err := os.ReadFile(w.checkpointPath()); err == nil {
		_ = json.Unmarshal(b, &cp)
	}
	if cp.Timeouts == nil {
		cp.Timeouts = map[string]int{}
	}
	if cp.Failures == nil || cp.FailRev != failRev {
		// counts from before failRev blamed sources for model-run errors
		cp.Failures, cp.FailRev = map[string]int{}, failRev
	}
	return cp
}

// MaxTimeoutShrink is the number of timeouts after which a source is sent
// with no file text at all (title, topic and peers only).
const MaxTimeoutShrink = 3

// ShrinkText halves the text budget once per earlier timeout of a source.
func ShrinkText(chars, timeouts int) int {
	if timeouts <= 0 || chars <= 0 {
		return chars
	}
	if timeouts >= MaxTimeoutShrink {
		return -1
	}
	return chars >> uint(timeouts)
}

func (w *Worker) saveCheckpoint(cp Checkpoint) error { return writeJSON(w.checkpointPath(), cp) }

type fixed struct {
	name   string
	result reason.Result
}

func (f fixed) Analyze(context.Context, reason.Input) (reason.Result, error) { return f.result, nil }
func (f fixed) Name() string                                                 { return f.name }
func (f fixed) Version(context.Context) string                               { return Version }

func isSource(em core.EmergION) bool {
	return strings.TrimSpace(em.REL["source_kind"]) == policy.KindSourceUpload
}

func related(em core.EmergION) []string {
	var out []string
	for _, r := range strings.Split(em.REL["source_related"], ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// pairKey is the unordered link key.
func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// Done reports what the ledger already holds: sources with an insight and
// linked pairs (any GOV state, so a held or rejected proposal is never
// regenerated).
func Done(st core.State) (insights map[string]bool, pairs map[string]bool) {
	insights, pairs = map[string]bool{}, map[string]bool{}
	for _, set := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Accepted, st.Held, st.Rejected, st.Returned} {
		for _, em := range set {
			switch strings.TrimSpace(em.REL["source_kind"]) {
			case policy.KindInsight:
				if id := strings.TrimSpace(em.REL["insight_of"]); id != "" {
					insights[id] = true
				}
			case policy.KindKinLink:
				pairs[pairKey(em.REL["kin_link_from"], em.REL["kin_link_to"])] = true
			}
		}
	}
	return insights, pairs
}

// Peers picks at most MaxPeers other accepted SOURCE records related to src:
// shared related tags first, then same topic, by ID.
func Peers(st core.State, src core.EmergION) []core.EmergION {
	tags := map[string]bool{}
	for _, t := range related(src) {
		tags[t] = true
	}
	type cand struct {
		em    core.EmergION
		score int
	}
	var cs []cand
	topic := strings.TrimSpace(src.REL["source_topic"])
	for id, em := range st.Accepted {
		if id == src.IDN || !isSource(em) || policy.LinkNoise(st, src.IDN, id) != "" {
			// only real documents sharing a real topic (quality rules)
			continue
		}
		score := 0
		for _, t := range related(em) {
			if tags[t] {
				score += 2
			}
		}
		if topic != "" && strings.TrimSpace(em.REL["source_topic"]) == topic {
			score++
		}
		if score > 0 {
			cs = append(cs, cand{em, score})
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].score != cs[j].score {
			return cs[i].score > cs[j].score
		}
		return cs[i].em.IDN < cs[j].em.IDN
	})
	if len(cs) > MaxPeers {
		cs = cs[:MaxPeers]
	}
	out := make([]core.EmergION, len(cs))
	for i, c := range cs {
		out[i] = c.em
	}
	return out
}

// StepReport is one bounded worker step.
type StepReport struct {
	Pending   int    `json:"pending"`
	Processed int    `json:"processed"`
	Insights  int    `json:"insights"`
	Links     int    `json:"links"`
	Gaps      int    `json:"gaps,omitempty"`
	Failed    int    `json:"failed"`
	Reasoner  string `json:"reasoner,omitempty"`
	// Unavailable is set when the reasoner could not run at all; the step
	// stopped without counting failures.
	Unavailable bool `json:"unavailable,omitempty"`
}

func (w *Worker) state() (core.State, error) {
	events, err := w.Store.Events()
	if err != nil {
		return core.State{}, err
	}
	return livefield.Rebuild(events)
}

// Step processes at most batch accepted SOURCE records that have no insight
// yet, starting after the checkpoint cursor. Safe to run repeatedly: what the
// ledger already holds is skipped.
func (w *Worker) Step(ctx context.Context, batch int) (StepReport, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if batch <= 0 {
		batch = DefaultBatch
	}
	var rep StepReport
	st, err := w.state()
	if err != nil {
		return rep, err
	}
	insights, pairs := Done(st)
	cp := w.LoadCheckpoint()
	var todo []string
	for id, em := range st.Accepted {
		if isSource(em) && !insights[id] && cp.Failures[id] < MaxFailures && policy.SourceNoise(st, id) == "" {
			todo = append(todo, id)
		}
	}
	sort.Strings(todo)
	rep.Pending = len(todo)
	// Gap pass first: no model call, bounded. It runs at the start of every
	// step so a slow or timed-out reasoner (or a fieldd restart mid-step)
	// can never starve it: SOURCE_GAP candidates for accepted insights are
	// proposed AtGOV (HUMAN_FINAL) even when the model never answers.
	if ctx.Err() == nil {
		for _, c := range w.PlanGaps(st, MaxGapsPerStep) {
			if _, err := w.captureGap(ctx, c); err != nil {
				return rep, err
			}
			rep.Gaps++
			cp.Gaps++
		}
		if rep.Gaps > 0 {
			if err := w.saveCheckpoint(cp); err != nil {
				return rep, err
			}
		}
	}
	// resume after the cursor, wrapping
	start := sort.SearchStrings(todo, cp.Cursor)
	if start < len(todo) && todo[start] == cp.Cursor {
		start++
	}
	order := append(append([]string(nil), todo[start:]...), todo[:start]...)
	for _, id := range order {
		if rep.Processed+rep.Failed >= batch || ctx.Err() != nil {
			break
		}
		src := st.Accepted[id]
		peers := Peers(st, src)
		in := reason.InsightInput{
			SourceID: id,
			Title:    src.REL["source_title"],
			Topic:    src.REL["source_topic"],
			Mime:     src.REL["source_mime"],
			Related:  related(src),
		}
		tc := w.TextChars
		if tc == 0 {
			tc = DefaultTextChars
		}
		// a source whose earlier runs timed out gets half the text per timeout
		// (none after MaxTimeoutShrink), so a slow phone still finishes it
		tc = ShrinkText(tc, cp.Timeouts[id])
		in.Text = ExtractText(ctx, w.SourcesRoot, src.REL["source_path"], src.REL["source_mime"], tc)
		for i, p := range peers {
			in.Peers = append(in.Peers, reason.InsightPeer{N: i + 1, ID: p.IDN, Title: p.REL["source_title"], Topic: p.REL["source_topic"]})
		}
		ins, err := w.Insighter.Insight(ctx, in)
		if sw, ok := w.Insighter.(interface{ LastUsed() string }); ok {
			rep.Reasoner = sw.LastUsed()
		} else {
			rep.Reasoner = w.Insighter.Name()
		}
		cp.LastUsed = rep.Reasoner
		if err != nil && ctx.Err() != nil {
			// The step was cancelled (fieldd stopping or restarting) while the
			// model was still working. The source is not to blame and was not
			// processed: keep its failure count and the cursor, so it is the
			// first record tried on the next start.
			cp.LastError = "interrupted while reasoning on " + id + " (fieldd stopped); it is retried first on the next start"
			_ = w.saveCheckpoint(cp)
			break
		}
		if err != nil && errors.Is(err, reason.ErrUnavailable) {
			// The reasoner cannot run at all: pause, do not blame the source.
			cp.LastError = err.Error()
			rep.Unavailable = true
			_ = w.saveCheckpoint(cp)
			break
		}
		if err != nil && errors.Is(err, reason.ErrModelRun) {
			// The model run itself failed: report it, move on, do not
			// blame the source.
			cp.LastError = err.Error()
			cp.Cursor = id
			rep.Failed++
			if errors.Is(err, reason.ErrModelTimeout) {
				// Remember the timeout so the next try sends less text, and
				// end the step: the next step starts with the next source
				// instead of queueing more work on a model that is busy.
				cp.Timeouts[id]++
				_ = w.saveCheckpoint(cp)
				break
			}
			_ = w.saveCheckpoint(cp)
			continue
		}
		if err != nil {
			cp.Failures[id]++
			cp.LastError = err.Error()
			cp.Cursor = id
			rep.Failed++
			_ = w.saveCheckpoint(cp)
			continue
		}
		var kinIDs []string
		for _, k := range ins.Kin {
			if k.Peer >= 1 && k.Peer <= len(peers) {
				kinIDs = append(kinIDs, peers[k.Peer-1].IDN)
			}
		}
		if _, err := w.captureInsight(ctx, src, ins, kinIDs, rep.Reasoner); err != nil {
			return rep, err
		}
		rep.Insights++
		cp.Insights++
		for i, k := range ins.Kin {
			if i >= MaxKinPerSource || k.Peer < 1 || k.Peer > len(peers) {
				continue
			}
			peer := peers[k.Peer-1]
			key := pairKey(id, peer.IDN)
			if pairs[key] || policy.LinkNoise(st, id, peer.IDN) != "" {
				continue
			}
			if _, err := w.captureLink(ctx, src, peer, k.Why); err != nil {
				return rep, err
			}
			pairs[key] = true
			rep.Links++
			cp.Links++
		}
		delete(cp.Timeouts, id)
		cp.Cursor = id
		cp.Processed++
		cp.LastError = ""
		cp.LastRun = time.Now().UTC().Format(time.RFC3339)
		rep.Processed++
		if err := w.saveCheckpoint(cp); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func (w *Worker) captureInsight(ctx context.Context, src core.EmergION, ins reason.Insight, kin []string, reasoner string) (core.EmergION, error) {
	var b strings.Builder
	b.WriteString("SOURCE_INSIGHT/1\n")
	fmt.Fprintf(&b, "OF:%s\nOF_SHA256:%s\nREASONER:%s\nWORKER:%s\n", src.IDN, src.REL["source_sha256"], reasoner, Version)
	fmt.Fprintf(&b, "SUMMARY:%s\n", ins.Summary)
	for _, c := range ins.Combinations {
		fmt.Fprintf(&b, "COMBO:%s\n", c)
	}
	for _, g := range ins.Gaps {
		fmt.Fprintf(&b, "GAP:%s\n", g)
	}
	fmt.Fprintf(&b, "KIN:%s\nZ\n", strings.Join(kin, ","))
	rel := map[string]string{
		"source_kind":      policy.KindInsight,
		"insight_of":       src.IDN,
		"source_topic":     src.REL["source_topic"],
		"insight_reasoner": reasoner,
		"worker_version":   Version,
	}
	if len(kin) > 0 {
		rel["insight_kin"] = strings.Join(kin, ",")
	}
	if len(ins.Combinations) > 0 {
		rel["insight_combo"] = strings.Join(ins.Combinations, " | ")
	}
	res := reason.Result{
		Summary:       "Insight on " + src.REL["source_title"] + ": " + ins.Summary,
		Relationships: rel,
		Capabilities:  []string{"OBS"},
		Facts:         []string{"worker_insight_of:" + src.IDN},
		Gaps:          ins.Gaps,
		Risk:          "L",
	}
	rt := fieldruntime.Runtime{Store: w.Store, Reasoner: fixed{name: "worker-insight", result: res}}
	em, _, err := rt.CaptureBytes(ctx, "source-insight", []byte(b.String()), policy.ProvenanceInsight)
	return em, err
}

func (w *Worker) captureLink(ctx context.Context, a, b core.EmergION, why string) (core.EmergION, error) {
	from, to := a.IDN, b.IDN
	if from > to {
		from, to = to, from
	}
	why = strings.Join(strings.Fields(why), " ")
	if why == "" {
		why = "reasoner"
	}
	desc := fmt.Sprintf("SOURCE_KIN_LINK/1\nFROM:%s\nTO:%s\nBASIS:%s\nWORKER:%s\nZ\n", from, to, why, Version)
	rel := map[string]string{
		"source_kind":    policy.KindKinLink,
		"kin_link_from":  from,
		"kin_link_to":    to,
		"kin_link_basis": "reasoner",
		"kin_link_why":   why,
		"source_topic":   a.REL["source_topic"],
		"worker_version": Version,
	}
	res := reason.Result{
		Summary:       fmt.Sprintf("Kin link: %s ↔ %s", a.REL["source_title"], b.REL["source_title"]),
		Relationships: rel,
		Capabilities:  []string{"OBS"},
		Facts:         []string{"worker_kin_link:" + from + "," + to},
		Risk:          "L",
	}
	rt := fieldruntime.Runtime{Store: w.Store, Reasoner: fixed{name: "worker-kin-link", result: res}}
	em, _, err := rt.CaptureBytes(ctx, "source-kin-link", []byte(desc), policy.ProvenanceKinLink)
	return em, err
}
