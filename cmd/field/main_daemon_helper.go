package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func daemonPaths() (string, string) {
	base := os.TempDir()
	return envOr("FIELD_SOCKET", filepath.Join(base, "fieldd.sock")),
		envOr("FIELD_PID", filepath.Join(base, "fieldd.pid"))
}

func fielddBinary() string {
	if configured := os.Getenv("FIELDD_BIN"); configured != "" {
		return configured
	}

	if executable, err := os.Executable(); err == nil {
		adjacent := filepath.Join(filepath.Dir(executable), "fieldd")
		if info, err := os.Stat(adjacent); err == nil && !info.IsDir() {
			return adjacent
		}
	}

	return "fieldd"
}

func daemonStartCommand(root, sockPath, pidPath string) *exec.Cmd {
	args := []string{
		"-state", root,
		"-socket", sockPath,
		"-pid", pidPath,
	}

	httpAddr := "127.0.0.1:8899"
	if configured, ok := os.LookupEnv("FIELD_HTTP"); ok {
		httpAddr = configured
	}
	if httpAddr != "" {
		args = append(args, "-http", httpAddr)
	}

	return exec.Command(fielddBinary(), args...)
}

func handleDaemonCommand(args []string, root string) {
	if len(args) < 2 {
		fmt.Println("Usage: field daemon <start|status|stop>")
		os.Exit(1)
	}

	sockPath, pidPath := daemonPaths()

	switch args[1] {
	case "start":
		cmd := daemonStartCommand(root, sockPath, pidPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fail(fmt.Errorf("failed to start fieldd daemon: %w", err))
		}
		fmt.Printf("Started fieldd daemon [PID %d]\n", cmd.Process.Pid)

	case "status":
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			fail(fmt.Errorf("fieldd daemon is not running or socket unreachable: %w", err))
		}
		defer conn.Close()

		req := map[string]string{"method": "status"}
		if err := json.NewEncoder(conn).Encode(req); err != nil {
			fail(err)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			fail(err)
		}
		printJSON(resp)

	case "stop":
		data, err := os.ReadFile(pidPath)
		if err != nil {
			fail(fmt.Errorf("could not read PID file %s: %w", pidPath, err))
		}
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
			fail(fmt.Errorf("invalid PID content in %s: %w", pidPath, err))
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			fail(err)
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fail(fmt.Errorf("failed to signal fieldd [PID %d]: %w", pid, err))
		}
		fmt.Printf("Sent SIGTERM to fieldd [PID %d]\n", pid)

	default:
		fmt.Printf("Unknown daemon subcommand: %s\n", args[1])
		os.Exit(1)
	}
}
