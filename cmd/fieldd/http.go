package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

func newHTTPMux(rt *fieldapi.Runtime, token string, loopback bool) *http.ServeMux {
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
		http.ServeFile(w, req, filepath.Join("outputs", "field.json"))
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
		http.ServeFile(w, req, filepath.Join("cmd", "fieldd", "living-universe.html"))
	})
	mux.HandleFunc("/living-universe", livingUniverse)
	// /living is a read-only alias for the same page (the phone uses /living).
	mux.HandleFunc("/living", livingUniverse)

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
