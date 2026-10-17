package executor

import (
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeGeminiOpenAICompatPayload removes OpenAI-only request fields that
// Google's OpenAI-compatible endpoint rejects, including explicit null values.
// Keep the rule tied to the official endpoint, not an operator-chosen provider
// label: changing a provider label must not affect unrelated upstreams.
func normalizeGeminiOpenAICompatPayload(baseURL string, payload []byte) []byte {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.User != nil ||
		!strings.EqualFold(u.Hostname(), "generativelanguage.googleapis.com") ||
		(u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Fragment != "" {
		return payload
	}
	path := strings.TrimRight(u.Path, "/")
	if path != "/v1beta/openai" && path != "/v1/openai" {
		return payload
	}
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return payload
	}
	for _, field := range []string{"store", "prompt_cache_key", "prompt_cache_retention"} {
		if updated, err := sjson.DeleteBytes(payload, field); err == nil {
			payload = updated
		}
	}
	return payload
}
