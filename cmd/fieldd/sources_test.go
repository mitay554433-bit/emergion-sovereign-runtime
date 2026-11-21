package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

func TestPostSourcesRecordsAtGOVAndApprove(t *testing.T) {
	tmp := t.TempDir()
	keep := filepath.Join(tmp, "sources")
	t.Setenv("UNIFUSION_SOURCES_DIR", keep)
	rt, err := fieldapi.Open(filepath.Join(tmp, "state"), humanFinalTestReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	mux := newHTTPMux(rt, testToken, true)

	body, ctype := multipartFile(t, "file", "hello.txt", []byte("sideload app source\n"), map[string]string{"title": "Hello", "topic": "ops"})
	post := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/sources", bytes.NewReader(body))
		req.Header.Set("Content-Type", ctype)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if a, g := acceptedCount(t, rt); a != 0 || g != 0 {
		t.Fatalf("state changed without token: accepted=%d at_gov=%d", a, g)
	}

	rec := post(testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.Bytes())
	}
	var res fieldapi.IngestUploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.ID == "" || res.Duplicate || !res.AtGOV {
		t.Fatalf("upload result %+v", res)
	}
	if a, g := acceptedCount(t, rt); a != 0 || g != 1 {
		t.Fatalf("after upload: accepted=%d at_gov=%d", a, g)
	}
	if _, err := os.Stat(filepath.Join(keep, res.SourceSHA256)); err != nil {
		t.Fatalf("kept source: %v", err)
	}

	again := post(testToken)
	var dup fieldapi.IngestUploadResult
	if err := json.Unmarshal(again.Body.Bytes(), &dup); err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || dup.ID != res.ID {
		t.Fatalf("duplicate result %+v", dup)
	}

	decide := postDecide(mux, res.ID, testToken)
	out, _ := io.ReadAll(decide.Body)
	if decide.StatusCode != http.StatusOK {
		t.Fatalf("decide: %d %s", decide.StatusCode, out)
	}
	if a, g := acceptedCount(t, rt); a != 1 || g != 0 {
		t.Fatalf("after approve: accepted=%d at_gov=%d", a, g)
	}
}

func TestPostSourcesMethodAndSocketFlag(t *testing.T) {
	if socketEnabled("") || socketEnabled("off") || socketEnabled("OFF") || !socketEnabled("/tmp/fieldd.sock") {
		t.Fatal("socketEnabled")
	}
	rt, err := fieldapi.Open(t.TempDir(), humanFinalTestReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	mux := newHTTPMux(rt, testToken, true)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /sources: %d", rec.Code)
	}
}

func multipartFile(t *testing.T, field, name string, data []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), w.FormDataContentType()
}
