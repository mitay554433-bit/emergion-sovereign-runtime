package reason

import (
	"strings"
	"testing"
)

func TestAnalyzeBudgetFitsWorstCaseTokens(t *testing.T) {
	tmpl := len(buildPrompt("x", "", ""))
	gov, content, err := analyzeBudget(4096, 256, tmpl, 2)
	if err != nil {
		t.Fatal(err)
	}
	// At the assumed 2 bytes/token, prompt + reserve + generation must fit.
	tokens := (tmpl+gov+content+1)/2 + chatTemplateReserve + 256
	if tokens > 4096 {
		t.Fatalf("budget overflows context: %d tokens", tokens)
	}
	if content > maxContentBytes || gov <= 0 {
		t.Fatalf("bad limits gov=%d content=%d", gov, content)
	}
}

func TestAnalyzeBudgetRejectsTinyContext(t *testing.T) {
	if _, _, err := analyzeBudget(1024, 512, 2000, 2); err == nil {
		t.Fatal("expected error for tiny context")
	}
}

func TestTruncateAtLineKeepsWholeRecords(t *testing.T) {
	s := "R1|aaaa\nR2|bbbb\nR3|cccc"
	got := truncateAtLine(s, 12)
	if got != "R1|aaaa" || strings.Contains(got, "R2|b") {
		t.Fatalf("got %q", got)
	}
}
