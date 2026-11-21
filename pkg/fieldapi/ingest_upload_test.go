package fieldapi

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
)

func TestIngestSourceUploadIdempotentAtGOV(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	keep := filepath.Join(dir, "sources")
	rt, err := Open(state, reason.Heuristic{})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("android app phase-1 source upload proof\n")
	first, err := rt.IngestSourceUpload(context.Background(), "notes/Hello World.txt", "Hello", "ops", "text/plain", data, keep)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Duplicate || !first.AtGOV || first.Bytes != int64(len(data)) || len(first.SourceSHA256) != 64 {
		t.Fatalf("first ingest = %+v", first)
	}
	st, err := rt.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.AtGOV != 1 || st.Accepted != 0 {
		t.Fatalf("status after ingest: at_gov=%d accepted=%d", st.AtGOV, st.Accepted)
	}
	kept := filepath.Join(keep, first.SourceSHA256)
	got, err := os.ReadFile(kept)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("kept source bytes do not match the upload")
	}

	second, err := rt.IngestSourceUpload(context.Background(), "uploads/Hello_World.txt", "Hello", "ops", "text/plain", data, keep)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.ID != first.ID || second.SourceSHA256 != first.SourceSHA256 {
		t.Fatalf("second ingest = %+v want duplicate of %s", second, first.ID)
	}
	st, err = rt.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.AtGOV != 1 || st.Accepted != 0 {
		t.Fatalf("duplicate changed state: at_gov=%d accepted=%d", st.AtGOV, st.Accepted)
	}
}

func TestIngestSourceUploadRejectsEmpty(t *testing.T) {
	rt, err := Open(t.TempDir(), reason.Heuristic{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.IngestSourceUpload(context.Background(), "a.txt", "", "", "", nil, ""); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestUploadRelPath(t *testing.T) {
	if got := uploadRelPath(`/etc/passwd`); got != "uploads/passwd" {
		t.Fatalf("absolute: %s", got)
	}
	if got := uploadRelPath(`..\\..\\secret.txt`); got != "uploads/secret.txt" {
		t.Fatalf("dots: %s", got)
	}
	if got := uploadRelPath(""); got != "uploads/upload.bin" {
		t.Fatalf("empty: %s", got)
	}
}
