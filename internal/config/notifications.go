// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

// NotificationsConfig holds the configuration for the notifications subsystem.
//
// The notifications subsystem lets sandboxed clients (e.g. agents inside
// network-isolated Tytus pods) send outbound messages to external services
// like Telegram via the gateway. The gateway holds the upstream credentials
// server-side so a compromised pod never sees a real bot token. Each channel
// (telegram today, slack/webhooks tomorrow) gets its own block.
type NotificationsConfig struct {
	// Telegram configures outbound Telegram bot relays.
	Telegram TelegramNotificationsConfig `yaml:"telegram,omitempty" json:"telegram,omitempty"`
}

// TelegramNotificationsConfig holds Telegram-specific notification settings.
type TelegramNotificationsConfig struct {
	// Enabled toggles the entire /v1/notifications/telegram/* endpoint family.
	// Default: derived from len(Bots) > 0. When false (or no bots configured),
	// the endpoint returns 503 with a clear "telegram notifications not configured"
	// message — never silently accepts requests.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Bots is the list of named Telegram bots the gateway can relay through.
	// Callers reference a bot by name; tokens are never exposed to clients.
	// At least one bot with name "default" is the convention; additional named
	// bots let one switchAILocal serve multiple Tytus pods with distinct bots.
	Bots []TelegramBot `yaml:"bots,omitempty" json:"bots,omitempty"`
}

// TelegramBot is a single named Telegram bot configuration entry.
type TelegramBot struct {
	// Name is the logical identifier callers use to select this bot.
	// The reserved name "default" is used when a request omits the bot field.
	Name string `yaml:"name" json:"name"`

	// Token is the bot HTTP API token from BotFather (e.g. "1234567890:AA...").
	// Server-side only — never echoed in responses or logs.
	Token string `yaml:"token" json:"token"`

	// AllowedChatIDs optionally restricts which chat_ids this bot will message.
	// Empty = allow any chat_id (the bot's own anti-spam still applies upstream).
	// Non-empty = explicit allowlist; requests for other chat_ids return 403.
	AllowedChatIDs []int64 `yaml:"allowed-chat-ids,omitempty" json:"allowed-chat-ids,omitempty"`
}

// LookupTelegramBot returns the bot config for the given name (or "default" if
// name is empty), and a bool indicating whether a match was found. The match
// is exact and case-sensitive.
func (n *NotificationsConfig) LookupTelegramBot(name string) (TelegramBot, bool) {
	if name == "" {
		name = "default"
	}
	for _, b := range n.Telegram.Bots {
		if b.Name == name {
			return b, true
		}
	}
	return TelegramBot{}, false
}

// IsChatAllowed reports whether the given chat_id is permitted for this bot.
// An empty AllowedChatIDs list means any chat_id is permitted.
func (b TelegramBot) IsChatAllowed(chatID int64) bool {
	if len(b.AllowedChatIDs) == 0 {
		return true
	}
	for _, allowed := range b.AllowedChatIDs {
		if allowed == chatID {
			return true
		}
	}
	return false
}
