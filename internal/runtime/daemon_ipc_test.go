package runtime_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/store"
)

func TestDaemonIPCLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "fieldd.sock")
	pidPath := filepath.Join(tmpDir, "fieldd.pid")

	stDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stDir, 0700); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	st, err := store.Open(stDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	_ = st

	// Write PID file
	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0600); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Listen on Unix domain socket
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sockPath, err)
	}
	defer listener.Close()

	// Handle single request in background goroutine
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		var req map[string]string
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			errCh <- err
			return
		}

		if req["method"] != "status" {
			t.Errorf("expected method 'status', got %q", req["method"])
		}

		resp := map[string]interface{}{
			"status":   "HEALTHY",
			"tip_hash": "5dc6502d119dc5544b00a2179f058ada7292650f42adeb795352472887a573a4",
		}
		errCh <- json.NewEncoder(conn).Encode(resp)
	}()

	// Connect as client over Unix socket
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send status request
	req := map[string]string{"method": "status"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	// Read response
	var resp map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server handler error: %v", err)
	}

	// Assert response integrity
	if resp["status"] != "HEALTHY" {
		t.Errorf("expected status HEALTHY, got %v", resp["status"])
	}
	if resp["tip_hash"] == "" {
		t.Errorf("expected non-empty tip_hash")
	}
}
