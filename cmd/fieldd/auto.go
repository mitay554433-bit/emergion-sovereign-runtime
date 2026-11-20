package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/worker"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// autoEngine is the fieldd background loop: the A01-AUTO policy pass (only
// when a HUMAN_FINAL switch has it on) and the nonstop worker (only when its
// control is on). Both default OFF.
type autoEngine struct {
	rt          *fieldapi.Runtime
	w           *worker.Worker
	sw          *reason.SwitchInsighter
	sourcesRoot string
	tick        time.Duration
	kick        chan struct{}
	mu          sync.Mutex
	lastStep    time.Time
	lastErr     string
}

// autoEng is set by main; routes are registered only when it exists.
var autoEng *autoEngine

func newAutoEngine(rt *fieldapi.Runtime, workDir string, sw *reason.SwitchInsighter) *autoEngine {
	if sw == nil {
		sw = reason.NewSwitchInsighter(reason.ModeLocal)
	}
	e := &autoEngine{
		rt:          rt,
		sw:          sw,
		sourcesRoot: strings.TrimSpace(os.Getenv("UNIFUSION_SOURCES_ROOT")),
		tick:        15 * time.Second,
		kick:        make(chan struct{}, 1),
	}
	e.w = rt.NewWorker(workDir, sw)
	if m := strings.TrimSpace(os.Getenv("UNIFUSION_REASONER_MODE")); m != "" && reason.ValidMode(m) {
		// env sets the initial mode only when the UI never chose one
		if _, err := os.Stat(workDir + "/worker.json"); err != nil {
			c := e.w.LoadControl()
			c.Mode = m
			_ = e.w.SaveControl(c)
		}
	}
	e.sw.SetMode(e.w.LoadControl().Mode)
	return e
}

func (e *autoEngine) poke() {
	select {
	case e.kick <- struct{}{}:
	default:
	}
}

// once runs one bounded pass: policy first, then a worker step when due,
// then policy again so fresh low-risk worker links are decided promptly.
func (e *autoEngine) once(ctx context.Context) {
	if res, err := e.rt.ApplyAutoPolicy(fieldapi.AutoPolicyDefaultBatch, e.sourcesRoot); err != nil {
		log.Printf("auto policy: %v", err)
	} else if res.Accepted+res.Held > 0 {
		log.Printf("auto policy: accepted %d held %d", res.Accepted, res.Held)
	}
	c := e.w.LoadControl()
	if e.sw.Mode() != c.Mode {
		e.sw.SetMode(c.Mode)
	}
	if !c.On {
		return
	}
	interval := worker.DefaultInterval
	if c.Interval > 0 {
		interval = time.Duration(c.Interval) * time.Second
	}
	e.mu.Lock()
	due := time.Since(e.lastStep) >= interval
	e.mu.Unlock()
	if !due {
		return
	}
	batch := c.Batch
	if batch <= 0 {
		batch = worker.DefaultBatch
		if c.Mode == reason.ModeOnline || c.Mode == reason.ModeAuto && e.sw.OnlineConfigured() {
			batch = 20
		}
	}
	rep, err := e.w.Step(ctx, batch)
	e.mu.Lock()
	e.lastStep = time.Now()
	e.lastErr = ""
	if err != nil {
		e.lastErr = err.Error()
	}
	e.mu.Unlock()
	if err != nil {
		log.Printf("worker: %v", err)
		return
	}
	if rep.Processed+rep.Failed > 0 {
		log.Printf("worker: processed %d insights %d links %d failed %d pending %d via %s", rep.Processed, rep.Insights, rep.Links, rep.Failed, rep.Pending, rep.Reasoner)
		if _, err := e.rt.ApplyAutoPolicy(fieldapi.AutoPolicyDefaultBatch, e.sourcesRoot); err != nil {
			log.Printf("auto policy: %v", err)
		}
	}
}

func (e *autoEngine) run(ctx context.Context) {
	for {
		e.once(ctx)
		wait := e.tick
		if c := e.w.LoadControl(); c.On && c.Interval > 0 && time.Duration(c.Interval)*time.Second < wait {
			wait = time.Duration(c.Interval) * time.Second
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-e.kick:
			t.Stop()
		}
	}
}

type autoPolicyParams struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type autoWorkerParams struct {
	On   *bool  `json:"on"`
	Mode string `json:"mode"`
}

type autoUndoParams struct {
	EmergIONID string `json:"emergion_id"`
	Reason     string `json:"reason"`
}

func writeJSONResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func registerAutoRoutes(mux *http.ServeMux, e *autoEngine, protect, mutate func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/auto/status", protect(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, err := e.rt.AutoStatusOf(e.w, e.sw)
		if err != nil {
			writeJSONResp(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		e.mu.Lock()
		body := struct {
			fieldapi.AutoStatus
			WorkerError string `json:"worker_error,omitempty"`
			LastStep    string `json:"last_step,omitempty"`
		}{AutoStatus: st, WorkerError: e.lastErr}
		if !e.lastStep.IsZero() {
			body.LastStep = e.lastStep.UTC().Format(time.RFC3339)
		}
		e.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
	}))
	// HUMAN_FINAL press: switch the A01-AUTO policy on or off (reason required).
	mux.HandleFunc("/auto/policy", mutate(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var p autoPolicyParams
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&p); err != nil {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "invalid policy params"})
			return
		}
		state := strings.ToLower(strings.TrimSpace(p.State))
		if state != "on" && state != "off" {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "state must be on or off"})
			return
		}
		id, err := e.rt.SetAutoPolicy(req.Context(), state == "on", p.Reason)
		if err != nil {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "switch_id": id})
			return
		}
		e.poke()
		writeJSONResp(w, http.StatusOK, map[string]string{"result": "OK", "switch_id": id, "state": state})
	}))
	// Worker on/off and reasoner mode. Not authority: the worker only proposes.
	mux.HandleFunc("/auto/worker", mutate(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var p autoWorkerParams
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&p); err != nil {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "invalid worker params"})
			return
		}
		c := e.w.LoadControl()
		if p.On != nil {
			c.On = *p.On
		}
		if m := strings.ToLower(strings.TrimSpace(p.Mode)); m != "" {
			if !reason.ValidMode(m) || m == reason.ModeHeuristic {
				writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "mode must be local, online or auto"})
				return
			}
			c.Mode = m
		}
		if err := e.w.SaveControl(c); err != nil {
			writeJSONResp(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		e.sw.SetMode(c.Mode)
		e.poke()
		writeJSONResp(w, http.StatusOK, c)
	}))
	// HUMAN_FINAL one-press undo of an AUTO_POLICY decision.
	mux.HandleFunc("/auto/undo", mutate(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var p autoUndoParams
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&p); err != nil {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "invalid undo params"})
			return
		}
		out, err := e.rt.UndoAuto(strings.TrimSpace(p.EmergIONID), p.Reason)
		if err != nil {
			writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSONResp(w, http.StatusOK, map[string]string{"result": out})
	}))
}
