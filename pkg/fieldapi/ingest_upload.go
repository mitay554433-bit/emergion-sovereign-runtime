package fieldapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	fieldruntime "emergion-sovereign-runtime/internal/runtime"
)

// MaxSourceUploadBytes is the largest file POST /sources will accept.
const MaxSourceUploadBytes = 16 << 20

// IngestUploadResult is the receipt of one SOURCE_UPLOAD ingest.
// The record is left AtGOV; nothing is decided or REG-accepted.
type IngestUploadResult struct {
	ID           string `json:"id"`
	SourceSHA256 string `json:"source_sha256"`
	Bytes        int64  `json:"bytes"`
	Duplicate    bool   `json:"duplicate"`
	AtGOV        bool   `json:"at_gov"`
}

// IngestSourceUpload records one uploaded file as a SOURCE_UPLOAD EmergION
// through the existing manifest ingest path. It is idempotent by the SHA-256
// of the file bytes. The ledger evidence is the SOURCE_UPLOAD/1 descriptor,
// not the raw file (same contract as `field ingest`). When keepDir is set,
// a copy of the bytes is written there as <sha256>, outside the ledger.
func (r *Runtime) IngestSourceUpload(
	ctx context.Context,
	filename, title, topic, mime string,
	data []byte,
	keepDir string,
) (IngestUploadResult, error) {
	out := IngestUploadResult{AtGOV: true}
	if r == nil || r.store == nil {
		return out, fmt.Errorf("runtime not configured")
	}
	if len(data) == 0 {
		return out, fmt.Errorf("empty source")
	}
	if len(data) > MaxSourceUploadBytes {
		return out, fmt.Errorf("source larger than %d bytes", MaxSourceUploadBytes)
	}

	sum := sha256.Sum256(data)
	sha := fmt.Sprintf("%x", sum[:])
	out.SourceSHA256 = sha
	out.Bytes = int64(len(data))

	rel := uploadRelPath(filename)
	if title = oneLineTitle(title); title == "" {
		title = path.Base(rel)
	}
	if mime = oneLineTitle(mime); mime == "" {
		mime = "application/octet-stream"
	}

	entry := fieldruntime.SourceManifestEntry{
		Path:       rel,
		SHA256:     sha,
		Bytes:      out.Bytes,
		Mime:       mime,
		Topic:      oneLineTitle(topic),
		Title:      title,
		SourceKind: "upload",
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return out, err
	}
	line = append(line, '\n')

	rt := fieldruntime.Runtime{Store: r.store, Reasoner: r.reasoner}
	report, err := rt.IngestSourceManifest(ctx, bytes.NewReader(line), fieldruntime.SourceIngestOptions{})
	if err != nil {
		return out, err
	}
	if report.Created > 0 {
		if len(report.CreatedIDs) == 0 {
			return out, fmt.Errorf("ingest created a record but returned no id")
		}
		out.ID = report.CreatedIDs[0]
		if err := writeKeptSource(keepDir, sha, data); err != nil {
			return out, err
		}
		return out, nil
	}
	if report.AlreadyPresent > 0 {
		id, ok, err := r.sourceUploadID(sha)
		if err != nil {
			return out, err
		}
		if !ok {
			return out, fmt.Errorf("source already present but id was not found")
		}
		out.ID = id
		out.Duplicate = true
		if err := writeKeptSource(keepDir, sha, data); err != nil {
			return out, err
		}
		return out, nil
	}
	if len(report.BadLines) > 0 {
		return out, fmt.Errorf("source rejected: %s", report.BadLines[0].Reason)
	}
	return out, fmt.Errorf("source was not recorded")
}

func (r *Runtime) sourceUploadID(sha string) (string, bool, error) {
	events, err := r.store.Events()
	if err != nil {
		return "", false, err
	}
	for _, ev := range events {
		if ev.EmergION == nil || ev.EmergION.REL["source_kind"] != fieldruntime.SourceUploadKind {
			continue
		}
		if ev.EmergION.REL["source_sha256"] == sha && ev.EmergION.IDN != "" {
			return ev.EmergION.IDN, true, nil
		}
	}
	return "", false, nil
}

func writeKeptSource(keepDir, sha string, data []byte) error {
	keepDir = strings.TrimSpace(keepDir)
	if keepDir == "" {
		return nil
	}
	if err := os.MkdirAll(keepDir, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(keepDir, sha)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func uploadRelPath(name string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r <= 0x1f || r == 0x7f || r == '/' || r == '\\' || r == ':' || r == 0:
			continue
		case unicode.IsSpace(r):
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" || out == "." || out == ".." {
		out = "upload.bin"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return "uploads/" + out
}

func oneLineTitle(v string) string {
	return strings.Join(strings.Fields(v), " ")
}
