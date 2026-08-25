package reason

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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
// source-specific edit. The runtime deterministically renders that edit as a
// unified Git diff. It does not mutate the repository and may legitimately
// return no patch when the evidence does not support a change.
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

	outputFile, err := os.CreateTemp("", "unifusion-program-*.txt")
	if err != nil {
		return nil, fmt.Errorf("create program proposal output: %w", err)
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		_ = os.Remove(outputPath)
		return nil, fmt.Errorf("close program proposal output: %w", err)
	}
	defer os.Remove(outputPath)

	args := programPatchArgs(g, prompt, g.MaxTokens)
	args = append(args, "--output-file", outputPath)

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

	fileOutput, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		return nil, fmt.Errorf("read program proposal output: %w", readErr)
	}

	candidates := []string{
		string(fileOutput),
		stdout.String(),
		stderr.String(),
		stdout.String() + "\n" + stderr.String(),
	}
	var parseErr error
	for _, candidate := range candidates {
		oldText, newText, noChange, err := parseProgramEdit(candidate, target)
		if err != nil {
			// Preserve compatibility with already-valid governed patch producers while
			// the live Gemma contract uses EDIT/1. Malformed raw diffs still fail.
			patch, rawNoChange, patchErr := parseProgramPatch(candidate, target)
			if patchErr == nil {
				if rawNoChange {
					return nil, nil
				}
				return patch, nil
			}
			parseErr = err
			continue
		}
		if noChange {
			return nil, nil
		}

		patch, err := renderProgramPatch(target, string(in.Content), oldText, newText)
		if err != nil {
			parseErr = err
			continue
		}
		patch, _, err = parseProgramPatch(string(patch), target)
		if err != nil {
			parseErr = err
			continue
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

Otherwise output exactly one bounded edit in this format:
EDIT/1
TARGET:` + target + `
===OLD===
<exact complete existing source line or contiguous source lines copied verbatim from SOURCE>
===NEW===
<replacement line or contiguous replacement lines>
===END===

Rules:
- TARGET must be exactly the TARGET shown above
- OLD is mandatory; never output ===NEW=== before a non-empty ===OLD=== block
- OLD must be copied verbatim from SOURCE and occur exactly once in TARGET
- if you cannot quote the exact OLD block, output exactly NO_CHANGE
- OLD and NEW must contain complete lines only
- NEW must differ from OLD and must not duplicate code already present in SOURCE
- modify TARGET only
- no new files, deletes, renames, binary patches, commits, pushes, deployments, or authority changes
- preserve HUMAN_FINAL, REG authority, provenance, RECOIL/WVC, and existing architecture
- smallest sufficient change only
- no markdown fences, Git diff syntax, or explanatory prose
`
}

func parseProgramEdit(raw, target string) (string, string, bool, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", "", false, fmt.Errorf("empty program proposal")
	}
	if text == "NO_CHANGE" {
		return "", "", true, nil
	}
	if strings.Contains(text, "```") || strings.Contains(text, "diff --git ") {
		return "", "", false, fmt.Errorf("program edit contains unsupported output")
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "EDIT/1" {
		return "", "", false, fmt.Errorf("program edit does not match EDIT/1 contract")
	}
	lines = lines[1:]

	if len(lines) > 0 && strings.HasPrefix(lines[0], "TARGET:") {
		proposalTarget := strings.TrimSpace(strings.TrimPrefix(lines[0], "TARGET:"))
		if proposalTarget != target {
			return "", "", false, fmt.Errorf("program edit targets unexpected file")
		}
		lines = lines[1:]
	}
	if len(lines) == 0 || (lines[0] != "OLD:" && lines[0] != "===OLD===") {
		return "", "", false, fmt.Errorf("program edit does not match EDIT/1 contract")
	}
	lines = lines[1:]

	newBoundary := -1
	for i, line := range lines {
		if line == "===NEW===" {
			if newBoundary >= 0 {
				return "", "", false, fmt.Errorf("program edit must contain one NEW boundary")
			}
			newBoundary = i
		}
	}
	if newBoundary < 0 {
		return "", "", false, fmt.Errorf("program edit must contain one NEW boundary")
	}

	oldLines := lines[:newBoundary]
	newLines := lines[newBoundary+1:]
	if len(newLines) > 0 && newLines[len(newLines)-1] == "===END===" {
		newLines = newLines[:len(newLines)-1]
	}
	for _, line := range newLines {
		if line == "===END===" {
			return "", "", false, fmt.Errorf("program edit END boundary must terminate proposal")
		}
	}

	oldText := strings.Join(oldLines, "\n")
	newText := strings.Join(newLines, "\n")
	if oldText == "" {
		return "", "", false, fmt.Errorf("program edit OLD is empty")
	}
	if oldText == newText {
		return "", "", false, fmt.Errorf("program edit does not change source")
	}
	return oldText, newText, false, nil
}

func renderProgramPatch(target, source, oldText, newText string) ([]byte, error) {
	if strings.Count(source, oldText) != 1 {
		return nil, fmt.Errorf("program edit OLD must occur exactly once in target")
	}

	idx := strings.Index(source, oldText)
	if idx < 0 {
		return nil, fmt.Errorf("program edit OLD not found in target")
	}
	if idx > 0 && source[idx-1] != '\n' {
		return nil, fmt.Errorf("program edit OLD must start at a line boundary")
	}
	end := idx + len(oldText)
	if end < len(source) && source[end] != '\n' {
		return nil, fmt.Errorf("program edit OLD must end at a line boundary")
	}
	if strings.HasPrefix(oldText, "\n") || strings.HasSuffix(oldText, "\n") || strings.HasPrefix(newText, "\n") || strings.HasSuffix(newText, "\n") {
		return nil, fmt.Errorf("program edit blocks must not include boundary newlines")
	}

	prefixLines := splitProgramLines(source[:idx])
	oldLines := splitProgramLines(oldText)
	newLines := splitProgramLines(newText)
	afterStart := end
	if afterStart < len(source) && source[afterStart] == '\n' {
		afterStart++
	}
	afterLines := splitProgramLines(source[afterStart:])

	contextBefore := 3
	if len(prefixLines) < contextBefore {
		contextBefore = len(prefixLines)
	}
	contextAfter := 3
	if len(afterLines) < contextAfter {
		contextAfter = len(afterLines)
	}

	oldStart := len(prefixLines) - contextBefore + 1
	newStart := oldStart
	oldCount := contextBefore + len(oldLines) + contextAfter
	newCount := contextBefore + len(newLines) + contextAfter

	var b strings.Builder
	b.WriteString("diff --git a/")
	b.WriteString(target)
	b.WriteString(" b/")
	b.WriteString(target)
	b.WriteByte('\n')
	b.WriteString("--- a/")
	b.WriteString(target)
	b.WriteByte('\n')
	b.WriteString("+++ b/")
	b.WriteString(target)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
	for _, line := range prefixLines[len(prefixLines)-contextBefore:] {
		b.WriteByte(' ')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range oldLines {
		b.WriteByte('-')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range newLines {
		b.WriteByte('+')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range afterLines[:contextAfter] {
		b.WriteByte(' ')
		b.WriteString(line)
		b.WriteByte('\n')
	}

	patch := []byte(b.String())
	if len(patch) > maxProgramPatchBytes {
		return nil, fmt.Errorf("program patch exceeds %d bytes", maxProgramPatchBytes)
	}
	return patch, nil
}

func splitProgramLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
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
