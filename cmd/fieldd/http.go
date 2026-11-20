package main

import (
	"encoding/json"
	"errors"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

func newHTTPMux(rt *fieldapi.Runtime, token string, loopback bool) *http.ServeMux {
	return newHTTPMuxOn(rt, token, loopback, "")
}

func newHTTPMuxOn(rt *fieldapi.Runtime, token string, loopback bool, bindAddr string) *http.ServeMux {
	mux := http.NewServeMux()
	protect := func(fn http.HandlerFunc) http.HandlerFunc {
		return maybeRequireBearer(token, loopback, fn)
	}
	mutate := func(fn http.HandlerFunc) http.HandlerFunc {
		return requireBearer(token, fn)
	}

	mux.HandleFunc("/status", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dispatch(rt, Request{Method: "status"}))
	}))

	mux.HandleFunc("/actions", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		emergionID := req.URL.Query().Get("emergion_id")
		if emergionID == "" {
			http.Error(w, "missing emergion_id", http.StatusBadRequest)
			return
		}

		actionsJSON, err := rt.ActionsJSON(emergionID, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(actionsJSON))
	}))

	mux.HandleFunc("/authorize", mutate(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var p AuthorizeParams
		if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
			http.Error(w, "invalid authorize params", http.StatusBadRequest)
			return
		}

		params, err := json.Marshal(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := dispatch(rt, Request{Method: "authorize", Params: params})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	mux.HandleFunc("/decide", mutate(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var p DecideParams
		if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
			http.Error(w, "invalid decide params", http.StatusBadRequest)
			return
		}
		params, err := json.Marshal(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := dispatch(rt, Request{Method: "decide", Params: params})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	// Batch HUMAN_FINAL: one human press over an explicit list of AtGOV IDs.
	// GET reports the server-side cap (read-only); POST requires the bearer
	// token, a reason and a confirm count equal to the number of IDs.
	batchGet := protect(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"cap": batchCap})
	})
	batchPost := mutate(func(w http.ResponseWriter, req *http.Request) {
		var p BatchParams
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4<<20)).Decode(&p); err != nil {
			http.Error(w, "invalid batch params", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		err := fieldapi.ValidateBatch(p.EmergIONIDs, p.Reason, p.ConfirmCount, batchCap)
		if err == nil && p.BatchID != "" && !fieldapi.ValidBatchID(p.BatchID) {
			err = errors.New("invalid batch id")
		}
		if err == nil {
			_, err = fieldapi.BatchDecision(p.Decision)
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(fieldapi.BatchResult{Requested: len(p.EmergIONIDs), Error: err.Error()})
			return
		}
		res, _ := rt.DecideBatchHumanDecision(p.EmergIONIDs, p.Reason, p.ConfirmCount, batchCap, p.BatchID, p.Decision)
		_ = json.NewEncoder(w).Encode(res)
	})
	mux.HandleFunc("/decide-batch-human", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			batchGet(w, req)
		case http.MethodPost:
			batchPost(w, req)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/field.json", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		receipt, err := rt.RenderCurrent("outputs")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Field-Tip", receipt.TipHash)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, req, filepath.Join("outputs", "field.json"))
	}))

	// Read-only linked projection from the projection engine (proj.Linked).
	mux.HandleFunc("/projection.json", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := rt.LinkedProjectionJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(body))
	}))

	// Read-only recent ledger events (newest last) for the event comets.
	mux.HandleFunc("/events.json", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		body, err := rt.RecentEventsJSON(limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(body))
	}))

	mux.HandleFunc("/real_temporal_wave.json", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, req, filepath.Join("outputs", "real_temporal_wave.json"))
	}))

	livingUniverse := protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		path := filepath.Join("cmd", "fieldd", "living-universe.html")
		if !canInjectLivingToken(loopback, bindAddr, token, req) {
			http.ServeFile(w, req, path)
			return
		}
		body, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, "living universe page unavailable", http.StatusInternalServerError)
			return
		}
		page, ok := applyLivingToken(string(body), token, w.Header())
		if !ok {
			http.ServeFile(w, req, path)
			return
		}
		_, _ = w.Write([]byte(page))
	})
	mux.HandleFunc("/living-universe", livingUniverse)
	// /living is a read-only alias for the same page (the phone uses /living).
	mux.HandleFunc("/living", livingUniverse)
	// Installable app shell (manifest, service worker, icons). See pwa.go.
	registerPWARoutes(mux, protect)

	mux.HandleFunc("/field.html", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		receipt, err := rt.RenderCurrent("outputs")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Field-Tip", receipt.TipHash)
		http.ServeFile(w, req, filepath.Join("outputs", "field.html"))
	}))

	mux.Handle("/deliverables/", maybeRequireBearerHandler(token, loopback, http.StripPrefix("/deliverables/", http.FileServer(http.Dir(filepath.Join("outputs", "deliverables"))))))

	return mux
}

const livingTokenMetaEmpty = `<meta name="fieldd-token" content="">`

func livingTokenMeta(token string) string {
	return `<meta name="fieldd-token" content="` + html.EscapeString(token) + `">`
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func livingInjectHostOK(reqHost, bindAddr string) bool {
	_, port, err := net.SplitHostPort(strings.TrimSpace(bindAddr))
	if err != nil || port == "" {
		return false
	}
	host := strings.TrimSpace(reqHost)
	return strings.EqualFold(host, "127.0.0.1:"+port) || strings.EqualFold(host, "localhost:"+port)
}

func canInjectLivingToken(loopback bool, bindAddr, token string, req *http.Request) bool {
	if !loopback || strings.TrimSpace(token) == "" || bindAddr == "" || req == nil {
		return false
	}
	if !isLoopbackRemoteAddr(req.RemoteAddr) {
		return false
	}
	return livingInjectHostOK(req.Host, bindAddr)
}

func cacheControlHas(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// livingSafeInjectHeaders is the last gate before a token is written into
// the page: Cache-Control must include no-store, Referrer-Policy must be
// no-referrer, and no CORS headers may be present. If any check fails the
// page is served without the token.
func livingSafeInjectHeaders(h http.Header) bool {
	if h == nil || !cacheControlHas(h.Get("Cache-Control"), "no-store") {
		return false
	}
	if h.Get("Referrer-Policy") != "no-referrer" {
		return false
	}
	for _, k := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Expose-Headers",
	} {
		if h.Get(k) != "" {
			return false
		}
	}
	return true
}

func applyLivingToken(page, token string, hdr http.Header) (string, bool) {
	if strings.TrimSpace(token) == "" || !strings.Contains(page, livingTokenMetaEmpty) {
		return page, false
	}
	if hdr == nil {
		return page, false
	}
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("Referrer-Policy", "no-referrer")
	// Never add CORS headers on this response.
	if !livingSafeInjectHeaders(hdr) {
		hdr.Del("Cache-Control")
		hdr.Del("Referrer-Policy")
		return page, false
	}
	return strings.Replace(page, livingTokenMetaEmpty, livingTokenMeta(token), 1), true
}
