// Package fieldapi exposes the serverless sovereign runtime to native shells.
// A desktop or mobile application can embed this package without invoking the CLI.
package fieldapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/analytics"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
)

type Runtime struct {
	store    *store.Store
	reasoner reason.Reasoner
}

func Open(stateRoot string, reasoner reason.Reasoner) (*Runtime, error) {
	if reasoner == nil {
		return nil, fmt.Errorf("reasoner required")
	}
	s, err := store.Open(stateRoot)
	if err != nil {
		return nil, err
	}
	return &Runtime{store: s, reasoner: reasoner}, nil
}
func (r *Runtime) Capture(ctx context.Context, path string) (core.EmergION, error) {
	em, _, err := (fieldruntime.Runtime{Store: r.store, Reasoner: r.reasoner}).Capture(ctx, path, false)
	return em, err
}
func (r *Runtime) state() (core.State, error) {
	ev, err := r.store.Events()
	if err != nil {
		return core.State{}, err
	}
	return livefield.Rebuild(ev)
}
func (r *Runtime) Status() (analytics.Metrics, error) {
	st, err := r.state()
	if err != nil {
		return analytics.Metrics{}, err
	}
	return analytics.Measure(st), nil
}
func (r *Runtime) Decide(id, decision, reasonText string) error {
	st, err := r.state()
	if err != nil {
		return err
	}
	em, ok := st.AtGOV[id]
	if !ok {
		return fmt.Errorf("candidate not at GOV")
	}
	em, receipt, err := gov.Decide(em, gov.Decision(decision), "HUMAN_FINAL", reasonText)
	if err != nil {
		return err
	}
	decisionID, err := r.store.SaveDecision(receipt)
	if err != nil {
		return err
	}
	if receipt.Decision == string(gov.Approve) {
		_, rr, err := reg.Accept(em, decisionID)
		if err != nil {
			return err
		}
		_, err = r.store.SaveAccepted(rr)
		return err
	}
	return nil
}
func (r *Runtime) Actions(
	emergionID string,
	localGemma bool,
) ([]adapters.ActionCandidate, error) {
	st, err := r.state()
	if err != nil {
		return nil, err
	}

	em, ok := st.Accepted[emergionID]
	if !ok {
		return nil, fmt.Errorf(
			"EmergION %q is not REG-accepted",
			emergionID,
		)
	}

	var facets []string
	if em.EVO.Metadata != nil {
		for _, facet := range em.EVO.Metadata.Facets {
			facets = append(facets, string(facet))
		}
	}

	return adapters.DeriveActionCandidates(
		facets,
		em.CAP,
		localGemma,
	), nil
}

func (r *Runtime) Authorize(
	emergionID string,
	adapter string,
	action string,
	reasonText string,
	localGemma bool,
) (string, error) {
	return (fieldruntime.Runtime{
		Store: r.store,
	}).AuthorizeAction(
		emergionID,
		adapter,
		action,
		reasonText,
		localGemma,
	)
}

func (r *Runtime) Execute(
	ctx context.Context,
	emergionID string,
	adapter string,
	action string,
	gemma reason.GemmaCLI,
) (adapters.ExecutionRequest, adapters.ExecutionResult, core.EmergION, bool, error) {
	return (fieldruntime.Runtime{
		Store: r.store,
	}).ExecuteAction(
		ctx,
		emergionID,
		adapter,
		action,
		gemma,
	)
}

func (r *Runtime) SafeAction(
	ctx context.Context,
	gemma reason.GemmaCLI,
) (core.EmergION, bool, error) {
	return (fieldruntime.Runtime{
		Store: r.store,
	}).ExecuteOneSafeAction(
		ctx,
		gemma,
	)
}

func (r *Runtime) GovernedCycle(
	ctx context.Context,
	gemma reason.GemmaCLI,
) ([]core.EmergION, core.EmergION, bool, error) {
	circulated := []core.EmergION{}

	saws, err := r.CirculateSAWs(ctx)
	if err != nil {
		return circulated, core.EmergION{}, false, err
	}
	circulated = append(circulated, saws...)

	program, proposed, err := (fieldruntime.Runtime{
		Store:    r.store,
		Reasoner: r.reasoner,
	}).ProposeOneProgramPatch(
		ctx,
		gemma,
		".",
	)
	if err != nil {
		return circulated, core.EmergION{}, false, err
	}
	if proposed {
		circulated = append(circulated, program)
	}

	signal, executed, err := (fieldruntime.Runtime{
		Store: r.store,
	}).ExecuteOneSafeAction(
		ctx,
		gemma,
	)
	if err != nil {
		return circulated, core.EmergION{}, false, err
	}

	return circulated, signal, executed, nil
}

func (r *Runtime) Run(
	ctx context.Context,
	dropzone string,
	interval time.Duration,
	gemma reason.GemmaCLI,
	onCapture func(string),
	onCycle func([]core.EmergION, core.EmergION, bool),
) error {
	runtime := fieldruntime.Runtime{
		Store:    r.store,
		Reasoner: r.reasoner,
	}

	return runtime.Run(
		ctx,
		dropzone,
		interval,
		onCapture,
		func(cycleCtx context.Context) error {
			for {
				circulated, signal, executed, err :=
					r.GovernedCycle(cycleCtx, gemma)
				if err != nil {
					return err
				}

				if onCycle != nil {
					onCycle(circulated, signal, executed)
				}

				if len(circulated) == 0 && !executed {
					st, stateErr := r.state()
					if stateErr != nil {
						return stateErr
					}
					if len(st.AtGOV) != 0 {
						return nil
					}

					// HUMAN_FINAL Returned candidates remain non-canonical.
					// Deterministically derive one bounded evidence-bearing SOURCE
					// and re-enter the existing Capture/rework admission path.
					var returned core.EmergION
					for id, em := range st.Returned {
						if returned.IDN == "" || id < returned.IDN {
							returned = em
						}
					}
					if returned.IDN != "" {
						evidence, marshalErr := json.Marshal(map[string]any{
							"source_kind":       "RETURNED_CONTINUATION",
							"returned_emergion": returned.IDN,
							"returned_state":    returned.STA,
							"source_hash":       returned.MEM.SourceHash,
						})
						if marshalErr != nil {
							return marshalErr
						}
						_, duplicate, captureErr := (fieldruntime.Runtime{
							Store:               r.store,
							Reasoner:            r.reasoner,
							ReturnedPredecessor: returned.IDN,
						}).CaptureBytes(
							cycleCtx,
							"returned-continuation",
							evidence,
							"governed_returned_continuation",
						)
						if captureErr != nil {
							return captureErr
						}
						if !duplicate {
							return nil
						}
					}

					var target core.EmergION
					for id, em := range st.Accepted {
						if em.REL["source_kind"] == "TARGET" && (target.IDN == "" || id < target.IDN) {
							target = em
						}
					}
					if target.IDN != "" {
						reality := make(map[string]core.EmergION)
						for id, em := range st.Accepted {
							kind := em.REL["source_kind"]
							if kind != "TARGET" && kind != "TARGET_COMPARISON" && kind != "IDLE_STATE_OBSERVATION" {
								reality[id] = em
							}
						}
						evidence, marshalErr := json.Marshal(map[string]any{"target_emergion": target.IDN, "target_state": target.REL["target_state"], "reality": reality})
						if marshalErr != nil {
							return marshalErr
						}
						_, duplicate, captureErr := (fieldruntime.Runtime{Store: r.store, Reasoner: r.reasoner}).CaptureTargetComparison(cycleCtx, target, reality, evidence)
						if captureErr != nil {
							return captureErr
						}
						if !duplicate {
							return nil
						}
					}
					accepted := make(map[string]core.EmergION)
					for id, em := range st.Accepted {
						if em.REL["source_kind"] != "IDLE_STATE_OBSERVATION" {
							accepted[id] = em
						}
					}
					evidence, marshalErr := json.Marshal(accepted)
					if marshalErr != nil {
						return marshalErr
					}
					_, _, captureErr := (fieldruntime.Runtime{Store: r.store}).CaptureIdleStateObservation(cycleCtx, evidence)
					return captureErr
				}
				if cycleCtx.Err() != nil {
					return nil
				}
			}
		},
	)
}

func (r *Runtime) Render(dir string) error {
	st, err := r.state()
	if err != nil {
		return err
	}
	_, err = proj.Current(dir, st)
	return err
}

func (r *Runtime) RenderCurrent(dir string) (proj.Receipt, error) {
	st, err := r.state()
	if err != nil {
		return proj.Receipt{}, err
	}
	return proj.Current(dir, st)
}

func (r *Runtime) CirculateSAWs(
	ctx context.Context,
) ([]core.EmergION, error) {
	st, err := r.state()
	if err != nil {
		return nil, err
	}

	sources, err := proj.SAWSources(st)
	if err != nil {
		return nil, err
	}

	runtime := fieldruntime.Runtime{
		Store:    r.store,
		Reasoner: r.reasoner,
	}

	out := make([]core.EmergION, 0, len(sources))

	for _, source := range sources {
		em, duplicate, err := runtime.CaptureBytes(
			ctx,
			source.ID,
			source.Content,
			"saw_projection",
		)
		if err != nil {
			return out, err
		}

		if duplicate {
			continue
		}

		out = append(out, em)
	}

	return out, nil
}

// StatusJSON exposes the existing runtime status through a primitive
// serialization boundary suitable for native hosts.
// It owns no state and derives no authority.
func (r *Runtime) StatusJSON() (string, error) {
	status, err := r.Status()
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(status)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// ActionsJSON exposes the existing bounded action derivation as JSON.
// Action authority and derivation remain owned by the existing runtime.
func (r *Runtime) ActionsJSON(
	emergionID string,
	localGemma bool,
) (string, error) {
	actions, err := r.Actions(emergionID, localGemma)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(actions)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// DecideBinding is a primitive native-host entrypoint to the existing
// HUMAN_FINAL decision path. It introduces no independent authority.
func (r *Runtime) DecideBinding(
	id string,
	decision string,
	reasonText string,
) error {
	return r.Decide(id, decision, reasonText)
}

// AuthorizeBinding is a primitive native-host entrypoint to the existing
// governed action-authorization path.
func (r *Runtime) AuthorizeBinding(
	emergionID string,
	adapter string,
	action string,
	reasonText string,
	localGemma bool,
) (string, error) {
	return r.Authorize(
		emergionID,
		adapter,
		action,
		reasonText,
		localGemma,
	)
}

// RenderCurrentJSON exposes the existing hash-bound projection receipt.
// FIELD remains a projection; this method creates no competing state.
func (r *Runtime) RenderCurrentJSON(dir string) (string, error) {
	receipt, err := r.RenderCurrent(dir)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
