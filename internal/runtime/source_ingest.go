package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/reason"
)

// SourceUploadKind is the source_kind every manifest ingest record carries.
// The Living Universe classifies any kind containing SOURCE as a source.
const SourceUploadKind = "SOURCE_UPLOAD"

const sourceUploadProvenance = "governed_source_upload"

// SourceManifestEntry is one line of sources/ingest-manifest.jsonl.
type SourceManifestEntry struct {
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	Bytes      int64    `json:"bytes"`
	Mime       string   `json:"mime"`
	Topic      string   `json:"topic"`
	Title      string   `json:"title"`
	SourceKind string   `json:"source_kind"`
	CapturedAt string   `json:"captured_at"`
	Related    []string `json:"related"`
}

// SourceIngestOptions controls IngestSourceManifest.
type SourceIngestOptions struct {
	// DryRun counts what would be recorded and writes nothing.
	DryRun bool
	// Root, when set, is the directory the manifest paths are relative to.
	// Files found there are hashed (read only, never executed) and a
	// sha256 mismatch makes that line bad. Missing files are counted only:
	// the SOURCE record is the manifest descriptor, not the file bytes.
	Root string
}

// SourceIngestBadLine reports one skipped manifest line.
type SourceIngestBadLine struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// SourceIngestReport is the count receipt of one ingest run.
type SourceIngestReport struct {
	DryRun          bool                  `json:"dry_run"`
	Lines           int                   `json:"lines"`
	Valid           int                   `json:"valid"`
	Bad             int                   `json:"bad"`
	AlreadyPresent  int                   `json:"already_present"`
	DuplicateInFile int                   `json:"duplicate_in_manifest"`
	WouldCreate     int                   `json:"would_create"`
	Created         int                   `json:"created"`
	FilesVerified   int                   `json:"files_verified"`
	FilesMissing    int                   `json:"files_missing"`
	CreatedIDs      []string              `json:"created_ids,omitempty"`
	BadLines        []SourceIngestBadLine `json:"bad_lines,omitempty"`
}

func validSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func oneLine(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// validate normalizes an entry and returns why it is unusable, if it is.
func (e *SourceManifestEntry) validate() string {
	e.SHA256 = strings.ToLower(strings.TrimSpace(e.SHA256))
	e.Path = strings.TrimSpace(e.Path)
	e.Title = oneLine(e.Title)
	e.Topic = oneLine(e.Topic)
	e.Mime = oneLine(e.Mime)
	e.SourceKind = strings.TrimSpace(e.SourceKind)
	e.CapturedAt = strings.TrimSpace(e.CapturedAt)
	switch {
	case !validSHA256(e.SHA256):
		return "sha256 must be 64 hex characters"
	case e.Path == "":
		return "path missing"
	case strings.HasPrefix(e.Path, "/") || filepath.IsAbs(e.Path):
		return "path must be relative"
	case path.Clean(e.Path) != e.Path || strings.HasPrefix(e.Path, "../") || e.Path == "..":
		return "path must be clean and stay inside the sources tree"
	case e.Bytes < 0:
		return "bytes must not be negative"
	case e.SourceKind != "upload":
		return fmt.Sprintf("source_kind %q is not upload", e.SourceKind)
	}
	if e.CapturedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, e.CapturedAt); err != nil {
			return "captured_at is not RFC3339"
		}
	}
	if e.Title == "" {
		e.Title = path.Base(e.Path)
	}
	related := make([]string, 0, len(e.Related))
	seen := map[string]bool{}
	for _, r := range e.Related {
		r = oneLine(r)
		if r == "" || strings.Contains(r, ",") || seen[r] {
			continue
		}
		seen[r] = true
		related = append(related, r)
	}
	sort.Strings(related)
	e.Related = related
	return ""
}

// descriptor is the preserved SOURCE evidence for one upload: a deterministic
// text form of the manifest line. The upload's own bytes are referenced by
// sha256 and are never executed.
func (e SourceManifestEntry) descriptor() []byte {
	var b strings.Builder
	b.WriteString("SOURCE_UPLOAD/1\n")
	fmt.Fprintf(&b, "SHA256:%s\n", e.SHA256)
	fmt.Fprintf(&b, "PATH:%s\n", e.Path)
	fmt.Fprintf(&b, "TITLE:%s\n", e.Title)
	fmt.Fprintf(&b, "TOPIC:%s\n", e.Topic)
	fmt.Fprintf(&b, "MIME:%s\n", e.Mime)
	fmt.Fprintf(&b, "BYTES:%d\n", e.Bytes)
	fmt.Fprintf(&b, "CAPTURED_AT:%s\n", e.CapturedAt)
	fmt.Fprintf(&b, "RELATED:%s\n", strings.Join(e.Related, ","))
	b.WriteString("Z\n")
	return []byte(b.String())
}

func (e SourceManifestEntry) result() reason.Result {
	rel := map[string]string{
		"source_kind":   SourceUploadKind,
		"source_sha256": e.SHA256,
		"source_path":   e.Path,
		"source_title":  e.Title,
		"source_bytes":  strconv.FormatInt(e.Bytes, 10),
	}
	if e.Topic != "" {
		rel["source_topic"] = e.Topic
	}
	if e.Mime != "" {
		rel["source_mime"] = e.Mime
	}
	if e.CapturedAt != "" {
		rel["source_captured_at"] = e.CapturedAt
	}
	if len(e.Related) > 0 {
		// Stored only: related ids are not EmergION ids, so no KIN link
		// is claimed.
		rel["source_related"] = strings.Join(e.Related, ",")
	}
	summary := "SOURCE upload: " + e.Title
	if e.Topic != "" {
		summary = "SOURCE upload (" + e.Topic + "): " + e.Title
	}
	return reason.Result{
		Summary: summary,
		// No Archonym: an Archonym is a sovereign name that only a
		// HUMAN_FINAL accept may claim. The title lives in REL source_title
		// and the summary, so a waiting upload can never take or block an
		// accepted record's name.
		Relationships: rel,
		Capabilities:  []string{"OBS"},
		Facts:         []string{"source_upload_observed", "source_sha256:" + e.SHA256},
		Risk:          "L",
	}
}

// recordedSourceUploads returns the sha256 of every SOURCE_UPLOAD already in
// the store, whatever its GOV state.
func (r Runtime) recordedSourceUploads() (map[string]bool, error) {
	events, err := r.Store.Events()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.EmergION == nil || ev.EmergION.REL["source_kind"] != SourceUploadKind {
			continue
		}
		if h := strings.TrimSpace(ev.EmergION.REL["source_sha256"]); h != "" {
			seen[h] = true
		}
	}
	return seen, nil
}

func hashLocalFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// IngestSourceManifest records each manifest line as one SOURCE_UPLOAD
// EmergION through the normal governed candidate path. Records stop AT_GOV:
// nothing is decided, approved or REG-accepted here. It is idempotent by
// sha256, and a bad line is skipped and reported without aborting the run.
func (r Runtime) IngestSourceManifest(
	ctx context.Context,
	manifest io.Reader,
	opts SourceIngestOptions,
) (SourceIngestReport, error) {
	report := SourceIngestReport{DryRun: opts.DryRun}
	if r.Store == nil {
		return report, fmt.Errorf("runtime not configured")
	}
	seen, err := r.recordedSourceUploads()
	if err != nil {
		return report, err
	}
	inFile := map[string]bool{}

	sc := bufio.NewScanner(manifest)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	bad := func(reason string) {
		report.Bad++
		report.BadLines = append(report.BadLines, SourceIngestBadLine{Line: line, Reason: reason})
	}
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		report.Lines++
		var e SourceManifestEntry
		dec := json.NewDecoder(strings.NewReader(raw))
		if err := dec.Decode(&e); err != nil {
			bad("invalid JSON: " + err.Error())
			continue
		}
		if why := e.validate(); why != "" {
			bad(why)
			continue
		}
		if opts.Root != "" {
			local := filepath.Join(opts.Root, filepath.FromSlash(e.Path))
			got, err := hashLocalFile(local)
			switch {
			case os.IsNotExist(err):
				report.FilesMissing++
			case err != nil:
				bad("file unreadable: " + err.Error())
				continue
			case got != e.SHA256:
				bad("file sha256 does not match manifest")
				continue
			default:
				report.FilesVerified++
			}
		}
		report.Valid++
		if inFile[e.SHA256] {
			report.DuplicateInFile++
			continue
		}
		inFile[e.SHA256] = true
		if seen[e.SHA256] {
			report.AlreadyPresent++
			continue
		}
		report.WouldCreate++
		if opts.DryRun {
			continue
		}

		ingest := r
		ingest.Reasoner = fixedReasoner{name: "source-ingest", version: "v1", result: e.result()}
		ingest.ReturnedPredecessor = ""
		em, duplicate, err := ingest.CaptureBytes(ctx, "source-upload", e.descriptor(), sourceUploadProvenance)
		if err != nil {
			return report, fmt.Errorf("line %d: %w", line, err)
		}
		seen[e.SHA256] = true
		if duplicate {
			report.AlreadyPresent++
			report.WouldCreate--
			continue
		}
		report.Created++
		report.CreatedIDs = append(report.CreatedIDs, em.IDN)
	}
	if err := sc.Err(); err != nil {
		return report, err
	}
	return report, nil
}

// IngestSourceManifestFile opens a manifest file and ingests it.
func (r Runtime) IngestSourceManifestFile(
	ctx context.Context,
	manifestPath string,
	opts SourceIngestOptions,
) (SourceIngestReport, error) {
	f, err := os.Open(manifestPath)
	if err != nil {
		return SourceIngestReport{DryRun: opts.DryRun}, err
	}
	defer f.Close()
	return r.IngestSourceManifest(ctx, f, opts)
}
