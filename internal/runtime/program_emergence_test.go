package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

type programEmergenceReasoner struct{}

func (programEmergenceReasoner) Analyze(context.Context, reason.Input) (reason.Result, error) {
	return reason.Result{}, nil
}
func (programEmergenceReasoner) Name() string                   { return "program-emergence-test" }
func (programEmergenceReasoner) Version(context.Context) string { return "1" }

func TestProposeOneProgramPatchAdmitsPatchAtGOVWithRuntimeOrigin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
		moduleFile := filepath.Join(repo, "go.mod")
		if err := os.WriteFile(moduleFile, []byte("module program-emergence-proof\n\ngo 1.23\n"), 0o600); err != nil {
			t.Fatal(err)
		}

	target := "internal/a.go"
	original := []byte("package internal\n\nconst value = 1\n")
	if err := os.WriteFile(filepath.Join(repo, target), original, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repo, "add", target, "go.mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	preserved, err := s.Preserve(original)
	if err != nil {
		t.Fatal(err)
	}
	parent := core.EmergION{
		IDN: "E-PROGRAM-PARENT",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: preserved.Hash,
			Codec:      preserved.Codec,
			Bytes:      preserved.Bytes,
			Stored:     preserved.Stored,
			Summary:    "accepted program source",
			Provenance: "test",
		},
		REL: map[string]string{"source_name": target},
		CAP: []string{"PROGRAM"},
		VAL: core.Validation{Facts: []string{"source_preserved"}, Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1, Metadata: &core.Metadata{
			CapturedAt:   time.Now().UTC(),
			Topology:     core.TopologyDodecahedronV1,
			PromptSchema: "MXPD/2",
			Facets:       []core.Facet{core.FacetProgramForge},
		}},
	}
	if _, err := s.SaveCandidate(parent); err != nil {
		t.Fatal(err)
	}
	approved, decision, err := gov.Decide(parent, gov.Approve, "HUMAN_FINAL", "accept program source")
	if err != nil {
		t.Fatal(err)
	}
	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "fake-gemma")
	script := "#!/bin/sh\n" +
		"cat <<'PATCH'\n" +
		"EDIT/1\n" +
		"TARGET:internal/a.go\n" +
		"===OLD===\n" +
		"const value = 1\n" +
		"===NEW===\n" +
		"const value = 2\n" +
		"===END===\n" +
		"PATCH\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	r := Runtime{Store: s, Reasoner: programEmergenceReasoner{}}
	em, proposed, err := r.ProposeOneProgramPatch(context.Background(), reason.GemmaCLI{
		Binary:    fake,
		Model:     model,
		Context:   2048,
		MaxTokens: 80,
		Threads:   1,
		Timeout:   5 * time.Second,
	}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !proposed {
		t.Fatal("program patch was not proposed")
	}
	if em.STA != core.StateAtGOV || !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatalf("proposal not GOV-ready: %#v", em)
	}
	if em.REL["origin"] != parent.IDN {
		t.Fatalf("origin = %q want %q", em.REL["origin"], parent.IDN)
	}
	if em.REL["source_kind"] != "PROGRAM_PATCH" {
		t.Fatalf("source_kind = %q", em.REL["source_kind"])
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.AtGOV[em.IDN]; !ok {
		t.Fatal("proposal missing from GOV")
	}
	if _, ok := state.Accepted[em.IDN]; ok {
		t.Fatal("proposal self-authorized into REG")
	}
        approvedPatch, decision, err := gov.Decide(
                em,
                gov.Approve,
                "HUMAN_FINAL",
                "approve generated PROGRAM patch",
        )
        if err != nil {
                t.Fatal(err)
        }

        decisionID, err = s.SaveDecision(decision)
        if err != nil {
                t.Fatal(err)
        }

        acceptedPatch, receipt, err := reg.Accept(approvedPatch, decisionID)
        if err != nil {
                t.Fatal(err)
        }
        if _, err := s.SaveAccepted(receipt); err != nil {
                t.Fatal(err)
        }

        if _, err := r.AuthorizeAction(
                acceptedPatch.IDN,
                "GITHUB",
                "PROGRAM",
                "authorize generated PROGRAM patch",
                false,
        ); err != nil {
                t.Fatal(err)
        }

        oldDir, err := os.Getwd()
        if err != nil {
                t.Fatal(err)
        }
        if err := os.Chdir(repo); err != nil {
                t.Fatal(err)
        }
        defer os.Chdir(oldDir)

        request, result, signal, duplicate, err := r.ExecuteAction(
                context.Background(),
                acceptedPatch.IDN,
                "GITHUB",
                "PROGRAM",
                reason.GemmaCLI{},
        )
        if err != nil {
                t.Fatal(err)
        }
        if duplicate {
                t.Fatal("PROGRAM execution RECAPTURE unexpectedly duplicate")
        }
        if !result.Succeeded {
                t.Fatalf("PROGRAM execution did not succeed: %#v", result)
        }
        if request.AuthorizationID == "" {
                t.Fatal("PROGRAM execution missing authorization identity")
        }
        if signal.STA != core.StateAtGOV {
                t.Fatalf("PROGRAM execution RECAPTURE state = %q want %q", signal.STA, core.StateAtGOV)
        }
        if !signal.VAL.Recoil || !signal.VAL.WVC {
                t.Fatal("PROGRAM execution RECAPTURE did not pass RECOIL/WVC")
        }
        if signal.REL["parent_emergion"] != acceptedPatch.IDN {
                t.Fatalf(
                        "PROGRAM execution parent = %q want %q",
                        signal.REL["parent_emergion"],
                        acceptedPatch.IDN,
                )
        }

        finalSource, err := os.ReadFile(filepath.Join(repo, target))
        if err != nil {
                t.Fatal(err)
        }
        if string(finalSource) != "package internal\n\nconst value = 2\n" {
                t.Fatalf("PROGRAM patch did not change source: %s", finalSource)
        }

        finalState, err := livefield.Rebuild(mustEvents(t, s))
        if err != nil {
                t.Fatal(err)
        }
        if _, ok := finalState.Accepted[acceptedPatch.IDN]; !ok {
                t.Fatal("accepted PROGRAM patch disappeared from FIELD")
        }
        if _, ok := finalState.AtGOV[signal.IDN]; !ok {
                t.Fatal("PROGRAM execution RECAPTURE did not return to GOV")
        }
}
