package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/store"
)

func ingestLine(i int) string {
	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint("upload-", i))))
	return fmt.Sprintf(`{"path":"sources/topic-%d/file-%d.docx","sha256":"%s","bytes":%d,"mime":"application/pdf","topic":"topic-%d","title":"Upload %d","source_kind":"upload","captured_at":"2026-11-19T09:48:16.178931-07:00","related":["TR-037","TR-012"]}`, i%3, i, sha, 1000+i, i%3, i)
}

func ingestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ingestState(t *testing.T, s *store.Store) core.State {
	t.Helper()
	ev, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	st, err := livefield.Rebuild(ev)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestIngestSourceManifestIsIdempotentBySHA256(t *testing.T) {
	s := ingestStore(t)
	rt := Runtime{Store: s}
	manifest := strings.Join([]string{ingestLine(1), ingestLine(2), ingestLine(3)}, "\n") + "\n"

	dry, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(manifest), SourceIngestOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.WouldCreate != 3 || dry.Created != 0 {
		t.Fatalf("dry run = %+v", dry)
	}
	if ev, _ := s.Events(); len(ev) != 0 {
		t.Fatalf("dry run wrote %d events", len(ev))
	}

	first, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(manifest), SourceIngestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 3 || first.AlreadyPresent != 0 {
		t.Fatalf("first run = %+v", first)
	}
	before, _ := s.Events()

	// Same sha256 with a changed title is still the same SOURCE.
	retitled := strings.Replace(manifest, `"title":"Upload 2"`, `"title":"Renamed"`, 1)
	second, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(retitled+ingestLine(1)+"\n"), SourceIngestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.AlreadyPresent != 3 || second.DuplicateInFile != 1 {
		t.Fatalf("second run = %+v", second)
	}
	after, _ := s.Events()
	if len(after) != len(before) {
		t.Fatalf("second run added events: %d -> %d", len(before), len(after))
	}
	if got := len(ingestState(t, s).AtGOV); got != 3 {
		t.Fatalf("AtGOV = %d want 3", got)
	}
}

func TestIngestSourceManifestSkipsBadLinesWithoutAborting(t *testing.T) {
	s := ingestStore(t)
	rt := Runtime{Store: s}
	lines := []string{
		ingestLine(1),
		`{not json`,
		`{"path":"sources/x.pdf","sha256":"abc","source_kind":"upload"}`,
		strings.Replace(ingestLine(4), `"source_kind":"upload"`, `"source_kind":"script"`, 1),
		strings.Replace(ingestLine(5), `sources/topic-2/file-5.docx`, `../escape.docx`, 1),
		strings.Replace(ingestLine(6), `"captured_at":"2026-11-19T09:48:16.178931-07:00"`, `"captured_at":"yesterday"`, 1),
		"",
		ingestLine(2),
	}
	report, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(strings.Join(lines, "\n")), SourceIngestOptions{})
	if err != nil {
		t.Fatalf("bad lines aborted the run: %v", err)
	}
	if report.Bad != 5 || len(report.BadLines) != 5 || report.Created != 2 || report.Lines != 7 {
		t.Fatalf("report = %+v", report)
	}
	for _, b := range report.BadLines {
		if b.Line < 2 || b.Line > 6 || b.Reason == "" {
			t.Fatalf("bad line report = %+v", b)
		}
	}
}

func TestIngestSourceManifestCreatesOnlySourceRecordsAtGOV(t *testing.T) {
	s := ingestStore(t)
	rt := Runtime{Store: s}
	manifest := strings.Join([]string{ingestLine(1), ingestLine(2), ingestLine(3), ingestLine(4)}, "\n")
	report, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(manifest), SourceIngestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Created != 4 {
		t.Fatalf("report = %+v", report)
	}
	ev, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ev {
		if e.Decision != nil || e.REG != nil || e.ActionAuthorization != nil || e.Type != "C" {
			t.Fatalf("ingest wrote a decision or REG receipt: %+v", e)
		}
		if e.EmergION == nil {
			t.Fatalf("ingest wrote a non-candidate event: %+v", e)
		}
		em := e.EmergION
		if em.REL["source_kind"] != SourceUploadKind || em.STA != core.StateAtGOV || em.MEM.Provenance != sourceUploadProvenance {
			t.Fatalf("non-SOURCE record created: %s kind=%q sta=%q", em.IDN, em.REL["source_kind"], em.STA)
		}
		for _, k := range []string{"source_sha256", "source_path", "source_title", "source_topic", "source_mime", "source_bytes", "source_captured_at", "source_related"} {
			if strings.TrimSpace(em.REL[k]) == "" {
				t.Fatalf("%s missing %s: %v", em.IDN, k, em.REL)
			}
		}
		if em.REL["protector"] != "NO_EXTERNAL_AUTHORITY_CLAIMED" || em.REL["protector_gate"] != "" {
			t.Fatalf("authority claimed: %v", em.REL)
		}
	}
	st := ingestState(t, s)
	if len(st.AtGOV) != 4 || len(st.Accepted) != 0 || len(st.Approved) != 0 || len(st.Held) != 0 {
		t.Fatalf("state AtGOV=%d Accepted=%d Approved=%d Held=%d", len(st.AtGOV), len(st.Accepted), len(st.Approved), len(st.Held))
	}
}

func TestIngestSourceManifestRootVerifiesHashesReadOnly(t *testing.T) {
	s := ingestStore(t)
	rt := Runtime{Store: s}
	root := t.TempDir()
	content := []byte("#!/bin/sh\necho must never run\n")
	rel := "sources/scripts/x.sh"
	if err := os.MkdirAll(filepath.Join(root, "sources", "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), content, 0o700); err != nil {
		t.Fatal(err)
	}
	good := fmt.Sprintf(`{"path":%q,"sha256":"%x","bytes":%d,"mime":"text/x-sh","topic":"t","title":"script","source_kind":"upload"}`, rel, sha256.Sum256(content), len(content))
	wrong := fmt.Sprintf(`{"path":%q,"sha256":"%x","bytes":1,"source_kind":"upload"}`, rel, sha256.Sum256([]byte("other")))
	missing := ingestLine(9)
	report, err := rt.IngestSourceManifest(context.Background(), strings.NewReader(good+"\n"+wrong+"\n"+missing), SourceIngestOptions{Root: root, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesVerified != 1 || report.FilesMissing != 1 || report.Bad != 1 || report.WouldCreate != 2 {
		t.Fatalf("report = %+v", report)
	}
}
