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
	"emergion-sovereign-runtime/internal/worker"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

type Request struct {
	Method        string          `json:"method"`
	Params        json.RawMessage `json:"params,omitempty"`
	Token         string          `json:"token,omitempty"`
	Authorization string          `json:"authorization,omitempty"`
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
	socketPath := flag.String("socket", filepath.Join(os.TempDir(), "fieldd.sock"), "Path to Unix domain socket; empty or off disables it")
	pidPath := flag.String("pid", filepath.Join(os.TempDir(), "fieldd.pid"), "Path to PID file")
	httpAddr := flag.String("http", "", "HTTP listen address. Empty disables HTTP. Default operator bind is 127.0.0.1:8899 via field daemon start. Non-loopback requires FIELDD_TOKEN or a 0600 fieldd.token in the state directory.")
	workDir := flag.String("work-dir", "", "Worker control and checkpoint directory (default <state>-work, outside the ledger)")
	flag.IntVar(&batchCap, "batch-cap", batchCapFromEnv(), "Largest batch one HUMAN_FINAL press may accept via /decide-batch-human (env FIELDD_BATCH_CAP)")
	flag.Parse()

	if err := writePIDFile(*pidPath); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}
	defer os.Remove(*pidPath)

	rt, err := fieldapi.Open(*stateDir, reason.GemmaFromEnv())
	if err != nil {
		log.Fatalf("Failed to open field runtime state: %v", err)
	}

	if *workDir == "" {
		*workDir = worker.WorkDir(*stateDir)
	}
	autoEng = newAutoEngine(rt, *workDir, nil)

	token, loopback, err := resolveListenToken(*stateDir, *httpAddr)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// -socket off (or empty) skips the Unix listener. The Android app uses
	// HTTP on loopback only and does not need the socket.
	var listener net.Listener
	if socketEnabled(*socketPath) {
		if err := os.RemoveAll(*socketPath); err != nil {
			log.Fatalf("Failed to clear socket path: %v", err)
		}
		var err error
		listener, err = net.Listen("unix", *socketPath)
		if err != nil {
			log.Fatalf("Failed to listen on socket %s: %v", *socketPath, err)
		}
		defer os.Remove(*socketPath)
		defer listener.Close()
		log.Printf("Field Sovereign Daemon listening on %s (PID %d)", *socketPath, os.Getpid())
	} else if *httpAddr == "" {
		log.Fatalf("either -http or -socket is required")
	} else {
		log.Printf("Field Sovereign Daemon HTTP-only (PID %d)", os.Getpid())
	}
	log.Printf("Mutating routes require a bearer token from %s or %s", tokenEnvVar, tokenFilePath(*stateDir))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if listener != nil {
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
				go handleConn(rt, conn, token)
			}
		}()
	}

	if *httpAddr != "" {
		warmFieldOnStart = true
		mux := newHTTPMuxOn(rt, token, loopback, *httpAddr)
		// Bind before serving: when another process (for example an older
		// fieldd) already holds the address, exit loudly instead of running
		// on without HTTP while the other process answers for us.
		httpLn, err := net.Listen("tcp", *httpAddr)
		if err != nil {
			os.Remove(*pidPath)
			os.Remove(*socketPath)
			log.Fatalf("HTTP listen on %s failed: %v (another process already holds this address; stop it and let the watchdog restart fieldd)", *httpAddr, err)
		}
		go func() {
			log.Printf("Field Sovereign HTTP status listening on %s", *httpAddr)
			if err := http.Serve(httpLn, mux); err != nil {
				log.Printf("HTTP status listener stopped: %v", err)
			}
		}()
	}

	go autoEng.run(ctx)
	log.Printf("A01-AUTO policy and worker loop running (%s); work dir %s", autoEng.describe(), *workDir)

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

func handleConn(rt *fieldapi.Runtime, conn net.Conn, token string) {
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

		if isMutatingIPCMethod(req.Method) && !tokenEqual(requestToken(req), token) {
			if err := encoder.Encode(Response{Error: "unauthorized"}); err != nil {
				log.Printf("Failed to encode response: %v", err)
				return
			}
			continue
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
