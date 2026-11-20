package fieldapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
)

const collidingArchonym = "BDC Innovation Funding Package"

// archonymReasoner proposes a fixed Archonym, the way the 2b54f72 ingest
// (and any model) could propose one.
type archonymReasoner struct{ archonym, kind string }

func (a archonymReasoner) Analyze(_ context.Context, in reason.Input) (reason.Result, error) {
	return reason.Result{
		Summary:       "archonym test source " + in.Name,
		Archonym:      a.archonym,
		Relationships: map[string]string{"source_kind": a.kind},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"archonym_test_source"},
		Risk:          "L",
	}, nil
}
func (archonymReasoner) Name() string                   { return "archonym-test" }
func (archonymReasoner) Version(context.Context) string { return "1" }

func captureNamed(t *testing.T, rt *Runtime, archonym, kind, body string) core.EmergION {
	t.Helper()
	em, dup, err := (fieldruntime.Runtime{Store: rt.store, Reasoner: archonymReasoner{archonym, kind}}).
		CaptureBytes(context.Background(), "archonym-test", []byte(body), "archonym_test")
	if err != nil || dup {
		t.Fatalf("capture %q: dup=%v err=%v", body, dup, err)
	}
	if em.EVO.Metadata == nil || em.EVO.Metadata.Archonym != archonym {
		t.Fatalf("capture did not carry Archonym %q: %+v", archonym, em.EVO.Metadata)
	}
	return em
}

func manifestLine(seed, title string) string {
	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(seed)))
	return fmt.Sprintf(`{"path":"sources/grants-funding/%s.jpg","sha256":"%s","bytes":1234,"mime":"image/jpeg","topic":"grants-funding","title":%q,"source_kind":"upload","captured_at":"2026-05-30T10:00:00-07:00"}`, seed, sha, title)
}

func readable(t *testing.T, rt *Runtime) (core.State, LinkedProjectionWire) {
	t.Helper()
	if _, err := rt.RenderCurrentJSON(filepath.Join(t.TempDir(), "outputs")); err != nil {
		t.Fatalf("/field.json path failed: %v", err)
	}
	body, err := rt.LinkedProjectionJSON()
	if err != nil {
		t.Fatalf("/projection.json path failed: %v", err)
	}
	var lp LinkedProjectionWire
	if err := json.Unmarshal([]byte(body), &lp); err != nil {
		t.Fatal(err)
	}
	st, err := rt.state()
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	return st, lp
}

type LinkedProjectionWire struct {
	Records []struct {
		ID       string `json:"id"`
		Archonym string `json:"archonym"`
	} `json:"records"`
	ArchonymConflicts []core.ArchonymConflict `json:"archonym_conflicts"`
	ArchonymWaiting   []string                `json:"archonym_waiting_conflicts"`
}

// Reproduces the phone state: an accepted record owns X, the 2b54f72 ingest
// recorded a SOURCE_UPLOAD titled X with Archonym X, and an accept of it was
// written without an Archonym check. Replay used to fail with
// "Archonym X already belongs to REG-accepted sovereign EmergION ..." and
// /field.json and /projection.json returned 500.
func TestArchonymCollisionFromIngestKeepsFieldReadable(t *testing.T) {
	rt, err := Open(filepath.Join(t.TempDir(), "state"), skyhookSilentReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	owner := captureNamed(t, rt, collidingArchonym, "TEST", "accepted owner evidence")
	if err := rt.Decide(owner.IDN, "APPROVE", "HUMAN_FINAL test owner"); err != nil {
		t.Fatal(err)
	}

	// Legacy (2b54f72) ingest record: waiting AtGOV with the colliding name.
	legacy := captureNamed(t, rt, collidingArchonym, fieldruntime.SourceUploadKind, "legacy ingest descriptor")
	st, lp0 := readable(t, rt)
	if _, ok := st.AtGOV[legacy.IDN]; !ok {
		t.Fatal("legacy waiting record not visible AtGOV")
	}
	if len(lp0.ArchonymWaiting) != 1 || lp0.ArchonymWaiting[0] != legacy.IDN {
		t.Fatalf("archonym_waiting_conflicts = %v", lp0.ArchonymWaiting)
	}

	// The exact error the phone saw is what the strict accept gate reports.
	if err := livefield.CheckArchonymForAccept(st, legacy); err == nil ||
		err.Error() != fmt.Sprintf("Archonym %q already belongs to REG-accepted sovereign EmergION %s", collidingArchonym, owner.IDN) {
		t.Fatalf("strict accept gate = %v", err)
	}

	// Write the ungated D + R the old accept path appended (the ledger now on
	// the phone). The ledger is never rewritten.
	em, receipt, err := gov.Decide(legacy, gov.Approve, "HUMAN_FINAL", "legacy ungated accept")
	if err != nil {
		t.Fatal(err)
	}
	did, err := rt.store.SaveDecision(receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, rr, err := reg.Accept(em, did)
	if err != nil {
		t.Fatal(err)
	}
	regEventID, err := rt.store.SaveAccepted(rr)
	if err != nil {
		t.Fatal(err)
	}

	st, lp := readable(t, rt)
	if got := st.Accepted[owner.IDN]; got.EVO.Metadata == nil || got.EVO.Metadata.Archonym != collidingArchonym {
		t.Fatal("accepted owner lost its Archonym")
	}
	if _, ok := st.Accepted[legacy.IDN]; ok {
		t.Fatal("colliding record entered REG")
	}
	if _, ok := st.Approved[legacy.IDN]; !ok {
		t.Fatal("refused record is not visible as Approved")
	}
	if len(lp.ArchonymConflicts) != 1 {
		t.Fatalf("archonym_conflicts = %+v", lp.ArchonymConflicts)
	}
	c := lp.ArchonymConflicts[0]
	if c.EmergIONID != legacy.IDN || c.OwnerID != owner.IDN || c.Archonym != collidingArchonym ||
		c.OwnerState != livefield.OwnerREGAccepted || c.REGEventID != regEventID {
		t.Fatalf("conflict = %+v", c)
	}
	owners := 0
	for _, r := range lp.Records {
		if strings.EqualFold(r.Archonym, collidingArchonym) {
			owners++
			if r.ID != owner.IDN {
				t.Fatalf("Archonym shown on %s, want owner %s", r.ID, owner.IDN)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("Archonym shown on %d records", owners)
	}

	// Accepting another colliding waiting record is still refused, and
	// nothing is written.
	again := captureNamed(t, rt, collidingArchonym, fieldruntime.SourceUploadKind, "second legacy ingest descriptor")
	before := len(ledger(t, rt))
	if err := rt.Decide(again.IDN, "APPROVE", "HUMAN_FINAL must be refused"); err == nil ||
		!strings.Contains(err.Error(), "already belongs to REG-accepted sovereign EmergION "+owner.IDN) {
		t.Fatalf("colliding accept not refused: %v", err)
	}
	res, err := rt.DecideBatchHuman([]string{again.IDN}, "batch must be refused", 1, DefaultBatchCap)
	if err == nil || res.Accepted != 0 || res.StoppedAt != again.IDN {
		t.Fatalf("colliding batch accept not refused: %+v %v", res, err)
	}
	if after := len(ledger(t, rt)); after != before {
		t.Fatalf("refused accept wrote %d events", after-before)
	}
	st, _ = readable(t, rt)
	if _, ok := st.AtGOV[again.IDN]; !ok {
		t.Fatal("refused record left GOV")
	}
	// HOLD of a colliding record is unaffected.
	if err := rt.Decide(again.IDN, "HOLD", "HUMAN_FINAL hold"); err != nil {
		t.Fatalf("HOLD refused: %v", err)
	}
}

func TestIngestDoesNotClaimArchonymAndStaysIdempotent(t *testing.T) {
	rt, err := Open(filepath.Join(t.TempDir(), "state"), skyhookSilentReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	owner := captureNamed(t, rt, collidingArchonym, "TEST", "accepted owner evidence")
	if err := rt.Decide(owner.IDN, "APPROVE", "HUMAN_FINAL test owner"); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Join([]string{
		manifestLine("bdc-a", collidingArchonym),
		manifestLine("bdc-b", collidingArchonym),
		manifestLine("other", "Other upload"),
	}, "\n")
	ing := fieldruntime.Runtime{Store: rt.store}
	first, err := ing.IngestSourceManifest(context.Background(), strings.NewReader(manifest), fieldruntime.SourceIngestOptions{})
	if err != nil || first.Created != 3 {
		t.Fatalf("first ingest = %+v %v", first, err)
	}
	st, lp := readable(t, rt)
	if len(lp.ArchonymConflicts) != 0 || len(lp.ArchonymWaiting) != 0 {
		t.Fatalf("conflicts = %+v waiting = %v", lp.ArchonymConflicts, lp.ArchonymWaiting)
	}
	for _, id := range first.CreatedIDs {
		em, ok := st.AtGOV[id]
		if !ok {
			t.Fatalf("%s not waiting AtGOV", id)
		}
		if em.EVO.Metadata != nil && em.EVO.Metadata.Archonym != "" {
			t.Fatalf("ingest claimed Archonym %q", em.EVO.Metadata.Archonym)
		}
		if em.REL["source_title"] == "" || !strings.Contains(em.MEM.Summary, em.REL["source_title"]) {
			t.Fatalf("title not kept as source_title/summary: %v %q", em.REL, em.MEM.Summary)
		}
	}
	// A waiting upload titled like an accepted record can be accepted
	// without taking the name.
	if err := rt.Decide(first.CreatedIDs[0], "APPROVE", "HUMAN_FINAL source"); err != nil {
		t.Fatalf("accept of untitled upload: %v", err)
	}
	st, _ = readable(t, rt)
	if st.Accepted[owner.IDN].EVO.Metadata.Archonym != collidingArchonym {
		t.Fatal("owner lost Archonym")
	}

	events := len(ledger(t, rt))
	second, err := ing.IngestSourceManifest(context.Background(), strings.NewReader(manifest), fieldruntime.SourceIngestOptions{})
	if err != nil || second.Created != 0 || second.AlreadyPresent != 3 {
		t.Fatalf("re-ingest = %+v %v", second, err)
	}
	if got := len(ledger(t, rt)); got != events {
		t.Fatalf("re-ingest wrote %d events", got-events)
	}
}
