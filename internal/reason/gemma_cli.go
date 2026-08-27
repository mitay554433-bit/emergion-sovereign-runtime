package reason

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type GemmaCLI struct {
	Binary    string
	Model     string
	Threads   int
	Context   int
	MaxTokens int
	Timeout   time.Duration
	ExtraArgs []string
}

func GemmaFromEnv() GemmaCLI {
	threads := envInt("GEMMA_THREADS", 4)
	ctx := envInt("GEMMA_CONTEXT", 4096)
	max := envInt("GEMMA_MAX_TOKENS", 256)
	timeout := time.Duration(envInt("GEMMA_TIMEOUT_SECONDS", 180)) * time.Second
	extra := strings.Fields(os.Getenv("GEMMA_EXTRA_ARGS"))
	return GemmaCLI{
		Binary:    discoverBinary(os.Getenv("GEMMA_BIN")),
		Model:     discoverModel(os.Getenv("GEMMA_MODEL")),
		Threads:   threads,
		Context:   ctx,
		MaxTokens: max,
		Timeout:   timeout,
		ExtraArgs: extra,
	}
}

func envInt(name string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
func discoverBinary(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if p, err := exec.LookPath("llama-cli"); err == nil {
		return p
	}
	if p, err := exec.LookPath("llama-completion"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, "llama.cpp", "build", "bin", "llama-completion"),
		filepath.Join(home, "bin", "llama-completion"),
		filepath.Join(home, "llama-completion"),
		"/usr/local/bin/llama-completion",
		filepath.Join(home, "llama.cpp", "build", "bin", "llama-cli"),
		filepath.Join(home, "bin", "llama-cli"),
		filepath.Join(home, "llama-cli"),
		"/usr/local/bin/llama-cli",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "llama-cli"
}

type modelCandidate struct {
	path     string
	score    int
	modified time.Time
}

func discoverModel(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, "models"), filepath.Join(home, "gguf"),
		filepath.Join(home, ".cache", "llama.cpp"), filepath.Join(home, ".local", "share", "models"),
		"models", "/opt/models",
	}
	if extra := os.Getenv("GEMMA_MODEL_DIRS"); extra != "" {
		dirs = append(strings.Split(extra, string(os.PathListSeparator)), dirs...)
	}
	var found []modelCandidate
	for _, root := range dirs {
		root = filepath.Clean(root)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(root, path)
				if rel != "." && strings.Count(rel, string(os.PathSeparator)) >= 2 {
					return filepath.SkipDir
				}
				return nil
			}
			name := strings.ToLower(d.Name())
			if !strings.HasSuffix(name, ".gguf") || !strings.Contains(name, "gemma") {
				return nil
			}
			score := 100
			if strings.Contains(name, "instruct") || strings.Contains(name, "-it-") || strings.Contains(name, "_it_") {
				score += 20
			}
			if strings.Contains(name, "q4_k_m") || strings.Contains(name, "q5_k_m") {
				score += 10
			}
			info, _ := d.Info()
			mod := time.Time{}
			if info != nil {
				mod = info.ModTime()
			}
			found = append(found, modelCandidate{path: path, score: score, modified: mod})
			return nil
		})
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return found[i].modified.After(found[j].modified)
	})
	return found[0].path
}

func (g GemmaCLI) Name() string { return "gemma-llama-cli" }

func (g GemmaCLI) Version(ctx context.Context) string {
	if g.Binary == "" {
		return "unknown"
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, g.Binary, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	v := strings.TrimSpace(string(out))
	if len(v) > 120 {
		v = v[:120]
	}
	return v
}

func (g GemmaCLI) Validate() error {
	if g.Model == "" {
		return fmt.Errorf("GEMMA_MODEL is not set")
	}
	if _, err := os.Stat(g.Model); err != nil {
		return fmt.Errorf("Gemma model unavailable: %w", err)
	}
	if _, err := exec.LookPath(g.Binary); err != nil {
		return fmt.Errorf("Gemma runtime unavailable: %w", err)
	}
	return nil
}

const mxpdGrammar = `root ::= summary risk fact capability? facet? end
summary ::= "S|" text "\n"
risk ::= "K|" ("L" | "M" | "H") "\n"
fact ::= "F|" text "\n"
capability ::= "C|" capability-token "\n"
capability-token ::= "OBS" | "CMP" | "RLT" | "VLD" | "REASON" | "ANALYZE" | "DRAFT" | "SIMULATE" | "PROGRAM" | "VERSION" | "PATENT_EVIDENCE" | "READ" | "SEND" | "PRODUCT" | "PRICE" | "LINK" | "RECEIPT" | "TRANSFER" | "CUSTOMER" | "LEAD" | "SALE" | "SUPPORT" | "SITE" | "STORE" | "DEPLOY" | "PATENT" | "GRANT" | "MARKET" | "MA"
facet ::= "T|" facet-token "\n"
facet-token ::= "FIELD_COMMAND" | "EMERGENCE_CAPTURE" | "PROGRAM_FORGE" | "PRODUCT_STORE" | "CUSTOMERS_SALES" | "COMMUNICATIONS" | "PAYMENTS_FINANCE" | "GRANT_FUNDING" | "PATENT_IP" | "MA_PARTNERSHIPS" | "DOCS_PROJECTION" | "ANALYTICS_FORECAST"
end ::= "Z"
text ::= safe-first text-tail | label-first label-next text-tail
safe-first ::= [ABDIJOPQRVWXYZabdijopqrvwxyz0-9]
label-first ::= [CEFGHKLMNSTUcefghklmnstu]
label-next ::= [^:=,|\r\n]
text-tail ::= [^|\r\n]*
`

func gemmaArgs(g GemmaCLI, prompt string) []string {
	args := []string{
		"-m", g.Model,
		"-p", prompt,
		"-n", strconv.Itoa(g.MaxTokens),
		"-c", strconv.Itoa(g.Context),
		"-t", strconv.Itoa(g.Threads),
		"--temp", "0.1",
	}

	args = append(args, g.ExtraArgs...)

	// Execution-boundary invariants. These intentionally come last.
	args = append(args,
		"--log-disable",
		"--color", "off",
		"--grammar", mxpdGrammar,
		"--single-turn",
		"--no-display-prompt",
		"--output-file", "/dev/stdout",
	)

	return args
}

func (g GemmaCLI) Analyze(ctx context.Context, in Input) (Result, error) {
	if err := g.Validate(); err != nil {
		return Result{}, err
	}
	if len(in.Content) == 0 {
		return Result{}, fmt.Errorf("empty input")
	}
	content := string(in.Content)
	governedState := strings.TrimSpace(in.GovernedState)

	inputTokens := g.Context - g.MaxTokens - 1024
	if inputTokens < 256 {
		return Result{}, fmt.Errorf("Gemma context too small for bounded analysis")
	}

	inputBytes := inputTokens * 3
	governedLimit := inputBytes / 4
	if len(governedState) > governedLimit {
		governedState = governedState[:governedLimit]
	}

	contentLimit := inputBytes - len(governedState)
	if contentLimit > 1000 {
		contentLimit = 1000
	}
	if contentLimit < 1 {
		return Result{}, fmt.Errorf("Gemma context exhausted by governed state")
	}
	if len(content) > contentLimit {
		content = content[:contentLimit]
	}
	prompt := buildPrompt(in.Name, content, governedState)

	for attempt := 0; attempt < 2; attempt++ {
		args := gemmaArgs(g, prompt)

		cctx, cancel := context.WithTimeout(ctx, g.Timeout)
		cmd := exec.CommandContext(cctx, g.Binary, args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		ctxErr := cctx.Err()
		cancel()

		if runErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return Result{}, fmt.Errorf("Gemma timed out: %w", ctxErr)
			}
			return Result{}, fmt.Errorf(
				"Gemma failed: %w: %s",
				runErr,
				trim(stderr.String(), 500),
			)
		}

		candidates := []string{
			stdout.String(),
			stderr.String(),
			stdout.String() + "\n" + stderr.String(),
		}

		var parseErr error
		var validationErr error

		for _, candidate := range candidates {
			res, err := parseResult(candidate)
			if err != nil {
				parseErr = err
				continue
			}

			if err := rejectPromptPlaceholders(res, content); err != nil {
				validationErr = err
				break
			}

			return res, nil
		}

		if validationErr != nil {
			if attempt == 0 {
				prompt = buildPrompt(in.Name, content, governedState) +
					"\n\nRETRY:\n" +
					"Regenerate from SOURCE only. " +
					"Do not describe validation, rejection, retry, prompt text, or GOVERNED_STATE. " +
					"Return only source-supported S, F, and C values."
				continue
			}
			return Result{}, validationErr
		}

		return Result{}, fmt.Errorf(
			"Gemma output invalid: %w; stdout=%s; stderr=%s",
			parseErr,
			trim(stdout.String(), 500),
			trim(stderr.String(), 500),
		)
	}

	return Result{}, fmt.Errorf("Gemma semantic retry exhausted")
}

func sourceSupportsFact(source, fact string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	fact = strings.ToLower(strings.TrimSpace(fact))
	if source == "" || fact == "" {
		return false
	}

	split := func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}

	stop := map[string]bool{
		"that":   true,
		"this":   true,
		"with":   true,
		"from":   true,
		"into":   true,
		"only":   true,
		"after":  true,
		"before": true,
		"system": true,
	}

	seen := map[string]bool{}
	required := 0
	matched := 0

	for _, token := range strings.FieldsFunc(fact, split) {
		if len(token) < 4 || stop[token] || seen[token] {
			continue
		}
		seen[token] = true
		required++
		if strings.Contains(source, token) {
			matched++
		}
	}

	if required == 0 {
		return strings.Contains(source, fact)
	}
	if required == 1 {
		return matched == 1
	}
	return matched >= 2
}

func rejectPromptPlaceholders(r Result, source string) error {
	if strings.EqualFold(strings.TrimSpace(r.Summary), "one sentence summary") {
		return fmt.Errorf("Gemma output rejected: prompt placeholder summary")
	}

	for _, fact := range r.Facts {
		if strings.EqualFold(strings.TrimSpace(fact), "one evidenced fact") {
			return fmt.Errorf("Gemma output rejected: prompt placeholder fact")
		}
	}

	for _, capability := range r.Capabilities {
		if strings.EqualFold(strings.TrimSpace(capability), "one transferable capability") {
			return fmt.Errorf("Gemma output rejected: prompt placeholder capability")
		}
	}

	for _, fact := range r.Facts {
		if !sourceSupportsFact(source, fact) {
			return fmt.Errorf("Gemma output rejected: fact lacks source support")
		}
	}

	upperSummary := strings.ToUpper(strings.TrimSpace(r.Summary))
	for _, prefix := range []string{"S:", "F:", "C:", "K:", "G:", "L:", "H:", "U:", "T:", "N:", "E:", "M:", "F,C:", "C,F:", "S,F:", "S,C:"} {
		if strings.HasPrefix(upperSummary, prefix) {
			return fmt.Errorf("Gemma output rejected: summary begins with field label %s", prefix)
		}

	}

	for _, capability := range r.Capabilities {
		capability = strings.TrimSpace(capability)
		for _, fact := range r.Facts {
			if strings.EqualFold(capability, strings.TrimSpace(fact)) {
				return fmt.Errorf("Gemma output rejected: capability duplicates fact")
			}
		}
	}

	if strings.HasPrefix(strings.TrimSpace(r.Summary), ":") {
		return fmt.Errorf("Gemma output rejected: summary begins with punctuation")
	}

	if fields := strings.Fields(upperSummary); len(fields) > 0 {
		onlyFieldTokens := true
		for _, field := range fields {
			switch field {
			case "S", "F", "C", "K", "G", "L", "H", "U", "T", "N", "E", "M":
			default:
				onlyFieldTokens = false
			}
		}
		if onlyFieldTokens {
			return fmt.Errorf("Gemma output rejected: summary contains only field tokens")
		}
	}

	for _, fact := range r.Facts {
		if strings.HasPrefix(strings.TrimSpace(fact), ":") {
			return fmt.Errorf("Gemma output rejected: fact begins with punctuation")
		}
	}

	for _, capability := range r.Capabilities {
		capability = strings.TrimSpace(capability)
		if strings.HasPrefix(capability, ":") {
			return fmt.Errorf("Gemma output rejected: capability begins with punctuation")
		}

		normalized := strings.ToLower(strings.TrimSpace(strings.TrimRight(capability, ".!?")))
		switch normalized {
		case "can be repeated", "can work", "can operate", "can function", "can perform", "can be used":
			return fmt.Errorf("Gemma output rejected: vacuous capability")
		}
	}

	return nil
}

func buildPrompt(name, content, governedState string) string {
	return `@L:MXPD/2
@T:REDUCE

SOURCE:` + filepath.Base(name) + `
` + content + `

Produce one bounded MXPD machine record from SOURCE.

S = concise natural-language statement of the main meaning proved by SOURCE.
K = L, M, or H.
F = one concrete source-supported observation or verified state.
C = one reusable behavior, mechanism, or operational ability evidenced by SOURCE.
T = optional canonical operational facet evidenced by SOURCE.
Z = final terminator.

S, F, C, and T MUST come from SOURCE.
S must state SOURCE meaning directly and must not repeat instruction wording such as
"summary supported by SOURCE", "meaningful natural-language summary", or field definitions.
F must describe what is evidenced or verified.
C must describe what the system can repeatedly do because of that evidence.
A test result alone is a fact, not a capability.
Examples of capability form include verifying integrity, preserving evidence,
preventing duplicate candidates, enforcing governed admission, or rebuilding projection state.
Do not use protocol tokens, governance tokens, record keys, field labels,
single letters, AX, GOV, REG, NI, or FC as S, F, or C values.
Do not invent facts, capabilities, relationships, or authority.

GOVERNED_STATE is comparison context only:
` + governedState + `

AX[S!=T;M!=T;GOV>D;REG>A;NI;FC]
Generated structure is constrained by the runtime grammar.
No markdown or explanatory prose.`
}
func parseResult(s string) (Result, error) {
	r := Result{Relationships: map[string]string{}}
	complete := false

	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		p := strings.Split(line, "|")
		switch p[0] {
		case "SUMMARY", "S":
			if len(p) >= 2 {
				r.Summary = strings.TrimSpace(strings.Join(p[1:], "|"))
			}
		case "REL", "L":
			if len(p) >= 3 {
				r.Relationships[strings.TrimSpace(p[1])] = strings.TrimSpace(strings.Join(p[2:], "|"))
			}
		case "CAP", "C":
			if len(p) >= 2 {
				r.Capabilities = append(r.Capabilities, strings.TrimSpace(strings.Join(p[1:], "|")))
			}
		case "FACT", "F":
			if len(p) >= 2 {
				r.Facts = append(r.Facts, strings.TrimSpace(strings.Join(p[1:], "|")))
			}
		case "GAP", "G":
			if len(p) >= 2 {
				r.Gaps = append(r.Gaps, strings.TrimSpace(strings.Join(p[1:], "|")))
			}
		case "RISK", "K":
			if len(p) == 2 {
				r.Risk = strings.TrimSpace(p[1])
			}
		case "SUPERSEDES", "U":
			if len(p) >= 2 {
				r.Supersedes = strings.TrimSpace(strings.Join(p[1:], "|"))
			}
		case "FACET", "T":
			if len(p) >= 2 {
				r.Facets = append(r.Facets, strings.TrimSpace(strings.Join(p[1:], "|")))
			}
		case "NODE", "N":
			if len(p) == 4 {
				r.BuildNodes = append(r.BuildNodes, BuildNode{
					ID: strings.TrimSpace(p[1]), System: strings.TrimSpace(p[2]), State: strings.TrimSpace(p[3]),
				})
			}
		case "EDGE", "E":
			if len(p) == 4 {
				r.BuildEdges = append(r.BuildEdges, BuildEdge{
					From: strings.TrimSpace(p[1]), To: strings.TrimSpace(p[2]), Kind: strings.TrimSpace(p[3]),
				})
			}
		case "ARCHONYM", "H":
			if len(p) == 2 {
				r.Archonym = strings.TrimSpace(p[1])
			}
		case "MONEY", "M":
			if len(p) == 5 {
				r.Monetization = &Monetization{
					Model: strings.TrimSpace(p[1]), Customer: strings.TrimSpace(p[2]),
					Value: strings.TrimSpace(p[3]), RevenuePath: strings.TrimSpace(p[4]),
				}
			}
		case "END", "Z":
			complete = true
		}
	}

	if !complete || r.Summary == "" || r.Risk == "" {
		return Result{}, fmt.Errorf("no complete primitive Result")
	}
	return r, nil
}

func FormatResult(r Result) string {
	var b strings.Builder

	if r.Summary != "" {
		fmt.Fprintf(&b, "S|%s\n", r.Summary)
	}

	if r.Archonym != "" {
		fmt.Fprintf(&b, "H|%s\n", r.Archonym)
	}

	for key, value := range r.Relationships {
		fmt.Fprintf(&b, "L|%s|%s\n", key, value)
	}

	for _, value := range r.Capabilities {
		fmt.Fprintf(&b, "C|%s\n", value)
	}

	for _, value := range r.Facts {
		fmt.Fprintf(&b, "F|%s\n", value)
	}

	for _, value := range r.Gaps {
		fmt.Fprintf(&b, "G|%s\n", value)
	}

	if r.Risk != "" {
		fmt.Fprintf(&b, "K|%s\n", r.Risk)
	}

	if r.Supersedes != "" {
		fmt.Fprintf(&b, "U|%s\n", r.Supersedes)
	}

	for _, value := range r.Facets {
		fmt.Fprintf(&b, "T|%s\n", value)
	}

	for _, node := range r.BuildNodes {
		fmt.Fprintf(&b, "N|%s|%s|%s\n", node.ID, node.System, node.State)
	}

	for _, edge := range r.BuildEdges {
		fmt.Fprintf(&b, "E|%s|%s|%s\n", edge.From, edge.To, edge.Kind)
	}

	if r.Monetization != nil {
		fmt.Fprintf(
			&b,
			"M|%s|%s|%s|%s\n",
			r.Monetization.Model,
			r.Monetization.Customer,
			r.Monetization.Value,
			r.Monetization.RevenuePath,
		)
	}

	b.WriteString("Z\n")
	return b.String()
}

func Calibrate(r Result) Result {
	r.Summary = strings.TrimSpace(r.Summary)
	r.Archonym = cleanText(r.Archonym, 160)
	if strings.EqualFold(r.Archonym, "null") {
		r.Archonym = ""
	}
	if len(r.Summary) > 480 {
		r.Summary = r.Summary[:480]
	}
	r.Relationships = cleanRelationships(r.Relationships)
	if r.Risk != "L" && r.Risk != "M" && r.Risk != "H" {
		r.Risk = "M"
	}
	r.Capabilities = clean(r.Capabilities, 16)
	r.Facts = clean(r.Facts, 24)
	r.Gaps = clean(r.Gaps, 24)
	r.Supersedes = cleanText(r.Supersedes, 80)
	if strings.EqualFold(r.Supersedes, "null") {
		r.Supersedes = ""
	}
	r.Delta = nil
	r.Facets = cleanFacets(r.Facets)
	r.BuildNodes, r.BuildEdges = cleanBuildGraph(r.BuildNodes, r.BuildEdges)
	if r.Monetization != nil {
		r.Monetization.Model = cleanText(r.Monetization.Model, 160)
		r.Monetization.Customer = cleanText(r.Monetization.Customer, 160)
		r.Monetization.Value = cleanText(r.Monetization.Value, 240)
		r.Monetization.RevenuePath = cleanText(r.Monetization.RevenuePath, 240)
		if r.Monetization.Model == "" && r.Monetization.Customer == "" && r.Monetization.Value == "" && r.Monetization.RevenuePath == "" {
			r.Monetization = nil
		}
	}
	return r
}

func cleanRelationships(in map[string]string) map[string]string {
	out := map[string]string{}
	if in == nil {
		return out
	}

	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		cleanKey := strings.TrimSpace(key)
		cleanValue := strings.TrimSpace(in[key])
		if cleanKey == "" || cleanValue == "" {
			continue
		}
		if _, exists := out[cleanKey]; exists {
			continue
		}
		out[cleanKey] = cleanValue
	}

	return out
}

func cleanBuildGraph(nodes []BuildNode, edges []BuildEdge) ([]BuildNode, []BuildEdge) {
	cleanNodes := make([]BuildNode, 0, 24)
	known := map[string]bool{}
	for _, node := range nodes {
		node.ID = cleanText(node.ID, 80)
		node.System = cleanText(node.System, 160)
		node.State = cleanText(node.State, 80)
		if node.ID == "" || node.System == "" || known[node.ID] {
			continue
		}
		known[node.ID] = true
		cleanNodes = append(cleanNodes, node)
		if len(cleanNodes) == 24 {
			break
		}
	}
	cleanEdges := make([]BuildEdge, 0, 48)
	seen := map[string]bool{}
	for _, edge := range edges {
		edge.From = cleanText(edge.From, 80)
		edge.To = cleanText(edge.To, 80)
		edge.Kind = cleanText(edge.Kind, 80)
		key := edge.From + "\x00" + edge.To + "\x00" + edge.Kind
		if !known[edge.From] || !known[edge.To] || seen[key] {
			continue
		}
		seen[key] = true
		cleanEdges = append(cleanEdges, edge)
		if len(cleanEdges) == 48 {
			break
		}
	}
	return cleanNodes, cleanEdges
}

func cleanText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func cleanFacets(in []string) []string {
	allowed := map[string]bool{
		"FIELD_COMMAND": true, "EMERGENCE_CAPTURE": true, "PROGRAM_FORGE": true,
		"PRODUCT_STORE": true, "CUSTOMERS_SALES": true, "COMMUNICATIONS": true,
		"PAYMENTS_FINANCE": true, "GRANT_FUNDING": true, "PATENT_IP": true,
		"MA_PARTNERSHIPS": true, "DOCS_PROJECTION": true, "ANALYTICS_FORECAST": true,
	}
	out := make([]string, 0, 12)
	seen := map[string]bool{}
	for _, facet := range in {
		facet = strings.TrimSpace(facet)
		if allowed[facet] && !seen[facet] {
			seen[facet] = true
			out = append(out, facet)
		}
	}
	return out
}
func clean(in []string, max int) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		if len(v) > 96 {
			v = v[:96]
		}
		seen[v] = true
		out = append(out, v)
		if len(out) == max {
			break
		}
	}
	return out
}
func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
