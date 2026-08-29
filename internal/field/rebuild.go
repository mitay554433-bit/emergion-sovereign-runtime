package field

import (
	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/pivot"
	"fmt"
	"strings"
)

func Rebuild(events []core.Event) (core.State, error) {
	st := core.EmptyState()
	decisionEvents := map[string]string{}
	for _, ev := range events {
		st.Events++
		st.TipHash = ev.SelfHash
		switch ev.Type {
		case "C":
			if ev.EmergION == nil {
				return st, fmt.Errorf("candidate event missing EmergION")
			}
			if _, exists := st.AtGOV[ev.EmergION.IDN]; exists {
				return st, fmt.Errorf("duplicate candidate %s", ev.EmergION.IDN)
			}
			st.AtGOV[ev.EmergION.IDN] = *ev.EmergION
		case "I":
			if ev.EmergION == nil {
				return st, fmt.Errorf("interpretation revision event missing EmergION")
			}

			revised := *ev.EmergION
			current, ok := st.AtGOV[revised.IDN]
			if !ok {
				return st, fmt.Errorf(
					"interpretation revision target not at GOV: %s",
					revised.IDN,
				)
			}

			if revised.IDN != current.IDN ||
				revised.MEM.SourceHash != current.MEM.SourceHash ||
				revised.MEM.Bytes != current.MEM.Bytes ||
				revised.MEM.Stored != current.MEM.Stored ||
				revised.MEM.Codec != current.MEM.Codec ||
				revised.MEM.Provenance != current.MEM.Provenance {
				return st, fmt.Errorf("interpretation revision changed evidence identity")
			}

			if revised.STA != core.StateAtGOV ||
				!revised.VAL.Recoil ||
				!revised.VAL.WVC ||
				revised.EVO.Version < 1 {
				return st, fmt.Errorf("interpretation revision is not GOV-ready")
			}

			if revised.EVO.Metadata == nil {
				return st, fmt.Errorf("interpretation revision metadata missing")
			}
			if err := revised.EVO.Metadata.Validate(); err != nil {
				return st, fmt.Errorf(
					"interpretation revision metadata invalid: %w",
					err,
				)
			}

			st.AtGOV[revised.IDN] = revised
		case "D":
			if ev.Decision == nil {
				return st, fmt.Errorf("decision event missing receipt")
			}

			_, err := pivot.Observe(
				"GOV_REPLAY",
				"DECISION_RECEIPT",
				"LEDGER_AUTHORITY_OBSERVATION",
				"HUMAN_FINAL_REQUIRED",
				func() error {
					if ev.Decision.Authority != "HUMAN_FINAL" {
						return fmt.Errorf("decision requires HUMAN_FINAL")
					}
					return nil
				},
			)
			if err != nil {
				return st, err
			}

			if ev.Decision.Decision == "RESUME" {
				em, ok := st.Held[ev.Decision.EmergIONID]
				if !ok {
					return st, fmt.Errorf("resume target not held: %s", ev.Decision.EmergIONID)
				}
				delete(st.Held, em.IDN)
				em.STA = core.StateAtGOV
				st.AtGOV[em.IDN] = em
				continue
			}
			em, ok := st.AtGOV[ev.Decision.EmergIONID]
			if !ok {
				return st, fmt.Errorf("decision target not at GOV: %s", ev.Decision.EmergIONID)
			}
			delete(st.AtGOV, em.IDN)
			switch ev.Decision.Decision {
			case "APPROVE":
				em.STA = core.StateApproved
				st.Approved[em.IDN] = em
				decisionEvents[em.IDN] = ev.ID
			case "HOLD":
				em.STA = core.StateHeld
				st.Held[em.IDN] = em
			case "REJECT":
				em.STA = core.StateRejected
				st.Rejected[em.IDN] = em
			case "RETURN":
				em.STA = core.StateReturned
				st.Returned[em.IDN] = em
			default:
				return st, fmt.Errorf("invalid decision %s", ev.Decision.Decision)
			}
		case "R":
			if ev.REG == nil {
				return st, fmt.Errorf("REG event missing receipt")
			}
			em, ok := st.Approved[ev.REG.EmergIONID]
			if !ok {
				return st, fmt.Errorf("REG target not approved: %s", ev.REG.EmergIONID)
			}
			_, err := pivot.Observe(
				"REG_REPLAY",
				"REG_RECEIPT",
				"APPROVING_DECISION_OBSERVATION",
				"EXACT_APPROVING_DECISION_LINK",
				func() error {
					if ev.REG.DecisionID == "" || ev.REG.DecisionID != decisionEvents[em.IDN] {
						return fmt.Errorf(
							"REG receipt does not reference approving decision for %s",
							em.IDN,
						)
					}
					return nil
				},
			)
			if err != nil {
				return st, err
			}
			if em.EVO.Metadata != nil {
				archonym := strings.TrimSpace(em.EVO.Metadata.Archonym)
				if archonym != "" {
					for acceptedID, accepted := range st.Accepted {
						if accepted.EVO.Metadata == nil {
							continue
						}

						acceptedArchonym := strings.TrimSpace(accepted.EVO.Metadata.Archonym)
						if acceptedArchonym == "" || !strings.EqualFold(archonym, acceptedArchonym) {
							continue
						}

						predecessor := strings.TrimSpace(em.EVO.Supersedes)
						seen := map[string]bool{}
						sameKin := false

						for predecessor != "" {
							if seen[predecessor] {
								return st, fmt.Errorf("Kin lineage cycle at %s", predecessor)
							}
							seen[predecessor] = true

							if predecessor == acceptedID {
								sameKin = true
								break
							}

							ancestor, ok := st.Accepted[predecessor]
							if !ok {
								return st, fmt.Errorf("Kin predecessor not REG-accepted: %s", predecessor)
							}

							predecessor = strings.TrimSpace(ancestor.EVO.Supersedes)
						}

						if sameKin {
							continue
						}

						return st, fmt.Errorf(
							"Archonym %q already belongs to REG-accepted sovereign EmergION %s",
							archonym,
							acceptedID,
						)

					}

					for returnedID, returned := range st.Returned {
						if returned.EVO.Metadata == nil {
							continue
						}

						returnedArchonym := strings.TrimSpace(returned.EVO.Metadata.Archonym)
						if returnedArchonym == "" || !strings.EqualFold(archonym, returnedArchonym) {
							continue
						}

						predecessor := strings.TrimSpace(em.EVO.Supersedes)
						seen := map[string]bool{}
						sameKin := false

						for predecessor != "" {
							if seen[predecessor] {
								return st, fmt.Errorf("Kin lineage cycle at %s", predecessor)
							}
							seen[predecessor] = true

							if predecessor == returnedID {
								sameKin = true
								break
							}

							ancestor, ok := st.Accepted[predecessor]
							if !ok {
								break
							}

							predecessor = strings.TrimSpace(ancestor.EVO.Supersedes)
						}

						if sameKin {
							continue
						}

						return st, fmt.Errorf(
							"Archonym %q already belongs to HUMAN_FINAL RETURNED sovereign EmergION %s",
							archonym,
							returnedID,
						)
					}
				}
			}

			delete(st.Approved, em.IDN)
			delete(decisionEvents, em.IDN)
			em.STA = core.StateAccepted
			st.Accepted[em.IDN] = em
		case "Q":
			if ev.ActionAuthorization == nil {
				return st, fmt.Errorf("action authorization event missing receipt")
			}

			receipt := *ev.ActionAuthorization
			receipt.EventID = ev.ID
			em, ok := st.Accepted[receipt.EmergIONID]
			if !ok {
				return st, fmt.Errorf("action authorization target not REG-accepted: %s", receipt.EmergIONID)
			}

			var facets []string
			if em.EVO.Metadata != nil {
				for _, facet := range em.EVO.Metadata.Facets {
					facets = append(facets, string(facet))
				}
			}

			candidates := adapters.DeriveActionCandidates(facets, em.CAP, true)
			matched := false
			for _, candidate := range candidates {
				if candidate.Adapter != receipt.Adapter || candidate.Action != receipt.Action {
					continue
				}

				matched = true
				if candidate.HumanFinalRequired && receipt.Authority != "HUMAN_FINAL" {
					return st, fmt.Errorf("action %s:%s requires HUMAN_FINAL", receipt.Adapter, receipt.Action)
				}
				break
			}

			if !matched {
				return st, fmt.Errorf("action %s:%s is not derivable from accepted EmergION %s", receipt.Adapter, receipt.Action, receipt.EmergIONID)
			}
			if !receipt.Authorized {
				return st, fmt.Errorf("action authorization is not authorized")
			}

			st.ActionAuthorizations = append(st.ActionAuthorizations, receipt)
		default:
			return st, fmt.Errorf("unknown event type %s", ev.Type)
		}
	}
	return st, nil
}

func AcceptedKinRoot(
	accepted map[string]core.EmergION,
	returned map[string]core.EmergION,
	id string,
) (string, error) {
	current := strings.TrimSpace(id)
	if current == "" {
		return "", fmt.Errorf("Kin root requires EmergION ID")
	}

	seen := map[string]bool{}

	for {
		if seen[current] {
			return "", fmt.Errorf("Kin lineage cycle at %s", current)
		}
		seen[current] = true

		em, ok := accepted[current]
		if !ok {
			return "", fmt.Errorf("Kin member not REG-accepted: %s", current)
		}

		predecessor := strings.TrimSpace(em.EVO.Supersedes)
		if predecessor == "" {
			return current, nil
		}

		if predecessor == current {
			return "", fmt.Errorf("Kin lineage self-reference at %s", current)
		}

		if _, ok := accepted[predecessor]; ok {
			current = predecessor
			continue
		}

		if _, ok := returned[predecessor]; ok {
			return current, nil
		}

		return "", fmt.Errorf(
			"Kin predecessor not REG-accepted or HUMAN_FINAL RETURNED: %s",
			predecessor,
		)
	}
}
