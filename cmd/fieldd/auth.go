package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	tokenEnvVar   = "FIELDD_TOKEN"
	tokenFileName = "fieldd.token"
	tokenBytes    = 32
)

func tokenFilePath(stateDir string) string {
	return filepath.Join(stateDir, tokenFileName)
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		// ":8899" is treated as all interfaces, not loopback.
		return false
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func httpBindHost(addr string) (host string, loopback bool, err error) {
	if addr == "" {
		return "", true, nil
	}
	host, _, err = net.SplitHostPort(addr)
	if err != nil {
		return "", false, fmt.Errorf("invalid HTTP listener address %q: %w", addr, err)
	}
	return host, isLoopbackHost(host), nil
}

// resolveListenToken returns the bearer token used to protect mutating
// routes. On loopback (or when HTTP is disabled) a 0600 token file is
// created in the state directory if neither FIELDD_TOKEN nor the file
// already provides one. A non-loopback bind is refused when no token
// is already available.
func resolveListenToken(stateDir, httpAddr string) (token string, loopback bool, err error) {
	_, loopback, err = httpBindHost(httpAddr)
	if err != nil {
		return "", false, err
	}
	token, err = loadToken(stateDir)
	if err != nil {
		return "", loopback, err
	}
	if token != "" {
		return token, loopback, nil
	}
	if !loopback {
		return "", false, fmt.Errorf("refusing to start mutating HTTP on non-loopback %s without %s or a 0600 %s", httpAddr, tokenEnvVar, tokenFilePath(stateDir))
	}
	token, err = createTokenFile(tokenFilePath(stateDir))
	if err != nil {
		return "", loopback, err
	}
	return token, loopback, nil
}

func loadToken(stateDir string) (string, error) {
	if env := strings.TrimSpace(os.Getenv(tokenEnvVar)); env != "" {
		return env, nil
	}
	path := tokenFilePath(stateDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := enforceTokenPerms(path); err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(data))
	if tok == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return tok, nil
}

func createTokenFile(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(tok + "\n"); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return tok, nil
}

func enforceTokenPerms(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		if err := os.Chmod(path, 0600); err != nil {
			return fmt.Errorf("token file %s must be mode 0600: %w", path, err)
		}
	}
	return nil
}

func tokenEqual(got, want string) bool {
	if want == "" {
		return false
	}
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

func bearerToken(req *http.Request) string {
	h := strings.TrimSpace(req.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	typ, rest, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(typ, "Bearer") {
		return ""
	}
	return strings.TrimSpace(rest)
}

func requireBearer(token string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !tokenEqual(bearerToken(req), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fn(w, req)
	}
}

func maybeRequireBearer(token string, loopback bool, fn http.HandlerFunc) http.HandlerFunc {
	if loopback {
		return fn
	}
	return requireBearer(token, fn)
}

func maybeRequireBearerHandler(token string, loopback bool, h http.Handler) http.Handler {
	if loopback {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !tokenEqual(bearerToken(req), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, req)
	})
}

func requestToken(req Request) string {
	if strings.TrimSpace(req.Token) != "" {
		return strings.TrimSpace(req.Token)
	}
	return strings.TrimSpace(req.Authorization)
}

func isMutatingIPCMethod(method string) bool {
	switch method {
	case "status", "":
		return false
	default:
		return true
	}
}
