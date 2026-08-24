package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type DecideParams struct {
	EmergIONID string `json:"emergion_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
}

type AuthorizeParams struct {
	EmergIONID string `json:"emergion_id"`
	Adapter    string `json:"adapter"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	LocalGemma bool   `json:"local_gemma"`
}

func main() {
	stateDir := flag.String("state", ".field", "Path to sovereign state directory")
	socketPath := flag.String("socket", "/tmp/fieldd.sock", "Path to Unix domain socket")
	pidPath := flag.String("pid", "/tmp/fieldd.pid", "Path to PID file")
	httpAddr := flag.String("http", "", "Optional loopback-only HTTP status listener")
	flag.Parse()

	if err := writePIDFile(*pidPath); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}
	defer os.Remove(*pidPath)

	rt, err := fieldapi.Open(*stateDir, reason.GemmaFromEnv())
	if err != nil {
		log.Fatalf("Failed to open field runtime state: %v", err)
	}

	if err := os.RemoveAll(*socketPath); err != nil {
		log.Fatalf("Failed to clear socket path: %v", err)
	}

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket %s: %v", *socketPath, err)
	}
	defer os.Remove(*socketPath)
	defer listener.Close()

	log.Printf("Field Sovereign Daemon listening on %s (PID %d)", *socketPath, os.Getpid())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Accept error: %v", err)
					continue
				}
			}
			go handleConn(rt, conn)
		}
	}()

	if *httpAddr != "" {
		host, _, err := net.SplitHostPort(*httpAddr)
		if err != nil {
			log.Fatalf("Invalid HTTP listener address %q: %v", *httpAddr, err)
		}
		if host != "127.0.0.1" && host != "localhost" {
			log.Fatalf("HTTP listener must be loopback-only, got %q", host)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dispatch(rt, Request{Method: "status"}))
		})

mux.HandleFunc("/actions", func(w http.ResponseWriter, req *http.Request) {
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
})


mux.HandleFunc("/authorize", func(w http.ResponseWriter, req *http.Request) {
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
})

mux.HandleFunc("/decide", func(w http.ResponseWriter, req *http.Request) {
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
})

		mux.HandleFunc("/field.html", func(w http.ResponseWriter, req *http.Request) {
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
		})

		go func() {
			log.Printf("Field Sovereign HTTP status listening on %s", *httpAddr)
			if err := http.ListenAndServe(*httpAddr, mux); err != nil {
				log.Printf("HTTP status listener stopped: %v", err)
			}
		}()
	}

	<-ctx.Done()
	log.Println("Shutting down Field Sovereign Daemon gracefully...")
}

func writePIDFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func handleConn(rt *fieldapi.Runtime, conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			encoder.Encode(Response{Error: fmt.Sprintf("invalid request payload: %v", err)})
			return
		}

		resp := dispatch(rt, req)
		if err := encoder.Encode(resp); err != nil {
			log.Printf("Failed to encode response: %v", err)
			return
		}
	}
}

func dispatch(rt *fieldapi.Runtime, req Request) Response {
	switch req.Method {
	case "status":
		statusJSON, err := rt.StatusJSON()
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{Result: json.RawMessage(statusJSON)}

	case "decide":
		var p DecideParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Error: "invalid decide params"}
		}
		if err := rt.DecideBinding(p.EmergIONID, p.Decision, p.Reason); err != nil {
			return Response{Error: err.Error()}
		}
		return Response{Result: "OK"}

	case "authorize":
		var p AuthorizeParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Error: "invalid authorize params"}
		}
		authJSON, err := rt.AuthorizeBinding(p.EmergIONID, p.Adapter, p.Action, p.Reason, p.LocalGemma)
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{Result: authJSON}

	default:
		return Response{Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}
