// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"sync"

	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

var (
	storeMu         sync.RWMutex
	registeredStore coreauth.Store
)

// RegisterTokenStore sets the global token store used by the authentication helpers.
func RegisterTokenStore(store coreauth.Store) {
	storeMu.Lock()
	registeredStore = store
	storeMu.Unlock()
}

// GetTokenStore returns the globally registered token store.
func GetTokenStore() coreauth.Store {
	storeMu.RLock()
	s := registeredStore
	storeMu.RUnlock()
	if s != nil {
		return s
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if registeredStore == nil {
		registeredStore = NewFileTokenStore()
	}
	return registeredStore
}
