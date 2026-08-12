package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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
func renderField(s *store.Store, out string) {
	if _, err := proj.EnsureOutput(out); err != nil {
		fail(err)
	}
	st := loadState(s)
	if err := proj.JSON(filepath.Join(out, "field.json"), st); err != nil {
		fail(err)
	}
	if err := proj.HTML(filepath.Join(out, "field.html"), st); err != nil {
		fail(err)
	}
}

func openField(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var commands [][]string
	if runtime.GOOS == "windows" {
		commands = [][]string{{"rundll32", "url.dll,FileProtocolHandler", abs}}
	} else if runtime.GOOS == "darwin" {
		commands = [][]string{{"open", abs}}
	} else {
		commands = [][]string{{"termux-open", abs}, {"xdg-open", abs}}
	}
	for _, command := range commands {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		return exec.Command(command[0], command[1:]...).Start()
	}
	return fmt.Errorf("no system file opener found; open %s", abs)
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
	case "once":
		rt := fieldruntime.Runtime{Store: s, Reasoner: mkReasoner()}
		ids, err := rt.Once(context.Background(), *dropzone)
		if err != nil {
			fail(err)
		}
		renderField(s, *output)
		printJSON(map[string]any{"captured": ids, "dropzone_cleared": true})
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		rt := fieldruntime.Runtime{Store: s, Reasoner: mkReasoner()}
		fmt.Println("LIVING_FIELD", filepath.Clean(*dropzone), "reasoner="+*reasonerName)
		if err := rt.Run(ctx, *dropzone, *poll, func(id string) { renderField(s, *output); fmt.Println(id, "AT_GOV") }); err != nil {
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
	case "accept":
		if len(args) < 2 {
			fail(fmt.Errorf("accept <id>"))
		}
		st := loadState(s)
		em, ok := st.Approved[args[1]]
		if !ok {
			fail(fmt.Errorf("candidate is not HUMAN_FINAL approved"))
		}
		decisionID, err := s.ApprovingDecisionID(em.IDN)
		if err != nil {
			fail(err)
		}
		_, regReceipt, err := reg.Accept(em, decisionID)
		if err != nil {
			fail(err)
		}
		if _, err = s.SaveAccepted(regReceipt); err != nil {
			fail(err)
		}
		renderField(s, *output)
		fmt.Println(em.IDN, "REG_ACCEPTED")
	case "status":
		printJSON(analytics.Measure(loadState(s)))
	case "render":
		out := *output
		if len(args) > 1 {
			out = args[1]
		}
		if _, err := proj.EnsureOutput(out); err != nil {
			fail(err)
		}
		st := loadState(s)
		if err := proj.JSON(filepath.Join(out, "field.json"), st); err != nil {
			fail(err)
		}
		if err := proj.HTML(filepath.Join(out, "field.html"), st); err != nil {
			fail(err)
		}
		fmt.Println(filepath.Join(out, "field.html"))
	case "open":
		renderField(s, *output)
		path := filepath.Join(*output, "field.html")
		if err := openField(path); err != nil {
			fail(err)
		}
		fmt.Println(path)
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
	case "adapters":
		printJSON(adapters.Catalog(gemma.Validate() == nil))
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
  once                         process and clear the dropzone once
  run                          run the local living FIELD event loop; no server
  decide <id> <decision> [why] HUMAN_FINAL decision; approval is then REG-accepted
  accept <id>                  resume REG acceptance after an interrupted approval
  status                       CPU and FIELD metrics
  render [directory]           static JSON and HTML FIELD projection
  open                         render and open the FIELD registry
  verify                       verify chain/evidence and remove orphan objects
  adapters                     show bounded capability adapters
  axioms                       show immutable semantic axioms

Environment:
  GEMMA_BIN, GEMMA_MODEL, GEMMA_THREADS, GEMMA_CONTEXT,
  GEMMA_MAX_TOKENS, GEMMA_TIMEOUT_SECONDS, GEMMA_EXTRA_ARGS,
  FIELD_HOME, FIELD_DROPZONE, FIELD_OUTPUT, FIELD_REASONER`)
}
