// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package observability

import (
	"fmt"
	"net/http"
	"net/http/pprof"

	log "github.com/sirupsen/logrus"
)

// StartPprofServer starts a pprof HTTP server on the given port.
// It runs in a background goroutine and never returns under normal operation.
// The server is bound to localhost only for security.
//
// Endpoints available:
//   - /debug/pprof/            — index page
//   - /debug/pprof/profile     — CPU profile (30s default)
//   - /debug/pprof/heap        — heap memory profile
//   - /debug/pprof/goroutine   — goroutine dump
//   - /debug/pprof/block       — blocking profile
//   - /debug/pprof/mutex       — mutex contention profile
func StartPprofServer(port int) {
	if port <= 0 {
		port = 6060
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Infof("Starting pprof server on %s", addr)

	// Create a dedicated mux for pprof instead of using the global DefaultServeMux
	// This prevents pprof from being exposed on all HTTP servers in the application
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
			log.Errorf("pprof server failed: %v", err)
		}
	}()
}
