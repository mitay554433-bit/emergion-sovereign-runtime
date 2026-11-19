package main

import (
	"emergion-sovereign-runtime/internal/gemmalive"
	"encoding/json"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

func TestNativeBridgeLogic(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state")

	rt, err := fieldapi.Open(statePath, reason.GemmaCLI{Binary: gemmalive.AbsentBinary, Model: gemmalive.AbsentModel})
	if err != nil {
		t.Fatalf("failed to open runtime: %v", err)
	}

	// 1. Verify StatusJSON output via fieldapi
	wire, err := rt.StatusJSON()
	if err != nil {
		t.Fatalf("StatusJSON failed: %v", err)
	}

	var statusMap map[string]interface{}
	if err := json.Unmarshal([]byte(wire), &statusMap); err != nil {
		t.Fatalf("StatusJSON produced invalid JSON: %v", err)
	}

	// 2. Verify REG acceptance rejection on ActionsJSON
	_, err = rt.ActionsJSON("E-NONEXISTENT", false)
	if err == nil {
		t.Fatal("expected error for non-REG accepted ID, got nil")
	}

	// 3. Verify DecideBinding path
	err = rt.DecideBinding("E-NONEXISTENT", "APPROVE", "test decision")
	if err == nil {
		t.Fatal("expected error for non-existent decision target, got nil")
	}
}
