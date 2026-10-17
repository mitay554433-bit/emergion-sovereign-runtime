package main

import (
"context"
"encoding/json"
"os"
"net"
"path/filepath"
"testing"
"time"

"emergion-sovereign-runtime/internal/reason"
"emergion-sovereign-runtime/pkg/fieldapi"
)

func TestDaemonIPCProtocol(t *testing.T) {
tmpDir := t.TempDir()
statePath := filepath.Join(tmpDir, "state")
sockPath := filepath.Join(tmpDir, "test.sock")

rt, err := fieldapi.Open(statePath, reason.GemmaFromEnv())
if err != nil {
t.Fatalf("failed to open runtime: %v", err)
}

listener, err := net.Listen("unix", sockPath)
if err != nil {
t.Fatalf("failed to listen on unix socket: %v", err)
}
defer listener.Close()

go func() {
conn, err := listener.Accept()
if err != nil {
return
}
handleConn(rt, conn)
}()

time.Sleep(50 * time.Millisecond)

conn, err := net.Dial("unix", sockPath)
if err != nil {
t.Fatalf("failed to connect to socket: %v", err)
}
defer conn.Close()

encoder := json.NewEncoder(conn)
decoder := json.NewDecoder(conn)

// 1. Test status call over IPC
req := Request{Method: "status"}
if err := encoder.Encode(req); err != nil {
t.Fatalf("failed to send status request: %v", err)
}

var resp Response
if err := decoder.Decode(&resp); err != nil {
t.Fatalf("failed to decode response: %v", err)
}

if resp.Error != "" {
t.Fatalf("unexpected daemon error: %s", resp.Error)
}

// 2. Test unknown method failure mode
badReq := Request{Method: "nonexistent_method"}
if err := encoder.Encode(badReq); err != nil {
t.Fatalf("failed to send bad request: %v", err)
}

var badResp Response
if err := decoder.Decode(&badResp); err != nil {
t.Fatalf("failed to decode error response: %v", err)
}

if badResp.Error == "" {
t.Fatal("expected error on unknown method, got nil")
}
}

type humanFinalTestReasoner struct{}

func (humanFinalTestReasoner) Analyze(_ context.Context, _ reason.Input) (reason.Result, error) {
return reason.Result{
Summary: "bounded HUMAN_FINAL fieldd decision proof",
Relationships: map[string]string{"source_kind": "TEST"},
Capabilities: []string{"OBS", "CMP"},
Facts: []string{"bounded fieldd test source"},
Risk: "L",
}, nil
}

func (humanFinalTestReasoner) Name() string { return "fieldd-human-final-test" }
func (humanFinalTestReasoner) Version(context.Context) string { return "1" }

func TestDecideDispatchUsesHumanFinalAndReachesREG(t *testing.T) {
tmpDir := t.TempDir()
statePath := filepath.Join(tmpDir, "state")
sourcePath := filepath.Join(tmpDir, "candidate.txt")

if err := os.WriteFile(sourcePath, []byte("bounded HUMAN_FINAL fieldd decision proof"), 0600); err != nil {
t.Fatal(err)
}

rt, err := fieldapi.Open(statePath, humanFinalTestReasoner{})
if err != nil {
t.Fatal(err)
}

em, err := rt.Capture(context.Background(), sourcePath)
if err != nil {
t.Fatal(err)
}

params, err := json.Marshal(DecideParams{
EmergIONID: em.IDN,
Decision: "APPROVE",
Reason: "fieldd HUMAN_FINAL dispatch proof",
})
if err != nil {
t.Fatal(err)
}

resp := dispatch(rt, Request{Method: "decide", Params: params})
if resp.Error != "" {
t.Fatalf("decide dispatch failed: %s", resp.Error)
}

status, err := rt.Status()
if err != nil {
t.Fatal(err)
}
if status.Accepted != 1 {
t.Fatalf("accepted = %d want 1", status.Accepted)
}
if status.AtGOV != 0 {
t.Fatalf("at_gov = %d want 0", status.AtGOV)
}
}
