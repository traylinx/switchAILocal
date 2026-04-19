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
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/telegram/sendMessage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
