package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/analytics"
	"emergion-sovereign-runtime/internal/axiom"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/doctor"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

func fail(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(b))
}
func openStore(root string) *store.Store {
	s, err := store.Open(root)
	if err != nil {
		fail(err)
	}
	return s
}
func loadState(s *store.Store) core.State {
	ev, err := s.Events()
	if err != nil {
		fail(err)
	}
	st, err := livefield.Rebuild(ev)
	if err != nil {
		fail(err)
	}
	return st
}
func findEmergION(st core.State, id string) (core.EmergION, bool) {
	groups := []map[string]core.EmergION{
		st.AtGOV,
		st.Approved,
		st.Accepted,
		st.Held,
		st.Rejected,
		st.Returned,
	}
	for _, group := range groups {
		if em, ok := group[id]; ok {
			return em, true
		}
	}
	return core.EmergION{}, false
}

func renderField(s *store.Store, out string) proj.Receipt {
	st := loadState(s)
	receipt, err := proj.Current(out, st)
	if err != nil {
		fail(err)
	}
	return receipt
}

type lineageExportReceipt struct {
	Bundle     string    `json:"bundle"`
	SHA256     string    `json:"sha256"`
	Head       string    `json:"head"`
	Branch     string    `json:"branch"`
	ExportedAt time.Time `json:"exported_at"`
}

func gitOutput(repo string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repo}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// exportLineage creates a complete, verified bundle at a human-granted local
// transfer boundary. It never changes repository state or uploads externally.
func exportLineage(destination string) (lineageExportReceipt, error) {
	repo, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return lineageExportReceipt{}, err
	}
	head, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		return lineageExportReceipt{}, err
	}
	branch, err := gitOutput(repo, "branch", "--show-current")
	if err != nil {
		return lineageExportReceipt{}, err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return lineageExportReceipt{}, err
	}
	name := "LINEAGE_HEAD_" + head[:7] + ".bundle"
	final := filepath.Join(destination, name)
	tmp, err := os.CreateTemp(destination, ".lineage-*.bundle")
	if err != nil {
		return lineageExportReceipt{}, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return lineageExportReceipt{}, err
	}
	defer os.Remove(tmpPath)
	if _, err := gitOutput(repo, "bundle", "create", tmpPath, "--all"); err != nil {
		return lineageExportReceipt{}, err
	}
	if _, err := gitOutput(repo, "bundle", "verify", tmpPath); err != nil {
		return lineageExportReceipt{}, err
	}
	if err := os.Rename(tmpPath, final); err != nil {
		return lineageExportReceipt{}, err
	}
	hash, err := hashFile(final)
	if err != nil {
		return lineageExportReceipt{}, err
	}
	return lineageExportReceipt{Bundle: final, SHA256: hash, Head: head, Branch: branch, ExportedAt: time.Now().UTC()}, nil
}

type deliverableReceipt struct {
	EmergIONID           string    `json:"emergion_id"`
	EvidenceHash         string    `json:"evidence_sha256"`
	SourceEmergIONID     string    `json:"source_emergion_id"`
	SourceHash           string    `json:"source_hash"`
	TransitionEmergIONID string    `json:"transition_emergion_id"`
	AuthorizationID      string    `json:"authorization_event"`
	Authority            string    `json:"authority"`
	Adapter              string    `json:"adapter"`
	Action               string    `json:"action"`
	OutputSHA256         string    `json:"output_sha256"`
	FieldTip             string    `json:"field_tip"`
	PublishedAt          time.Time `json:"published_at"`
}

type deliverableIndex struct {
	FieldTip     string               `json:"field_tip"`
	Deliverables []deliverableReceipt `json:"deliverables"`
}

func publishDeliverableIndex(root, fieldTip string, receipts []deliverableReceipt) (string, error) {
	dir := filepath.Join(root, "deliverables")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	index := deliverableIndex{
		FieldTip:     fieldTip,
		Deliverables: receipts,
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, ".index-*.json")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	final := filepath.Join(dir, "index.json")
	if err := os.Rename(tmpPath, final); err != nil {
		return "", fmt.Errorf("publish deliverable index: %w", err)
	}
	return final, nil
}

func publishAcceptedDeliverables(s *store.Store, out string) ([]string, error) {
	st := loadState(s)

	ids := make([]string, 0)
	for id, em := range st.Accepted {
		if em.REL["source_kind"] == "EXECUTION_RESULT" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	published := make([]string, 0, len(ids))
	receipts := make([]deliverableReceipt, 0, len(ids))
	rt := fieldruntime.Runtime{Store: s}

	for _, id := range ids {
		deliverable, err := rt.MaterializeAcceptedExecutionResult(id)
		if err != nil {
			if errors.Is(err, fieldruntime.ErrExecutionResultUnsuccessful) {
				continue
			}
			return published, err
		}

		receipt, _, err := publishDeliverable(out, deliverable)
		if err != nil {
			return published, err
		}
		receipts = append(receipts, receipt)
		published = append(published, id)
	}

	if _, err := publishDeliverableIndex(out, st.TipHash, receipts); err != nil {
		return published, err
	}

	return published, nil
}

func publishDeliverable(root string, d fieldruntime.Deliverable) (deliverableReceipt, string, error) {
	dir := filepath.Join(root, "deliverables", d.EmergIONID)
	outputFinal := filepath.Join(dir, "output")
	receiptFinal := filepath.Join(dir, "receipt.json")

	if receiptBytes, err := os.ReadFile(receiptFinal); err == nil {
		var existing deliverableReceipt
		if json.Unmarshal(receiptBytes, &existing) == nil {
			outputHash, hashErr := hashFile(outputFinal)
			expectedHash := sha256.Sum256([]byte(d.Output))
			expectedOutputHash := fmt.Sprintf("%x", expectedHash[:])

			if hashErr == nil &&
				existing.EmergIONID == d.EmergIONID &&
				existing.EvidenceHash == d.EvidenceHash &&
				existing.SourceEmergIONID == d.SourceEmergIONID &&
				existing.SourceHash == d.SourceHash &&
				existing.TransitionEmergIONID == d.TransitionEmergIONID &&
				existing.AuthorizationID == d.AuthorizationID &&
				existing.Authority == d.Authority &&
				existing.Adapter == d.Adapter &&
				existing.Action == d.Action &&
				existing.OutputSHA256 == outputHash &&
				existing.OutputSHA256 == expectedOutputHash &&
				existing.FieldTip != "" &&
				!existing.PublishedAt.IsZero() {
				return existing, outputFinal, nil
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return deliverableReceipt{}, "", err
	}

	tmp, err := os.MkdirTemp(dir, ".publish-*")
	if err != nil {
		return deliverableReceipt{}, "", err
	}
	defer os.RemoveAll(tmp)

	outputTmp := filepath.Join(tmp, "output")
	if err := os.WriteFile(outputTmp, []byte(d.Output), 0o644); err != nil {
		return deliverableReceipt{}, "", err
	}

	outputHash, err := hashFile(outputTmp)
	if err != nil {
		return deliverableReceipt{}, "", err
	}

	receipt := deliverableReceipt{
		EmergIONID:           d.EmergIONID,
		EvidenceHash:         d.EvidenceHash,
		SourceEmergIONID:     d.SourceEmergIONID,
		SourceHash:           d.SourceHash,
		TransitionEmergIONID: d.TransitionEmergIONID,
		AuthorizationID:      d.AuthorizationID,
		Authority:            d.Authority,
		Adapter:              d.Adapter,
		Action:               d.Action,
		OutputSHA256:         outputHash,
		FieldTip:             d.FieldTip,
		PublishedAt:          time.Now().UTC(),
	}

	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return deliverableReceipt{}, "", err
	}

	receiptTmp := filepath.Join(tmp, "receipt.json")
	if err := os.WriteFile(receiptTmp, receiptBytes, 0o644); err != nil {
		return deliverableReceipt{}, "", err
	}

	if err := os.Rename(outputTmp, outputFinal); err != nil {
		return deliverableReceipt{}, "", fmt.Errorf("publish deliverable output: %w", err)
	}
	if err := os.Rename(receiptTmp, receiptFinal); err != nil {
		return deliverableReceipt{}, "", fmt.Errorf("publish deliverable receipt: %w", err)
	}

	return receipt, outputFinal, nil
}
func main() {
	state := flag.String("state", envOr("FIELD_HOME", ".field"), "local runtime state")
	reasonerName := flag.String("reasoner", envOr("FIELD_REASONER", "gemma"), "gemma or heuristic")
	dropzone := flag.String("dropzone", envOr("FIELD_DROPZONE", "dropzone"), "transient intake directory")
	poll := flag.Duration("poll", 2*time.Second, "dropzone polling interval")
	output := flag.String("output", envOr("FIELD_OUTPUT", "outputs"), "living FIELD projection directory")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		return
	}

	gemma := reason.GemmaFromEnv()
	if args[0] == "doctor" {
		printJSON(doctor.Run(context.Background(), *state, gemma))
		return
	}
	s := openStore(*state)
	mkReasoner := func() reason.Reasoner {
		switch *reasonerName {
		case "gemma":
			return gemma
		case "heuristic":
			return reason.Heuristic{}
		default:
			fail(fmt.Errorf("unknown reasoner %q", *reasonerName))
			return nil
		}
	}

	switch args[0] {
	case "init":
		if err := os.MkdirAll(*dropzone, 0o700); err != nil {
			fail(err)
		}
		fmt.Println("READY", *state, *dropzone)
	case "capture":
		if len(args) < 2 {
			fail(fmt.Errorf("capture requires a file"))
		}
		rt := fieldruntime.Runtime{Store: s, Reasoner: mkReasoner()}
		em, duplicate, err := rt.Capture(context.Background(), args[1], false)
		if err != nil {
			fail(err)
		}
		renderField(s, *output)
		if duplicate {
			fmt.Println(em.IDN, "AT_GOV", "DUPLICATE_SOURCE")
		} else {
			fmt.Println(em.IDN, "AT_GOV")
		}
	case "implementation-evidence":
		if len(args) != 2 {
			fail(fmt.Errorf("implementation-evidence requires a file"))
		}

		evidence, err := os.ReadFile(args[1])
		if err != nil {
			fail(err)
		}

		rt := fieldruntime.Runtime{
			Store:    s,
			Reasoner: mkReasoner(),
		}
		em, duplicate, err := rt.CaptureImplementationEvidence(
			context.Background(),
			evidence,
		)
		if err != nil {
			fail(err)
		}

		receipt := renderField(s, *output)
		if duplicate {
			fmt.Println(
				em.IDN,
				"AT_GOV",
				"DUPLICATE_SOURCE",
				"PROJECTION_TIP",
				receipt.TipHash,
			)
		} else {
			fmt.Println(
				em.IDN,
				"AT_GOV",
				"IMPLEMENTATION_EVIDENCE",
				"PROJECTION_TIP",
				receipt.TipHash,
			)
		}

	case "target":
		if len(args) < 2 {
			fail(fmt.Errorf("target requires exact target text"))
		}
		rt := fieldruntime.Runtime{Store: s}
		em, duplicate, err := rt.CaptureTarget(context.Background(), strings.Join(args[1:], " "))
		if err != nil {
			fail(err)
		}
		renderField(s, *output)
		if duplicate {
			fmt.Println(em.IDN, "AT_GOV", "DUPLICATE_SOURCE")
		} else {
			fmt.Println(em.IDN, "AT_GOV")
		}
	case "rework":
		if len(args) < 3 {
			fail(fmt.Errorf("rework <returned-id> <file>"))
		}
		rt := fieldruntime.Runtime{
			Store:               s,
			Reasoner:            mkReasoner(),
			ReturnedPredecessor: args[1],
		}
		em, duplicate, err := rt.Capture(context.Background(), args[2], false)
		if err != nil {
			fail(err)
		}
		renderField(s, *output)
		if duplicate {
			fmt.Println(em.IDN, "AT_GOV", "DUPLICATE_SOURCE")
		} else {
			fmt.Println(em.IDN, "AT_GOV", "REWORK_OF", args[1])
		}
	case "reinterpret":
		if len(args) != 3 {
			fail(fmt.Errorf("reinterpret <at-gov-id> <source-name>"))
		}

		rt := fieldruntime.Runtime{
			Store:    s,
			Reasoner: mkReasoner(),
		}

		em, err := rt.RevalidateInterpretation(
			context.Background(),
			args[1],
			args[2],
		)
		if err != nil {
			fail(err)
		}

		receipt := renderField(s, *output)
		fmt.Println(
			em.IDN,
			"AT_GOV",
			"INTERPRETATION_REVISION",
			"PROJECTION_TIP",
			receipt.TipHash,
		)
	case "once":
		rt := fieldruntime.Runtime{Store: s, Reasoner: mkReasoner()}
		ids, err := rt.Once(context.Background(), *dropzone)
		if err != nil {
			fail(err)
		}

		receipt := renderField(s, *output)
		printJSON(map[string]any{
			"captured":         ids,
			"dropzone_cleared": true,
			"projection":       receipt,
		})

	case "run":
		ctx, stop := signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)
		defer stop()

		sawRuntime, err := fieldapi.Open(*state, mkReasoner())
		if err != nil {
			fail(err)
		}

		fmt.Println(
			"LIVING_FIELD",
			filepath.Clean(*dropzone),
			"reasoner="+*reasonerName,
		)

		if err := sawRuntime.Run(
			ctx,
			*dropzone,
			*poll,
			gemma,
			func(id string) {
				receipt := renderField(s, *output)
				fmt.Println(id, "AT_GOV", "PROJECTION_TIP", receipt.TipHash)
			},
			func(circulated []core.EmergION, safeSignal core.EmergION, safeExecuted bool) {
				for _, em := range circulated {
					renderField(s, *output)
					fmt.Println(em.IDN, "SAW_AT_GOV")
				}
				if safeExecuted {
					renderField(s, *output)
					fmt.Println(safeSignal.IDN, "SAFE_ACTION_AT_GOV")
				}
				published, err := publishAcceptedDeliverables(s, *output)
				if err != nil {
					fail(err)
				}
				for _, id := range published {
					fmt.Println(id, "DELIVERABLE_READY")
				}
			},
		); err != nil {
			fail(err)
		}

	case "decide":
		if len(args) < 3 {
			fail(fmt.Errorf("decide <id> <APPROVE|HOLD|REJECT|RETURN> [reason]"))
		}
		st := loadState(s)
		em, ok := st.AtGOV[args[1]]
		if !ok {
			fail(fmt.Errorf("candidate not at GOV"))
		}
		reasonText := ""
		if len(args) > 3 {
			reasonText = args[3]
		}
		em, receipt, err := gov.Decide(em, gov.Decision(args[2]), "HUMAN_FINAL", reasonText)
		if err != nil {
			fail(err)
		}
		decisionEventID, err := s.SaveDecision(receipt)
		if err != nil {
			fail(err)
		}
		if receipt.Decision == string(gov.Approve) {
			_, regReceipt, err := reg.Accept(em, decisionEventID)
			if err != nil {
				fail(err)
			}
			if _, err = s.SaveAccepted(regReceipt); err != nil {
				fail(err)
			}
			renderField(s, *output)
			fmt.Println(em.IDN, "REG_ACCEPTED")
		} else {
			renderField(s, *output)
			fmt.Println(em.IDN, receipt.Decision)
		}
	case "resume":
		if len(args) < 2 {
			fail(fmt.Errorf("resume <held-id> [reason]"))
		}
		st := loadState(s)
		em, ok := st.Held[args[1]]
		if !ok {
			fail(fmt.Errorf("EmergION is not held"))
		}
		reasonText := ""
		if len(args) > 2 {
			reasonText = args[2]
		}
		em, receipt, err := gov.ResumeHeld(em, "HUMAN_FINAL", reasonText)
		if err != nil {
			fail(err)
		}
		if _, err := s.SaveDecision(receipt); err != nil {
			fail(err)
		}
		renderField(s, *output)
		fmt.Println(em.IDN, "AT_GOV", "RESUMED")
	case "status":
		printJSON(analytics.Measure(loadState(s)))
	case "symbolic":
		if len(args) < 2 {
			fail(fmt.Errorf("symbolic requires an EmergION id"))
		}
		em, ok := findEmergION(loadState(s), args[1])
		if !ok {
			fail(fmt.Errorf("EmergION %q not found", args[1]))
		}
		fmt.Print(em.Symbolic())
	case "actions":
		if len(args) < 2 {
			fail(fmt.Errorf("actions requires a REG-accepted EmergION id"))
		}

		st := loadState(s)
		em, ok := st.Accepted[args[1]]
		if !ok {
			fail(fmt.Errorf("EmergION %q is not REG-accepted", args[1]))
		}

		var facets []string
		if em.EVO.Metadata != nil {
			for _, facet := range em.EVO.Metadata.Facets {
				facets = append(facets, string(facet))
			}
		}

		actions := adapters.DeriveActionCandidates(
			facets,
			em.CAP,
			gemma.Validate() == nil,
		)

		printJSON(map[string]any{
			"emergion": em.IDN,
			"state":    em.STA,
			"actions":  actions,
		})
	case "authorize":
		if len(args) < 4 {
			fail(fmt.Errorf("authorize <emergion-id> <adapter> <action> [reason]"))
		}

		reasonText := ""
		if len(args) > 4 {
			reasonText = args[4]
		}

		rt := fieldruntime.Runtime{
			Store: s,
		}

		if _, err := rt.AuthorizeAction(
			args[1],
			args[2],
			args[3],
			reasonText,
			gemma.Validate() == nil,
		); err != nil {
			fail(err)
		}

		renderField(s, *output)
		fmt.Println(args[1], args[2], args[3], "AUTHORIZED")

	case "execute":
		if len(args) != 4 {
			fail(fmt.Errorf("usage: field execute <emergion-id> <adapter> <action>"))
		}

		rt := fieldruntime.Runtime{
			Store: s,
		}

		request, result, signal, duplicate, execErr := rt.ExecuteAction(
			context.Background(),
			args[1],
			args[2],
			args[3],
			gemma,
		)

		printJSON(map[string]any{
			"request":   request,
			"result":    result,
			"signal":    signal,
			"duplicate": duplicate,
		})

		if execErr != nil {
			fail(execErr)
		}

	case "safe-action":
		if len(args) != 1 {
			fail(fmt.Errorf("usage: field safe-action"))
		}

		rt := fieldruntime.Runtime{
			Store: s,
		}

		signal, executed, err := rt.ExecuteOneSafeAction(
			context.Background(),
			gemma,
		)
		if err != nil {
			fail(err)
		}

		if executed {
			renderField(s, *output)
			printJSON(map[string]any{
				"executed": true,
				"signal":   signal,
			})
		} else {
			printJSON(map[string]any{
				"executed": false,
			})
		}

	case "materialize":
		if len(args) != 2 {
			fail(fmt.Errorf("usage: field materialize <accepted-execution-result-id>"))
		}
		deliverable, err := (fieldruntime.Runtime{Store: s}).MaterializeAcceptedExecutionResult(args[1])
		if err != nil {
			fail(err)
		}
		receipt, artifact, err := publishDeliverable(*output, deliverable)
		if err != nil {
			fail(err)
		}
		printJSON(map[string]any{
			"artifact": artifact,
			"receipt":  filepath.Join(filepath.Dir(artifact), "receipt.json"),
			"lineage":  receipt,
		})

	case "render":
		out := *output
		if len(args) > 1 {
			out = args[1]
		}
		receipt := renderField(s, out)
		printJSON(map[string]any{
			"html":       filepath.Join(out, "field.html"),
			"receipt":    filepath.Join(out, "projection.current.json"),
			"projection": receipt,
		})
	case "verify":
		if _, err := s.Events(); err != nil {
			fail(err)
		}
		n, err := s.VerifyEvidence()
		if err != nil {
			fail(err)
		}
		removed, err := s.PruneOrphans()
		if err != nil {
			fail(err)
		}
		printJSON(map[string]any{"status": "PASS", "evidence_verified": n, "orphans_removed": removed})
	case "export-lineage":
		destination := "."
		if len(args) > 1 {
			destination = args[1]
		}
		receipt, err := exportLineage(destination)
		if err != nil {
			fail(err)
		}
		printJSON(receipt)
	case "adapters":
		printJSON(adapters.Catalog(gemma.Validate() == nil))
	case "daemon":
		handleDaemonCommand(args, *state)
	case "axioms":
		printJSON(axiom.Dictionary)
	default:
		usage()
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func usage() {
	fmt.Println(`field [flags] <command>

Commands:
  init                         initialize local state and dropzone
  doctor                       verify state, COSL chain, evidence, Gemma runtime and model
  capture <file>               analyze with local Gemma and create one GOV-ready EmergION
  implementation-evidence <file> preserve bounded implementation evidence as one GOV-ready EmergION
  rework <returned-id> <file>   re-enter a HUMAN_FINAL RETURNED EmergION with corrected source
  once                         process and clear the dropzone once
  run                          run the local living FIELD event loop; no server
  decide <id> <decision> [why] HUMAN_FINAL decision; approval is then REG-accepted
  resume <held-id> [why]       HUMAN_FINAL resume of a held EmergION back to GOV
  status                       CPU and FIELD metrics
  symbolic <id>                print native symbolic EmergION representation
  actions <id>                 derive read-only bounded actions from REG-accepted state
  authorize <id> <adapter> <action> [why] HUMAN_FINAL authorization for a derivable gated action
  execute <id> <adapter> <action> execute one governed local action and recapture its result
  safe-action                  execute at most one eligible bounded CAP_ONLY action
  materialize <accepted-execution-result-id>  read verified output from a REG-accepted successful execution result
  render [directory]           static JSON and HTML FIELD projection
  verify                       verify chain/evidence and remove orphan objects
  export-lineage [directory]   create and verify a complete named Git bundle for transfer
  adapters                     show bounded capability adapters
  axioms                       show immutable semantic axioms
  daemon <start|status|stop>   manage background runtime daemon via IPC

Environment:
  GEMMA_BIN, GEMMA_MODEL, GEMMA_THREADS, GEMMA_CONTEXT,
  GEMMA_MAX_TOKENS, GEMMA_TIMEOUT_SECONDS, GEMMA_EXTRA_ARGS,
  FIELD_HOME, FIELD_DROPZONE, FIELD_OUTPUT, FIELD_REASONER`)
}
