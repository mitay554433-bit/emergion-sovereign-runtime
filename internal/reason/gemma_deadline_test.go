package reason

import (
    "context"
    "os"
    "path/filepath"
    "strings"
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
