package runtime

import (
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/pivot"
)

// Section 11: genuine-remainder BRIDGEGAP.
//
// A Difference never becomes a BRIDGEGAP by itself. Only a requirement that
// the deterministic Section 8 step derived after searching accepted reality
// (the estate) and found genuinely unmet (FACT, RELATIONSHIP or CAPABILITY)
// is recorded as a BRIDGEGAP. SATISFIED and UNRESOLVED outcomes, and any
// outcome where the model's vote removed the requirement, record none.
//
// The record reuses the existing COVERAGE gap tokens and the existing
// mapping in requiredCapabilityFromDivergence, so the recipe and provider
// path (sections 9 and 10) is unchanged. The BRIDGEGAP is observation only:
// the comparison stays a candidate subject to RECOIL, WVC, GOV, HUMAN_FINAL
// and REG, and nothing is promoted automatically.
var genuineRemainderBridgeGaps = []struct {
	basisClass string
	required   string
	gap        string
}{
	{"FACT:", "ESTABLISH_FACT", "BRIDGEGAP:facts"},
	{"RELATIONSHIP:", "DERIVE_RELATIONSHIP", "BRIDGEGAP:relationships"},
	{"CAPABILITY:", "DERIVE_CAPABILITY", "BRIDGEGAP:capabilities"},
}

// genuineRemainderBridgeGap returns the BRIDGEGAP token for a Section 8
// outcome, or "" when the outcome is not a genuine remainder. The required
// token, the requirement_basis class and the existing COVERAGE mapping must
// all agree; any disagreement fails closed to no BRIDGEGAP.
func genuineRemainderBridgeGap(required, basis string) string {
	required = strings.TrimSpace(required)
	basis = strings.TrimSpace(basis)
	if required == "" || basis == "" {
		return ""
	}
	for _, kind := range genuineRemainderBridgeGaps {
		if kind.required != required || !strings.HasPrefix(basis, kind.basisClass) {
			continue
		}
		existing := requiredCapabilityFromDivergence(pivot.Result{
			Name:       "COVERAGE",
			Divergence: kind.gap,
		})
		if existing != required || len(requiredCapabilityRecipe(required)) == 0 {
			return ""
		}
		return kind.gap
	}
	return ""
}

// unfillableRemainderBridgeGap returns the genuine-remainder BRIDGEGAP token
// only when the remainder is unfillable: no REG-accepted EmergION in the
// accepted reality provides every capability of the existing recipe for the
// requirement (the same acceptedCapabilityProviders search the
// capability_providers fill already uses). When accepted reality can fill
// it, no gap is written; the comparison keeps its required_capability and
// the existing resolution path records the fill as COMPOSABLE_CANDIDATE with
// capability_providers. Accepted facts and accepted KIN links are already
// searched by the Section 8 rules before a requirement exists, and
// NOT_FOUND_YET outcomes never reach this point. Nothing is triggered here:
// fracture probes stay manual.
func unfillableRemainderBridgeGap(required, basis string, reality map[string]core.EmergION) string {
	gap := genuineRemainderBridgeGap(required, basis)
	if gap == "" {
		return ""
	}
	if _, fillable := acceptedCapabilityProviders(required, core.State{Accepted: reality}); fillable {
		return ""
	}
	return gap
}
