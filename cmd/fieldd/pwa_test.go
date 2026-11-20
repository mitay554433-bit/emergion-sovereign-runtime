package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestPWARoutesContentTypes(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, true, livingBind)
	for _, tc := range []struct{ path, ctype string }{
		{"/manifest.webmanifest", "application/manifest+json"},
		{"/sw.js", "text/javascript; charset=utf-8"},
		{"/icon.svg", "image/svg+xml"},
		{"/icon-192.png", "image/png"},
		{"/icon-512.png", "image/png"},
	} {
		rec := getLiving(t, mux, tc.path, "127.0.0.1:8899", "127.0.0.1:54321", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d want 200", tc.path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != tc.ctype {
			t.Fatalf("%s Content-Type = %q want %q", tc.path, got, tc.ctype)
		}
		if body := rec.Body.String(); strings.Contains(body, testToken) {
			t.Fatalf("%s must never carry the fieldd token", tc.path)
		}
	}
}

func TestPWAManifestFields(t *testing.T) {
	var m struct {
		Name, ShortName, StartURL, Scope, Display, BackgroundColor, ThemeColor string
		Icons                                                                  []struct{ Src, Sizes, Type, Purpose string }
	}
	raw := strings.NewReplacer(`"short_name"`, `"ShortName"`, `"start_url"`, `"StartURL"`, `"background_color"`, `"BackgroundColor"`, `"theme_color"`, `"ThemeColor"`).Replace(pwaManifest)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.Name != "UNIFUSION" || m.Display != "standalone" || m.StartURL != "/living" {
		t.Fatalf("manifest name/display/start_url = %q/%q/%q", m.Name, m.Display, m.StartURL)
	}
	if m.ThemeColor != "#0a0a0a" || m.BackgroundColor != "#0a0a0a" {
		t.Fatalf("theme/background = %q/%q want #0a0a0a", m.ThemeColor, m.BackgroundColor)
	}
	sizes := map[string]bool{}
	for _, ic := range m.Icons {
		if ic.Type == "image/png" {
			sizes[ic.Sizes] = true
		}
	}
	if !sizes["192x192"] || !sizes["512x512"] {
		t.Fatal("manifest needs 192 and 512 PNG icons for install")
	}
}

func TestPWAIconsArePNGInPalette(t *testing.T) {
	a, b := pwaIcons()
	for _, tc := range []struct {
		data []byte
		size int
	}{{a, 192}, {b, 512}} {
		img, err := png.Decode(bytes.NewReader(tc.data))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != tc.size || img.Bounds().Dy() != tc.size {
			t.Fatalf("icon size %v want %d", img.Bounds(), tc.size)
		}
		r, g, bl, _ := img.At(2, 2).RGBA()
		if r>>8 != 0x0a || g>>8 != 0x0a || bl>>8 != 0x0a {
			t.Fatalf("icon corner not carbon black: %x %x %x", r>>8, g>>8, bl>>8)
		}
	}
	for _, src := range []string{pwaManifest, pwaIconSVG, pwaSWSource} {
		low := strings.ToLower(src)
		for _, banned := range []string{"cyan", "#0ff", "#00ffff", "rainbow"} {
			if strings.Contains(low, banned) {
				t.Fatalf("palette violation %q in PWA asset", banned)
			}
		}
	}
}

func TestPWAServiceWorkerNeverCachesDataOrDecide(t *testing.T) {
	sw := pwaSWSource
	m := regexp.MustCompile(`const SHELL = \[([^\]]*)\];`).FindStringSubmatch(sw)
	if m == nil {
		t.Fatal("sw.js has no SHELL list")
	}
	shell := map[string]bool{}
	for _, s := range strings.Split(m[1], ",") {
		shell[strings.Trim(strings.TrimSpace(s), "'")] = true
	}
	allowed := map[string]bool{"/manifest.webmanifest": true, "/icon.svg": true, "/icon-192.png": true, "/icon-512.png": true}
	for s := range shell {
		if !allowed[s] {
			t.Fatalf("sw.js SHELL caches %q; only static shell assets are allowed", s)
		}
	}
	for _, never := range []string{"/field.json", "/projection.json", "/events.json", "/decide", "/decide-batch-human", "/authorize", "/actions", "/status", "/living", "/living-universe", "/real_temporal_wave.json", "/field.html"} {
		if shell[never] {
			t.Fatalf("sw.js SHELL must never contain %s", never)
		}
		found := false
		for _, n := range pwaNeverCache {
			if n == never {
				found = true
			}
		}
		if !found {
			t.Fatalf("pwaNeverCache missing %s", never)
		}
	}
	// The only write into Cache Storage is the install-time addAll(SHELL).
	for _, bad := range []string{"cache.put", ".put(", "addAll(NEVER", "add(req", "add(e.request"} {
		if strings.Contains(sw, bad) {
			t.Fatalf("sw.js must not write responses into the cache at runtime (%q)", bad)
		}
	}
	if strings.Count(sw, "addAll(") != 1 || !strings.Contains(sw, "addAll(SHELL)") {
		t.Fatal("sw.js must cache exactly the SHELL list at install")
	}
	if !strings.Contains(sw, "fetch(req, {cache: 'no-store'})") {
		t.Fatal("sw.js must fetch /living with no-store so the token meta is fresh per request")
	}
	if !strings.Contains(sw, "if (req.method !== 'GET') return;") {
		t.Fatal("sw.js must pass non-GET (decide/authorize) straight through")
	}
}

func TestLivingStillInjectsTokenNoStoreWithPWA(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, true, livingBind)
	for _, path := range []string{"/living", "/living-universe"} {
		rec := getLiving(t, mux, path, "127.0.0.1:8899", "127.0.0.1:54321", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !hasInjectedToken(body, testToken) {
			t.Fatalf("%s lost the injected fieldd-token meta", path)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s Cache-Control = %q want no-store", path, rec.Header().Get("Cache-Control"))
		}
		if !strings.Contains(body, `<link rel="manifest" href="/manifest.webmanifest">`) || !strings.Contains(body, `serviceWorker.register("/sw.js"`) {
			t.Fatalf("%s missing manifest link or sw registration", path)
		}
	}
	// A foreign Host still gets no token, PWA or not.
	rec := getLiving(t, mux, "/living", "evil.example:8899", "127.0.0.1:54321", "")
	if hasInjectedToken(rec.Body.String(), testToken) {
		t.Fatal("token injected for non-exact Host")
	}
}

func TestPWARoutesFollowBearerPolicyOffLoopback(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, false, "0.0.0.0:8899")
	rec := getLiving(t, mux, "/sw.js", "10.0.0.2:8899", "10.0.0.9:5555", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("non-loopback /sw.js without bearer = %d want 401", rec.Code)
	}
}
