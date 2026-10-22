package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

const realizationIdentitySourceKind = "REALIZATION_IDENTITY"

// CaptureRealizationIdentity admits an evidence-bound assertion that an
// already REG-accepted EmergION is a realization of a named THING.
//
// The assertion itself stops at AtGOV. It does not rewrite the realization,
// form a population, select a strongest member, authorize, or execute.
func (r Runtime) CaptureRealizationIdentity(
	ctx context.Context,
	realizationID string,
	targetThing string,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	realizationID = strings.TrimSpace(realizationID)
	targetThing = strings.TrimSpace(targetThing)
	if realizationID == "" || targetThing == "" {
		return core.EmergION{}, false, fmt.Errorf(
			"realization identity requires accepted realization and target thing",
		)
	}

	st, _, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}
	realization, ok := st.Accepted[realizationID]
	if !ok || realization.STA != core.StateAccepted {
		return core.EmergION{}, false, fmt.Errorf(
			"realization %q is not REG-accepted",
			realizationID,
		)
	}

	evidence := []byte(fmt.Sprintf(
		"REALIZATION_IDENTITY/1\nREALIZATION:%s\nTARGET_THING:%s\nSOURCE_HASH:%s\n",
		realizationID,
		targetThing,
		realization.MEM.SourceHash,
	))

	identityRuntime := r
	identityRuntime.Reasoner = fixedReasoner{
		name: "realization-identity",
		version: "v1",
		result: reason.Result{
			Summary: "evidence-bound realization identity assertion",
			Relationships: map[string]string{
				"source_kind": realizationIdentitySourceKind,
				"realization": realizationID,
				"realization_of": targetThing,
				"realization_source_hash": realization.MEM.SourceHash,
			},
			Capabilities: []string{"OBS", "RLT"},
			Facts: []string{"realization_identity_asserted"},
			Risk: "L",
		},
	}

	return identityRuntime.CaptureBytes(
		ctx,
		"realization-identity",
		evidence,
		"governed_realization_identity",
	)
}

// DiscoverRealizationPopulations groups only identities that have themselves
// crossed HUMAN_FINAL + REG. It invents no comparison or ordering evidence.
func DiscoverRealizationPopulations(st core.State) []RealizationPopulation {
	byThing := map[string]map[string]bool{}

	for _, assertion := range st.Accepted {
		if assertion.STA != core.StateAccepted ||
			strings.TrimSpace(assertion.REL["source_kind"]) != realizationIdentitySourceKind {
			continue
		}

		realizationID := strings.TrimSpace(assertion.REL["realization"])
		targetThing := strings.TrimSpace(assertion.REL["realization_of"])
		sourceHash := strings.TrimSpace(assertion.REL["realization_source_hash"])
		if realizationID == "" || targetThing == "" || sourceHash == "" {
			continue
		}

		realization, ok := st.Accepted[realizationID]
		if !ok || realization.STA != core.StateAccepted ||
			realization.MEM.SourceHash != sourceHash {
			continue
		}

		if byThing[targetThing] == nil {
			byThing[targetThing] = map[string]bool{}
		}
		byThing[targetThing][realizationID] = true
	}

	things := make([]string, 0, len(byThing))
	for thing := range byThing {
		things = append(things, thing)
	}
	sort.Strings(things)

	out := make([]RealizationPopulation, 0, len(things))
	for _, thing := range things {
		if len(byThing[thing]) < 2 {
			continue
		}
		members := make([]string, 0, len(byThing[thing]))
		for id := range byThing[thing] {
			members = append(members, id)
		}
		sort.Strings(members)
		out = append(out, RealizationPopulation{
			TargetThing: thing,
			Members: members,
		})
	}
	return out
}
