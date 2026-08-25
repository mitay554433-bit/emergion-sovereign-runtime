package reason

import "testing"

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

func TestProgramPatchArgsDoNotUseSemanticGrammar(t *testing.T) {
	args := programPatchArgs(GemmaCLI{Model: "model.gguf", Context: 2048, Threads: 4}, "prompt", 256)
	for i, arg := range args {
		if arg == "--grammar" {
			t.Fatalf("program patch args unexpectedly include semantic grammar at %d", i)
		}
	}
}
