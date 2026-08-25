package adapters

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/store"
)

func initProgramExecutorRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustRunProgramCommand(t, root, "git", "init", "-q")
	mustRunProgramCommand(t, root, "git", "config", "user.email", "proof@example.invalid")
	mustRunProgramCommand(t, root, "git", "config", "user.name", "Program Proof")

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module program-proof\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "value.go"), []byte("package proof\n\nconst Value = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mustRunProgramCommand(t, root, "git", "add", "go.mod", "value.go")
	mustRunProgramCommand(t, root, "git", "commit", "-qm", "baseline")
	return root
}

func mustRunProgramCommand(t *testing.T, dir string, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}

func programPatch(t *testing.T, root string, content string) []byte {
	t.Helper()
	path := filepath.Join(root, "value.go")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := mustRunProgramCommand(t, root, "git", "diff", "--", "value.go")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	return patch
}

func TestGitHubProgramExecutorAppliesAuthorizedPatchAndVerifiesRepository(t *testing.T) {
	root := initProgramExecutorRepo(t)
	patch := programPatch(t, root, "package proof\n\nconst Value = 2\n")

	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := s.Preserve(patch)
	if err != nil {
		t.Fatal(err)
	}

	request := ExecutionRequest{
		EmergIONID:      "E-PROGRAM-PROOF",
		SourceHash:      evidence.Hash,
		AuthorizationID: "EV-Q-PROGRAM",
		Adapter:         "GITHUB",
		Action:          "PROGRAM",
		Authority:       "BOUNDED_CAP",
	}
	request.TransitionEmergIONID = ExecutionTransitionID(request)

	result, err := (GitHubProgramExecutor{Store: s, WorkDir: root}).Execute(request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded {
		t.Fatal("authorized PROGRAM patch did not succeed")
	}
	if err := VerifyExecutionResult(request, result); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, "value.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Value = 2") {
		t.Fatalf("authorized patch was not applied: %s", got)
	}
}

func TestGitHubProgramExecutorRollsBackPatchWhenVerificationFails(t *testing.T) {
	root := initProgramExecutorRepo(t)
	patch := programPatch(t, root, "package proof\n\nconst Value =\n")

	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := s.Preserve(patch)
	if err != nil {
		t.Fatal(err)
	}

	request := ExecutionRequest{
		EmergIONID:      "E-PROGRAM-FAIL-PROOF",
		SourceHash:      evidence.Hash,
		AuthorizationID: "EV-Q-PROGRAM",
		Adapter:         "GITHUB",
		Action:          "PROGRAM",
		Authority:       "BOUNDED_CAP",
	}
	request.TransitionEmergIONID = ExecutionTransitionID(request)

	result, err := (GitHubProgramExecutor{Store: s, WorkDir: root}).Execute(request)
	if err == nil {
		t.Fatal("invalid PROGRAM patch unexpectedly succeeded")
	}
	if result.Succeeded {
		t.Fatal("invalid PROGRAM patch reported success")
	}

	got, err := os.ReadFile(filepath.Join(root, "value.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Value = 1") {
		t.Fatalf("failed PROGRAM patch was not rolled back: %s", got)
	}

	status := mustRunProgramCommand(t, root, "git", "status", "--porcelain")
	if len(strings.TrimSpace(string(status))) != 0 {
		t.Fatalf("rollback left dirty worktree: %s", status)
	}
}
