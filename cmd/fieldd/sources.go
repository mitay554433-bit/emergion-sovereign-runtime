package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

const defaultMaxSourceUpload = fieldapi.MaxSourceUploadBytes

func sourcesDirFromEnv() string {
	return strings.TrimSpace(os.Getenv("UNIFUSION_SOURCES_DIR"))
}

// POST /sources accepts one file as multipart form field "file" (optional
// title, topic, mime fields) and records a SOURCE_UPLOAD at AtGOV. Nothing
// is decided. Idempotent by SHA-256. Requires the bearer token.
func handlePostSource(rt *fieldapi.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, int64(defaultMaxSourceUpload)+1<<20)
		if err := req.ParseMultipartForm(int64(defaultMaxSourceUpload) + 1<<20); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		file, hdr, err := req.FormFile("file")
		if err != nil {
			http.Error(w, "file field required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, int64(defaultMaxSourceUpload)+1))
		if err != nil {
			http.Error(w, "could not read file", http.StatusBadRequest)
			return
		}
		if len(data) > defaultMaxSourceUpload {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		name := ""
		if hdr != nil {
			name = hdr.Filename
		}
		title := strings.TrimSpace(req.FormValue("title"))
		topic := strings.TrimSpace(req.FormValue("topic"))
		mime := strings.TrimSpace(req.FormValue("mime"))
		if mime == "" && hdr != nil {
			mime = hdr.Header.Get("Content-Type")
		}

		res, err := rt.IngestSourceUpload(req.Context(), name, title, topic, mime, data, sourcesDirFromEnv())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}
}

func socketEnabled(path string) bool {
	p := strings.TrimSpace(path)
	return p != "" && !strings.EqualFold(p, "off")
}
