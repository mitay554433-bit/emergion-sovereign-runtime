package reason

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProgramPatchAcceptsSingleTargetDiff(t *testing.T) {
	patch := "diff --git a/internal/a.go b/internal/a.go\n" +
		"--- a/internal/a.go\n" +
		"+++ b/internal/a.go\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"

	got, noChange, err := parseProgramPatch(patch, "internal/a.go")
	if err != nil {
		t.Fatal(err)
	}
	if noChange {
		t.Fatal("patch parsed as NO_CHANGE")
	}
	if string(got) != patch {
		t.Fatalf("patch = %q want %q", string(got), patch)
	}
}

func TestParseProgramPatchAllowsNoChange(t *testing.T) {
	patch, noChange, err := parseProgramPatch("NO_CHANGE\n", "internal/a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !noChange || patch != nil {
		t.Fatalf("NO_CHANGE = patch %q noChange=%v", patch, noChange)
	}
}

func TestParseProgramPatchRejectsOtherTarget(t *testing.T) {
	patch := "diff --git a/internal/b.go b/internal/b.go\n" +
		"--- a/internal/b.go\n" +
		"+++ b/internal/b.go\n" +
		"@@ -1 +1 @@\n-old\n+new\n"

	if _, _, err := parseProgramPatch(patch, "internal/a.go"); err == nil {
		t.Fatal("unexpectedly accepted patch for another target")
	}
}

func TestParseProgramEdit(t *testing.T) {
	oldText, newText, noChange, err := parseProgramEdit("EDIT/1\nOLD:\nold\n===NEW===\nnew\n===END===", "internal/a.go")
	if err != nil {
		t.Fatal(err)
	}
	if noChange || oldText != "old" || newText != "new" {
		t.Fatalf("edit = old %q new %q noChange=%v", oldText, newText, noChange)
	}
}

func TestParseProgramEditAcceptsObservedGemmaFraming(t *testing.T) {
	oldText, newText, noChange, err := parseProgramEdit("EDIT/1\nTARGET:internal/a.go\n===OLD===\nold\n===NEW===\nnew", "internal/a.go")
	if err != nil {
		t.Fatal(err)
	}
	if noChange || oldText != "old" || newText != "new" {
		t.Fatalf("edit = old %q new %q noChange=%v", oldText, newText, noChange)
	}
}

func TestParseProgramEditRejectsUnexpectedTarget(t *testing.T) {
	_, _, _, err := parseProgramEdit("EDIT/1\nTARGET:internal/b.go\n===OLD===\nold\n===NEW===\nnew", "internal/a.go")
	if err == nil {
		t.Fatal("unexpectedly accepted edit for another target")
	}
}

func TestParseProgramEditRejectsMultipleNewBoundaries(t *testing.T) {
	_, _, _, err := parseProgramEdit("EDIT/1\n===OLD===\nold\n===NEW===\nnew\n===NEW===\nother", "internal/a.go")
	if err == nil {
		t.Fatal("unexpectedly accepted multiple NEW boundaries")
	}
}

func TestParseProgramEditAllowsNoChange(t *testing.T) {
	oldText, newText, noChange, err := parseProgramEdit("NO_CHANGE\n", "internal/a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !noChange || oldText != "" || newText != "" {
		t.Fatalf("NO_CHANGE = old %q new %q noChange=%v", oldText, newText, noChange)
	}
}

func TestRenderProgramPatchProducesApplicableTargetDiff(t *testing.T) {
	target := "internal/a.go"
	source := "package a\n\nvar value = \"old\"\n\nfunc keep() {}\n"
	patch, err := renderProgramPatch(target, source, "var value = \"old\"", "var value = \"new\"")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseProgramPatch(string(patch), target); err != nil {
		t.Fatalf("rendered patch rejected by parser: %v\n%s", err, patch)
	}

	root := t.TempDir()
	path := filepath.Join(root, target)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "apply", "--check", "-")
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(patch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git apply --check failed: %v: %s\n%s", err, out, patch)
	}
}

func TestRenderProgramPatchRejectsAmbiguousOldText(t *testing.T) {
	if _, err := renderProgramPatch("a.go", "x\nx\n", "x", "y"); err == nil {
		t.Fatal("unexpectedly accepted ambiguous OLD text")
	}
}

func TestProgramPatchArgsDoNotUseSemanticGrammar(t *testing.T) {
	args := programPatchArgs(GemmaCLI{Model: "model.gguf", Context: 2048, Threads: 4}, "prompt", 256)
	for i, arg := range args {
		if arg == "--grammar" {
			t.Fatalf("program patch args unexpectedly include semantic grammar at %d", i)
		}
	}
}

func TestBuildProgramPatchPromptRequiresExactObservedEditContract(t *testing.T) {
	prompt := buildProgramPatchPrompt("internal/a.go", "old\n", "state")
	wants := []string{
		"EDIT/1\nTARGET:internal/a.go\n===OLD===",
		"OLD is mandatory",
		"if you cannot quote the exact OLD block, output exactly NO_CHANGE",
		"must not duplicate code already present in SOURCE",
		"no markdown fences, Git diff syntax, or explanatory prose",
	}
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("program patch prompt missing %q\n%s", want, prompt)
		}
	}
}
