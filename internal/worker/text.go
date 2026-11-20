package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// DefaultTextChars is how much extracted file text an insight prompt gets.
const DefaultTextChars = 1500

// TextCharsFromEnv reads UNIFUSION_WORKER_TEXT_CHARS (default 1500; 0 or a
// negative value turns file text off).
func TextCharsFromEnv() int {
	v := strings.TrimSpace(os.Getenv("UNIFUSION_WORKER_TEXT_CHARS"))
	if v == "" {
		return DefaultTextChars
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return DefaultTextChars
	}
	if n <= 0 {
		return -1
	}
	return n
}

// sourceFile resolves a manifest source_path under root, refusing anything
// that could leave it.
func sourceFile(root, rel string) string {
	rel = strings.TrimSpace(rel)
	if root == "" || rel == "" || path.Clean(rel) != rel || strings.HasPrefix(rel, "../") || rel == ".." || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

var textExt = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".json": true, ".xml": true,
	".html": true, ".htm": true, ".yaml": true, ".yml": true, ".log": true, ".rtf": true, ".tex": true,
	".go": true, ".py": true, ".js": true, ".ts": true, ".sh": true, ".ini": true, ".toml": true,
}

// ExtractText returns up to max characters of readable text from the source
// file (plain text, .docx, and .pdf when pdftotext is installed), whitespace
// collapsed. "" when the file is missing or has no readable text. File bytes
// are only read locally; the online reasoner gets only what the prompt holds.
func ExtractText(ctx context.Context, root, rel, mime string, max int) string {
	if max <= 0 {
		return ""
	}
	p := sourceFile(root, rel)
	if p == "" {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(p))
	var raw string
	switch {
	case ext == ".docx":
		raw = docxText(p, max*4)
	case ext == ".pdf" || mime == "application/pdf":
		raw = pdfText(ctx, p, max*4)
	case textExt[ext] || strings.HasPrefix(mime, "text/"):
		raw = plainText(p, max*4)
		if ext == ".html" || ext == ".htm" || ext == ".xml" {
			raw = stripTags(raw)
		}
	}
	return clip(raw, max)
}

func plainText(p string, n int) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, int64(n)))
	if bytes.IndexByte(b, 0) >= 0 {
		return "" // binary
	}
	return string(b)
}

func docxText(p string, n int) string {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		defer rc.Close()
		var b strings.Builder
		dec := xml.NewDecoder(io.LimitReader(rc, 4<<20))
		for b.Len() < n {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.CharData:
				b.Write(t)
			case xml.EndElement:
				if t.Name.Local == "p" {
					b.WriteString(" ")
				}
			}
		}
		return b.String()
	}
	return ""
}

func pdfText(ctx context.Context, p string, n int) string {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(cctx, bin, "-l", "3", "-enc", "UTF-8", p, "-")
	cmd.Stdout = &limitWriter{w: &out, n: n}
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return ""
	}
	return out.String()
}

type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(b []byte) (int, error) {
	if room := l.n - l.w.Len(); room > 0 {
		if len(b) > room {
			l.w.Write(b[:room])
		} else {
			l.w.Write(b)
		}
	}
	return len(b), nil
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
			b.WriteRune(' ')
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clip keeps printable text, collapses whitespace and cuts at max runes.
func clip(s string, max int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, " ")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}
