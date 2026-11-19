package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

const testToken = "fieldd-test-token-84721"

func govCandidate(t *testing.T) (*fieldapi.Runtime, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state")
	sourcePath := filepath.Join(tmpDir, "candidate.txt")
	if err := os.WriteFile(sourcePath, []byte("bounded HUMAN_FINAL fieldd auth proof"), 0600); err != nil {
		t.Fatal(err)
	}
	rt, err := fieldapi.Open(statePath, humanFinalTestReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	em, err := rt.Capture(context.Background(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	return rt, em.IDN, statePath
}

func postDecide(handler http.Handler, id, token string) *http.Response {
	body, _ := json.Marshal(DecideParams{
		EmergIONID: id,
		Decision:   "APPROVE",
		Reason:     "fieldd HTTP auth proof",
	})
	req := httptest.NewRequest(http.MethodPost, "/decide", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

func acceptedCount(t *testing.T, rt *fieldapi.Runtime) (accepted, atGOV int) {
	t.Helper()
	status, err := rt.Status()
	if err != nil {
		t.Fatal(err)
	}
	return status.Accepted, status.AtGOV
}

func TestUnauthenticatedMutationRejectedWithNoStateChange(t *testing.T) {
	rt, id, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)

	resp := postDecide(mux, id, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
	accepted, atGOV := acceptedCount(t, rt)
	if accepted != 0 {
		t.Fatalf("accepted = %d want 0 after unauthenticated decide", accepted)
	}
	if atGOV != 1 {
		t.Fatalf("at_gov = %d want 1 after unauthenticated decide", atGOV)
	}
}

func TestWrongTokenRejectedWithNoStateChange(t *testing.T) {
	rt, id, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)

	resp := postDecide(mux, id, "wrong-token")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", resp.StatusCode)
	}
	accepted, atGOV := acceptedCount(t, rt)
	if accepted != 0 || atGOV != 1 {
		t.Fatalf("state changed after wrong token: accepted=%d at_gov=%d", accepted, atGOV)
	}
}

func TestRightTokenDecideSucceeds(t *testing.T) {
	rt, id, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)

	resp := postDecide(mux, id, testToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d want 200 body=%s", resp.StatusCode, body)
	}
	var payload Response
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "" {
		t.Fatalf("decide error: %s", payload.Error)
	}
	accepted, atGOV := acceptedCount(t, rt)
	if accepted != 1 {
		t.Fatalf("accepted = %d want 1", accepted)
	}
	if atGOV != 0 {
		t.Fatalf("at_gov = %d want 0", atGOV)
	}
}

func TestReadOnlyStatusOpenOnLoopbackWithoutToken(t *testing.T) {
	rt, _, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	var payload Response
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "" {
		t.Fatalf("status error: %s", payload.Error)
	}
}

func TestReadOnlyStatusRequiresTokenOffLoopback(t *testing.T) {
	rt, _, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, false)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated non-loopback status = %d want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated non-loopback status = %d want 200", rec.Code)
	}
}

func TestAuthorizeRequiresBearerAndDoesNotChangeStateWithoutIt(t *testing.T) {
	rt, id, _ := govCandidate(t)
	mux := newHTTPMux(rt, testToken, true)
	body, _ := json.Marshal(AuthorizeParams{
		EmergIONID: id,
		Adapter:    "local",
		Action:     "OBS",
		Reason:     "should not run",
	})
	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rec.Code)
	}
	accepted, atGOV := acceptedCount(t, rt)
	if accepted != 0 || atGOV != 1 {
		t.Fatalf("authorize without token changed state: accepted=%d at_gov=%d", accepted, atGOV)
	}
}

func TestNonLoopbackWithoutTokenRefusesToStart(t *testing.T) {
	t.Setenv("FIELDD_TOKEN", "")
	stateDir := t.TempDir()
	_, _, err := resolveListenToken(stateDir, "0.0.0.0:8899")
	if err == nil {
		t.Fatal("expected non-loopback bind without token to be refused")
	}
	if !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("error = %q, want non-loopback refusal", err)
	}
	if _, statErr := os.Stat(tokenFilePath(stateDir)); !os.IsNotExist(statErr) {
		t.Fatal("refused non-loopback start must not create a token file")
	}
}

func TestLoopbackStartCreates0600TokenFile(t *testing.T) {
	t.Setenv("FIELDD_TOKEN", "")
	stateDir := t.TempDir()
	token, loopback, err := resolveListenToken(stateDir, "127.0.0.1:8899")
	if err != nil {
		t.Fatal(err)
	}
	if !loopback {
		t.Fatal("127.0.0.1 should be treated as loopback")
	}
	if token == "" {
		t.Fatal("expected generated token")
	}
	path := tokenFilePath(stateDir)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token mode = %o want 0600", info.Mode().Perm())
	}
}

func TestEnvTokenUsedWithoutWritingFile(t *testing.T) {
	t.Setenv("FIELDD_TOKEN", testToken)
	stateDir := t.TempDir()
	token, _, err := resolveListenToken(stateDir, "0.0.0.0:8899")
	if err != nil {
		t.Fatal(err)
	}
	if token != testToken {
		t.Fatalf("token = %q want env value", token)
	}
	if _, err := os.Stat(tokenFilePath(stateDir)); !os.IsNotExist(err) {
		t.Fatal("env token should not be written to the state directory")
	}
}

func TestExistingTokenFileAllowsNonLoopback(t *testing.T) {
	t.Setenv("FIELDD_TOKEN", "")
	stateDir := t.TempDir()
	path := tokenFilePath(stateDir)
	if err := os.WriteFile(path, []byte(testToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token, loopback, err := resolveListenToken(stateDir, "192.168.1.10:8899")
	if err != nil {
		t.Fatal(err)
	}
	if loopback {
		t.Fatal("192.168.1.10 should not be loopback")
	}
	if token != testToken {
		t.Fatalf("token = %q want file value", token)
	}
}

func TestTokenEqualRejectsEmptyWant(t *testing.T) {
	if tokenEqual("", "") || tokenEqual("x", "") {
		t.Fatal("empty configured token must never match")
	}
	if !tokenEqual(testToken, testToken) {
		t.Fatal("matching tokens should compare equal")
	}
	if tokenEqual("a", "b") {
		t.Fatal("different tokens should not compare equal")
	}
}

func TestMutatingIPCRequiresToken(t *testing.T) {
	rt, id, _ := govCandidate(t)
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		handleConn(rt, conn, testToken)
	}()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	params, err := json.Marshal(DecideParams{EmergIONID: id, Decision: "APPROVE", Reason: "ipc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(Request{Method: "decide", Params: params}); err != nil {
		t.Fatal(err)
	}
	var denied Response
	if err := dec.Decode(&denied); err != nil {
		t.Fatal(err)
	}
	if denied.Error != "unauthorized" {
		t.Fatalf("error = %q want unauthorized", denied.Error)
	}
	accepted, atGOV := acceptedCount(t, rt)
	if accepted != 0 || atGOV != 1 {
		t.Fatalf("unauthorized IPC decide changed state: accepted=%d at_gov=%d", accepted, atGOV)
	}

	if err := enc.Encode(Request{Method: "decide", Params: params, Token: testToken}); err != nil {
		t.Fatal(err)
	}
	var ok Response
	if err := dec.Decode(&ok); err != nil {
		t.Fatal(err)
	}
	if ok.Error != "" {
		t.Fatalf("authorized IPC decide failed: %s", ok.Error)
	}
	accepted, atGOV = acceptedCount(t, rt)
	if accepted != 1 || atGOV != 0 {
		t.Fatalf("authorized IPC decide state: accepted=%d at_gov=%d", accepted, atGOV)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		if !isLoopbackHost(host) {
			t.Fatalf("%s should be loopback", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "192.168.1.10", ""} {
		if isLoopbackHost(host) {
			t.Fatalf("%s should not be loopback", host)
		}
	}
}

func TestBearerTokenParsing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer  abc")
	if got := bearerToken(req); got != "abc" {
		t.Fatalf("got %q want abc", got)
	}
	req.Header.Set("Authorization", "bearer "+testToken)
	if got := bearerToken(req); got != testToken {
		t.Fatalf("scheme should be case-insensitive, got %q", got)
	}
	req.Header.Set("Authorization", "Basic x")
	if got := bearerToken(req); got != "" {
		t.Fatalf("non-bearer scheme returned %q", got)
	}
}

func TestHTTPBindHostEmptyIsDisabledLoopback(t *testing.T) {
	_, loopback, err := httpBindHost("")
	if err != nil {
		t.Fatal(err)
	}
	if !loopback {
		t.Fatal("disabled HTTP should be treated as loopback for token generation")
	}
	_, _, err = httpBindHost("not-an-addr")
	if err == nil {
		t.Fatal("expected invalid address error")
	}
}
