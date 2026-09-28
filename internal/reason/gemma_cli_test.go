package reason

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanFacets(t *testing.T) {
	got := cleanFacets([]string{"PROGRAM_FORGE", "INVALID", "PROGRAM_FORGE", "GRANT_FUNDING"})
	if len(got) != 2 || got[0] != "PROGRAM_FORGE" || got[1] != "GRANT_FUNDING" {
		t.Fatalf("bad facets: %#v", got)
	}
}

func TestCleanBuildGraph(t *testing.T) {
	nodes, edges := cleanBuildGraph(
		[]BuildNode{{ID: " source ", System: " SOURCE "}, {ID: "source", System: "duplicate"}, {ID: "target", System: "REG"}},
		[]BuildEdge{{From: "source", To: "target", Kind: "builds"}, {From: "missing", To: "target", Kind: "invalid"}},
	)
	if len(nodes) != 2 || len(edges) != 1 || nodes[0].ID != "source" {
		t.Fatalf("bad graph: %#v %#v", nodes, edges)
	}
}

func TestGemmaArgsEnforceOneShotExecution(t *testing.T) {
	g := GemmaCLI{
		Model:     "/models/gemma.gguf",
		MaxTokens: 128,
		Context:   2048,
		Threads:   4,

		// Deliberately hostile configuration. The runtime invariant
		// must override this by appearing later in argv.
		ExtraArgs: []string{"--conversation"},
	}

	args := gemmaArgs(g, "one prompt")

	lastIndex := func(value string) int {
		index := -1
		for i, arg := range args {
			if arg == value {
				index = i
			}
		}
		return index
	}

	required := []string{
		"--log-disable",
		"--color",
		"--single-turn",
		"--no-display-prompt",
	}

	for _, arg := range required {
		if lastIndex(arg) < 0 {
			t.Fatalf("missing execution invariant %q: %#v", arg, args)
		}
	}

}

func TestCalibrateNormalizesNullSupersedes(t *testing.T) {
	got := Calibrate(Result{
		Summary:       "bounded",
		Relationships: map[string]string{"source": "probe"},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"observed"},
		Risk:          "L",
		Supersedes:    "null",
	})

	if got.Supersedes != "" {
		t.Fatalf("null supersedes survived calibration: %q", got.Supersedes)
	}
}

func TestArchonymPrimitiveRoundTrip(t *testing.T) {
	want := Result{
		Summary:  "bounded semantic identity",
		Archonym: "VERITEX CORE",
		Risk:     "L",
	}

	wire := FormatResult(want)

	got, err := parseResult(wire)
	if err != nil {
		t.Fatal(err)
	}

	if got.Archonym != want.Archonym {
		t.Fatalf(
			"Archonym roundtrip = %q want %q",
			got.Archonym,
			want.Archonym,
		)
	}

	calibrated := Calibrate(Result{
		Summary:  "bounded",
		Archonym: "  VERITEX CORE  ",
		Risk:     "L",
	})
	if calibrated.Archonym != "VERITEX CORE" {
		t.Fatalf("calibrated Archonym = %q", calibrated.Archonym)
	}
}

func TestCalibrateNormalizesRelationshipsWithoutChangingIdentity(t *testing.T) {
	longValue := strings.Repeat("accepted-context-", 20)

	got := Calibrate(Result{
		Summary: "bounded",
		Risk:    "L",
		Relationships: map[string]string{
			" source ":       " probe ",
			"empty-value":    "   ",
			"   ":            "discard",
			" relationship ": " " + longValue + " ",
		},
	})

	if got.Relationships["source"] != "probe" {
		t.Fatalf("normalized source relationship = %q", got.Relationships["source"])
	}

	if got.Relationships["relationship"] != longValue {
		t.Fatalf(
			"long relationship identity changed: got length %d want %d",
			len(got.Relationships["relationship"]),
			len(longValue),
		)
	}

	if _, ok := got.Relationships["empty-value"]; ok {
		t.Fatal("empty relationship value survived calibration")
	}

	if _, ok := got.Relationships[""]; ok {
		t.Fatal("empty relationship key survived calibration")
	}
}

func TestCalibrateRelationshipTrimCollisionIsDeterministic(t *testing.T) {
	got := Calibrate(Result{
		Summary: "bounded",
		Risk:    "L",
		Relationships: map[string]string{
			" composition ": "first",
			"composition":   "second",
		},
	})

	if len(got.Relationships) != 1 {
		t.Fatalf("trim collision produced %d relationships", len(got.Relationships))
	}

	if got.Relationships["composition"] != "first" {
		t.Fatalf(
			"deterministic trim collision = %q want first",
			got.Relationships["composition"],
		)
	}
}

func TestGemmaCLIAnalyzeInstalledRuntime(t *testing.T) {
	binary := os.Getenv("GEMMA_BIN")
	model := os.Getenv("GEMMA_MODEL")

	if binary == "" || model == "" {
		t.Skip("set GEMMA_BIN and GEMMA_MODEL for installed-runtime integration test")
	}

	g := GemmaCLI{
		Binary:    binary,
		Model:     model,
		Threads:   4,
		Context:   2048,
		MaxTokens: 80,
		Timeout:   180 * time.Second,
	}

	got, err := g.Analyze(context.Background(), Input{
		Name:    "gemma-integration-probe",
		Content: []byte("The local inference execution probe confirms that Gemma inference executes through llama-cli."),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Summary == "" {
		t.Fatal("Gemma Analyze returned empty summary")
	}

	if got.Risk != "L" && got.Risk != "M" && got.Risk != "H" {
		t.Fatalf("Gemma Analyze returned invalid risk %q", got.Risk)
	}
}

func TestBuildPromptDiagnostic(t *testing.T) {
	source, err := os.ReadFile("../../docs/SYSTEM_STATUS.md")
	if err != nil {
		t.Fatal(err)
	}

	governedState := strings.Repeat("G", 1723)

	prompt := buildPrompt(
		"SYSTEM_STATUS.md",
		string(source),
		governedState,
	)

	t.Logf(
		"source_bytes=%d governed_state_bytes=%d final_prompt_bytes=%d",
		len(source),
		len(governedState),
		len(prompt),
	)
}

func TestRejectPromptPlaceholders(t *testing.T) {
	tests := []struct {
		name   string
		result Result
	}{
		{
			name: "summary",
			result: Result{
				Summary: "one sentence summary",
				Risk:    "L",
			},
		},
		{
			name: "fact",
			result: Result{
				Summary: "real summary",
				Risk:    "L",
				Facts:   []string{"one evidenced fact"},
			},
		},
		{
			name: "capability",
			result: Result{
				Summary:      "real summary",
				Risk:         "L",
				Capabilities: []string{"one transferable capability"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := rejectPromptPlaceholders(tt.result, "source evidence for test"); err == nil {
				t.Fatal("prompt placeholder was accepted")
			}
		})
	}
}

func TestRejectPromptPlaceholdersAllowsRealSemanticResult(t *testing.T) {
	result := Result{
		Summary:      "governed runtime verifies preserved source evidence",
		Risk:         "L",
		Facts:        []string{"COSL chain verification is implemented"},
		Capabilities: []string{"verify governed execution evidence"},
	}

	if err := rejectPromptPlaceholders(result, "COSL chain verification is implemented and governed execution evidence can be verified"); err != nil {
		t.Fatalf("real semantic result rejected: %v", err)
	}
}

func TestRejectPromptPlaceholdersRejectsSummaryFieldLabel(t *testing.T) {
	for _, summary := range []string{
		"F: The full test suite passes.",
		"F,C: The full test suite passes.",
		"C,F: The full test suite passes.",
		"S,F: The full test suite passes.",
		"F C",
		"C F",
		"S F",
		"S C",
		"F C S",

		"S,C: The full test suite passes.",
	} {
		result := Result{
			Summary:      summary,
			Risk:         "M",
			Facts:        []string{"The full test suite passes."},
			Capabilities: []string{"verify runtime integrity"},
		}

		if err := rejectPromptPlaceholders(
			result,
			"The full test suite passes and runtime integrity can be verified.",
		); err == nil {
			t.Fatalf("summary field label %q was accepted", summary)
		}
	}
}

func TestRejectPromptPlaceholdersRejectsCapabilityFactCollision(t *testing.T) {
	result := Result{
		Summary:      "runtime verification succeeds",
		Risk:         "L",
		Facts:        []string{"The full test suite passes."},
		Capabilities: []string{"  the full test suite passes.  "},
	}

	if err := rejectPromptPlaceholders(result, "The full test suite passes."); err == nil {
		t.Fatal("capability identical to fact was accepted")
	}
}

func TestRejectPromptPlaceholdersAllowsDistinctFactAndCapability(t *testing.T) {
	result := Result{
		Summary:      "runtime verification and evidence controls are implemented",
		Risk:         "L",
		Facts:        []string{"COSL event hashes and previous-hash chain verify."},
		Capabilities: []string{"verify COSL hash-chain integrity"},
	}

	if err := rejectPromptPlaceholders(result, "COSL event hashes and previous-hash chain verify. The runtime can verify COSL hash-chain integrity."); err != nil {
		t.Fatalf("distinct semantic result rejected: %v", err)
	}
}

func TestSourceSupportsFactRejectsUnsupportedClaim(t *testing.T) {
	source := "COSL event hashes and previous-hash chain verify."

	if sourceSupportsFact(source, "Payments are automatically transferred to customers.") {
		t.Fatal("unsupported fact was treated as source-supported")
	}
}

func TestSourceSupportsFactAcceptsSupportedClaim(t *testing.T) {
	source := "Go standard-library sovereign runtime compiles and the full test suite passes."

	if !sourceSupportsFact(source, "The sovereign runtime compiles and the full test suite passes.") {
		t.Fatal("source-supported fact was rejected")
	}
}

func TestRejectPromptPlaceholdersRejectsLeadingPunctuation(t *testing.T) {
	tests := []Result{
		{
			Summary:      ": The full test suite passes.",
			Risk:         "M",
			Facts:        []string{"The full test suite passes."},
			Capabilities: []string{"can verify runtime integrity"},
		},
		{
			Summary:      "runtime verification succeeds",
			Risk:         "M",
			Facts:        []string{": The full test suite passes."},
			Capabilities: []string{"can verify runtime integrity"},
		},
	}

	source := "The full test suite passes and runtime integrity can be verified."

	for _, result := range tests {
		if err := rejectPromptPlaceholders(result, source); err == nil {
			t.Fatal("leading punctuation was accepted")
		}
	}
}

func TestRejectPromptPlaceholdersRejectsVacuousCapability(t *testing.T) {
	result := Result{
		Summary:      "runtime verification succeeds",
		Risk:         "M",
		Facts:        []string{"The full test suite passes."},
		Capabilities: []string{"can be repeated."},
	}

	if err := rejectPromptPlaceholders(
		result,
		"The full test suite passes.",
	); err == nil {
		t.Fatal("vacuous capability was accepted")
	}
}

func TestRejectPromptPlaceholdersAllowsConcreteCapabilityForm(t *testing.T) {
	result := Result{
		Summary:      "COSL integrity verification is implemented",
		Risk:         "L",
		Facts:        []string{"COSL event hashes and previous-hash chain verify."},
		Capabilities: []string{"can verify COSL hash-chain integrity"},
	}

	if err := rejectPromptPlaceholders(
		result,
		"COSL event hashes and previous-hash chain verify.",
	); err != nil {
		t.Fatalf("concrete capability rejected: %v", err)
	}
}

func TestMXPDGrammarOwnsKnownBadLeadingLexicalForms(t *testing.T) {
	required := []string{
		`text ::= safe-first text-tail | label-first label-next text-tail`,
		`safe-first ::= [ABDIJOPQRVWXYZabdijopqrvwxyz0-9]`,
		`label-first ::= [CEFGHKLMNSTUcefghklmnstu]`,
		`label-next ::= [^:=,|\r\n]`,
		`text-tail ::= [^|\r\n]*`,
	}

	for _, want := range required {
		if !strings.Contains(mxpdGrammar, want) {
			t.Fatalf("MXPD grammar missing lexical constraint %q", want)
		}
	}

	if strings.Contains(mxpdGrammar, `text ::= [^|\r\n]+`) {
		t.Fatal("MXPD grammar still permits unrestricted leading text characters")
	}
}

func TestMXPDGrammarMakesCapabilityOptionalAndCanonical(t *testing.T) {
	if !strings.Contains(
		mxpdGrammar,
		`root ::= summary risk fact relationship? capability? facet? end`,
	) {
		t.Fatal("MXPD grammar still requires a capability")
	}

	if !strings.Contains(
		mxpdGrammar,
		`capability ::= "C|" capability-token "\n"`,
	) {
		t.Fatal("MXPD capability is not machine-token constrained")
	}

	for _, token := range []string{
		"OBS",
		"CMP",
		"RLT",
		"VLD",
		"REASON",
		"ANALYZE",
		"DRAFT",
		"SIMULATE",
	} {
		if !strings.Contains(mxpdGrammar, `"`+token+`"`) {
			t.Fatalf("canonical capability %q missing from MXPD grammar", token)
		}
	}

	if !strings.Contains(mxpdGrammar, `facet ::= "T|" facet-token "\n"`) {
		t.Fatal("MXPD facet is not machine-token constrained")
	}

	if !strings.Contains(mxpdGrammar, `"PROGRAM_FORGE"`) {
		t.Fatal("canonical PROGRAM_FORGE facet missing from MXPD grammar")
	}

	if strings.Contains(mxpdGrammar, `capability ::= "C|can " text "\n"`) {
		t.Fatal("free-form natural-language capability generation remains enabled")
	}
}

func TestMXPDGrammarAllowsGovernedCompositionRelationship(t *testing.T) {
	if !strings.Contains(
		mxpdGrammar,
		`root ::= summary risk fact relationship? capability? facet? end`,
	) {
		t.Fatal("MXPD grammar does not permit an optional governed relationship")
	}

	if !strings.Contains(
		mxpdGrammar,
		`relationship ::= "L|COMPOSITION_KIN|" text "\n"`,
	) {
		t.Fatal("MXPD grammar does not constrain relationship output to COMPOSITION_KIN")
	}
}

func TestParseResultAcceptsLlamaEndMarker(t *testing.T) {
	want := Result{
		Summary:      "bounded semantic result",
		Risk:         "L",
		Facts:        []string{"verified state"},
		Capabilities: []string{"ANALYZE"},
		Facets:       []string{"ANALYTICS_FORECAST"},
	}

	raw := "S|bounded semantic result\n" +
		"K|L\n" +
		"F|verified state\n" +
		"C|ANALYZE\n" +
		"T|ANALYTICS_FORECAST\n" +
		"Z [end of text]\n"

	got, err := parseResult(raw)
	if err != nil {
		t.Fatalf("parseResult returned error: %v", err)
	}

	if got.Summary != want.Summary {
		t.Fatalf("summary = %q want %q", got.Summary, want.Summary)
	}
	if got.Risk != want.Risk {
		t.Fatalf("risk = %q want %q", got.Risk, want.Risk)
	}
	if len(got.Facts) != 1 || got.Facts[0] != want.Facts[0] {
		t.Fatalf("facts = %#v want %#v", got.Facts, want.Facts)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != want.Capabilities[0] {
		t.Fatalf("capabilities = %#v want %#v", got.Capabilities, want.Capabilities)
	}
	if len(got.Facets) != 1 || got.Facets[0] != want.Facets[0] {
		t.Fatalf("facets = %#v want %#v", got.Facets, want.Facets)
	}
}

func TestAnalyzeStripsLlamaCLITranscriptBeforeParsing(t *testing.T) {
	root := t.TempDir()

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(root, "llama-cli")
	script := `#!/bin/sh
prompt=""
while [ "$#" -gt 0 ]; do
case "$1" in
-p|--prompt)
shift
prompt="$1"
;;
esac
shift
done

printf 'User:\n%s\n\nAssistant:\n' "$prompt"
cat <<'OUT'
S|clean assistant result
K|L
F|clean assistant fact
C|CMP
T|DOCS_PROJECTION
Z
OUT
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	g := GemmaCLI{
		Binary:    binary,
		Model:     model,
		Context:   2048,
		MaxTokens: 80,
		Threads:   1,
		Timeout:   5 * time.Second,
	}

	result, err := g.Analyze(context.Background(), Input{
		Name: "SAW:SAAB:E-A+E-B",
		Content: []byte(
			"clean assistant fact\n" +
				"SAW/1\n" +
				`C|"OBS"` + "\n" +
				`C|"RLT"` + "\n" +
				`L|"E-A"|"E-B"|"COMPOSITION_KIN"` + "\n" +
				"Z\n",
		),
		GovernedState: "accepted_context_present",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Summary != "clean assistant result" {
		t.Fatalf("summary = %q", result.Summary)
	}
	if result.Risk != "L" {
		t.Fatalf("risk = %q want L", result.Risk)
	}
	if len(result.Facts) != 1 || result.Facts[0] != "clean assistant fact" {
		t.Fatalf("facts leaked prompt content: %#v", result.Facts)
	}
	if len(result.Capabilities) != 1 || result.Capabilities[0] != "CMP" {
		t.Fatalf("capabilities leaked prompt content: %#v", result.Capabilities)
	}
	if len(result.Relationships) != 0 {
		t.Fatalf("relationships leaked prompt content: %#v", result.Relationships)
	}
	if len(result.Facets) != 1 || result.Facets[0] != "DOCS_PROJECTION" {
		t.Fatalf("facets = %#v", result.Facets)
	}
}

func TestAnalyzeStripsLiveLlamaCLITranscriptBeforeParsing(t *testing.T) {
	root := t.TempDir()

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(root, "llama-cli")
	script := `#!/bin/sh
prompt=""
while [ "$#" -gt 0 ]; do
case "$1" in
-p|--prompt)
shift
prompt="$1"
;;
esac
shift
done

# Match the live llama-cli transport shape:
# displayed prompt/source, terminal control bytes, then generated MXPD.
printf '%s\n' "$prompt"
printf '\033[2K\033[1G'
cat <<'OUT'
S|clean assistant result
K|L
F|clean assistant fact
C|CMP
T|DOCS_PROJECTION
Z
OUT
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	g := GemmaCLI{
		Binary:    binary,
		Model:     model,
		Context:   2048,
		MaxTokens: 80,
		Threads:   1,
		Timeout:   5 * time.Second,
	}

	result, err := g.Analyze(context.Background(), Input{
		Name: "SAW:SAAB:E-A+E-B",
		Content: []byte(
			"clean assistant fact\n" +
				"SAW/1\n" +
				`C|"OBS"` + "\n" +
				`C|"RLT"` + "\n" +
				`L|"E-A"|"E-B"|"COMPOSITION_KIN"` + "\n" +
				"Z\n",
		),
		GovernedState: "accepted_context_present",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Summary != "clean assistant result" {
		t.Fatalf("summary = %q", result.Summary)
	}
	if result.Risk != "L" {
		t.Fatalf("risk = %q want L", result.Risk)
	}
	if len(result.Facts) != 1 || result.Facts[0] != "clean assistant fact" {
		t.Fatalf("facts leaked displayed prompt: %#v", result.Facts)
	}
	if len(result.Capabilities) != 1 || result.Capabilities[0] != "CMP" {
		t.Fatalf("capabilities leaked displayed prompt: %#v", result.Capabilities)
	}
	if len(result.Relationships) != 0 {
		t.Fatalf("relationships leaked displayed prompt: %#v", result.Relationships)
	}
	if len(result.Facets) != 1 || result.Facets[0] != "DOCS_PROJECTION" {
		t.Fatalf("facets = %#v", result.Facets)
	}
}
