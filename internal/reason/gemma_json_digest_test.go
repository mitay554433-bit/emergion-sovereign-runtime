package reason

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	want := "Source summary: locked true; frames sampled 14; wave states 14; " +
		"compressed states 6; compression ratio 0.42857; " +
		"state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED."
	if got != want {
		t.Fatalf("digest =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "compressed_wave") || strings.Contains(got, "acronyms") {
		t.Fatalf("digest included array or object: %q", got)
	}
}

func TestSourceDigestBoundsValuesAndTotalLengthWithoutPartialEntries(t *testing.T) {
	fits := strings.Repeat("x", 70)
	var b strings.Builder
	b.WriteString(`{"too_long": "` + strings.Repeat("y", sourceDigestValueLimit+1) + `"`)
	for i := 0; i < 20; i++ {
		b.WriteString(`, "k` + string(rune('a'+i)) + `": "` + fits + `"`)
	}
	b.WriteString("}")

	got := sourceDigest(b.String())
	if len(got) > sourceDigestLimit {
		t.Fatalf("digest length = %d want <= %d", len(got), sourceDigestLimit)
	}
	if strings.Contains(got, "yyyy") {
		t.Fatalf("over-long value was cut instead of skipped: %q", got)
	}
	if !strings.HasPrefix(got, "Source summary: ka "+fits+"; kb "+fits+"; ") || !strings.HasSuffix(got, fits+".") {
		t.Fatalf("digest = %q", got)
	}
	for _, entry := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(got, "Source summary: "), "."), "; ") {
		if len(entry) != len("ka ")+len(fits) {
			t.Fatalf("partial entry %q in %q", entry, got)
		}
	}
}

func TestSourceDigestFactRestatementIsGrounded(t *testing.T) {
	source := `{"frames_sampled": 1, "wave_states": 1, "compressed_states": 1, "compression_ratio": 1.0}`
	content := withSourceDigest(source)
	for _, fact := range []string{
		"1 frames sampled, 1 wave states",
		"frames sampled 1; wave states 1; compressed states 1; compression ratio 1.0",
	} {
		if !sourceSupportsFact(content, fact) {
			t.Fatalf("fact %q not grounded in %q", fact, content)
		}
	}
}

func TestRejectsSummaryThatRestatesSourceName(t *testing.T) {
	r := Result{Summary: "SOURCE_NAME:c5050781dc3f84f0_real_temporal_wave.json", Risk: "L", Facts: []string{"frames sampled 1"}}
	if err := rejectPromptPlaceholders(r, "Source summary: frames sampled 1."); err == nil {
		t.Fatal("expected rejection of file-name summary")
	}
}

func TestBuildPromptAsksForSummaryNotFileName(t *testing.T) {
	if !strings.Contains(buildPrompt("x.json", "c", "s"), "S states what SOURCE shows in one sentence, not the file name.\n") {
		t.Fatal("prompt missing summary line")
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
	want := "Source summary: frames sampled 14.\n\n" + source
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
	if len(result.Facts) != 2 || !strings.Contains(result.Facts[0], "14 frames sampled") || result.Facts[1] != "compression ratio 0.42857" {
		t.Fatalf("facts = %#v", result.Facts)
	}

	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "BEGIN_SOURCE\nSource summary: locked true; frames sampled 14; wave states 14; compressed states 6; compression ratio 0.42857;") {
		t.Fatalf("prompt missing digest:\n%s", prompt)
	}
	if !strings.Contains(string(prompt), "state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED.\n\n{") {
		t.Fatalf("digest not followed by blank line and original content:\n%s", prompt)
	}
}

func TestSourceDigestSkipsIdentifierLikeValues(t *testing.T) {
	source := `{
  "id": "MAXIOM_REAL_TEMPORAL_c5050781dc3f84f0",
  "ts": "2026-11-17T13:33:11.916351+00:00",
  "state": "MAXIOM_REAL_TEMPORAL_WAVE_LOCKED",
  "source": "/storage/emulated/0/ECOPIVOT/VISION/MAXIOM/INBOX/79799fccd2b44d5c_MAXIOM_AXIOM_SEMANTIC_WAVE_FIELD.mp4",
  "source_hash": "c5050781dc3f84f0706bc4ee4c48f8133333d46f7c4b6546dfc4ac3542a56fb4",
  "frames_sampled": 1,
  "wave_states": 1,
  "compressed_states": 1,
  "compression_ratio": 1.0,
  "label": "x 0123456789abcdef0123",
  "acronyms": {"MV": "Motion Vector"},
  "compressed_wave": [{"t": 0}]
}`
	got := sourceDigest(source)
	want := "Source summary: frames sampled 1; wave states 1; compressed states 1; " +
		"compression ratio 1.0; state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED."
	if got != want {
		t.Fatalf("digest =\n%q\nwant\n%q", got, want)
	}
}

func TestGemmaOutputTailDropsBannerAndKeepsTail(t *testing.T) {
	banner := "Loading model...\n\n\u2584\u2584 \u2584\u2584\n\u2588\u2588 \u2588\u2588\n    \u2580\u2580\n"
	got := gemmaOutputTail(banner+"S|wave\nK|L\nF|1 frame", 1000)
	if got != "S|wave\nK|L\nF|1 frame" {
		t.Fatalf("tail = %q", got)
	}
	long := strings.Repeat("\u00e9", 50) + "END"
	got = gemmaOutputTail(long, 11)
	if !strings.HasSuffix(got, "END") || !utf8.ValidString(got) || len(got) > 3+11 {
		t.Fatalf("tail = %q", got)
	}
}

func TestAnalyzeInvalidOutputErrorShowsGeneratedTail(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "llama-completion")
	script := "#!/bin/sh\nprintf 'Loading model...\\n\\n\u2584\u2584 \u2584\u2584\\n\u2588\u2588 \u2588\u2588\\n'\n" +
		"printf 'S|MAXIOM wave\\nK|L\\nF|one frame sampled and still going'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	_, err := g.Analyze(context.Background(), Input{Name: "w.json", Content: []byte(`{"frames_sampled": 1}`)})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "F|one frame sampled and still going") {
		t.Fatalf("error hides generated output: %s", msg)
	}
	if strings.Contains(msg, "\u2588") || strings.Contains(msg, "Loading model") {
		t.Fatalf("error still shows banner: %s", msg)
	}
	if !strings.Contains(msg, "GEMMA_MAX_TOKENS=80 may be too low") {
		t.Fatalf("error missing stop-before-Z hint: %s", msg)
	}
}

func oneFrameWaveSourceForTest() string {
	return `{
  "id": "MAXIOM_REAL_TEMPORAL_c5050781dc3f84f0",
  "ts": "2026-11-17T13:33:11.916351+00:00",
  "state": "MAXIOM_REAL_TEMPORAL_WAVE_LOCKED",
  "frames_sampled": 1,
  "wave_states": 1,
  "compressed_states": 1,
  "compression_ratio": 1.0
}
`
}

func TestNumberTokens(t *testing.T) {
	got := strings.Join(numberTokens("14 frames, ratio 0.42857, 1.0 at 2026-11-17T13:33 end."), ",")
	if got != "14,0.42857,1.0,2026,11,17,13,33" {
		t.Fatalf("tokens = %s", got)
	}
}

func TestNumericGroundingRejectsGovernedStateNumbers(t *testing.T) {
	source := withSourceDigest(oneFrameWaveSourceForTest())
	bad := "The source shows 14 frames sampled, 1 wave state, 6 compressed states, and a compression ratio of 1.0."
	r := Result{Summary: bad, Risk: "L", Facts: []string{bad}}
	err := rejectPromptPlaceholders(r, source)
	if err == nil || !strings.Contains(err.Error(), "number 14 not in SOURCE") {
		t.Fatalf("err = %v", err)
	}

	good := "The source shows 1 frames sampled, 1 wave states, 1 compressed states, and a compression ratio of 1.0."
	r = Result{Summary: good, Risk: "L", Facts: []string{"1 frames sampled, 1 wave states, 1 compressed states (ratio 1)"}}
	if err := rejectPromptPlaceholders(r, source); err != nil {
		t.Fatalf("grounded numbers rejected: %v", err)
	}
}

func TestNumericGroundingAcceptsFourteenFrameWave(t *testing.T) {
	source := withSourceDigest(waveSourceForDigestTest())
	if len(source) > 1000 {
		source = truncateUTF8(source, 1000)
	}
	fact := "14 frames sampled, 14 wave states, 6 compressed states (ratio 0.42857)"
	r := Result{Summary: "MAXIOM real temporal wave: " + fact + ".", Risk: "L", Facts: []string{fact}}
	if err := rejectPromptPlaceholders(r, source); err != nil {
		t.Fatalf("grounded wave rejected: %v", err)
	}
}

func TestAnalyzeRegeneratesWhenNumbersComeFromGovernedState(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	count := filepath.Join(root, "count")
	binary := filepath.Join(root, "llama-completion")
	script := `#!/bin/sh
if [ -f "` + count + `" ]; then
cat <<'OUT'
S|MAXIOM real temporal wave with 1 frames sampled and 1 wave states
K|L
F|1 frames sampled, 1 wave states, 1 compressed states, ratio 1.0
C|VLD
Z
OUT
else
touch "` + count + `"
cat <<'OUT'
S|The source shows 14 frames sampled and 6 compressed states
K|L
F|14 frames sampled, 1 wave state, 6 compressed states, ratio 1.0
C|VLD
Z
OUT
fi
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	res, err := g.Analyze(context.Background(), Input{
		Name:          "c5050781-wave-v3.json",
		Content:       []byte(oneFrameWaveSourceForTest()),
		GovernedState: "ACCEPTED_FACT: 14 frames sampled, 14 wave states, 6 compressed states (ratio 0.42857)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Facts) != 2 || strings.Contains(res.Facts[0], "14") || res.Facts[1] != "compression ratio 1.0" {
		t.Fatalf("facts = %#v", res.Facts)
	}
}

func TestBuildPromptKeepsNumbersInSource(t *testing.T) {
	if !strings.Contains(buildPrompt("x.json", "c", "s"), "Numbers in S and F must come from SOURCE, never from GOVERNED_STATE.\n") {
		t.Fatal("prompt missing numeric grounding line")
	}
}

func TestMaskStandaloneNumbers(t *testing.T) {
	got := maskStandaloneNumbers("E-9AE69D1AA444275B ACCEPTED_FACT: 14 frames sampled, 6 compressed states (ratio 0.42857) 2a9")
	want := "E-9AE69D1AA444275B ACCEPTED_FACT: # frames sampled, # compressed states (ratio #) 2a9"
	if got != want {
		t.Fatalf("masked = %q want %q", got, want)
	}
}

func analyzeWithPromptCapture(t *testing.T, name, source, governed string) string {
	t.Helper()
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
S|Observed source record
K|L
F|observed source record
C|OBS
Z
OUT
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	_, _ = g.Analyze(context.Background(), Input{Name: name, Content: []byte(source), GovernedState: governed})
	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatal(err)
	}
	p := string(prompt)
	i := strings.Index(p, "GOVERNED_STATE is comparison context only:\n")
	if i < 0 {
		t.Fatalf("prompt missing GOVERNED_STATE:\n%s", p)
	}
	return p[i:]
}

const governedWithWaveNumbers = "E-9AE69D1AA444275B ACCEPTED_FACT: 14 frames sampled, 14 wave states, 6 compressed states (ratio 0.42857)"

func TestJSONDigestSourceMasksGovernedNumbersInPromptOnly(t *testing.T) {
	gov := analyzeWithPromptCapture(t, "w.json", oneFrameWaveSourceForTest(), governedWithWaveNumbers)
	for _, n := range []string{"14", "0.42857", " 6 "} {
		if strings.Contains(gov, n) {
			t.Fatalf("prompt GOVERNED_STATE still has %q:\n%s", n, gov)
		}
	}
	if !strings.Contains(gov, "E-9AE69D1AA444275B ACCEPTED_FACT: # frames sampled") {
		t.Fatalf("prompt GOVERNED_STATE = %s", gov)
	}
}

func TestPlainTextSourceKeepsGovernedNumbers(t *testing.T) {
	gov := analyzeWithPromptCapture(t, "note.txt", "observed source record with 14 frames", governedWithWaveNumbers)
	if !strings.Contains(gov, governedWithWaveNumbers) {
		t.Fatalf("plain-text prompt GOVERNED_STATE changed:\n%s", gov)
	}
}

func TestTargetComparisonKeepsGovernedNumbers(t *testing.T) {
	source := "TARGET_COMPARISON/1\nTARGET_STATE:\n{\"frames_sampled\": 14}\nREALITY:\nobserved source record\nZ\n"
	gov := analyzeWithPromptCapture(t, "cmp.txt", source, governedWithWaveNumbers)
	if !strings.Contains(gov, governedWithWaveNumbers) {
		t.Fatalf("TARGET_COMPARISON prompt GOVERNED_STATE changed:\n%s", gov)
	}
}

func TestRetryPromptNamesRejectedNumber(t *testing.T) {
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
S|The source shows 14 frames sampled
K|L
F|14 frames sampled
C|VLD
Z
OUT
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	_, err := g.Analyze(context.Background(), Input{Name: "w.json", Content: []byte(oneFrameWaveSourceForTest()), GovernedState: governedWithWaveNumbers})
	if err == nil || !strings.Contains(err.Error(), "number 14 not in SOURCE") {
		t.Fatalf("err = %v", err)
	}
	prompt, _ := os.ReadFile(promptFile)
	if !strings.Contains(string(prompt), "Previous answer used 14, which is not in SOURCE.") {
		t.Fatalf("retry prompt missing rejected number:\n%s", prompt)
	}
}

const oneFrameDigest = "Source summary: frames sampled 1; wave states 1; compressed states 1; compression ratio 1.0; state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED."

func TestDigestNumericEntries(t *testing.T) {
	got := strings.Join(digestNumericEntries(oneFrameDigest), "|")
	if got != "frames sampled 1|wave states 1|compressed states 1|compression ratio 1.0" {
		t.Fatalf("entries = %q", got)
	}
	if sourceDigest(oneFrameWaveSourceForTest()) != oneFrameDigest {
		t.Fatalf("digest = %q", sourceDigest(oneFrameWaveSourceForTest()))
	}
}

func TestWithDigestFactsAppendsMissingNumericEntries(t *testing.T) {
	r := Result{Summary: "The source shows a single frame sampled from a wave field.", Risk: "L", Facts: []string{"1 frame sampled"}}
	got := withDigestFacts(r, oneFrameDigest).Facts
	want := []string{"1 frame sampled", "wave states 1", "compressed states 1", "compression ratio 1.0"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("facts = %#v want %#v", got, want)
	}
	source := withSourceDigest(oneFrameWaveSourceForTest())
	if err := rejectPromptPlaceholders(Result{Summary: r.Summary, Risk: "L", Facts: got}, source); err != nil {
		t.Fatalf("appended facts not grounded: %v", err)
	}
}

func TestWithDigestFactsKeepsCompleteFactsUnchanged(t *testing.T) {
	r := Result{Facts: []string{"1 frame sampled, 1 wave state, 1 compressed state, compression ratio 1"}}
	if got := withDigestFacts(r, oneFrameDigest).Facts; len(got) != 1 {
		t.Fatalf("facts = %#v", got)
	}
}

func TestWithDigestFactsRespectsFactLimit(t *testing.T) {
	var facts []string
	for i := 0; i < 23; i++ {
		facts = append(facts, "observed item "+strings.Repeat("a", i+1))
	}
	if got := withDigestFacts(Result{Facts: facts}, oneFrameDigest).Facts; len(got) != 24 || got[23] != "frames sampled 1" {
		t.Fatalf("facts = %#v", got)
	}
}

func TestWithDigestFactsNoDigestNoChange(t *testing.T) {
	r := Result{Facts: []string{"1 frame sampled"}}
	if got := withDigestFacts(r, "").Facts; len(got) != 1 {
		t.Fatalf("facts = %#v", got)
	}
}

func TestDigestFactLineOnlyForJSONDigestSources(t *testing.T) {
	gov := "accepted_context_present"
	plain := buildPrompt("note.txt", "observed source record", gov)
	if withDigestFactLine(plain, "") != plain {
		t.Fatal("plain-text prompt changed")
	}
	withLine := withDigestFactLine(plain, oneFrameDigest)
	if !strings.Contains(withLine, digestFactLine+"Z = final terminator.\n") {
		t.Fatalf("fact line missing:\n%s", withLine)
	}
}

func TestAnalyzePlainTextFactsUnchanged(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "llama-completion")
	script := "#!/bin/sh\nprintf 'S|Observed source record\\nK|L\\nF|observed source record\\nC|OBS\\nZ\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := GemmaCLI{Binary: binary, Model: model, Context: 2048, MaxTokens: 80, Threads: 1, Timeout: 5 * time.Second}
	res, err := g.Analyze(context.Background(), Input{Name: "note.txt", Content: []byte("observed source record with frames sampled 1")})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Facts) != 1 {
		t.Fatalf("facts = %#v", res.Facts)
	}
}

func TestWithDigestFactsSplitsCopiedSummaryList(t *testing.T) {
	source := withSourceDigest(oneFrameWaveSourceForTest())
	for _, tc := range []struct {
		fact string
		want []string
	}{
		{ // phone record E-D849885EE72B3510, after the 96-byte cut
			"frames sampled 1; wave states 1; compressed states 1; compression ratio 1.0; state MAXIOM_REAL_T",
			[]string{"frames sampled 1", "wave states 1", "compressed states 1", "compression ratio 1.0"},
		},
		{ // as Gemma emits it, before any cut
			"frames sampled 1; wave states 1; compressed states 1; compression ratio 1.0; state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED",
			[]string{"frames sampled 1", "wave states 1", "compressed states 1", "compression ratio 1.0", "state MAXIOM_REAL_TEMPORAL_WAVE_LOCKED"},
		},
		{
			"Source summary: frames sampled 1; wave states 1.",
			[]string{"frames sampled 1", "wave states 1", "compressed states 1", "compression ratio 1.0"},
		},
		{ // cut-off number is dropped, then the whole entry is appended
			"frames sampled 1; compression ratio 1",
			[]string{"frames sampled 1", "wave states 1", "compressed states 1", "compression ratio 1.0"},
		},
	} {
		r := Result{Summary: "The source shows a single frame sampled from a wave field.", Risk: "L", Facts: []string{tc.fact}}
		got := withDigestFacts(r, oneFrameDigest).Facts
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Fatalf("fact %q:\nfacts = %#v\nwant  %#v", tc.fact, got, tc.want)
		}
		for _, f := range got {
			if len(f) > 96 {
				t.Fatalf("fact over 96 bytes: %q", f)
			}
		}
		if err := rejectPromptPlaceholders(Result{Summary: r.Summary, Risk: "L", Facts: got}, source); err != nil {
			t.Fatalf("split facts not grounded: %v", err)
		}
	}
}

func TestSplitDigestListFactsKeepsOtherFactsAndDedupes(t *testing.T) {
	got := splitDigestListFacts([]string{"1 frame sampled", "wave states 1; locked wave", "wave states 1"}, oneFrameDigest)
	want := []string{"1 frame sampled", "wave states 1", "locked wave"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("facts = %#v want %#v", got, want)
	}
}
