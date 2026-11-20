package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The HTTP mux is built in newHTTPMux(); registering one pattern twice makes
// net/http panic at startup. Every route must be registered once, and the
// full set must register on a fresh mux without panicking.
func TestFielddRoutesRegisteredOnce(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	seen := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			seen[m[1]]++
		}
	}
	for _, p := range []string{"/status", "/field.json", "/living-universe", "/living", "/field.html", "/decide", "/authorize", "/actions", "/real_temporal_wave.json", "/deliverables/"} {
		if seen[p] == 0 {
			t.Fatalf("route %s not registered", p)
		}
	}
	mux := http.NewServeMux()
	for p, n := range seen {
		if n != 1 {
			t.Fatalf("route %s registered %d times", p, n)
		}
		mux.HandleFunc(p, func(http.ResponseWriter, *http.Request) {})
	}
}
