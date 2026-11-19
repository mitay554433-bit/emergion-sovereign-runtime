package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

const (
	contradictionMarkSourceKind = "CONTRADICTION_MARK"
	contradictsREL              = "contradicts"
)

// ContradictionMark is a REG-accepted human mark that two accepted
// records contradict. It is a plain REL link, not a kin relationship.
type ContradictionMark struct {
	MarkIDN string
	Left    string
	Right   string
}

func canonicalContradictionPair(a, b string) (string, string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", "", fmt.Errorf("contradiction mark requires two EmergION identities")
	}
	if a == b {
		return "", "", fmt.Errorf("contradiction mark requires two distinct records")
	}
	if a > b {
		a, b = b, a
	}
	return a, b, nil
}

func contradictsValue(a, b string) string {
	return a + "," + b
}

func parseContradictsPair(v string) (string, string, bool) {
	parts := strings.Split(v, ",")
	if len(parts) != 2 {
		return "", "", false
	}
	a := strings.TrimSpace(parts[0])
	b := strings.TrimSpace(parts[1])
	if a == "" || b == "" || a == b {
		return "", "", false
	}
	if a > b {
		a, b = b, a
	}
	return a, b, true
}

// CaptureContradictionMark admits an evidence-bound proposal that two
// already REG-accepted records contradict. The proposal uses the same
// CaptureBytes path as other assertions, such as realization identity:
// it stops at AtGOV and changes nothing until HUMAN_FINAL and REG.
func (r Runtime) CaptureContradictionMark(
	ctx context.Context,
	firstID string,
	secondID string,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	left, right, err := canonicalContradictionPair(firstID, secondID)
	if err != nil {
		return core.EmergION{}, false, err
	}

	st, _, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}
	first, ok := st.Accepted[left]
	if !ok || first.STA != core.StateAccepted {
		return core.EmergION{}, false, fmt.Errorf("record %q is not REG-accepted", left)
	}
	second, ok := st.Accepted[right]
	if !ok || second.STA != core.StateAccepted {
		return core.EmergION{}, false, fmt.Errorf("record %q is not REG-accepted", right)
	}

	evidence := []byte(fmt.Sprintf(
		"CONTRADICTION_MARK/1\nA:%s\nA_SOURCE_HASH:%s\nB:%s\nB_SOURCE_HASH:%s\n",
		left,
		first.MEM.SourceHash,
		right,
		second.MEM.SourceHash,
	))

	markRuntime := r
	markRuntime.Reasoner = fixedReasoner{
		name:    "contradiction-mark",
		version: "v1",
		result: reason.Result{
			Summary: "human-proposed contradiction mark between two accepted records",
			Relationships: map[string]string{
				"source_kind":  contradictionMarkSourceKind,
				contradictsREL: contradictsValue(left, right),
			},
			Capabilities: []string{"OBS", "RLT"},
			Facts:        []string{"contradiction_mark_proposed"},
			Risk:         "L",
		},
	}

	return markRuntime.CaptureBytes(
		ctx,
		"contradiction-mark",
		evidence,
		"governed_contradiction_mark",
	)
}

// ListAcceptedContradictionMarks is a read-only listing of REG-accepted
// contradiction marks only. It does not infer contradictions from facts,
// and it does not treat FIELD as authority.
func ListAcceptedContradictionMarks(st core.State) []ContradictionMark {
	var out []ContradictionMark
	for _, em := range st.Accepted {
		if em.STA != core.StateAccepted {
			continue
		}
		left, right, ok := parseContradictsPair(em.REL[contradictsREL])
		if !ok {
			continue
		}
		if _, ok := st.Accepted[left]; !ok {
			continue
		}
		if _, ok := st.Accepted[right]; !ok {
			continue
		}
		out = append(out, ContradictionMark{
			MarkIDN: em.IDN,
			Left:    left,
			Right:   right,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Left != out[j].Left {
			return out[i].Left < out[j].Left
		}
		if out[i].Right != out[j].Right {
			return out[i].Right < out[j].Right
		}
		return out[i].MarkIDN < out[j].MarkIDN
	})
	return out
}
