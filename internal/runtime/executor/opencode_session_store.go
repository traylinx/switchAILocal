// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"sync"
	"time"
)

// OpenCodeSessionStore manages the mapping between client session IDs and OpenCode session IDs.
// It provides thread-safe access to session data with optional TTL-based expiration.
type OpenCodeSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
	ttl      time.Duration
	done     chan struct{}
}

type sessionEntry struct {
	openCodeSessionID string
	createdAt         time.Time
	lastAccessedAt    time.Time
}

// NewOpenCodeSessionStore creates a new session store with the specified TTL.
// If ttl is 0, sessions never expire.
func NewOpenCodeSessionStore(ttl time.Duration) *OpenCodeSessionStore {
	store := &OpenCodeSessionStore{
		sessions: make(map[string]*sessionEntry),
		ttl:      ttl,
		done:     make(chan struct{}),
	}

	// Start cleanup goroutine if TTL is set
	if ttl > 0 {
		go store.cleanupLoop()
	}

	return store
}

// Stop gracefully shuts down the session store's cleanup goroutine.
// This should be called when the store is no longer needed.
func (s *OpenCodeSessionStore) Stop() {
	select {
	case <-s.done:
		// Already stopped
	default:
		close(s.done)
	}
}

// Get retrieves the OpenCode session ID for a client session ID.
// Returns empty string if not found or expired.
func (s *OpenCodeSessionStore) Get(clientSessionID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.sessions[clientSessionID]
	if !exists {
		return "", false
	}

	// Check expiration
	if s.ttl > 0 && time.Since(entry.lastAccessedAt) > s.ttl {
		return "", false
	}

	return entry.openCodeSessionID, true
}

// Set stores the mapping between client session ID and OpenCode session ID.
func (s *OpenCodeSessionStore) Set(clientSessionID, openCodeSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.sessions[clientSessionID] = &sessionEntry{
		openCodeSessionID: openCodeSessionID,
		createdAt:         now,
		lastAccessedAt:    now,
	}
}

// GetOrCreate retrieves an existing OpenCode session ID or signals that a new one should be created.
// Returns (openCodeSessionID, isNew).
func (s *OpenCodeSessionStore) GetOrCreate(clientSessionID string) (string, bool) {
	// First try read-only access
	if sessionID, exists := s.Get(clientSessionID); exists {
		// Update last accessed time
		s.mu.Lock()
		if entry, ok := s.sessions[clientSessionID]; ok {
			entry.lastAccessedAt = time.Now()
		}
		s.mu.Unlock()
		return sessionID, false
	}

	return "", true // Signal that a new session should be created
}

// Touch updates the last accessed time for a session.
func (s *OpenCodeSessionStore) Touch(clientSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.sessions[clientSessionID]; exists {
		entry.lastAccessedAt = time.Now()
	}
}

// Delete removes a session mapping.
func (s *OpenCodeSessionStore) Delete(clientSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, clientSessionID)
}

// Count returns the number of active sessions.
func (s *OpenCodeSessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.sessions)
}

// cleanupLoop periodically removes expired sessions.
// It exits when Stop() is called.
func (s *OpenCodeSessionStore) cleanupLoop() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *OpenCodeSessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for clientID, entry := range s.sessions {
		if now.Sub(entry.lastAccessedAt) > s.ttl {
			delete(s.sessions, clientID)
		}
	}
}
