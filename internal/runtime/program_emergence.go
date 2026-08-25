package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/reason"
)

// ProposeOneProgramPatch lets the existing governed runtime turn one accepted
// PROGRAM_FORGE source into one exact patch proposal. The proposal is only
// admitted to GOV; it is never authorized, applied, committed, pushed, or
// accepted into REG by this method.
func (r Runtime) ProposeOneProgramPatch(
	ctx context.Context,
	gemma reason.GemmaCLI,
	workDir string,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}
	if err := gemma.Validate(); err != nil {
		return core.EmergION{}, false, nil
	}

	events, err := r.Store.Events()
	if err != nil {
		return core.EmergION{}, false, err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return core.EmergION{}, false, err
	}

	boundary, governedState, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}

	ids := make([]string, 0, len(st.Accepted))
	for id := range st.Accepted {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		parent := st.Accepted[id]
		if !acceptedProgramActionEnabled(parent, gemma.Validate() == nil) {
			continue
		}
		if programPatchPending(st, parent.IDN) {
			continue
		}

		target, content, err := currentProgramTarget(workDir, parent)
		if err != nil {
			continue
		}

		patch, err := gemma.ProposeProgramPatch(ctx, reason.ProgramPatchInput{
			Name:          target,
			Content:       content,
			GovernedState: governedState,
		})
		if err != nil {
			return core.EmergION{}, false, err
		}
		if len(patch) == 0 {
			continue
		}

		if _, err := adapters.CheckGitHubProgramPatch(workDir, patch); err != nil {
			return core.EmergION{}, false, fmt.Errorf("program emergence patch rejected: %w", err)
		}

		analysis := reason.Result{
			Summary: "bounded program patch proposal for " + target,
			Relationships: map[string]string{
				"source_name":    target + ".patch",
				"source_kind":    "PROGRAM_PATCH",
				"program_target": target,
			},
			Capabilities: []string{"PROGRAM"},
			Facts:        []string{"program patch proposal preserved"},
			Risk:         "H",
			Facets:       []string{"PROGRAM_FORGE"},
		}

		em, duplicate, err := r.admitAnalyzedCandidate(
			ctx,
			target+".patch",
			patch,
			"program_emergence",
			boundary,
			governedState,
			parent.IDN,
			analysis,
		)
		if err != nil {
			return em, false, err
		}
		if duplicate {
			continue
		}
		return em, true, nil
	}

	return core.EmergION{}, false, nil
}

func acceptedProgramActionEnabled(em core.EmergION, localGemma bool) bool {
	var facets []string
	if em.EVO.Metadata != nil {
		for _, facet := range em.EVO.Metadata.Facets {
			facets = append(facets, string(facet))
		}
	}

	for _, action := range adapters.DeriveActionCandidates(facets, em.CAP, localGemma) {
		if action.Adapter == "GITHUB" &&
			action.Action == "PROGRAM" &&
			action.Enabled &&
			action.HumanFinalRequired {
			return true
		}
	}
	return false
}

func programPatchPending(st core.State, origin string) bool {
	groups := []map[string]core.EmergION{
		st.AtGOV,
		st.Approved,
		st.Accepted,
		st.Held,
	}
	for _, group := range groups {
		for _, em := range group {
			if em.REL["source_kind"] == "PROGRAM_PATCH" && em.REL["origin"] == origin {
				return true
			}
		}
	}
	return false
}

func currentProgramTarget(workDir string, parent core.EmergION) (string, []byte, error) {
	sourceName := filepath.ToSlash(strings.TrimSpace(parent.REL["source_name"]))
	if sourceName == "" {
		return "", nil, fmt.Errorf("PROGRAM source %s has no source_name", parent.IDN)
	}

	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = "."
	}

	rootBytes, err := exec.Command("git", "-C", workDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", nil, fmt.Errorf("PROGRAM workdir is not a git repository: %w", err)
	}
	root := strings.TrimSpace(string(rootBytes))

	tracked, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return "", nil, fmt.Errorf("PROGRAM could not enumerate tracked files: %w", err)
	}

	sourceBase := filepath.Base(sourceName)
	var matches []string
	for _, raw := range strings.Split(string(tracked), "\x00") {
		rel := filepath.ToSlash(strings.TrimSpace(raw))
		if rel == "" {
			continue
		}
		if rel == sourceName || filepath.Base(rel) == sourceBase {
			matches = append(matches, rel)
		}
	}
	sort.Strings(matches)

	if len(matches) == 0 {
		return "", nil, fmt.Errorf("PROGRAM target %q is not tracked", sourceName)
	}
	if len(matches) > 1 && !containsExact(matches, sourceName) {
		return "", nil, fmt.Errorf("PROGRAM target %q is ambiguous", sourceName)
	}

	target := matches[0]
	if containsExact(matches, sourceName) {
		target = sourceName
	}

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(target)))
	if err != nil {
		return "", nil, err
	}
	if len(content) == 0 {
		return "", nil, fmt.Errorf("PROGRAM target %q is empty", target)
	}

	return target, content, nil
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
