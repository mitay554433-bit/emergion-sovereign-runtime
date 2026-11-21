package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/policy"
)

// timeoutInsighter times out on every call and records the text it got.
type timeoutInsighter struct {
	calls int
	texts []int
}

func (t *timeoutInsighter) Name() string { return "timeout" }
func (t *timeoutInsighter) Insight(_ context.Context, in reason.InsightInput) (reason.Insight, error) {
	t.calls++
	t.texts = append(t.texts, len([]rune(in.Text)))
	return reason.Insight{}, fmt.Errorf("%w: %w: local model timed out after 5m0s", reason.ErrModelRun, reason.ErrModelTimeout)
}

func TestShrinkTextHalvesPerTimeoutThenDrops(t *testing.T) {
	for _, c := range []struct{ in, n, want int }{{1500, 0, 1500}, {1500, 1, 750}, {1500, 2, 375}, {1500, 3, -1}, {1500, 9, -1}, {-1, 1, -1}} {
		if got := ShrinkText(c.in, c.n); got != c.want {
			t.Fatalf("ShrinkText(%d,%d) = %d want %d", c.in, c.n, got, c.want)
		}
	}
}

// A model timeout is never blamed on the source, ends the step at once (no
// more work queued on a busy model), keeps the cursor, and the next try of
// that source gets half the file text.
func TestModelTimeoutEndsStepKeepsProgressAndShrinksText(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sources", "inventions-ip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "inventions-ip", "r1.txt"), []byte(strings.Repeat("Strain gauge drift measured. ", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ids := acceptLines(t, s, []string{titled("r1", "inventions-ip", "Helix bench protocol", "sources/inventions-ip/r1.txt")})
	ti := &timeoutInsighter{}
	w := &Worker{Store: s, Insighter: ti, Dir: t.TempDir(), SourcesRoot: root}
	for i := 0; i < 4; i++ {
		rep, err := w.Step(context.Background(), 3)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Failed != 1 || rep.Processed != 0 {
			t.Fatalf("step %d: %+v (one call, then stop)", i, rep)
		}
	}
	if ti.calls != 4 {
		t.Fatalf("calls %d, want one per step", ti.calls)
	}
	want := []int{DefaultTextChars, DefaultTextChars / 2, DefaultTextChars / 4, 0}
	for i, n := range want {
		if ti.texts[i] != n {
			t.Fatalf("text sizes %v, want %v", ti.texts, want)
		}
	}
	cp := w.LoadCheckpoint()
	if cp.Failures[ids[0]] != 0 || cp.Timeouts[ids[0]] != 4 || cp.Cursor != ids[0] || !strings.Contains(cp.LastError, "timed out") {
		t.Fatalf("checkpoint %+v", cp)
	}
	// a later success clears the timeout count
	w.Insighter = &fakeInsighter{}
	if rep, err := w.Step(context.Background(), 3); err != nil || rep.Processed != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if cp := w.LoadCheckpoint(); cp.Timeouts[ids[0]] != 0 {
		t.Fatalf("timeouts not cleared: %+v", cp.Timeouts)
	}
}

// The gap pass runs at the start of every step, so SOURCE_GAP candidates for
// accepted insights are proposed even when every model call times out.
func TestGapPassRunsEvenWhenModelTimesOut(t *testing.T) {
	s, _ := setup(t, 2)
	w := &Worker{Store: s, Insighter: gapInsighter{gaps: []string{"Backup and restore untested"}}, Dir: t.TempDir()}
	if _, err := w.Step(context.Background(), 1); err != nil { // one insight AtGOV
		t.Fatal(err)
	}
	acceptAtGOV(t, s, policy.KindInsight)
	w.Insighter = &timeoutInsighter{}
	rep, err := w.Step(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Gaps != 1 || rep.Failed != 1 {
		t.Fatalf("gap pass starved by the model timeout: %+v", rep)
	}
	if g := gapsIn(state(t, s)); len(g) != 1 {
		t.Fatalf("gaps %d", len(g))
	}
}
