package reason

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The inference component cannot renew its budget after a rejected answer.
// This test uses a fake executable: no installed model or device is needed.
func TestGemmaAnalyzeSemanticRetrySharesDeadline(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-llama-cli")
	model := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(model, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	// Each response is well-formed but unsupported by the supplied source.
	// A retry must be interrupted by the original 300ms budget.
	script := "#!/bin/sh\nsleep 0.20\nprintf 'S|bounded\\nK|L\\nF|unsupported claim\\nZ'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{
		Binary: binary, Model: model, Threads: 1, Context: 2048,
		MaxTokens: 64, Timeout: 300 * time.Millisecond,
	}
	started := time.Now()
	_, err := g.Analyze(context.Background(), Input{
		Name: "source.txt", Content: []byte("Grounded source evidence."),
	})
	elapsed := time.Since(started)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected shared-deadline timeout, got %v", err)
	}
	if elapsed > 550*time.Millisecond {
		t.Fatalf("retry renewed inference deadline: %v", elapsed)
	}
}

func TestGemmaAnalyzeCompletesBeforeCLIExit(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "llama-cli")
	model := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(model, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(dir, "pid")
	script := "#!/bin/sh\nprintf '%s\\n' $$ > " + strconv.Quote(pidPath) + "\nprintf 'S|clean assistant result\\nK|L\\nF|clean assistant fact\\nZ'\nexec sleep 15\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{
		Binary: binary, Model: model, Threads: 1, Context: 2048,
		MaxTokens: 64, Timeout: 2 * time.Second,
	}
	started := time.Now()
	result, err := g.Analyze(context.Background(), Input{
		Name:    "source.txt",
		Content: []byte("clean assistant result\nclean assistant fact\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("validated record waited for llama-cli to exit: %v", elapsed)
	}
	waitProcessDead(t, readPID(t, pidPath))
	if result.Summary != "clean assistant result" ||
		result.Risk != "L" ||
		len(result.Facts) != 1 || result.Facts[0] != "clean assistant fact" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// A llama-cli that never exits must not pin the governed cycle inside os/exec.
// The shell backgrounds sleep, which inherits the stdout pipe; killing only the
// shell leaves that child alive and Wait blocked. Both processes have to die,
// and Analyze has to return the shared deadline error.
func TestGemmaAnalyzeDeadlineKillsHungCLI(t *testing.T) {
	dir := t.TempDir()
	binary, shellPID, childPID := writeHungLlamaCLI(t, dir)
	model := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(model, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{
		Binary: binary, Model: model, Threads: 1, Context: 2048,
		MaxTokens: 64, Timeout: 400 * time.Millisecond,
	}
	started := time.Now()
	_, err := g.Analyze(context.Background(), Input{
		Name:    "source.txt",
		Content: []byte("hung llama-cli source"),
	})
	elapsed := time.Since(started)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if elapsed > g.Timeout+time.Second {
		t.Fatalf("hung llama-cli outlived deadline %s: %v", g.Timeout, elapsed)
	}
	waitProcessDead(t, readPID(t, shellPID))
	waitProcessDead(t, readPID(t, childPID))
}

// GovernedCycle calls ProposeProgramPatch before safe-work Analyze. The same
// deadline has to bound that llama-cli, or the cycle never reaches safe work.
func TestGemmaProgramProposalDeadlineKillsHungCLI(t *testing.T) {
	dir := t.TempDir()
	binary, shellPID, childPID := writeHungLlamaCLI(t, dir)
	model := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(model, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{
		Binary: binary, Model: model, Threads: 1, Context: 2048,
		MaxTokens: 64, Timeout: 400 * time.Millisecond,
	}
	started := time.Now()
	patch, err := g.ProposeProgramPatch(context.Background(), ProgramPatchInput{
		Name:    "notes.txt",
		Content: []byte("package notes\n"),
	})
	elapsed := time.Since(started)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected program proposal deadline, got patch=%q err=%v", patch, err)
	}
	if elapsed > g.Timeout+time.Second {
		t.Fatalf("hung program-proposal llama-cli outlived deadline %s: %v", g.Timeout, elapsed)
	}
	waitProcessDead(t, readPID(t, shellPID))
	waitProcessDead(t, readPID(t, childPID))
}

func writeHungLlamaCLI(t *testing.T, dir string) (binary, shellPID, childPID string) {
	t.Helper()
	binary = filepath.Join(dir, "llama-cli")
	shellPID = filepath.Join(dir, "shell.pid")
	childPID = filepath.Join(dir, "child.pid")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' $$ > %s\nsleep 999999 &\nprintf '%%s\\n' $! > %s\nwait\n", strconv.Quote(shellPID), strconv.Quote(childPID))
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, path := range []string{shellPID, childPID} {
			text, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
			if err != nil {
				continue
			}
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return binary, shellPID, childPID
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid %s: %v", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		t.Fatalf("parse pid %s: %v", path, err)
	}
	return pid
}

func waitProcessDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still running after deadline", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
