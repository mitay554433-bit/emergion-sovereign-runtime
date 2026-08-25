package reason

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxProgramPatchBytes = 32 * 1024

type ProgramPatchInput struct {
	Name          string
	Content       []byte
	GovernedState string
}

// ProposeProgramPatch asks the existing local Gemma reasoner for one bounded,
// source-specific unified diff. It does not mutate the repository and may
// legitimately return no patch when the evidence does not support a change.
func (g GemmaCLI) ProposeProgramPatch(ctx context.Context, in ProgramPatchInput) ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	target := filepath.ToSlash(strings.TrimSpace(in.Name))
	if target == "" || filepath.IsAbs(target) || strings.HasPrefix(target, "../") || strings.Contains(target, "/../") {
		return nil, fmt.Errorf("invalid program patch target %q", in.Name)
	}
	if len(in.Content) == 0 {
		return nil, fmt.Errorf("empty program source")
	}

	source := string(in.Content)
	governed := strings.TrimSpace(in.GovernedState)

	inputTokens := g.Context - g.MaxTokens - 1024
	if inputTokens < 256 {
		return nil, fmt.Errorf("Gemma context too small for bounded program proposal")
	}

	inputBytes := inputTokens * 3
	governedLimit := inputBytes / 4
	if len(governed) > governedLimit {
		governed = governed[:governedLimit]
	}

	sourceLimit := inputBytes - len(governed)
	if sourceLimit > 12*1024 {
		sourceLimit = 12 * 1024
	}
	if sourceLimit < 1 {
		return nil, fmt.Errorf("Gemma context exhausted by governed state")
	}
	if len(source) > sourceLimit {
		source = source[:sourceLimit]
	}

	prompt := buildProgramPatchPrompt(target, source, governed)
	args := programPatchArgs(g, prompt, g.MaxTokens)

	cctx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, g.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("Gemma program proposal timed out: %w", cctx.Err())
		}
		return nil, fmt.Errorf("Gemma program proposal failed: %w: %s", runErr, trim(stderr.String(), 500))
	}

	candidates := []string{stdout.String(), stderr.String(), stdout.String() + "\n" + stderr.String()}
	var parseErr error
	for _, candidate := range candidates {
		patch, noChange, err := parseProgramPatch(candidate, target)
		if err != nil {
			parseErr = err
			continue
		}
		if noChange {
			return nil, nil
		}
		return patch, nil
	}

	return nil, fmt.Errorf("Gemma program proposal invalid: %w", parseErr)
}

func programPatchArgs(g GemmaCLI, prompt string, outputTokens int) []string {
	args := []string{
		"-m", g.Model,
		"-p", prompt,
		"-n", fmt.Sprintf("%d", outputTokens),
		"-c", fmt.Sprintf("%d", g.Context),
		"-t", fmt.Sprintf("%d", g.Threads),
		"--temp", "0.1",
	}
	args = append(args, g.ExtraArgs...)
	args = append(args,
		"--log-disable",
		"--color", "off",
		"--single-turn",
		"--no-display-prompt",
		"--output-file", "/dev/stdout",
	)
	return args
}

func buildProgramPatchPrompt(target, source, governed string) string {
	return `@L:MXPD/2
@T:PROGRAM_DELTA

TARGET:` + target + `
SOURCE:
` + source + `

GOVERNED_STATE is comparison context only:
` + governed + `

Determine whether one small code change to TARGET is directly justified by SOURCE plus GOVERNED_STATE and advances the governed system without changing authority boundaries.
If no exact change is justified, output exactly:
NO_CHANGE

Otherwise output only one standard unified Git diff for TARGET.
Rules:
- modify TARGET only
- no new files, deletes, renames, binary patches, commits, pushes, deployments, or authority changes
- preserve HUMAN_FINAL, REG authority, provenance, RECOIL/WVC, and existing architecture
- smallest sufficient change only
- no markdown fences or explanatory prose
- patch must begin: diff --git a/` + target + ` b/` + target
}

func parseProgramPatch(raw, target string) ([]byte, bool, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, false, fmt.Errorf("empty program proposal")
	}
	if text == "NO_CHANGE" {
		return nil, true, nil
	}
	if len(text) > maxProgramPatchBytes {
		return nil, false, fmt.Errorf("program patch exceeds %d bytes", maxProgramPatchBytes)
	}
	if strings.Contains(text, "```") || strings.Contains(text, "GIT binary patch") || strings.Contains(text, "Binary files ") {
		return nil, false, fmt.Errorf("program patch contains unsupported output")
	}

	expectedDiff := "diff --git a/" + target + " b/" + target
	expectedOld := "--- a/" + target
	expectedNew := "+++ b/" + target

	diffCount := 0
	oldSeen := false
	newSeen := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			diffCount++
			if line != expectedDiff {
				return nil, false, fmt.Errorf("program patch targets unexpected file")
			}
		case strings.HasPrefix(line, "--- "):
			if line == "--- /dev/null" || line != expectedOld {
				return nil, false, fmt.Errorf("program patch has invalid old path")
			}
			oldSeen = true
		case strings.HasPrefix(line, "+++ "):
			if line == "+++ /dev/null" || line != expectedNew {
				return nil, false, fmt.Errorf("program patch has invalid new path")
			}
			newSeen = true
		}
	}

	if diffCount != 1 || !oldSeen || !newSeen {
		return nil, false, fmt.Errorf("program patch is not one complete target diff")
	}

	return []byte(text + "\n"), false, nil
}
