// Package gemmalive keeps tests hermetic: tests that would run the real
// local Gemma model (llama-cli or llama-completion plus a .gguf file) run
// only when UNIFUSION_GEMMA_LIVE_TESTS=1. Having GEMMA_BIN and GEMMA_MODEL
// exported in the shell, or llama-cli on PATH, is not enough on its own.
// Production code does not use this package.
package gemmalive

import (
	"os"
	"testing"
)

// Env is the opt-in switch for live Gemma tests.
const Env = "UNIFUSION_GEMMA_LIVE_TESTS"

// AbsentBinary and AbsentModel are paths that never exist, for tests that
// need a Gemma configuration but must not discover the installed one.
const (
	AbsentBinary = "/nonexistent/unifusion-test/llama-cli"
	AbsentModel  = "/nonexistent/unifusion-test/gemma.gguf"
)

// Enabled reports whether live Gemma tests were explicitly opted in.
func Enabled() bool {
	return os.Getenv(Env) == "1"
}

// Skip skips t unless live Gemma tests were explicitly opted in.
func Skip(t testing.TB) {
	t.Helper()
	if !Enabled() {
		t.Skip("live Gemma test: set " + Env + "=1 (plus GEMMA_BIN and GEMMA_MODEL) to run")
	}
}
