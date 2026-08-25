package adapters

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"emergion-sovereign-runtime/internal/store"
)

const programExecutionOutputLimit = 16 * 1024

type GitHubProgramExecutor struct {
	Store   *store.Store
	WorkDir string
}

func (e GitHubProgramExecutor) Execute(
	request ExecutionRequest,
) (ExecutionResult, error) {
	result := BindExecutionResult(request, ExecutionResult{})

	if request.Adapter != "GITHUB" {
		err := fmt.Errorf(
			"GITHUB executor cannot execute adapter %s",
			request.Adapter,
		)
		result.Error = err.Error()
		return result, err
	}

	if request.Action != "PROGRAM" {
		err := fmt.Errorf(
			"GITHUB executor does not support action %s",
			request.Action,
		)
		result.Error = err.Error()
		return result, err
	}

	if strings.TrimSpace(request.AuthorizationID) == "" {
		err := fmt.Errorf("GITHUB:PROGRAM requires HUMAN_FINAL authorization")
		result.Error = err.Error()
		return result, err
	}

	if e.Store == nil {
		err := fmt.Errorf("GITHUB executor store not configured")
		result.Error = err.Error()
		return result, err
	}

	if strings.TrimSpace(request.SourceHash) == "" {
		err := fmt.Errorf("execution request missing source hash")
		result.Error = err.Error()
		return result, err
	}

	patch, err := e.Store.ReadEvidence(request.SourceHash)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if len(bytes.TrimSpace(patch)) == 0 {
		err := fmt.Errorf("GITHUB:PROGRAM patch evidence is empty")
		result.Error = err.Error()
		return result, err
	}
	if bytes.Contains(patch, []byte("GIT binary patch")) ||
		bytes.Contains(patch, []byte("Binary files ")) {
		err := fmt.Errorf("GITHUB:PROGRAM binary patches are not supported")
		result.Error = err.Error()
		return result, err
	}

	workDir := strings.TrimSpace(e.WorkDir)
	if workDir == "" {
		workDir = "."
	}

	rootBytes, err := exec.Command("git", "-C", workDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		err = fmt.Errorf("GITHUB:PROGRAM working directory is not a git repository: %w", err)
		result.Error = err.Error()
		return result, err
	}
	root := strings.TrimSpace(string(rootBytes))

	status, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		err = fmt.Errorf("GITHUB:PROGRAM could not inspect repository state: %w", err)
		result.Error = err.Error()
		return result, err
	}
	if len(bytes.TrimSpace(status)) != 0 {
		err = fmt.Errorf("GITHUB:PROGRAM requires a clean worktree")
		result.Error = err.Error()
		return result, err
	}

	if output, applyErr := runPatchCommand(root, patch, "apply", "--check", "--whitespace=error-all", "-"); applyErr != nil {
		err = fmt.Errorf("GITHUB:PROGRAM patch check failed: %s", boundedExecutionOutput(output))
		result.Error = err.Error()
		return result, err
	}

	if output, applyErr := runPatchCommand(root, patch, "apply", "--whitespace=error-all", "-"); applyErr != nil {
		err = fmt.Errorf("GITHUB:PROGRAM patch apply failed: %s", boundedExecutionOutput(output))
		result.Error = err.Error()
		return result, err
	}

	testCmd := exec.Command("go", "test", "./...")
	testCmd.Dir = root
	testOutput, testErr := testCmd.CombinedOutput()
	if testErr != nil {
		revertOutput, revertErr := runPatchCommand(root, patch, "apply", "-R", "--whitespace=nowarn", "-")
		if revertErr != nil {
			err = fmt.Errorf(
				"GITHUB:PROGRAM verification failed and rollback failed; tests=%s rollback=%s",
				boundedExecutionOutput(testOutput),
				boundedExecutionOutput(revertOutput),
			)
		} else {
			err = fmt.Errorf(
				"GITHUB:PROGRAM verification failed; patch rolled back: %s",
				boundedExecutionOutput(testOutput),
			)
		}
		result.Error = err.Error()
		return result, err
	}

	result.Succeeded = true
	result.Output = "GITHUB:PROGRAM applied authorized patch\ngo test ./...: PASS"
	if summary := boundedExecutionOutput(testOutput); summary != "" {
		result.Output += "\n" + summary
	}

	return result, nil
}

func runPatchCommand(root string, patch []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Stdin = bytes.NewReader(patch)
	return cmd.CombinedOutput()
}

func boundedExecutionOutput(output []byte) string {
	output = bytes.TrimSpace(output)
	if len(output) <= programExecutionOutputLimit {
		return string(output)
	}
	return string(output[len(output)-programExecutionOutputLimit:])
}
