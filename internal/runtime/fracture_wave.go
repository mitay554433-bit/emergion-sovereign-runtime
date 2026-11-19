package runtime

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
)

// Recursive fracture wave.
//
// TARGET PRESSURE -> GOVERNED REALITY -> SATISFIED / GAP -> probe ->
// stoppage -> smallest structural addition -> same probe again -> recompute
// target.
//
// ProposeNextFractureWaveRound advances the wave by at most one governed
// step per call. It reads only REG-accepted records and their preserved
// evidence, proposes at most one candidate, and never applies, authorizes
// or executes anything. Every probe still needs HUMAN_FINAL and a fresh Q
// per run; probes never run automatically.
//
// The probe ladder is fixed and walks the existing execution seams, one
// level deeper each time the previous level's probe passes:
//
//	catalog  - an adapter in adapters.Catalog expresses each recipe token
//	prepare  - adapters.PrepareExecution derives the action for each token
//	enabled  - an enabled adapter would execute each derived action
//
// A deeper probe is generated only from the boundary record plus the
// previous probe's accepted PASS evidence (and the stoppage it had shown),
// never from a preselected fix. When the ladder is exhausted the target is
// recomputed against current REG reality (probes stay excluded), so a closed
// gap becomes SATISFIED and an open one is probed again from its new
// boundary.

const (
	fractureLevelCatalog = "catalog"
	fractureLevelPrepare = "prepare"
	fractureLevelEnabled = "enabled"
)

var fractureLadder = []string{fractureLevelCatalog, fractureLevelPrepare, fractureLevelEnabled}

// Wave statuses, one per active boundary.
const (
	waveSatisfied          = "SATISFIED"
	waveNoRemainder        = "NO_GENUINE_REMAINDER"
	waveProposedProbe      = "PROPOSED_PROBE"
	waveAwaitingHumanFinal = "AWAITING_HUMAN_FINAL"
	waveAwaitingRun        = "AWAITING_Q_AND_RUN"
	waveRerunSameProbe     = "RERUN_SAME_PROBE_AFTER_STRUCTURAL_ADDITION"
	waveNoNewStoppage      = "NO_NEW_STOPPAGE"
	waveMaxRounds          = "MAX_ROUNDS"
	waveRecomputeProposed  = "RECOMPUTE_PROPOSED"
	waveRecomputeNeeded    = "RECOMPUTE_NEEDS_OBSERVER"
	waveTargetUnchanged    = "TARGET_UNCHANGED"
	waveTargetNotFound     = "TARGET_NOT_FOUND"
)

// FractureWaveStatus reports the wave state of one active boundary.
type FractureWaveStatus struct {
	Target    string `json:"target"`
	Boundary  string `json:"boundary"`
	Status    string `json:"status"`
	Level     string `json:"level,omitempty"`
	Round     int    `json:"round"`
	Probe     string `json:"probe,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Stoppage  string `json:"stoppage,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// fracturePrevious is the accepted evidence a deeper probe is derived from.
type fracturePrevious struct {
	Probe        string
	Level        string
	Evidence     string
	EvidenceHash string
	Stoppage     string
}

func fractureProbeLevel(em core.EmergION) string {
	if level := strings.TrimSpace(em.REL["fracture_level"]); level != "" {
		return level
	}
	return fractureLevelCatalog
}

func fractureLevelIndex(level string) int {
	for i, candidate := range fractureLadder {
		if candidate == level {
			return i
		}
	}
	return -1
}

// fractureProbePathAt keeps the round-one catalog path unchanged and names
// deeper probes by level.
func fractureProbePathAt(boundaryID, level string) string {
	if level == fractureLevelCatalog {
		return fractureProbePath(boundaryID)
	}
	return strings.TrimSuffix(fractureProbePath(boundaryID), "_test.go") + "_" + level + "_test.go"
}

// firstFractureStoppage returns the first truthful stoppage line of a probe
// run: the probe's own FRACTURE_STOPPAGE line, else the first failing test,
// else the first non-empty line.
func firstFractureStoppage(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if i := strings.Index(line, "FRACTURE_STOPPAGE"); i >= 0 {
			return strings.TrimSpace(line[i:])
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "--- FAIL") {
			return strings.TrimSpace(line)
		}
	}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// renderFractureProbeAt renders the probe for one ladder level. The catalog
// level is the unchanged round-one probe. Deeper levels also carry the
// previous accepted evidence they were derived from.
func renderFractureProbeAt(em core.EmergION, level string, previous fracturePrevious) ([]byte, error) {
	if level == fractureLevelCatalog {
		return renderFractureProbe(em)
	}
	if fractureLevelIndex(level) < 1 {
		return nil, fmt.Errorf("unknown fracture level %q", level)
	}
	if previous.Probe == "" || previous.Evidence == "" || previous.EvidenceHash == "" {
		return nil, fmt.Errorf("fracture level %s needs accepted previous evidence", level)
	}
	gap, required, err := fractureBoundary(em)
	if err != nil {
		return nil, err
	}
	recipe := requiredCapabilityRecipe(required)
	if len(recipe) == 0 {
		return nil, fmt.Errorf("fracture boundary %s requirement %s has no recipe", em.IDN, required)
	}

	quoted := func(values []string) string {
		parts := make([]string, 0, len(values))
		for _, value := range values {
			parts = append(parts, fmt.Sprintf("%q", value))
		}
		return "[]string{" + strings.Join(parts, ", ") + "}"
	}
	differences := append([]string(nil), em.VAL.Differences...)
	sort.Strings(differences)
	funcName := "TestFracture" + strings.ReplaceAll(strings.TrimSpace(em.IDN), "-", "_") + "_" + level

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by the UNIFUSION TORQUE fracture wave. DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// Boundary %s, %s, level %s. Derived from accepted evidence %s of\n", em.IDN, gap, level, previous.Evidence)
	fmt.Fprintf(&b, "// probe %s. Expected to fail: its first stoppage is boundary evidence.\n", previous.Probe)
	fmt.Fprintf(&b, "// It carries no fix.\n\n")
	fmt.Fprintf(&b, "package probes\n\n")
	imports := "\"testing\"\n\n\"emergion-sovereign-runtime/internal/adapters\"\n"
	if level == fractureLevelPrepare {
		imports += "\"emergion-sovereign-runtime/internal/core\"\n"
	}
	fmt.Fprintf(&b, "import (\n%s)\n\n", imports)
	fmt.Fprintf(&b, "func %s(t *testing.T) {\n", funcName)
	fmt.Fprintf(&b, "level := %q\n", level)
	fmt.Fprintf(&b, "boundary := %q\n", em.IDN)
	fmt.Fprintf(&b, "targetEmergION := %q\n", em.REL["target_emergion"])
	fmt.Fprintf(&b, "gap := %q\n", strings.TrimPrefix(gap, "BRIDGEGAP:"))
	fmt.Fprintf(&b, "basis := %q\n", em.REL["requirement_basis"])
	fmt.Fprintf(&b, "required := %q\n", required)
	fmt.Fprintf(&b, "targetState := %q\n", strings.TrimSpace(em.REL["target_state"]))
	fmt.Fprintf(&b, "differences := %s\n", quoted(differences))
	fmt.Fprintf(&b, "boundaryEvidence := %q\n", em.MEM.SourceHash)
	fmt.Fprintf(&b, "recipe := %s\n", quoted(recipe))
	fmt.Fprintf(&b, "previousProbe := %q\n", previous.Probe)
	fmt.Fprintf(&b, "previousLevel := %q\n", previous.Level)
	fmt.Fprintf(&b, "previousEvidence := %q\n", previous.Evidence)
	fmt.Fprintf(&b, "previousEvidenceHash := %q\n", previous.EvidenceHash)
	fmt.Fprintf(&b, "previousStoppage := %q\n\n", strings.ReplaceAll(previous.Stoppage, "BRIDGEGAP:", ""))
	fmt.Fprintf(&b, "t.Logf(\"FRACTURE_PROBE level=%%s boundary=%%s target=%%s gap=%%s basis=%%s required=%%s target_state=%%q differences=%%q evidence=%%s previous=%%s/%%s/%%s/%%s previous_stoppage=%%q\", level, boundary, targetEmergION, gap, basis, required, targetState, differences, boundaryEvidence, previousProbe, previousLevel, previousEvidence, previousEvidenceHash, previousStoppage)\n\n")

	switch level {
	case fractureLevelPrepare:
		fmt.Fprintf(&b, "// Ask the existing action seam to derive each recipe action for a\n")
		fmt.Fprintf(&b, "// record carrying exactly the boundary's recipe.\n")
		fmt.Fprintf(&b, "st := core.State{Accepted: map[string]core.EmergION{boundary: {IDN: boundary, STA: core.StateAccepted, MEM: core.Memory{SourceHash: boundaryEvidence}, CAP: recipe}}}\n")
		fmt.Fprintf(&b, "for _, token := range recipe {\nprepared := false\nvar last error\n")
		fmt.Fprintf(&b, "for _, adapter := range adapters.Catalog(true) {\nfor _, capability := range adapter.Capabilities {\nif capability != token {\ncontinue\n}\n")
		fmt.Fprintf(&b, "if _, err := adapters.PrepareExecution(st, boundary, adapter.ID, token, true); err != nil {\nlast = err\ncontinue\n}\nprepared = true\n}\n}\n")
		fmt.Fprintf(&b, "if !prepared {\nt.Fatalf(\"FRACTURE_STOPPAGE level=%%s boundary=%%s gap=%%s required=%%s token=%%s: action not derivable: %%v\", level, boundary, gap, required, token, last)\n}\n}\n")
	case fractureLevelEnabled:
		fmt.Fprintf(&b, "// Ask whether an enabled adapter would execute each derived action.\n")
		fmt.Fprintf(&b, "for _, token := range recipe {\nenabled := false\n")
		fmt.Fprintf(&b, "for _, candidate := range adapters.DeriveActionCandidates(nil, recipe, true) {\nif candidate.Action == token && candidate.Enabled {\nenabled = true\n}\n}\n")
		fmt.Fprintf(&b, "if !enabled {\nt.Fatalf(\"FRACTURE_STOPPAGE level=%%s boundary=%%s gap=%%s required=%%s token=%%s: no enabled adapter executes the action\", level, boundary, gap, required, token)\n}\n}\n")
	}
	fmt.Fprintf(&b, "}\n")
	return format.Source(b.Bytes())
}

type waveSignal struct {
	em        core.EmergION
	succeeded bool
	stoppage  string
}

// ProposeNextFractureWaveRound advances the recursive fracture wave by at
// most one proposal. maxRounds bounds probe generations per boundary
// (<= 0 means the ladder length).
func (r Runtime) ProposeNextFractureWaveRound(
	ctx context.Context,
	workDir string,
	maxRounds int,
) ([]FractureWaveStatus, core.EmergION, error) {
	if r.Store == nil {
		return nil, core.EmergION{}, fmt.Errorf("runtime store not configured")
	}
	if maxRounds <= 0 || maxRounds > len(fractureLadder) {
		maxRounds = len(fractureLadder)
	}
	events, err := r.Store.Events()
	if err != nil {
		return nil, core.EmergION{}, err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return nil, core.EmergION{}, err
	}

	// REG order: the latest accepted comparison per target is the active
	// boundary; accepted execution results are ordered the same way.
	active := map[string]string{}
	acceptedOrder := map[string]int{}
	for i, event := range events {
		if event.REG == nil {
			continue
		}
		id := event.REG.EmergIONID
		acceptedOrder[id] = i
		em, ok := st.Accepted[id]
		if !ok || em.REL["source_kind"] != "TARGET_COMPARISON" {
			continue
		}
		if target := strings.TrimSpace(em.REL["target_emergion"]); target != "" {
			active[target] = id
		}
	}
	targets := make([]string, 0, len(active))
	for target := range active {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	var statuses []FractureWaveStatus
	for _, target := range targets {
		status, candidate, err := r.fractureWaveStep(ctx, st, acceptedOrder, target, active[target], workDir, maxRounds)
		if err != nil {
			return statuses, core.EmergION{}, err
		}
		statuses = append(statuses, status)
		if candidate.IDN != "" {
			return statuses, candidate, nil
		}
	}
	return statuses, core.EmergION{}, nil
}

func (r Runtime) fractureWaveStep(
	ctx context.Context,
	st core.State,
	acceptedOrder map[string]int,
	target string,
	boundaryID string,
	workDir string,
	maxRounds int,
) (FractureWaveStatus, core.EmergION, error) {
	boundary := st.Accepted[boundaryID]
	status := FractureWaveStatus{Target: target, Boundary: boundaryID}

	gap, _, err := fractureBoundary(boundary)
	if err != nil {
		status.Status = waveNoRemainder
		if strings.HasPrefix(boundary.REL["requirement_basis"], "SATISFIED:") {
			status.Status = waveSatisfied
		}
		return status, core.EmergION{}, nil
	}

	// Probes for this boundary, accepted or still pending.
	var probes []core.EmergION
	pending := false
	for _, group := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Accepted, st.Held} {
		for _, em := range group {
			if em.REL["origin"] != boundaryID || strings.TrimSpace(em.REL["fracture_gap"]) == "" {
				continue
			}
			if em.STA != core.StateAccepted {
				pending = true
				continue
			}
			probes = append(probes, em)
		}
	}
	status.Round = len(probes)
	if pending {
		status.Status = waveAwaitingHumanFinal
		return status, core.EmergION{}, nil
	}

	boundaryState, governedState, err := r.governedStateContext()
	if err != nil {
		return status, core.EmergION{}, err
	}

	if len(probes) == 0 {
		content, err := renderFractureProbeAt(boundary, fractureLevelCatalog, fracturePrevious{})
		if err != nil {
			return status, core.EmergION{}, err
		}
		em, _, err := r.admitFractureProbe(ctx, boundary, gap,
			fractureProbePathAt(boundaryID, fractureLevelCatalog), content, nil,
			workDir, boundaryState, governedState)
		if err != nil {
			return status, core.EmergION{}, err
		}
		status.Status, status.Level, status.Round = waveProposedProbe, fractureLevelCatalog, 1
		status.Candidate, status.Probe = em.IDN, em.IDN
		return status, em, nil
	}

	// The deepest accepted probe is the current round.
	sort.Slice(probes, func(i, j int) bool {
		li, lj := fractureLevelIndex(fractureProbeLevel(probes[i])), fractureLevelIndex(fractureProbeLevel(probes[j]))
		if li != lj {
			return li < lj
		}
		return probes[i].IDN < probes[j].IDN
	})
	current := probes[len(probes)-1]
	level := fractureProbeLevel(current)
	status.Probe, status.Level = current.IDN, level

	// Its execution evidence, in REG order; any pending result waits.
	var signals []waveSignal
	for _, group := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Held} {
		for _, em := range group {
			if em.REL["source_kind"] == "EXECUTION_RESULT" && em.REL["kin_emergion"] == current.IDN {
				status.Status = waveAwaitingHumanFinal
				return status, core.EmergION{}, nil
			}
		}
	}
	var accepted []core.EmergION
	for _, em := range st.Accepted {
		if em.REL["source_kind"] == "EXECUTION_RESULT" && em.REL["kin_emergion"] == current.IDN {
			accepted = append(accepted, em)
		}
	}
	sort.Slice(accepted, func(i, j int) bool { return acceptedOrder[accepted[i].IDN] < acceptedOrder[accepted[j].IDN] })
	for _, em := range accepted {
		raw, err := r.Store.ReadEvidence(em.MEM.SourceHash)
		if err != nil {
			return status, core.EmergION{}, err
		}
		result, err := adapters.ParseExecutionResultBytes(raw)
		if err != nil {
			return status, core.EmergION{}, err
		}
		signals = append(signals, waveSignal{em: em, succeeded: result.Succeeded, stoppage: firstFractureStoppage(result.Error)})
	}
	if len(signals) == 0 {
		status.Status = waveAwaitingRun
		return status, core.EmergION{}, nil
	}

	last := signals[len(signals)-1]
	status.Evidence, status.Stoppage = last.em.IDN, last.stoppage
	if !last.succeeded {
		// Stoppage: the next step is the smallest structural addition, made
		// and governed outside the probe, then the same probe again under a
		// fresh Q. Two identical stoppages in a row mean nothing changed.
		if len(signals) >= 2 && !signals[len(signals)-2].succeeded && signals[len(signals)-2].stoppage == last.stoppage {
			status.Status = waveNoNewStoppage
		} else {
			status.Status = waveRerunSameProbe
		}
		return status, core.EmergION{}, nil
	}

	// The level was crossed. Go one level deeper, or recompute the target.
	next := fractureLevelIndex(level) + 1
	if next <= 0 || next >= len(fractureLadder) {
		return r.recomputeFractureTarget(ctx, st, status, target)
	}
	if len(probes) >= maxRounds {
		status.Status = waveMaxRounds
		return status, core.EmergION{}, nil
	}
	previous := fracturePrevious{
		Probe:        current.IDN,
		Level:        level,
		Evidence:     last.em.IDN,
		EvidenceHash: last.em.MEM.SourceHash,
	}
	// The stoppage this probe had shown before it was crossed.
	for i := len(signals) - 1; i >= 0; i-- {
		if !signals[i].succeeded {
			previous.Stoppage = signals[i].stoppage
			break
		}
	}
	nextLevel := fractureLadder[next]
	content, err := renderFractureProbeAt(boundary, nextLevel, previous)
	if err != nil {
		return status, core.EmergION{}, err
	}
	em, _, err := r.admitFractureProbe(ctx, boundary, gap,
		fractureProbePathAt(boundaryID, nextLevel), content,
		map[string]string{"fracture_level": nextLevel, "fracture_evidence": last.em.IDN},
		workDir, boundaryState, governedState)
	if err != nil {
		return status, core.EmergION{}, err
	}
	status.Status, status.Level, status.Round = waveProposedProbe, nextLevel, len(probes)+1
	status.Candidate, status.Probe = em.IDN, em.IDN
	return status, em, nil
}

// recomputeFractureTarget captures a fresh TARGET_COMPARISON against
// current REG reality. Probes and their evidence are excluded inside
// CaptureTargetComparison. The comparison is a candidate at GOV only.
func (r Runtime) recomputeFractureTarget(
	ctx context.Context,
	st core.State,
	status FractureWaveStatus,
	target string,
) (FractureWaveStatus, core.EmergION, error) {
	var targetEM core.EmergION
	for _, group := range []map[string]core.EmergION{st.Accepted, st.Approved, st.AtGOV} {
		if em, ok := group[target]; ok && em.REL["source_kind"] == "TARGET" {
			targetEM = em
			break
		}
	}
	if targetEM.IDN == "" {
		status.Status = waveTargetNotFound
		return status, core.EmergION{}, nil
	}
	if r.Reasoner == nil {
		status.Status = waveRecomputeNeeded
		return status, core.EmergION{}, nil
	}
	reality := map[string]core.EmergION{}
	for id, em := range st.Accepted {
		switch em.REL["source_kind"] {
		case "TARGET", "TARGET_COMPARISON", "IDLE_STATE_OBSERVATION":
			continue
		}
		reality[id] = em
	}
	comparison, duplicate, err := r.CaptureTargetComparison(ctx, targetEM, reality, nil)
	if err != nil {
		return status, core.EmergION{}, err
	}
	if duplicate {
		// Same evidence as an existing comparison. If that comparison is not
		// the active boundary and is not yet accepted, it waits at GOV.
		if _, ok := st.Accepted[comparison.IDN]; !ok && comparison.IDN != status.Boundary {
			status.Status, status.Candidate = waveAwaitingHumanFinal, comparison.IDN
			return status, core.EmergION{}, nil
		}
		status.Status = waveTargetUnchanged
		return status, core.EmergION{}, nil
	}
	status.Status, status.Candidate = waveRecomputeProposed, comparison.IDN
	return status, comparison, nil
}
