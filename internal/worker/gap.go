package worker

// Gap step: turn the gaps an ACCEPTED SOURCE_INSIGHT already names (its
// GAP: evidence lines and its VAL.Gaps) into SOURCE_GAP candidates, one per
// (source, class). Deterministic: a keyword map picks the class, no model is
// called. Every SOURCE_GAP is admitted AtGOV through the normal CaptureBytes
// path and the policy never decides it, so each one waits for HUMAN_FINAL.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/policy"
)

// MaxGapsPerStep caps SOURCE_GAP proposals per worker step.
const MaxGapsPerStep = 6

// maxGapText bounds the gap text carried in the descriptor and summary.
const maxGapText = 240

// gapKeywords are matched against lowercase words. A trailing '*' matches a
// word prefix; otherwise the word must match exactly.
var gapKeywords = map[string][]string{
	policy.GapOperational: {
		"deploy*", "monitor*", "logging", "logs", "error*", "fail*", "bug*", "crash*",
		"latency", "performance", "slow", "memory", "cpu", "battery", "config*",
		"install*", "runtime", "process*", "operat*", "maintenance", "maintain*",
		"security", "permission*", "scal*", "throughput", "uptime", "ops",
	},
	policy.GapCommercial: {
		"market*", "customer*", "revenue", "pricing", "price*", "sales", "sell*",
		"buyer*", "investor*", "funding", "fund*", "grant*", "monetiz*", "business*",
		"commercial*", "cost*", "licens*", "pilot*", "adoption", "competitor*",
		"valuation", "go-to-market", "gtm", "profit*", "client*",
	},
	policy.GapIntegration: {
		"integrat*", "adapter*", "api", "apis", "connector*", "interface*",
		"protocol*", "compatib*", "interop*", "import*", "export*", "sync*",
		"webhook*", "plugin*", "sdk", "schema*", "bridge*", "wiring", "endpoint*",
		"pipeline*", "hook*", "binding*",
	},
	policy.GapContinuity: {
		"backup*", "restore*", "recover*", "boot*", "restart*", "reboot*",
		"watchdog*", "persist*", "resum*", "checkpoint*", "outage*", "downtime",
		"durab*", "replica*", "archiv*", "continuity", "migrat*", "succession",
		"handover", "lost", "loss",
	},
	policy.GapProof: {
		"proof*", "prove*", "evidence", "verif*", "validat*", "test*", "benchmark*",
		"measur*", "metric*", "citation*", "reproduc*", "audit*", "claim*",
		"unproven", "demonstrat*", "experiment*", "baseline*", "data",
	},
}

// ClassifyGap picks the gap class with the most keyword hits; ties go to the
// earlier class in policy.GapClasses. No hit falls back to proof: a gap the
// map cannot place is treated as a claim that still needs evidence.
func ClassifyGap(text string) (class, pivot string) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-')
	})
	best, bestScore := policy.GapProof, 0
	for _, c := range policy.GapClasses {
		score := 0
		for _, w := range words {
			for _, k := range gapKeywords[c] {
				if strings.HasSuffix(k, "*") {
					if strings.HasPrefix(w, strings.TrimSuffix(k, "*")) {
						score++
						break
					}
				} else if w == k {
					score++
					break
				}
			}
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best, policy.GapPivot[best]
}

func gapKey(sourceID, class string) string { return sourceID + "|" + class }

func cleanGap(g string) string {
	g = strings.Join(strings.Fields(g), " ")
	if len(g) > maxGapText {
		g = strings.TrimSpace(g[:maxGapText])
	}
	return g
}

// DoneGaps reports the (source, class) keys the ledger already holds in any
// GOV state, so a held or rejected gap is never regenerated.
func DoneGaps(st core.State) map[string]bool {
	out := map[string]bool{}
	for _, set := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Accepted, st.Held, st.Rejected, st.Returned} {
		for _, em := range set {
			if strings.TrimSpace(em.REL["source_kind"]) == policy.KindGap {
				out[gapKey(strings.TrimSpace(em.REL["gap_of"]), strings.TrimSpace(em.REL["gap_class"]))] = true
			}
		}
	}
	return out
}

// insightGaps is the union of the insight's VAL.Gaps and the GAP: lines of
// its preserved evidence (read only), without engine BRIDGEGAP tokens.
func (w *Worker) insightGaps(em core.EmergION) []string {
	seen := map[string]bool{}
	var out []string
	add := func(g string) {
		g = cleanGap(g)
		if g == "" || strings.HasPrefix(g, "BRIDGEGAP:") || seen[g] {
			return
		}
		seen[g] = true
		out = append(out, g)
	}
	for _, g := range em.VAL.Gaps {
		add(g)
	}
	if h := strings.TrimSpace(em.MEM.SourceHash); h != "" && w.Store != nil {
		if b, err := w.Store.ReadEvidence(h); err == nil {
			lines := strings.Split(string(b), "\n")
			if len(lines) > 0 && lines[0] == "SOURCE_INSIGHT/1" {
				for _, l := range lines[1:] {
					if v, ok := strings.CutPrefix(l, "GAP:"); ok {
						add(v)
					}
				}
			}
		}
	}
	return out
}

// GapCandidate is one SOURCE_GAP the step would write.
type GapCandidate struct {
	Of, Basis, Class, Pivot, Gap, Title, Topic string
}

// PlanGaps lists, in a stable order, the SOURCE_GAP candidates for accepted
// insights: one per (source, class) not already in the ledger, at most max.
func (w *Worker) PlanGaps(st core.State, max int) []GapCandidate {
	done := DoneGaps(st)
	ids := make([]string, 0)
	for id, em := range st.Accepted {
		if strings.TrimSpace(em.REL["source_kind"]) == policy.KindInsight && em.MEM.Provenance == policy.ProvenanceInsight {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var out []GapCandidate
	for _, id := range ids {
		ins := st.Accepted[id]
		of := strings.TrimSpace(ins.REL["insight_of"])
		if of == "" {
			continue
		}
		src, ok := st.Accepted[of]
		if !ok {
			continue
		}
		for _, g := range w.insightGaps(ins) {
			if max > 0 && len(out) >= max {
				return out
			}
			class, pivot := ClassifyGap(g)
			key := gapKey(of, class)
			if done[key] {
				continue
			}
			done[key] = true
			out = append(out, GapCandidate{
				Of: of, Basis: id, Class: class, Pivot: pivot, Gap: g,
				Title: strings.TrimSpace(src.REL["source_title"]), Topic: src.REL["source_topic"],
			})
		}
	}
	return out
}

func (w *Worker) captureGap(ctx context.Context, c GapCandidate) (core.EmergION, error) {
	desc := fmt.Sprintf("SOURCE_GAP/1\nOF:%s\nCLASS:%s\nBASIS:%s\nGAP:%s\nPROPOSED:%s\nWORKER:%s\nZ\n",
		c.Of, c.Class, c.Basis, c.Gap, c.Pivot, Version)
	rel := map[string]string{
		"source_kind":    policy.KindGap,
		"gap_of":         c.Of,
		"gap_class":      c.Class,
		"gap_basis":      c.Basis,
		"gap_pivot":      c.Pivot,
		"source_topic":   c.Topic,
		"worker_version": Version,
	}
	title := c.Title
	if title == "" {
		title = c.Of
	}
	res := reason.Result{
		Summary:       fmt.Sprintf("Gap (%s) in %s: %s", c.Class, title, c.Gap),
		Relationships: rel,
		Capabilities:  []string{"OBS"},
		Facts:         []string{"worker_gap_of:" + c.Of + "," + c.Class},
		Risk:          "M",
	}
	rt := fieldruntime.Runtime{Store: w.Store, Reasoner: fixed{name: "worker-gap", result: res}}
	em, _, err := rt.CaptureBytes(ctx, "source-gap", []byte(desc), policy.ProvenanceGap)
	return em, err
}
