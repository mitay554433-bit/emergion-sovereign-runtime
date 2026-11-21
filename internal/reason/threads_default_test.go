package reason

import (
	"runtime"
	"strings"
	"testing"
)

// The kept-loaded llama-server starts with 4 threads unless GEMMA_THREADS
// says otherwise (phones: 4 fast cores; -t 8 waits on the slow ones).
func TestLlamaServerDefaultsToFourThreadsWithEnvOverride(t *testing.T) {
	t.Setenv("GEMMA_THREADS", "")
	t.Setenv("GEMMA_MODEL", "/x/gemma.gguf")
	want := "4"
	if runtime.NumCPU() < 4 {
		want = strings.TrimSpace(string(rune('0' + runtime.NumCPU())))
	}
	s := ServerInsighterFromEnv()
	if a := strings.Join(s.ServerArgs(), " "); !strings.Contains(a, "-t "+want+" ") {
		t.Fatalf("default server args %q want -t %s", a, want)
	}
	if a := strings.Join((&ServerInsighter{Exec: LocalInsighter{Model: "m"}}).ServerArgs(), " "); !strings.Contains(a, "-t "+want+" ") {
		t.Fatalf("zero Threads must use the default: %q", a)
	}
	if a := strings.Join((LocalInsighter{Binary: "llama-completion", Model: "m"}).Args("p"), " "); !strings.Contains(a, "-t "+want+" ") {
		t.Fatalf("one-shot default: %q", a)
	}
	t.Setenv("GEMMA_THREADS", "6")
	if a := strings.Join(ServerInsighterFromEnv().ServerArgs(), " "); !strings.Contains(a, "-t 6 ") {
		t.Fatalf("GEMMA_THREADS override lost: %q", a)
	}
	t.Setenv("GEMMA_THREADS", "0")
	if LocalThreads() != DefaultLocalThreads && runtime.NumCPU() >= 4 {
		t.Fatal("non-positive override falls back to the default")
	}
}

func TestPromptCharsDefaultIs1200(t *testing.T) {
	t.Setenv("UNIFUSION_LLAMA_PROMPT_CHARS", "")
	if DefaultLocalPromptChars != 1200 || LocalPromptChars() != 1200 {
		t.Fatalf("default %d", LocalPromptChars())
	}
	t.Setenv("UNIFUSION_LLAMA_PROMPT_CHARS", "2400")
	if LocalPromptChars() != 2400 {
		t.Fatal("env override lost")
	}
	if n := len(LocalPrompt(CapLocalInput(bigInput(), LocalPromptChars()))); n > 2400 {
		t.Fatalf("prompt %d", n)
	}
}
