package runtime

import (
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

// Section 8: Difference -> Requirement.
//
// deriveDifferenceRequirement decides, deterministically, what an admitted
// target-relative Difference requires. It returns one of the existing
// requirement tokens (ESTABLISH_FACT, DERIVE_RELATIONSHIP, DERIVE_CAPABILITY)
// or "" for no requirement, plus a requirement_basis naming the outcome and
// the rule that fired.
//
// It never writes a BRIDGEGAP, never claims MISSING, never changes
// comparison_state and never derives TORQUE. The model's R| line is only a
// vote: it can agree with a rule, never stand in for one. SATISFIED is a
// no-action classification only.
const (
	basisFactNotInAcceptedFacts      = "FACT:target_not_in_accepted_facts"
	basisRelationshipUnlinked        = "RELATIONSHIP:accepted_emergions_unlinked"
	basisCapabilityNotAccepted       = "CAPABILITY:capability_token_not_accepted"
	basisSatisfiedAcceptedFact       = "SATISFIED:target_equals_accepted_fact"
	basisNoGroundedDifference        = "UNRESOLVED:no_grounded_difference"
	basisContradiction               = "UNRESOLVED:difference_contradicts_accepted_fact"
	basisEmergIONNotFoundYet         = "UNRESOLVED:emergion_not_found_yet"
	basisPartiallySupported          = "UNRESOLVED:target_partially_supported"
	basisUnderdetermined             = "UNRESOLVED:underdetermined"
	basisNoRule                      = "UNRESOLVED:no_rule"
	basisModelRequirementDisagrees   = "UNRESOLVED:model_requirement_disagrees"
	basisModelRequirementWithoutRule = "UNRESOLVED:model_requirement_without_rule"
	basisModelRequirementAmbiguous   = "UNRESOLVED:model_requirement_ambiguous"
)

func deriveDifferenceRequirement(
	target core.EmergION,
	reality map[string]core.EmergION,
	observed reason.Result,
) (string, string) {
	targetState := strings.TrimSpace(target.REL["target_state"])
	evidence := string(targetComparisonEvidence(target, reality))

	grounded := 0
	for _, difference := range observed.Differences {
		if reason.SourceSupportsDifference(evidence, difference) {
			grounded++
		}
	}

	facts := acceptedRealityFacts(reality)
	satisfied := false
	normalizedTarget := normalizeRequirementText(targetState)
	for _, fact := range facts {
		if normalizedTarget != "" && normalizeRequirementText(fact) == normalizedTarget {
			satisfied = true
			break
		}
	}

	if grounded == 0 {
		if satisfied {
			return modelVote(observed, "", basisSatisfiedAcceptedFact)
		}
		return modelVote(observed, "", basisNoGroundedDifference)
	}
	if satisfied {
		return modelVote(observed, "", basisContradiction)
	}

	ids, unknownIDs := targetEmergIONReferences(targetState, reality)
	if unknownIDs > 0 {
		return modelVote(observed, "", basisEmergIONNotFoundYet)
	}

	tokens := targetCapabilityTokens(targetState)
	accepted := acceptedRealityCapabilities(reality)

	capabilityRule := len(tokens) == 1 && !accepted[tokens[0]]
	relationshipRule := len(ids) >= 2 && !acceptedEmergIONsLinked(ids, reality)
	factRule := len(tokens) == 0 && len(ids) == 0

	if factRule && reason.SourceSupportsFact(strings.Join(facts, "\n"), targetState) {
		return modelVote(observed, "", basisPartiallySupported)
	}

	fired := 0
	required, basis := "", basisNoRule
	if capabilityRule {
		fired++
		required, basis = "DERIVE_CAPABILITY", basisCapabilityNotAccepted
	}
	if relationshipRule {
		fired++
		required, basis = "DERIVE_RELATIONSHIP", basisRelationshipUnlinked
	}
	if factRule {
		fired++
		required, basis = "ESTABLISH_FACT", basisFactNotInAcceptedFacts
	}
	if fired > 1 {
		return modelVote(observed, "", basisUnderdetermined)
	}
	return modelVote(observed, required, basis)
}

// modelVote applies the model's R| proposal as a vote against the
// deterministic outcome. Agreement or silence keeps the rule; anything else
// yields no requirement.
func modelVote(observed reason.Result, required, basis string) (string, string) {
	if len(observed.Requirements) > 1 {
		return "", basisModelRequirementAmbiguous
	}
	if len(observed.Requirements) == 0 {
		return required, basis
	}
	if required == "" {
		if basis == basisNoRule || basis == basisSatisfiedAcceptedFact {
			return "", basisModelRequirementWithoutRule
		}
		return "", basis
	}
	if strings.ToUpper(strings.TrimSpace(observed.Requirements[0])) != required {
		return "", basisModelRequirementDisagrees
	}
	return required, basis
}

func acceptedRealityFacts(reality map[string]core.EmergION) []string {
	var facts []string
	for _, id := range sortedRealityIDs(reality) {
		for _, fact := range reality[id].VAL.Facts {
			if fact = strings.TrimSpace(fact); fact != "" {
				facts = append(facts, fact)
			}
		}
	}
	return facts
}

func acceptedRealityCapabilities(reality map[string]core.EmergION) map[string]bool {
	out := map[string]bool{}
	for _, em := range reality {
		for _, capability := range em.CAP {
			if capability = strings.ToUpper(strings.TrimSpace(capability)); capability != "" {
				out[capability] = true
			}
		}
	}
	return out
}

func sortedRealityIDs(reality map[string]core.EmergION) []string {
	ids := make([]string, 0, len(reality))
	for id := range reality {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func normalizeRequirementText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimRight(s, ".")
}

// requirementVocabulary is the existing capability vocabulary: the adapter
// catalog plus the existing recipe tokens. No new operator constant.
func requirementVocabulary() map[string]bool {
	vocabulary := map[string]bool{}
	for _, adapter := range adapters.Catalog(false) {
		for _, capability := range adapter.Capabilities {
			vocabulary[strings.ToUpper(strings.TrimSpace(capability))] = true
		}
	}
	for _, required := range []string{"ESTABLISH_FACT", "DERIVE_RELATIONSHIP", "DERIVE_CAPABILITY"} {
		for _, capability := range requiredCapabilityRecipe(required) {
			vocabulary[capability] = true
		}
	}
	return vocabulary
}

func requirementTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	})
}

// targetCapabilityTokens returns the distinct exact (case-sensitive)
// vocabulary tokens named by the target. Ordinary prose words such as
// "read" or "price" are not capability tokens.
func targetCapabilityTokens(targetState string) []string {
	vocabulary := requirementVocabulary()
	seen := map[string]bool{}
	var out []string
	for _, token := range requirementTokens(targetState) {
		if vocabulary[token] && !seen[token] {
			seen[token] = true
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

// targetEmergIONReferences returns the accepted EmergION IDNs the target
// names, and how many IDN-shaped references are not in accepted reality
// (NOT_FOUND_YET, never MISSING).
func targetEmergIONReferences(targetState string, reality map[string]core.EmergION) ([]string, int) {
	seen := map[string]bool{}
	var ids []string
	unknown := 0
	for _, token := range requirementTokens(targetState) {
		if seen[token] {
			continue
		}
		if _, ok := reality[token]; ok {
			seen[token] = true
			ids = append(ids, token)
			continue
		}
		if isEmergIONShaped(token) {
			seen[token] = true
			unknown++
		}
	}
	sort.Strings(ids)
	return ids, unknown
}

func isEmergIONShaped(token string) bool {
	if len(token) != 18 || !strings.HasPrefix(token, "E-") {
		return false
	}
	for _, r := range token[2:] {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func acceptedEmergIONsLinked(ids []string, reality map[string]core.EmergION) bool {
	named := map[string]bool{}
	for _, id := range ids {
		named[id] = true
	}
	for _, id := range ids {
		for _, value := range reality[id].REL {
			value = strings.TrimSpace(value)
			if value != id && named[value] {
				return true
			}
		}
	}
	return false
}
