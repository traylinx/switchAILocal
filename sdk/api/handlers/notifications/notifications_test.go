// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package notifications

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/sdk/api/handlers"
)

func init() { gin.SetMode(gin.TestMode) }

// fakeTelegram returns an httptest server that records the last sendMessage
// it received and replies with the given status + body. The recorded path,
// authorization header, and body are exposed via the returned struct.
type fakeTelegram struct {
	server      *httptest.Server
	calls       int32
	lastPath    string
	lastBody    string
	lastHeaders http.Header
}

func newFakeTelegram(status int, replyBody string) *fakeTelegram {
	ft := &fakeTelegram{}
	ft.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ft.calls, 1)
		ft.lastPath = r.URL.Path
		ft.lastHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		ft.lastBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(replyBody))
	}))
	return ft
}

// newTestHandler builds a Handler whose telegram base URL points at the fake
// server, with a SDKConfig populated from the given Notifications block.
func newTestHandler(t *testing.T, fake *fakeTelegram, n config.NotificationsConfig) (*Handler, func()) {
	t.Helper()
	prevBase, prevTimeout := telegramAPIBase, telegramHTTPTimeout
	telegramAPIBase = fake.server.URL
	telegramHTTPTimeout = 3 * time.Second
	cleanup := func() {
		telegramAPIBase = prevBase
		telegramHTTPTimeout = prevTimeout
		fake.server.Close()
	}
	cfg := &config.SDKConfig{Notifications: n}
	base := &handlers.BaseAPIHandler{Cfg: cfg}
	return NewHandler(base), cleanup
}

func doRequest(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequestWithHeaders(t, h, body, nil)
}

func doRequestWithHeaders(t *testing.T, h *Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/telegram/sendMessage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	c.Set("apiKey", "test-principal")
	h.TelegramSendMessage(c)
	return rec
}

func TestTelegramSendMessage(t *testing.T) {
	defaultBots := config.NotificationsConfig{
		Telegram: config.TelegramNotificationsConfig{
			Enabled: true,
			Bots: []config.TelegramBot{
				{Name: "default", Token: "TOKEN-DEFAULT"},
				{Name: "alerts", Token: "TOKEN-ALERTS", AllowedChatIDs: []int64{12345}},
			},
		},
	}

	cases := []struct {
		name           string
		notifs         config.NotificationsConfig
		body           string
		fakeStatus     int
		fakeBody       string
		wantStatus     int
		wantUpstream   bool   // did we expect a call to telegram?
		wantPathSubstr string // a fragment of the upstream path we expect
		wantErrCode    string // code in the error envelope, if status != 200
		wantBodyField  string // a JSON field expected in the response body
	}{
		{
			name:           "happy path default bot",
			notifs:         defaultBots,
			body:           `{"chat_id":12345,"text":"hello"}`,
			fakeStatus:     http.StatusOK,
			fakeBody:       `{"ok":true,"result":{"message_id":42}}`,
			wantStatus:     http.StatusOK,
			wantUpstream:   true,
			wantPathSubstr: "/botTOKEN-DEFAULT/sendMessage",
			wantBodyField:  "message_id",
		},
		{
			name:           "named bot route",
			notifs:         defaultBots,
			body:           `{"bot":"alerts","chat_id":12345,"text":"hi"}`,
			fakeStatus:     http.StatusOK,
			fakeBody:       `{"ok":true,"result":{"message_id":7}}`,
			wantStatus:     http.StatusOK,
			wantUpstream:   true,
			wantPathSubstr: "/botTOKEN-ALERTS/sendMessage",
		},
		{
			name:        "no telegram configured",
			notifs:      config.NotificationsConfig{},
			body:        `{"chat_id":12345,"text":"hi"}`,
			wantStatus:  http.StatusServiceUnavailable,
			wantErrCode: "telegram_disabled",
		},
		{
			name:        "missing chat_id",
			notifs:      defaultBots,
			body:        `{"text":"hi"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "missing_chat_id",
		},
		{
			name:        "empty text",
			notifs:      defaultBots,
			body:        `{"chat_id":12345,"text":""}`,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "missing_text",
		},
		{
			name:        "text too long",
			notifs:      defaultBots,
			body:        `{"chat_id":12345,"text":"` + strings.Repeat("x", 4097) + `"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "text_too_long",
		},
		{
			name:        "invalid json",
			notifs:      defaultBots,
			body:        `{not json`,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_json",
		},
		{
			name:        "unknown bot name",
			notifs:      defaultBots,
			body:        `{"bot":"nope","chat_id":12345,"text":"hi"}`,
			wantStatus:  http.StatusNotFound,
			wantErrCode: "unknown_bot",
		},
		{
			name:        "chat_id not allowed for restricted bot",
			notifs:      defaultBots,
			body:        `{"bot":"alerts","chat_id":99999,"text":"hi"}`,
			wantStatus:  http.StatusForbidden,
			wantErrCode: "chat_not_allowed",
		},
		{
			name:         "upstream 4xx is forwarded to client",
			notifs:       defaultBots,
			body:         `{"chat_id":12345,"text":"hi"}`,
			fakeStatus:   http.StatusBadRequest,
			fakeBody:     `{"ok":false,"description":"chat not found"}`,
			wantStatus:   http.StatusBadRequest,
			wantUpstream: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeTelegram(tc.fakeStatus, tc.fakeBody)
			h, cleanup := newTestHandler(t, fake, tc.notifs)
			defer cleanup()

			rec := doRequest(t, h, tc.body)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			calls := int(atomic.LoadInt32(&fake.calls))
			if tc.wantUpstream && calls != 1 {
				t.Fatalf("upstream call count = %d, want 1", calls)
			}
			if !tc.wantUpstream && calls != 0 {
				t.Fatalf("upstream call count = %d, want 0", calls)
			}

			if tc.wantPathSubstr != "" && !strings.Contains(fake.lastPath, tc.wantPathSubstr) {
				t.Fatalf("upstream path = %q, want substring %q", fake.lastPath, tc.wantPathSubstr)
			}

			if tc.wantErrCode != "" {
				var env errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
					t.Fatalf("error envelope unmarshal failed: %v (body: %s)", err, rec.Body.String())
				}
				if env.Error.Code != tc.wantErrCode {
					t.Fatalf("error code = %q, want %q", env.Error.Code, tc.wantErrCode)
				}
			}
			if tc.wantBodyField != "" {
				if !strings.Contains(rec.Body.String(), tc.wantBodyField) {
					t.Fatalf("response body missing %q: %s", tc.wantBodyField, rec.Body.String())
				}
			}
		})
	}
}

// Per-request token path (v0.5.1): per-pod tokens supplied by the client via
// header OR body. This is the primary real-world path — droplet operators
// don't know customer bot tokens.
func TestTelegramSendMessagePerRequestToken(t *testing.T) {
	// enabled=true but zero bots configured — real wannolot case.
	openConfig := config.NotificationsConfig{
		Telegram: config.TelegramNotificationsConfig{Enabled: true},
	}

	t.Run("header token, no config", func(t *testing.T) {
		fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		h, cleanup := newTestHandler(t, fake, openConfig)
		defer cleanup()

		rec := doRequestWithHeaders(t, h, `{"chat_id":123,"text":"hi"}`, map[string]string{
			"X-Telegram-Bot-Token": "USER-TOKEN-123",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(fake.lastPath, "/botUSER-TOKEN-123/sendMessage") {
			t.Fatalf("upstream path = %q, want header token in it", fake.lastPath)
		}
	})

	t.Run("body token, no config", func(t *testing.T) {
		fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		h, cleanup := newTestHandler(t, fake, openConfig)
		defer cleanup()

		rec := doRequest(t, h, `{"bot_token":"BODY-TOKEN-456","chat_id":123,"text":"hi"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(fake.lastPath, "/botBODY-TOKEN-456/sendMessage") {
			t.Fatalf("upstream path = %q, want body token in it", fake.lastPath)
		}
		// bot_token must NOT be forwarded to Telegram in the outgoing body.
		if strings.Contains(fake.lastBody, "bot_token") || strings.Contains(fake.lastBody, "BODY-TOKEN-456") {
			t.Fatalf("upstream body leaked bot_token: %s", fake.lastBody)
		}
	})

	t.Run("header wins over body", func(t *testing.T) {
		fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		h, cleanup := newTestHandler(t, fake, openConfig)
		defer cleanup()

		rec := doRequestWithHeaders(t, h, `{"bot_token":"BODY","chat_id":123,"text":"hi"}`, map[string]string{
			"X-Telegram-Bot-Token": "HEADER",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if !strings.Contains(fake.lastPath, "/botHEADER/sendMessage") {
			t.Fatalf("upstream path = %q, header should have won", fake.lastPath)
		}
	})

	t.Run("header wins over config bot", func(t *testing.T) {
		fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		cfgWithBot := config.NotificationsConfig{
			Telegram: config.TelegramNotificationsConfig{
				Enabled: true,
				Bots:    []config.TelegramBot{{Name: "default", Token: "CONFIG-TOKEN"}},
			},
		}
		h, cleanup := newTestHandler(t, fake, cfgWithBot)
		defer cleanup()

		rec := doRequestWithHeaders(t, h, `{"chat_id":123,"text":"hi"}`, map[string]string{
			"X-Telegram-Bot-Token": "HEADER-WINS",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if !strings.Contains(fake.lastPath, "/botHEADER-WINS/sendMessage") {
			t.Fatalf("upstream path = %q, per-request token should override config", fake.lastPath)
		}
	})

	t.Run("no token anywhere returns 400 no_bot_token", func(t *testing.T) {
		fake := newFakeTelegram(http.StatusOK, ``)
		h, cleanup := newTestHandler(t, fake, openConfig)
		defer cleanup()

		rec := doRequest(t, h, `{"chat_id":123,"text":"hi"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d want 400, body = %s", rec.Code, rec.Body.String())
		}
		var env errorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if env.Error.Code != "no_bot_token" {
			t.Fatalf("code = %q want no_bot_token", env.Error.Code)
		}
		if atomic.LoadInt32(&fake.calls) != 0 {
			t.Fatalf("upstream was called, should not have been")
		}
	})

	t.Run("allowlist bypassed when using per-request token", func(t *testing.T) {
		// Config has an allowlist for bot "ops" that restricts to chat 1.
		// A caller passing their OWN header token can target any chat — the
		// allowlist is a server-side policy only for server-managed bots.
		fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		cfg := config.NotificationsConfig{
			Telegram: config.TelegramNotificationsConfig{
				Enabled: true,
				Bots:    []config.TelegramBot{{Name: "ops", Token: "OPS", AllowedChatIDs: []int64{1}}},
			},
		}
		h, cleanup := newTestHandler(t, fake, cfg)
		defer cleanup()

		rec := doRequestWithHeaders(t, h, `{"chat_id":9999,"text":"hi"}`, map[string]string{
			"X-Telegram-Bot-Token": "MY-OWN-TOKEN",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestTelegramSendMessageStripsExtraFields(t *testing.T) {
	// Defence-in-depth: a client passing a parameter we don't whitelist
	// (e.g. inline_keyboard) must NOT have it forwarded to Telegram.
	fake := newFakeTelegram(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
	h, cleanup := newTestHandler(t, fake, config.NotificationsConfig{
		Telegram: config.TelegramNotificationsConfig{
			Enabled: true,
			Bots:    []config.TelegramBot{{Name: "default", Token: "T"}},
		},
	})
	defer cleanup()

	body := `{"chat_id":12345,"text":"hi","reply_markup":{"inline_keyboard":[[{"text":"x","url":"http://evil"}]]}}`
	rec := doRequest(t, h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(fake.lastBody, "reply_markup") || strings.Contains(fake.lastBody, "inline_keyboard") {
		t.Fatalf("upstream body leaked extra fields: %s", fake.lastBody)
	}
}

func TestLookupTelegramBot(t *testing.T) {
	n := config.NotificationsConfig{
		Telegram: config.TelegramNotificationsConfig{
			Bots: []config.TelegramBot{
				{Name: "default", Token: "D"},
				{Name: "ops", Token: "O"},
			},
		},
	}
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "default", true},
		{"default", "default", true},
		{"ops", "ops", true},
		{"missing", "", false},
		{"DEFAULT", "", false}, // case-sensitive
	}
	for _, tc := range cases {
		got, ok := n.LookupTelegramBot(tc.in)
		if ok != tc.ok || (ok && got.Name != tc.want) {
			t.Errorf("LookupTelegramBot(%q) = (%q, %v), want (%q, %v)", tc.in, got.Name, ok, tc.want, tc.ok)
		}
	}
}

func TestIsChatAllowed(t *testing.T) {
	open := config.TelegramBot{}                                     // empty allowlist => any
	restricted := config.TelegramBot{AllowedChatIDs: []int64{1, 2}}  // explicit
	if !open.IsChatAllowed(999) {
		t.Errorf("open bot must allow any chat_id")
	}
	if !restricted.IsChatAllowed(2) {
		t.Errorf("restricted bot must allow listed chat_id 2")
	}
	if restricted.IsChatAllowed(3) {
		t.Errorf("restricted bot must reject unlisted chat_id 3")
	}
}
