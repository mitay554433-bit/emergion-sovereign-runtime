package main

import (
	"strings"
	"testing"
)

func TestLivingHasSourceSubmitWithoutCyan(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		`id="upGo"`,
		"Submit a source",
		"POST /sources",
		"Confirm submit",
		"armSourceSubmit",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living is missing %q", want)
		}
	}
	if !strings.Contains(src, "Press again to record this file at AtGOV") {
		t.Fatal("submit is missing the two-press confirm")
	}
	low := strings.ToLower(src)
	for _, bad := range []string{"#00ffff", "#0ff0", "#22d3ee", "#00e5ff", "color:cyan", "fill:cyan"} {
		if strings.Contains(low, bad) {
			t.Fatalf("living uses cyan color %q", bad)
		}
	}
}
