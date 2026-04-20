// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package notifications implements outbound notification relays so sandboxed
// clients (Tytus pods, restricted agents, etc.) can emit messages to external
// services like Telegram without ever holding the upstream credentials.
//
// The first relay is /v1/notifications/telegram/sendMessage, which forwards a
// curated subset of Telegram's Bot API sendMessage to api.telegram.org using a
// bot token configured server-side. The pattern generalises: a future
// /v1/notifications/slack/postMessage can live alongside it with the same
// "named credential, server-side token, allowlisted destination" shape.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/sdk/api/handlers"
)

// telegramAPIBase is the upstream Telegram Bot API host. Exposed as a package
// var so tests can swap it for an httptest server.
var telegramAPIBase = "https://api.telegram.org"

// telegramHTTPTimeout is the per-request timeout for the upstream call.
// Telegram's sendMessage is normally < 1s; 15s is generous and bounded.
var telegramHTTPTimeout = 15 * time.Second

// Handler exposes the /v1/notifications/* endpoints.
type Handler struct {
	base *handlers.BaseAPIHandler
}

// NewHandler constructs a notifications handler. The handler reads its config
// (bot tokens, allowlists) from base.Cfg.Notifications on every request, so
// runtime config reloads take effect without restart.
func NewHandler(base *handlers.BaseAPIHandler) *Handler {
	return &Handler{base: base}
}

// telegramSendMessageRequest is the curated subset of Telegram's sendMessage
// parameters we accept from clients. We deliberately omit fields that could
// be abused (e.g. inline_keyboard with arbitrary URLs) — operators who need
// them can add them explicitly later.
//
// Token sourcing (highest priority first):
//  1. X-Telegram-Bot-Token HTTP header
//  2. bot_token field in the JSON body
//  3. Named bot lookup in notifications.telegram.bots (via Bot field)
//  4. Named bot "default" if Bot is empty
//
// In the wannolot/Tytus model each pod belongs to a different customer and
// brings its OWN bot token — the droplet operator doesn't know or hold
// customer tokens. So per-request token passing (header or body) is the
// primary path; server-side config.bots is only for operator-shared bots
// (e.g. platform-wide alerts).
type telegramSendMessageRequest struct {
	// Bot is the named bot to relay through (matches a Name in
	// notifications.telegram.bots). Only consulted when no token is supplied
	// in the header or body. Empty defaults to "default".
	Bot string `json:"bot,omitempty"`

	// BotToken is the per-request bot token supplied by the client. Takes
	// precedence over named-bot config lookup. Prefer the
	// X-Telegram-Bot-Token header over putting the token in the body.
	BotToken string `json:"bot_token,omitempty"`

	// ChatID is the Telegram chat the message should be sent to. Required.
	// Telegram chat_ids are 64-bit signed integers (channels and supergroups
	// are negative). We accept JSON number; clients passing a string number
	// will get a 400 — they should marshal as int.
	ChatID int64 `json:"chat_id"`

	// Text is the message body. Required, max 4096 chars per Telegram.
	Text string `json:"text"`

	// ParseMode is one of "Markdown", "MarkdownV2", "HTML", or "" (plain).
	ParseMode string `json:"parse_mode,omitempty"`

	// DisableWebPagePreview suppresses link previews when true.
	DisableWebPagePreview bool `json:"disable_web_page_preview,omitempty"`

	// DisableNotification sends silently when true (no push).
	DisableNotification bool `json:"disable_notification,omitempty"`

	// ReplyToMessageID makes this message a reply to an existing one.
	ReplyToMessageID int64 `json:"reply_to_message_id,omitempty"`
}

// telegramHeaderToken is the header name clients use to pass per-request bot
// tokens. Preferred over bot_token in the body because headers are not
// normally captured by body-logging middleware.
const telegramHeaderToken = "X-Telegram-Bot-Token"

// errorResponse is the gateway's error envelope. It mirrors the shape the
// rest of the OpenAI-compatible surface uses, so clients can parse uniformly.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func writeError(c *gin.Context, status int, errType, code, msg string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: errorBody{
		Message: msg, Type: errType, Code: code,
	}})
}

// TelegramSendMessage handles POST /v1/notifications/telegram/sendMessage.
//
// It validates the request, looks up the named bot's token from server-side
// config, applies the chat-ID allowlist if any, and relays to api.telegram.org.
// The Telegram response body is returned verbatim to the caller so they can
// see message_id, date, etc.
//
// Audit: every call (success or failure) emits a structured log line with the
// principal (api key id), bot name, chat_id, and outcome — never the token.
func (h *Handler) TelegramSendMessage(c *gin.Context) {
	cfg := h.base.Cfg
	if cfg == nil {
		writeError(c, http.StatusServiceUnavailable, "service_unavailable", "no_config",
			"notifications: gateway has no active config")
		return
	}

	tg := cfg.Notifications.Telegram
	// v0.5.1: endpoint is available whenever master switch is true — tokens
	// come per-request from the caller. Server-side bots are only used as a
	// fallback for operator-managed shared bots.
	if !tg.Enabled {
		writeError(c, http.StatusServiceUnavailable, "service_unavailable", "telegram_disabled",
			"notifications.telegram is disabled on this gateway")
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "read_body_failed",
			"could not read request body: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "empty_body",
			"request body is empty")
		return
	}

	var req telegramSendMessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "invalid_json",
			"request body is not valid JSON: "+err.Error())
		return
	}
	if req.ChatID == 0 {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "missing_chat_id",
			"chat_id is required and must be a non-zero integer")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "missing_text",
			"text is required and must be non-empty")
		return
	}
	if len(req.Text) > 4096 {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "text_too_long",
			"text exceeds Telegram's 4096-character limit")
		return
	}

	// Token resolution: header > body field > named bot > "default" bot
	token, tokenSource, botName := resolveTelegramToken(c, req, &cfg.Notifications)
	if token == "" {
		// Distinguish "user asked for a named bot that doesn't exist" (404)
		// from "no token supplied anywhere" (400). If the caller passed
		// bot:"foo" explicitly and we got here, it's the former.
		if strings.TrimSpace(req.Bot) != "" {
			writeError(c, http.StatusNotFound, "invalid_request_error", "unknown_bot",
				fmt.Sprintf("no telegram bot configured with name %q and no per-request token supplied", req.Bot))
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_request_error", "no_bot_token",
			"no bot token provided: supply X-Telegram-Bot-Token header, bot_token body field, "+
				"or configure a named bot on the gateway")
		return
	}

	// Server-side named bots can declare an allowed-chat-ids list. Per-request
	// tokens (header/body) bypass this — the caller owns the token so they
	// own the allowlist concern themselves.
	if tokenSource == "config" {
		if bot, ok := cfg.Notifications.LookupTelegramBot(req.Bot); ok && !bot.IsChatAllowed(req.ChatID) {
			writeError(c, http.StatusForbidden, "permission_denied", "chat_not_allowed",
				fmt.Sprintf("bot %q is not permitted to message chat_id %d", bot.Name, req.ChatID))
			return
		}
	}

	principal := principalFromContext(c)
	apiURL := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(telegramAPIBase, "/"), token)

	// Marshal the curated payload for upstream. Note we re-marshal rather than
	// forward `body` verbatim: this drops any client-supplied fields we don't
	// explicitly allow (defence in depth).
	upstreamPayload := map[string]any{
		"chat_id": req.ChatID,
		"text":    req.Text,
	}
	if req.ParseMode != "" {
		upstreamPayload["parse_mode"] = req.ParseMode
	}
	if req.DisableWebPagePreview {
		upstreamPayload["disable_web_page_preview"] = true
	}
	if req.DisableNotification {
		upstreamPayload["disable_notification"] = true
	}
	if req.ReplyToMessageID != 0 {
		upstreamPayload["reply_to_message_id"] = req.ReplyToMessageID
	}
	upstreamBody, _ := json.Marshal(upstreamPayload)

	ctx, cancel := context.WithTimeout(c.Request.Context(), telegramHTTPTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(upstreamBody))
	if err != nil {
		log.WithError(err).Error("notifications.telegram: build request failed")
		writeError(c, http.StatusInternalServerError, "internal_error", "build_request_failed",
			"could not build upstream request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := newTelegramHTTPClient(cfg.ProxyURL)
	resp, err := client.Do(httpReq)
	if err != nil {
		log.WithFields(log.Fields{
			"principal": principal, "bot": botName, "token_source": tokenSource, "chat_id": req.ChatID,
		}).WithError(err).Warn("notifications.telegram: upstream call failed")
		writeError(c, http.StatusBadGateway, "upstream_error", "telegram_unreachable",
			"telegram api unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.WithError(err).Warn("notifications.telegram: read upstream response failed")
		writeError(c, http.StatusBadGateway, "upstream_error", "telegram_response_read_failed",
			"could not read telegram response: "+err.Error())
		return
	}

	log.WithFields(log.Fields{
		"principal":    principal,
		"bot":          botName,
		"token_source": tokenSource,
		"chat_id":      req.ChatID,
		"text_len":     len(req.Text),
		"upstream":     "telegram",
		"http_status":  resp.StatusCode,
	}).Info("notifications.telegram: relayed")

	c.Data(resp.StatusCode, "application/json; charset=utf-8", respBody)
}

// resolveTelegramToken picks the bot token + friendly identifier for this
// call. Priority order:
//  1. X-Telegram-Bot-Token header — takes precedence, preferred for clients
//     that don't want tokens appearing in body logs
//  2. bot_token field in the JSON body — fallback for clients that can't set
//     custom headers (some HTTP clients strip non-standard ones)
//  3. Named bot from notifications.telegram.bots (req.Bot, or "default" when
//     empty) — for operator-managed shared bots
//
// Returns (token, source, displayName). source is "header" | "body" |
// "config" | "" (none found). displayName is a log-safe identifier — never
// the token itself.
func resolveTelegramToken(c *gin.Context, req telegramSendMessageRequest, n *config.NotificationsConfig) (string, string, string) {
	if h := strings.TrimSpace(c.GetHeader(telegramHeaderToken)); h != "" {
		return h, "header", "client-supplied"
	}
	if b := strings.TrimSpace(req.BotToken); b != "" {
		return b, "body", "client-supplied"
	}
	if bot, ok := n.LookupTelegramBot(req.Bot); ok {
		return bot.Token, "config", bot.Name
	}
	return "", "", coalesceBot(req.Bot)
}

// principalFromContext returns the API key principal stored by AuthMiddleware,
// or "anonymous" if missing (which should not happen since the route is
// behind v1.Use(AuthMiddleware)).
func principalFromContext(c *gin.Context) string {
	if v, ok := c.Get("apiKey"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "anonymous"
}

// coalesceBot reports the bot name as the user supplied it, or "default" if
// the field was empty (which is what LookupTelegramBot maps to internally).
func coalesceBot(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

// newTelegramHTTPClient returns an http.Client honouring the gateway's global
// ProxyURL if set. Telegram requests are simple enough that we don't need the
// full proxy_helpers.go machinery (per-auth proxies, RoundTripper-from-ctx);
// if those become necessary later we can swap this for newProxyAwareHTTPClient.
func newTelegramHTTPClient(proxyURL string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(proxyURL); p != "" {
		if u, err := url.Parse(p); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   telegramHTTPTimeout,
	}
}
