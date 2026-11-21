package worker

import (
	"context"
	"sort"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
)

// cancelInsighter cancels the step's context mid-call, the way a fieldd
// restart does while a long Gemma call is running, and returns ctx.Err().
type cancelInsighter struct {
	cancel context.CancelFunc
	calls  []string
}

func (c *cancelInsighter) Name() string { return "cancel" }
func (c *cancelInsighter) Insight(ctx context.Context, in reason.InsightInput) (reason.Insight, error) {
	c.calls = append(c.calls, in.SourceID)
	c.cancel()
	<-ctx.Done()
	return reason.Insight{}, ctx.Err()
}

// A restart that cancels the step mid-reasoning must not blame the source,
// must not advance the cursor past it, and the next start must retry the
// same record first, so progress is never lost across restarts.
func TestInterruptedStepKeepsProgressAndDoesNotBlameSource(t *testing.T) {
	s, ids := setup(t, 3)
	sort.Strings(ids)
	dir := t.TempDir()
	for round := 0; round < MaxFailures+2; round++ {
		ctx, cancel := context.WithCancel(context.Background())
		ci := &cancelInsighter{cancel: cancel}
		w := &Worker{Store: s, Insighter: ci, Dir: dir}
		rep, err := w.Step(ctx, 3)
		cancel()
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if rep.Failed != 0 || rep.Processed != 0 || rep.Pending != 3 {
			t.Fatalf("round %d: interrupted step must not count as a failure: %+v", round, rep)
		}
		if len(ci.calls) != 1 || ci.calls[0] != ids[0] {
			t.Fatalf("round %d: must retry the same first record, got %v want %s", round, ci.calls, ids[0])
		}
		cp := w.LoadCheckpoint()
		if len(cp.Failures) != 0 || cp.Cursor != "" || !strings.Contains(cp.LastError, "interrupted") {
			t.Fatalf("round %d: checkpoint must keep progress: %+v", round, cp)
		}
	}
	// After the interruptions the record is processed normally.
	w := &Worker{Store: s, Insighter: &fakeInsighter{}, Dir: dir}
	rep, err := w.Step(context.Background(), 1)
	if err != nil || rep.Processed != 1 {
		t.Fatalf("resume: %+v %v", rep, err)
	}
	if cp := w.LoadCheckpoint(); cp.Cursor != ids[0] || cp.LastError != "" {
		t.Fatalf("resume checkpoint: %+v", cp)
	}
}
