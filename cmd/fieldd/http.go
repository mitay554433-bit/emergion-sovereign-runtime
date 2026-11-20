package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

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
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(fieldapi.BatchResult{Requested: len(p.EmergIONIDs), Error: err.Error()})
			return
		}
		res, _ := rt.DecideBatchHumanAs(p.EmergIONIDs, p.Reason, p.ConfirmCount, batchCap, p.BatchID)
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
