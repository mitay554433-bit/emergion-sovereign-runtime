package fieldapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gemmalive"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

// skyhookValue is the exact persistent value from
// MILESTONE_LOCK_PERSISTENT_FIELD_COGNITION_V1 (22 Jun 2026):
// "Skyhook test value is 84721" was written, the runtime was restarted,
// and the value was still recalled. The historical conclusion was that
// state lives outside the model weights. Current canon: REG holds accepted
// state; FIELD is rebuilt from it; the model is a replaceable proposer.
const skyhookValue = "Skyhook test value is 84721"

// skyhookSilentReasoner proposes analysis that does not mention 84721, so
// a passing test cannot be explained by the reasoner inventing the value.
type skyhookSilentReasoner struct{}

func (skyhookSilentReasoner) Analyze(_ context.Context, _ reason.Input) (reason.Result, error) {
	return reason.Result{
		Summary:       "bounded skyhook persistence source",
		Relationships: map[string]string{"source_kind": "TEST"},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"bounded skyhook persistence source"},
		Risk:          "L",
	}, nil
}

func (skyhookSilentReasoner) Name() string                   { return "skyhook-silent-test" }
func (skyhookSilentReasoner) Version(context.Context) string { return "1" }

func TestSkyhookValueSurvivesRestartWithoutModelWeights(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	sourcePath := filepath.Join(root, "skyhook.txt")
	if err := os.WriteFile(sourcePath, []byte(skyhookValue+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	rt, err := Open(stateRoot, skyhookSilentReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	em, err := rt.Capture(context.Background(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(em.MEM.Summary, "84721") {
		t.Fatal("reasoner summary must not carry the Skyhook value")
	}
	for _, fact := range em.VAL.Facts {
		if strings.Contains(fact, "84721") {
			t.Fatalf("reasoner fact unexpectedly contains Skyhook value: %q", fact)
		}
	}
	if err := rt.Decide(em.IDN, "APPROVE", "accept skyhook persistence evidence"); err != nil {
		t.Fatal(err)
	}

	// Restart: reopen the same state directory with no model at all.
	reopenedStore, err := store.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(stateRoot, reason.GemmaCLI{Binary: gemmalive.AbsentBinary, Model: gemmalive.AbsentModel})
	if err != nil {
		t.Fatal(err)
	}

	events, err := reopenedStore.Events()
	if err != nil {
		t.Fatal(err)
	}
	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := state.Accepted[em.IDN]
	if !ok {
		t.Fatalf("Skyhook record %s did not survive restart/FIELD rebuild", em.IDN)
	}
	if accepted.MEM.SourceHash != em.MEM.SourceHash {
		t.Fatalf("source hash changed across restart: %q -> %q", em.MEM.SourceHash, accepted.MEM.SourceHash)
	}

	evidence, err := reopenedStore.ReadEvidence(accepted.MEM.SourceHash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(evidence), skyhookValue) {
		t.Fatalf("preserved evidence after restart = %q, want %q", evidence, skyhookValue)
	}

	status, err := reopened.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 1 {
		t.Fatalf("accepted after restart = %d want 1", status.Accepted)
	}
}
