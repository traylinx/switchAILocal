// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/traylinx/switchAILocal/internal/auth/claude"
	"github.com/traylinx/switchAILocal/internal/misc"
	"github.com/traylinx/switchAILocal/internal/util"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

const (
	anthropicCallbackPort = 54545
)

func (h *Handler) RequestAnthropicToken(c *gin.Context) {
	ctx := context.Background()

	fmt.Println("Initializing Claude authentication...")

	// Generate PKCE codes
	pkceCodes, err := claude.GeneratePKCECodes()
	if err != nil {
		log.Errorf("Failed to generate PKCE codes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PKCE codes"})
		return
	}

	// Generate random state parameter
	state, err := misc.GenerateRandomState()
	if err != nil {
		log.Errorf("Failed to generate state parameter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state parameter"})
		return
	}

	// Initialize Claude auth service
	anthropicAuth := claude.NewClaudeAuth(h.cfg)

	// Generate authorization URL (then override redirect_uri to reuse server port)
	authURL, state, err := anthropicAuth.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		log.Errorf("Failed to generate authorization URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization url"})
		return
	}

	RegisterOAuthSession(state, "anthropic")

	isWebUI := isWebUIRequest(c)
	var forwarder *callbackForwarder
	if isWebUI {
		targetURL, errTarget := h.managementCallbackURL("/anthropic/callback")
		if errTarget != nil {
			log.WithError(errTarget).Error("failed to compute anthropic callback target")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "callback server unavailable"})
			return
		}
		var errStart error
		if forwarder, errStart = startCallbackForwarder(anthropicCallbackPort, "anthropic", targetURL); errStart != nil {
			log.WithError(errStart).Error("failed to start anthropic callback forwarder")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start callback server"})
			return
		}
	}

	go func() {
		if isWebUI {
			defer stopCallbackForwarderInstance(anthropicCallbackPort, forwarder)
		}

		// Helper: wait for callback file
		waitFile := filepath.Join(h.cfg.AuthDir, fmt.Sprintf(".oauth-anthropic-%s.oauth", state))
		waitForFile := func(path string, timeout time.Duration) (map[string]string, error) {
			deadline := time.Now().Add(timeout)
			for {
				if !IsOAuthSessionPending(state, "anthropic") {
					return nil, errOAuthSessionNotPending
				}
				if time.Now().After(deadline) {
					SetOAuthSessionError(state, "Timeout waiting for OAuth callback")
					return nil, fmt.Errorf("timeout waiting for OAuth callback")
				}
				data, errRead := os.ReadFile(path)
				if errRead == nil {
					var m map[string]string
					_ = json.Unmarshal(data, &m)
					_ = os.Remove(path)
					return m, nil
				}
				time.Sleep(500 * time.Millisecond)
			}
		}

		fmt.Println("Waiting for authentication callback...")
		// Wait up to 5 minutes
		resultMap, errWait := waitForFile(waitFile, 5*time.Minute)
		if errWait != nil {
			if errors.Is(errWait, errOAuthSessionNotPending) {
				return
			}
			authErr := claude.NewAuthenticationError(claude.ErrCallbackTimeout, errWait)
			log.Error(claude.GetUserFriendlyMessage(authErr))
			return
		}
		if errStr := resultMap["error"]; errStr != "" {
			oauthErr := claude.NewOAuthError(errStr, "", http.StatusBadRequest)
			log.Error(claude.GetUserFriendlyMessage(oauthErr))
			SetOAuthSessionError(state, "Bad request")
			return
		}
		if resultMap["state"] != state {
			authErr := claude.NewAuthenticationError(claude.ErrInvalidState, fmt.Errorf("expected %s, got %s", state, resultMap["state"]))
			log.Error(claude.GetUserFriendlyMessage(authErr))
			SetOAuthSessionError(state, "State code error")
			return
		}

		// Parse code (Claude may append state after '#')
		rawCode := resultMap["code"]
		code := strings.Split(rawCode, "#")[0]

		// Exchange code for tokens (replicate logic using updated redirect_uri)
		// Extract client_id from the modified auth URL
		clientID := ""
		if u2, errP := url.Parse(authURL); errP == nil {
			clientID = u2.Query().Get("client_id")
		}
		// Build request
		bodyMap := map[string]any{
			"code":          code,
			"state":         state,
			"grant_type":    "authorization_code",
			"client_id":     clientID,
			"redirect_uri":  "http://localhost:54545/callback",
			"code_verifier": pkceCodes.CodeVerifier,
		}
		bodyJSON, _ := json.Marshal(bodyMap)

		httpClient := util.SetProxy(&h.cfg.SDKConfig, &http.Client{})
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://console.anthropic.com/v1/oauth/token", strings.NewReader(string(bodyJSON)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, errDo := httpClient.Do(req)
		if errDo != nil {
			authErr := claude.NewAuthenticationError(claude.ErrCodeExchangeFailed, errDo)
			log.Errorf("Failed to exchange authorization code for tokens: %v", authErr)
			SetOAuthSessionError(state, "Failed to exchange authorization code for tokens")
			return
		}
		defer func() {
			if errClose := resp.Body.Close(); errClose != nil {
				log.Errorf("failed to close response body: %v", errClose)
			}
		}()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			log.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(respBody))
			SetOAuthSessionError(state, fmt.Sprintf("token exchange failed with status %d", resp.StatusCode))
			return
		}
		var tResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			Account      struct {
				EmailAddress string `json:"email_address"`
			} `json:"account"`
		}
		if errU := json.Unmarshal(respBody, &tResp); errU != nil {
			log.Errorf("failed to parse token response: %v", errU)
			SetOAuthSessionError(state, "Failed to parse token response")
			return
		}
		bundle := &claude.ClaudeAuthBundle{
			TokenData: claude.ClaudeTokenData{
				AccessToken:  tResp.AccessToken,
				RefreshToken: tResp.RefreshToken,
				Email:        tResp.Account.EmailAddress,
				Expire:       time.Now().Add(time.Duration(tResp.ExpiresIn) * time.Second).Format(time.RFC3339),
			},
			LastRefresh: time.Now().Format(time.RFC3339),
		}

		// Create token storage
		tokenStorage := anthropicAuth.CreateTokenStorage(bundle)
		record := &coreauth.Auth{
			ID:       fmt.Sprintf("claude-%s.json", tokenStorage.Email),
			Provider: "claude",
			FileName: fmt.Sprintf("claude-%s.json", tokenStorage.Email),
			Storage:  tokenStorage,
			Metadata: map[string]any{"email": tokenStorage.Email},
		}
		savedPath, errSave := h.saveTokenRecord(ctx, record)
		if errSave != nil {
			log.Errorf("Failed to save authentication tokens: %v", errSave)
			SetOAuthSessionError(state, "Failed to save authentication tokens")
			return
		}

		fmt.Printf("Authentication successful! Token saved to %s\n", savedPath)
		if bundle.APIKey != "" {
			fmt.Println("API key obtained and saved")
		}
		fmt.Println("You can now use Claude services through this CLI")
		CompleteOAuthSession(state)
		CompleteOAuthSessionsByProvider("anthropic")
	}()

	c.JSON(200, gin.H{"status": "ok", "url": authURL, "state": state})
}
