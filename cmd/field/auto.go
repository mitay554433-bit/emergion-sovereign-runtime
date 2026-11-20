package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/worker"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// runAutoCommand handles `policy`, `work` and `undo-auto`. It returns false
// when cmd is not one of them.
func runAutoCommand(state string, args []string) bool {
	switch args[0] {
	case "policy":
		// field policy auto-accept <status|on|off> [-reason R]
		if len(args) < 3 || args[1] != "auto-accept" {
			fail(fmt.Errorf("policy auto-accept <status|on|off> [-reason R]"))
		}
		fs := flag.NewFlagSet("policy", flag.ContinueOnError)
		why := fs.String("reason", "", "HUMAN_FINAL reason (required for on/off)")
		if err := fs.Parse(args[3:]); err != nil {
			fail(err)
		}
		rt, err := fieldapi.Open(state, reason.Heuristic{})
		if err != nil {
			fail(err)
		}
		switch args[2] {
		case "status":
		case "on", "off":
			// This command IS the HUMAN_FINAL press for the switch.
			id, err := rt.SetAutoPolicy(context.Background(), args[2] == "on", *why)
			if err != nil {
				fail(err)
			}
			fmt.Println(id, "AUTO_POLICY_SWITCH", strings.ToUpper(args[2]), "REG_ACCEPTED")
		default:
			fail(fmt.Errorf("policy auto-accept <status|on|off>"))
		}
		ps, err := rt.PolicyStatus()
		if err != nil {
			fail(err)
		}
		printJSON(ps)
	case "work":
		// field work [-once] [-batch N] [-mode local|online|auto|heuristic] [-interval D]
		fs := flag.NewFlagSet("work", flag.ContinueOnError)
		once := fs.Bool("once", false, "run one bounded step and exit")
		batch := fs.Int("batch", worker.DefaultBatch, "sources per step")
		mode := fs.String("mode", envOr("UNIFUSION_REASONER_MODE", reason.ModeLocal), "reasoner: local, online, auto or heuristic")
		interval := fs.Duration("interval", worker.DefaultInterval, "pause between steps")
		dir := fs.String("work-dir", "", "control/checkpoint directory (default <state>-work)")
		sources := fs.String("sources-root", os.Getenv("UNIFUSION_SOURCES_ROOT"), "optional directory to re-hash SOURCE files before auto-accept")
		if err := fs.Parse(args[1:]); err != nil {
			fail(err)
		}
		if !reason.ValidMode(*mode) {
			fail(fmt.Errorf("unknown mode %q", *mode))
		}
		if *dir == "" {
			*dir = worker.WorkDir(state)
		}
		rt, err := fieldapi.Open(state, reason.Heuristic{})
		if err != nil {
			fail(err)
		}
		sw := reason.NewSwitchInsighter(*mode)
		w := rt.NewWorker(*dir, sw)
		w.SourcesRoot = *sources
		w.TextChars = worker.TextCharsFromEnv()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		for {
			pol, err := rt.ApplyAutoPolicy(0, *sources)
			if err != nil {
				fail(err)
			}
			rep, err := w.Step(ctx, *batch)
			if err != nil {
				fail(err)
			}
			pol2, err := rt.ApplyAutoPolicy(0, *sources)
			if err != nil {
				fail(err)
			}
			fmt.Printf("WORK processed %d insights %d links %d failed %d pending %d reasoner %s auto_accepted %d auto_held %d\n",
				rep.Processed, rep.Insights, rep.Links, rep.Failed, rep.Pending, rep.Reasoner,
				pol.Accepted+pol2.Accepted, pol.Held+pol2.Held)
			if *once || ctx.Err() != nil {
				return true
			}
			select {
			case <-ctx.Done():
				return true
			case <-time.After(*interval):
			}
		}
	case "undo-auto":
		if len(args) < 3 {
			fail(fmt.Errorf("undo-auto <id> <reason>"))
		}
		rt, err := fieldapi.Open(state, reason.Heuristic{})
		if err != nil {
			fail(err)
		}
		out, err := rt.UndoAuto(args[1], strings.Join(args[2:], " "))
		if err != nil {
			fail(err)
		}
		fmt.Println(args[1], out)
	default:
		return false
	}
	return true
}
