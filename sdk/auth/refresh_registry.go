// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"time"

	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

func init() {
	registerRefreshLead("codex", func() Authenticator { return NewCodexAuthenticator() })
	registerRefreshLead("claude", func() Authenticator { return NewClaudeAuthenticator() })
	registerRefreshLead("qwen", func() Authenticator { return NewQwenAuthenticator() })
	registerRefreshLead("iflow", func() Authenticator { return NewIFlowAuthenticator() })
	registerRefreshLead("gemini", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("geminicli", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("antigravity", func() Authenticator { return NewAntigravityAuthenticator() })
}

func registerRefreshLead(provider string, factory func() Authenticator) {
	switchailocalauth.RegisterRefreshLeadProvider(provider, func() *time.Duration {
		if factory == nil {
			return nil
		}
		auth := factory()
		if auth == nil {
			return nil
		}
		return auth.RefreshLead()
	})
}
