package runtime

import (
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

// originatedFromQuestionKey is the additive, optional REL key approved by
// HUMAN_FINAL (Nov 19 2026, TR-042). It links a new capture to the IDN of
// the TARGET EmergION (the question) it came from. It is runtime-owned: no
// model may claim it, and it is derived only from runtime-owned lineage
// already on the record. Records without it (all older records) keep their
// exact COSL encoding and hash; absence means NOT_FOUND_YET, never MISSING.
const originatedFromQuestionKey = "originated_from_question"

// originatedFromQuestion derives the originating question for a candidate
// from the record itself and the state boundary it was captured against.
// It never writes anything and never invents a question:
//
//   - a TARGET is the question itself and carries no key;
//   - a TARGET_COMPARISON names its question through the runtime-set
//     target_emergion, honoured only when that IDN is a known TARGET;
//   - a provider population candidate (origin), a fracture probe (origin)
//     or an execution record (kin_emergion) carries the question of the
//     record it is KIN to, one hop, when that record carries one;
//   - a closed SAW recirculated through CaptureBytes carries the question
//     only when every member that carries one names the same question.
//
// Anything else, or any disagreement, yields "".
func originatedFromQuestion(
	em core.EmergION,
	provenance string,
	name string,
	boundary core.State,
	lookup func(string) (core.EmergION, bool),
) string {
	if em.REL["source_kind"] == "TARGET" {
		return ""
	}

	if provenance == "governed_target_comparison" &&
		em.REL["source_kind"] == "TARGET_COMPARISON" {
		target := strings.TrimSpace(em.REL["target_emergion"])
		if found, ok := lookup(target); ok && found.REL["source_kind"] == "TARGET" {
			return target
		}
		return ""
	}

	for _, key := range []string{"origin", "kin_emergion"} {
		id := strings.TrimSpace(em.REL[key])
		if id == "" || id == em.IDN {
			continue
		}
		if found, ok := lookup(id); ok {
			return questionOf(found)
		}
		return ""
	}

	if provenance == "saw_projection" {
		return sawQuestion(name, boundary)
	}
	return ""
}

// questionOf returns the question a known record answers: its own key, or
// its IDN when the record is itself a TARGET.
func questionOf(em core.EmergION) string {
	if em.REL["source_kind"] == "TARGET" {
		return em.IDN
	}
	return strings.TrimSpace(em.REL[originatedFromQuestionKey])
}

// sawQuestion reads the accepted member IDNs from the existing SAW source
// name ("SAW:SAAB:<IDN>+<IDN>...") and returns their shared question.
func sawQuestion(name string, boundary core.State) string {
	members, ok := strings.CutPrefix(strings.TrimSpace(name), "SAW:SAAB:")
	if !ok || members == "" {
		return ""
	}
	question := ""
	for _, id := range strings.Split(members, "+") {
		member, ok := boundary.Accepted[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		q := questionOf(member)
		if q == "" {
			continue
		}
		if question != "" && q != question {
			return ""
		}
		question = q
	}
	return question
}

// knownEmergION finds a record by IDN in the capture boundary, then in the
// ledger (a candidate saved after the boundary was taken, such as the
// comparison a provider population answers). Read only.
func (r Runtime) knownEmergION(boundary core.State) func(string) (core.EmergION, bool) {
	return func(id string) (core.EmergION, bool) {
		if id == "" {
			return core.EmergION{}, false
		}
		for _, group := range []map[string]core.EmergION{
			boundary.Accepted,
			boundary.Approved,
			boundary.AtGOV,
			boundary.Held,
			boundary.Returned,
			boundary.Rejected,
		} {
			if em, ok := group[id]; ok {
				return em, true
			}
		}
		if r.Store == nil {
			return core.EmergION{}, false
		}
		events, err := r.Store.Events()
		if err != nil {
			return core.EmergION{}, false
		}
		for i := len(events) - 1; i >= 0; i-- {
			if em := events[i].EmergION; em != nil && em.IDN == id {
				return *em, true
			}
		}
		return core.EmergION{}, false
	}
}
