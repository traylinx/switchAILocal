// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"context"
	"encoding/json"
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

	"github.com/traylinx/switchAILocal/internal/misc"
	"github.com/traylinx/switchAILocal/internal/secret"
	"github.com/traylinx/switchAILocal/internal/util"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

func (h *Handler) RequestAntigravityToken(c *gin.Context) {
	const (
		antigravityCallbackPort = 51121
	)
	var (
		antigravityClientID     = secret.AntigravityClientID()
		antigravityClientSecret = secret.AntigravityClientSecret()
	)
	var antigravityScopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}

	ctx := context.Background()

	fmt.Println("Initializing Antigravity authentication...")

	state, errState := misc.GenerateRandomState()
	if errState != nil {
		log.Errorf("Failed to generate state parameter: %v", errState)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state parameter"})
		return
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/oauth-callback", antigravityCallbackPort)

	params := url.Values{}
	params.Set("access_type", "offline")
	params.Set("client_id", antigravityClientID)
	params.Set("prompt", "consent")
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(antigravityScopes, " "))
	params.Set("state", state)
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()

	RegisterOAuthSession(state, "antigravity")

	isWebUI := isWebUIRequest(c)
	var forwarder *callbackForwarder
	if isWebUI {
		targetURL, errTarget := h.managementCallbackURL("/antigravity/callback")
		if errTarget != nil {
			log.WithError(errTarget).Error("failed to compute antigravity callback target")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "callback server unavailable"})
			return
		}
		var errStart error
		if forwarder, errStart = startCallbackForwarder(antigravityCallbackPort, "antigravity", targetURL); errStart != nil {
			log.WithError(errStart).Error("failed to start antigravity callback forwarder")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start callback server"})
			return
		}
	}

	go func() {
		if isWebUI {
			defer stopCallbackForwarderInstance(antigravityCallbackPort, forwarder)
		}

		waitFile := filepath.Join(h.cfg.AuthDir, fmt.Sprintf(".oauth-antigravity-%s.oauth", state))
		deadline := time.Now().Add(5 * time.Minute)
		var authCode string
		for {
			if !IsOAuthSessionPending(state, "antigravity") {
				return
			}
			if time.Now().After(deadline) {
				log.Error("oauth flow timed out")
				SetOAuthSessionError(state, "OAuth flow timed out")
				return
			}
			if data, errReadFile := os.ReadFile(waitFile); errReadFile == nil {
				var payload map[string]string
				_ = json.Unmarshal(data, &payload)
				_ = os.Remove(waitFile)
				if errStr := strings.TrimSpace(payload["error"]); errStr != "" {
					log.Errorf("Authentication failed: %s", errStr)
					SetOAuthSessionError(state, "Authentication failed")
					return
				}
				if payloadState := strings.TrimSpace(payload["state"]); payloadState != "" && payloadState != state {
					log.Errorf("Authentication failed: state mismatch")
					SetOAuthSessionError(state, "Authentication failed: state mismatch")
					return
				}
				authCode = strings.TrimSpace(payload["code"])
				if authCode == "" {
					log.Error("Authentication failed: code not found")
					SetOAuthSessionError(state, "Authentication failed: code not found")
					return
				}
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		httpClient := util.SetProxy(&h.cfg.SDKConfig, &http.Client{})
		form := url.Values{}
		form.Set("code", authCode)
		form.Set("client_id", antigravityClientID)
		form.Set("client_secret", antigravityClientSecret)
		form.Set("redirect_uri", redirectURI)
		form.Set("grant_type", "authorization_code")

		req, errNewRequest := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
		if errNewRequest != nil {
			log.Errorf("Failed to build token request: %v", errNewRequest)
			SetOAuthSessionError(state, "Failed to build token request")
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, errDo := httpClient.Do(req)
		if errDo != nil {
			log.Errorf("Failed to execute token request: %v", errDo)
			SetOAuthSessionError(state, "Failed to exchange token")
			return
		}
		defer func() {
			if errClose := resp.Body.Close(); errClose != nil {
				log.Errorf("antigravity token exchange close error: %v", errClose)
			}
		}()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			bodyBytes, _ := io.ReadAll(resp.Body)
			log.Errorf("Antigravity token exchange failed with status %d: %s", resp.StatusCode, string(bodyBytes))
			SetOAuthSessionError(state, fmt.Sprintf("Token exchange failed: %d", resp.StatusCode))
			return
		}

		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
			TokenType    string `json:"token_type"`
		}
		if errDecode := json.NewDecoder(resp.Body).Decode(&tokenResp); errDecode != nil {
			log.Errorf("Failed to parse token response: %v", errDecode)
			SetOAuthSessionError(state, "Failed to parse token response")
			return
		}

		email := ""
		if strings.TrimSpace(tokenResp.AccessToken) != "" {
			infoReq, errInfoReq := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
			if errInfoReq != nil {
				log.Errorf("Failed to build user info request: %v", errInfoReq)
				SetOAuthSessionError(state, "Failed to build user info request")
				return
			}
			infoReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

			infoResp, errInfo := httpClient.Do(infoReq)
			if errInfo != nil {
				log.Errorf("Failed to execute user info request: %v", errInfo)
				SetOAuthSessionError(state, "Failed to execute user info request")
				return
			}
			defer func() {
				if errClose := infoResp.Body.Close(); errClose != nil {
					log.Errorf("antigravity user info close error: %v", errClose)
				}
			}()

			if infoResp.StatusCode >= http.StatusOK && infoResp.StatusCode < http.StatusMultipleChoices {
				var infoPayload struct {
					Email string `json:"email"`
				}
				if errDecodeInfo := json.NewDecoder(infoResp.Body).Decode(&infoPayload); errDecodeInfo == nil {
					email = strings.TrimSpace(infoPayload.Email)
				}
			}
		}

		log.Infof("Successfully authenticated Antigravity session %s (email: %s)", state, email)

		projectID := ""
		if strings.TrimSpace(tokenResp.AccessToken) != "" {
			fetchedProjectID, errProject := sdkAuth.FetchAntigravityProjectID(ctx, tokenResp.AccessToken, httpClient)
			if errProject != nil {
				log.Warnf("antigravity: failed to fetch project ID: %v", errProject)
			} else {
				projectID = fetchedProjectID
				log.Infof("antigravity: obtained project ID %s", projectID)
			}
		}

		now := time.Now()
		metadata := map[string]any{
			"type":          "antigravity",
			"access_token":  tokenResp.AccessToken,
			"refresh_token": tokenResp.RefreshToken,
			"expires_in":    tokenResp.ExpiresIn,
			"timestamp":     now.UnixMilli(),
			"expired":       now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
		}
		if email != "" {
			metadata["email"] = email
		}
		if projectID != "" {
			metadata["project_id"] = projectID
		}

		fileName := sanitizeAntigravityFileName(email)
		label := strings.TrimSpace(email)
		if label == "" {
			label = "antigravity"
		}

		record := &coreauth.Auth{
			ID:       fileName,
			Provider: "antigravity",
			FileName: fileName,
			Label:    label,
			Metadata: metadata,
		}
		savedPath, errSave := h.saveTokenRecord(ctx, record)
		if errSave != nil {
			log.Errorf("Failed to save token to file: %v", errSave)
			SetOAuthSessionError(state, "Failed to save token to file")
			return
		}

		fmt.Printf("Authentication successful! Token saved to %s\n", savedPath)
		if projectID != "" {
			fmt.Printf("Using GCP project: %s\n", projectID)
		}
		fmt.Println("You can now use Antigravity services through this CLI")
		CompleteOAuthSession(state)
		CompleteOAuthSessionsByProvider("antigravity")
	}()

	c.JSON(200, gin.H{"status": "ok", "url": authURL, "state": state})
}
