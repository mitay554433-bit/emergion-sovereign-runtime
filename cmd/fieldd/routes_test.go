package main

import (
	"net/http"
	"os"
	"regexp"
	"testing"
)

// The HTTP mux is built inline in main(); registering one pattern twice makes
// net/http panic at startup. Every route in main.go must be registered once,
// and the full set must register on a fresh mux without panicking.
func TestFielddRoutesRegisteredOnce(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	seen := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]]++
	}
	for _, p := range []string{"/status", "/field.json", "/living-universe", "/field.html"} {
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
