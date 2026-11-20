package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const livingBind = "127.0.0.1:8899"

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func getLiving(t *testing.T, h http.Handler, path, host, remote, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	req.RemoteAddr = remote
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func hasInjectedToken(body, token string) bool {
	want := livingTokenMeta(token)
	return strings.Contains(body, want) && !strings.Contains(body, livingTokenMetaEmpty)
}

func TestLivingTokenInjectedOnLoopbackWithExactHost(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, true, livingBind)

	for _, tc := range []struct {
		name, path, host string
	}{
		{"living 127.0.0.1", "/living", "127.0.0.1:8899"},
		{"living localhost", "/living", "localhost:8899"},
		{"alias 127.0.0.1", "/living-universe", "127.0.0.1:8899"},
		{"alias localhost case", "/living", "LOCALHOST:8899"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getLiving(t, mux, tc.path, tc.host, "127.0.0.1:54321", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d want 200", rec.Code)
			}
			body := rec.Body.String()
			if !hasInjectedToken(body, testToken) {
				t.Fatal("expected injected fieldd-token meta for loopback request with exact Host")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q want no-store", rec.Header().Get("Cache-Control"))
			}
			if rec.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatalf("Referrer-Policy = %q want no-referrer", rec.Header().Get("Referrer-Policy"))
			}
			for _, k := range []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Credentials",
				"Access-Control-Allow-Headers",
				"Access-Control-Allow-Methods",
			} {
				if v := rec.Header().Get(k); v != "" {
					t.Fatalf("must not set CORS header %s=%q", k, v)
				}
			}
			if !livingSafeInjectHeaders(rec.Header()) {
				t.Fatal("injected response failed livingSafeInjectHeaders")
			}
		})
	}

	// IPv6 loopback remote is still a loopback client; Host must still be exact.
	rec := getLiving(t, mux, "/living", "127.0.0.1:8899", "[::1]:54321", "")
	if rec.Code != http.StatusOK || !hasInjectedToken(rec.Body.String(), testToken) {
		t.Fatalf("::1 RemoteAddr with exact Host should inject, status=%d", rec.Code)
	}
}

func TestLivingTokenNotInjectedForWrongHostOrRemote(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, true, livingBind)
	plain := getLiving(t, mux, "/living", "example.com", "192.0.2.1:1234", "")
	if plain.Code != http.StatusOK {
		t.Fatalf("baseline status = %d", plain.Code)
	}

	for _, tc := range []struct {
		name, host, remote string
	}{
		{"dns rebinding Host", "evil.com:8899", "127.0.0.1:54321"},
		{"wrong port", "127.0.0.1:9999", "127.0.0.1:54321"},
		{"missing port", "127.0.0.1", "127.0.0.1:54321"},
		{"ipv6 Host", "[::1]:8899", "127.0.0.1:54321"},
		{"rebinding suffix", "127.0.0.1:8899.evil.com", "127.0.0.1:54321"},
		{"non-loopback remote", "127.0.0.1:8899", "192.0.2.1:1234"},
		{"default httptest remote+host", "example.com", "192.0.2.1:1234"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getLiving(t, mux, "/living", tc.host, tc.remote, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d want 200", rec.Code)
			}
			body := rec.Body.String()
			if hasInjectedToken(body, testToken) || strings.Contains(body, testToken) {
				t.Fatalf("token must not be injected for host=%q remote=%q", tc.host, tc.remote)
			}
			if !strings.Contains(body, livingTokenMetaEmpty) {
				t.Fatal("uninjected page must keep the empty fieldd-token meta")
			}
		})
	}
}

func TestLivingTokenNotInjectedOffLoopbackBind(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, false, "0.0.0.0:8899")
	rec := getLiving(t, mux, "/living", "127.0.0.1:8899", "127.0.0.1:54321", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if hasInjectedToken(rec.Body.String(), testToken) || strings.Contains(rec.Body.String(), livingTokenMeta(testToken)) {
		t.Fatal("non-loopback bind must not inject the token")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("must not add CORS headers")
	}
}

func TestLivingTokenNotInjectedWithoutBindAddr(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMux(nil, testToken, true)
	rec := getLiving(t, mux, "/living", "127.0.0.1:8899", "127.0.0.1:54321", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), livingTokenMeta(testToken)) {
		t.Fatal("newHTTPMux without bind address must not inject")
	}
}

func TestApplyLivingTokenRequiresNoStoreAndNoReferrer(t *testing.T) {
	page := "<head>" + livingTokenMetaEmpty + "</head>"
	h := make(http.Header)
	got, ok := applyLivingToken(page, testToken, h)
	if !ok || !strings.Contains(got, livingTokenMeta(testToken)) {
		t.Fatal("applyLivingToken should inject when it can set safe headers")
	}
	if !livingSafeInjectHeaders(h) {
		t.Fatal("injected headers must pass livingSafeInjectHeaders")
	}

	// No no-store: must not inject.
	empty := make(http.Header)
	if livingSafeInjectHeaders(empty) {
		t.Fatal("headers missing no-store must not be considered safe to inject")
	}
	noStoreMissing := make(http.Header)
	noStoreMissing.Set("Referrer-Policy", "no-referrer")
	if livingSafeInjectHeaders(noStoreMissing) {
		t.Fatal("no-store header missing: must not inject")
	}
	noRef := make(http.Header)
	noRef.Set("Cache-Control", "no-store")
	if livingSafeInjectHeaders(noRef) {
		t.Fatal("Referrer-Policy missing: must not inject")
	}
	cors := make(http.Header)
	cors.Set("Cache-Control", "no-store")
	cors.Set("Referrer-Policy", "no-referrer")
	cors.Set("Access-Control-Allow-Origin", "*")
	if livingSafeInjectHeaders(cors) {
		t.Fatal("CORS header present: must not inject")
	}

	// A header map that cannot accept the required values stays uninjected.
	blocked := http.Header{}
	blocked.Set("Cache-Control", "public")
	// applyLivingToken overwrites Cache-Control/Referrer-Policy, then re-checks.
	// Simulate the gate alone: a response without no-store is refused.
	if livingSafeInjectHeaders(blocked) {
		t.Fatal("public cache is not safe for token injection")
	}
}

func TestLivingMutatingRoutesStillRequireBearer(t *testing.T) {
	chdirRepoRoot(t)
	mux := newHTTPMuxOn(nil, testToken, true, livingBind)
	page := getLiving(t, mux, "/living", "127.0.0.1:8899", "127.0.0.1:1", "")
	if !hasInjectedToken(page.Body.String(), testToken) {
		t.Fatal("setup: expected injected page")
	}
	req := httptest.NewRequest(http.MethodPost, "/decide", strings.NewReader(`{"emergion_id":"E-x","decision":"APPROVE","reason":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:8899"
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /decide without bearer = %d want 401", rec.Code)
	}
}

func TestLivingPageUsesInjectedTokenAutomatically(t *testing.T) {
	src := livingUniverseHTML(t)
	for _, want := range []string{
		livingTokenMetaEmpty,
		`function injectedToken()`,
		`document.querySelector('meta[name="fieldd-token"]')`,
		`const t=injectedToken();if(t)return t`,
		`if(injectedToken()){w.append(acceptButton(id));return w}`,
		`'Authorization':'Bearer '+tok`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("living-universe.html missing %q", want)
		}
	}
	// Paste UI and message stay as a fallback, but are skipped when a token was injected.
	if !strings.Contains(src, "Paste the fieldd token first (Next Governed Step panel).") {
		t.Fatal("fallback paste message missing")
	}
	if !strings.Contains(src, "if(!injectedToken())alert('Paste the fieldd token first") {
		t.Fatal("paste alert must be skipped when a token was injected")
	}
	// Static source must not itself contain a real token.
	if strings.Contains(src, testToken) {
		t.Fatal("static page must not contain the test token")
	}
}
