package store

import (
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func TestCheckExpectedTip(t *testing.T) {
	events := []core.Event{
		{ID: "EV-1", SelfHash: "aaa"},
		{ID: "EV-2", SelfHash: "bbb"},
	}

	if err := CheckExpectedTip(events, "", nil); err != nil {
		t.Fatalf("no expectation should be a no-op: %v", err)
	}
	if err := CheckExpectedTip(events, "bbb", nil); err != nil {
		t.Fatalf("matching tip: %v", err)
	}
	count := 2
	if err := CheckExpectedTip(events, "bbb", &count); err != nil {
		t.Fatalf("matching tip and count: %v", err)
	}

	err := CheckExpectedTip(events, "ccc", nil)
	if err == nil {
		t.Fatal("mismatched tip unexpectedly passed")
	}
	if !strings.Contains(err.Error(), "expected tip ccc") || !strings.Contains(err.Error(), "actual bbb") {
		t.Fatalf("error did not name expected vs actual tip: %v", err)
	}

	wrong := 3
	err = CheckExpectedTip(events, "", &wrong)
	if err == nil {
		t.Fatal("mismatched count unexpectedly passed")
	}
	if !strings.Contains(err.Error(), "expected count 3") || !strings.Contains(err.Error(), "actual 2") {
		t.Fatalf("error did not name expected vs actual count: %v", err)
	}

	truncated := events[:1]
	err = CheckExpectedTip(truncated, "bbb", &count)
	if err == nil {
		t.Fatal("deleted tip unexpectedly passed an earlier receipt")
	}
	msg := err.Error()
	if !strings.Contains(msg, "expected tip bbb") || !strings.Contains(msg, "actual aaa") {
		t.Fatalf("deleted tip error missing hashes: %v", err)
	}
	if !strings.Contains(msg, "expected count 2") || !strings.Contains(msg, "actual 1") {
		t.Fatalf("deleted tip error missing counts: %v", err)
	}
}
