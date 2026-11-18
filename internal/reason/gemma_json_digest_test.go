package reason

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waveSourceForDigestTest() string {
	var frames strings.Builder
	for i := 0; i < 40; i++ {
		if i > 0 {
			frames.WriteString(",")
		}
		frames.WriteString(`{"t": 0, "acr": "VPS", "MV": 0.02345, "SP": 0.1248}`)
	}
	return `{
  "state": "MAXIOM_REAL_TEMPORAL_WAVE_LOCKED",
  "id": "MAXIOM_REAL_TEMPORAL_79799fccd2b44d5c",
  "acronyms": {"MV": "Motion Vector"},
  "compressed_wave": [` + frames.String() + `],
  "locked": true,
  "frames_sampled": 14,
  "wave_states": 14,
  "compressed_states": 6,
  "compression_ratio": 0.42857
}`
}

func TestSourceDigestListsSortedTopLevelScalarsAndSkipsArraysAndObjects(t *testing.T) {
	got := sourceDigest(waveSourceForDigestTest())
	want := "SOURCE_DIGEST: compressed_states=6; compression_ratio=0.42857; " +
		"frames_sampled=14; id=MAXIOM_REAL_TEMPORAL_79799fccd2b44d5c; locked=true; " +
		"state=MAXIOM_REAL_TEMPORAL_WAVE_LOCKED; wave_states=14"
	if got != want {
		t.Fatalf("digest =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "compressed_wave") || strings.Contains(got, "acronyms") {
		t.Fatalf("digest included array or object: %q", got)
	}
}

func TestSourceDigestBoundsValuesAndTotalLength(t *testing.T) {
	long := strings.Repeat("x", 200)
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k` + string(rune('a'+i)) + `": "` + long + `"`)
	}
	b.WriteString("}")

	got := sourceDigest(b.String())
	if got == "" {
		t.Fatal("expected digest")
	}
	if len(got) > sourceDigestLimit {
		t.Fatalf("digest length = %d want <= %d", len(got), sourceDigestLimit)
	}
	if !strings.HasPrefix(got, "SOURCE_DIGEST: ka="+strings.Repeat("x", sourceDigestValueLimit)+"; kb=") {
		t.Fatalf("value not truncated to %d bytes: %q", sourceDigestValueLimit, got)
	}
}

func TestSourceDigestOmittedForNonJSONAndTargetComparison(t *testing.T) {
	for _, source := range []string{
		"plain text evidence with frames_sampled: 14",
		`["array", "not", "object"]`,
		`{"broken": `,
		`{"a": 1} trailing`,
		"TARGET_COMPARISON/1\nTARGET_STATE:\n{\"a\": 1}\nREALITY:\naccepted\nZ\n",
		`{"only": ["arrays"], "and": {"objects": 1}}`,
	} {
		if got := sourceDigest(source); got != "" {
			t.Fatalf("source %q produced digest %q", source, got)
		}
		if got := withSourceDigest(source); got != source {
			t.Fatalf("source %q changed to %q", source, got)
		}
	}
}

func TestWithSourceDigestPrependsDigestAndBlankLine(t *testing.T) {
	source := `{"frames_sampled": 14, "frames": [1, 2]}`
	got := withSourceDigest(source)
	want := "SOURCE_DIGEST: frames_sampled=14\n\n" + source
	if got != want {
		t.Fatalf("content = %q want %q", got, want)
	}
}

func TestGemmaFromEnvDefaultTimeoutIs360Seconds(t *testing.T) {
	t.Setenv("GEMMA_TIMEOUT_SECONDS", "")
	if got := GemmaFromEnv().Timeout; got != 360*time.Second {
		t.Fatalf("default timeout = %s want 6m0s", got)
	}
	t.Setenv("GEMMA_TIMEOUT_SECONDS", "90")
	if got := GemmaFromEnv().Timeout; got != 90*time.Second {
		t.Fatalf("override timeout = %s want 1m30s", got)
	}
}

func TestBuildPromptCapabilityLineMatchesRequiredGrammar(t *testing.T) {
	p := buildPrompt("x.json", "content", "state")
	if !strings.Contains(p, "C = the single capability SOURCE most directly supports.\n") {
		t.Fatal("prompt missing required capability line")
	}
	if strings.Contains(p, "C = a reusable capability only when SOURCE explicitly establishes one.") {
		t.Fatal("prompt still describes capability as optional")
	}
	if !strings.Contains(p, "For optional D, R, L, or T fields, omission means SOURCE did not establish that field.") {
		t.Fatal("prompt optional-field list not updated")
	}
	if strings.Contains(p, "D, R, C, L, or T") {
		t.Fatal("prompt still lists C as optional")
	}
}

func TestAnalyzeSendsDigestToGemmaWithoutChangingEvidence(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	promptFile := filepath.Join(root, "prompt.txt")
	binary := filepath.Join(root, "llama-completion")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
case "$1" in
-sys)
shift
printf '%s' "$1" > "` + promptFile + `"
;;
esac
shift
done
cat <<'OUT'
S|MAXIOM real temporal wave with 14 frames sampled
K|L
F|14 frames sampled, 14 wave states, 6 compressed states
C|VLD
T|ANALYTICS_FORECAST
Z
OUT
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	source := waveSourceForDigestTest()
	if len(source) <= 1000 || strings.Index(source, `"frames_sampled"`) < 1000 {
		t.Fatal("test source must place counts beyond the 1000-byte prompt window")
	}
	evidence := []byte(source)
	original := append([]byte(nil), evidence...)

	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	result, err := g.Analyze(context.Background(), Input{
		Name:          "79799fccd2b44d5c_real_temporal_wave.json",
		Content:       evidence,
		GovernedState: "accepted_context_present",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(evidence, original) {
		t.Fatal("evidence bytes changed")
	}
	if len(result.Facts) != 1 || !strings.Contains(result.Facts[0], "14 frames sampled") {
		t.Fatalf("facts = %#v", result.Facts)
	}

	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "SOURCE:\nSOURCE_DIGEST: compressed_states=6; compression_ratio=0.42857; frames_sampled=14;") {
		t.Fatalf("prompt missing digest:\n%s", prompt)
	}
	if !strings.Contains(string(prompt), "wave_states=14\n\n{") {
		t.Fatalf("digest not followed by blank line and original content:\n%s", prompt)
	}
}
