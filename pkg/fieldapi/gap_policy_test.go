package fieldapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/policy"
)

type gapOnlyInsighter struct{}

func (gapOnlyInsighter) Name() string { return "gapfake" }
func (gapOnlyInsighter) Insight(_ context.Context, in reason.InsightInput) (reason.Insight, error) {
	return reason.Insight{Summary: "about " + in.Title, Gaps: []string{"No backup or restore after reboot", "No paying customer"}}, nil
}

func gapManifestLine(seed string) string {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(seed)))
	return fmt.Sprintf(`{"path":"sources/inventions-ip/%s.pdf","sha256":"%s","bytes":10,"mime":"application/pdf","topic":"inventions-ip","title":"Helix brief %s","source_kind":"upload","related":["TR-1"]}`, seed, h, seed)
}

// With A01-AUTO switched on, uploads and insights auto-accept but every
// SOURCE_GAP stays AtGOV with no decision receipt: HUMAN_FINAL only.
func TestAutoPolicyOnLeavesSourceGapAtGOV(t *testing.T) {
	rt := openTestRuntime(t)
	if _, err := rt.SetAutoPolicy(context.Background(), true, "Michael approved A01-AUTO"); err != nil {
		t.Fatal(err)
	}
	ingestLines(t, rt, gapManifestLine("g1"), gapManifestLine("g2"))
	if _, err := rt.ApplyAutoPolicy(0, ""); err != nil {
		t.Fatal(err)
	}
	w := rt.NewWorker(t.TempDir(), gapOnlyInsighter{})
	for i := 0; i < 3; i++ {
		if _, err := w.Step(context.Background(), 5); err != nil {
			t.Fatal(err)
		}
		if _, err := rt.ApplyAutoPolicy(0, ""); err != nil {
			t.Fatal(err)
		}
	}
	st, err := rt.state()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	insights, gaps := 0, 0
	for _, em := range st.Accepted {
		switch em.REL["source_kind"] {
		case policy.KindInsight:
			insights++
		case policy.KindGap:
			t.Fatalf("SOURCE_GAP %s was accepted without HUMAN_FINAL", em.IDN)
		}
	}
	for id, em := range st.AtGOV {
		if em.REL["source_kind"] != policy.KindGap {
			continue
		}
		gaps++
		if d := decisionsFor(t, rt, id); len(d) != 0 {
			t.Fatalf("SOURCE_GAP %s got a decision: %+v", id, d)
		}
	}
	for _, em := range st.Held {
		if em.REL["source_kind"] == policy.KindGap {
			t.Fatalf("SOURCE_GAP %s was auto-held", em.IDN)
		}
	}
	if insights != 2 || gaps != 4 {
		t.Fatalf("insights %d gaps %d, want 2 and 4", insights, gaps)
	}
}
