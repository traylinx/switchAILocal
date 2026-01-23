// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ExecuteRequest struct {
	Binary string   `json:"binary"`
	Args   []string `json:"args"`
}

type ExecuteResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func main() {
	listenAddr := flag.String("listen-address", "", "Address to listen on (e.g., 127.0.0.1:18888)")
	flag.Parse()

	secret := os.Getenv("BRIDGE_AGENT_SECRET")
	if secret == "" {
		log.Fatal("FATAL: BRIDGE_AGENT_SECRET environment variable must be set")
	}

	http.Handle("/run", authMiddleware(secret, http.HandlerFunc(handleRun)))

	port := os.Getenv("BRIDGE_PORT")
	if port == "" {
		port = "18888"
	}

	var addr string
	if *listenAddr != "" {
		addr = *listenAddr
	} else {
		// Secure default: bind to localhost only
		addr = "127.0.0.1:" + port
	}

	log.Printf("SwitchAILocal Bridge Agent listening on %s...", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Strict Binary Whitelist
	allowedBinaries := map[string]bool{
		"gemini": true,
		"claude": true,
		"vibe":   true,
		"codex":  true,
	}

	base := filepath.Base(req.Binary)
	if !allowedBinaries[base] {
		log.Printf("Security Block: Attempted to run non-whitelisted binary: %s", base)
		http.Error(w, "Forbidden: Binary not in whitelist", http.StatusBadRequest)
		return
	}

	// Strict Argument Sanitization
	// Block shell metacharacters that could allow command injection
	if strings.ContainsAny(strings.Join(req.Args, ""), ";|&`$<>") {
		log.Printf("Security Block: Dangerous arguments detected: %v", req.Args)
		http.Error(w, "Forbidden: Dangerous characters in arguments", http.StatusBadRequest)
		return
	}

	// Resolve binary path
	binary := req.Binary
	if path, err := exec.LookPath(base); err == nil {
		binary = path
	} else {
		// Try common locations if not in PATH (especially for LaunchAgents)
		home, _ := os.UserHomeDir()
		commonPaths := []string{
			"/usr/local/bin/" + base,
			"/opt/homebrew/bin/" + base,
			home + "/.nvm/versions/node/*/bin/" + base,
		}

		for _, p := range commonPaths {
			if strings.Contains(p, "*") {
				matches, _ := filepath.Glob(p)
				if len(matches) > 0 {
					binary = matches[0]
					break
				}
			} else if _, err := os.Stat(p); err == nil {
				binary = p
				break
			}
		}
	}

	log.Printf("Executing on Host: %s %v", binary, req.Args)
	cmd := exec.Command(binary, req.Args...)

	// Inject existing environment and update PATH to include the binary's directory.
	// This helps tools like 'gemini' (Node scripts) find 'node' if it's in the same bin folder.
	env := os.Environ()
	binDir := filepath.Dir(binary)
	foundPath := false
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + binDir + ":" + e[len("PATH="):]
			foundPath = true
			break
		}
	}
	if !foundPath {
		env = append(env, "PATH="+binDir)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	resp := ExecuteResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		resp.Error = err.Error()
		if exitError, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitError.ExitCode()
		} else {
			resp.ExitCode = 1
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

func authMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token != secret {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
