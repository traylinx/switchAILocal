package executor

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeGeminiOpenAICompatPayload(t *testing.T) {
	input := []byte(`{"model":"gemini-3.8-flash","store":false,"prompt_cache_key":null,"prompt_cache_retention":"24h","stream":true,"stream_options":{"include_usage":true},"reasoning_effort":"low","max_completion_tokens":2048,"messages":[{"role":"user","content":"Keep store and prompt_cache_key in this text."},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"check","arguments":"{}"},"extra_content":{"google":{"thought_signature":"synthetic-signature"}}}]}],"tools":[{"type":"function","function":{"name":"check","parameters":{"type":"object"}}}]}`)
	for _, base := range []string{
		"https://generativelanguage.googleapis.com/v1beta/openai",
		"https://generativelanguage.googleapis.com/v1beta/openai/",
		"https://generativelanguage.googleapis.com:443/v1beta/openai",
		"https://GENERATIVELANGUAGE.GOOGLEAPIS.COM/v1/openai",
	} {
		t.Run(base, func(t *testing.T) {
			original := bytes.Clone(input)
			out := normalizeGeminiOpenAICompatPayload(base, input)
			for _, key := range []string{"store", "prompt_cache_key", "prompt_cache_retention"} {
				if gjson.GetBytes(out, key).Exists() {
					t.Fatalf("unsupported field retained: %s", key)
				}
			}
			var expected, got map[string]any
			if err := json.Unmarshal(original, &expected); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"store", "prompt_cache_key", "prompt_cache_retention"} {
				delete(expected, key)
			}
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, got) {
				t.Fatal("normalization changed supported fields, reasoning, or tool history")
			}
			if !bytes.Equal(original, input) {
				t.Fatal("normalization mutated caller-owned input")
			}
		})
	}
}

func TestNormalizeGeminiOpenAICompatPayloadUnrelatedEndpointsUnchanged(t *testing.T) {
	input := []byte(`{"store":false,"messages":[]}`)
	for _, base := range []string{
		"https://api.deepseek.com/v1",
		"http://127.0.0.1:11435/v1",
		"https://generativelanguage.googleapis.com.attacker.invalid/v1beta/openai",
		"https://attacker.invalid/generativelanguage.googleapis.com/v1beta/openai",
		"https://user@generativelanguage.googleapis.com/v1beta/openai",
		"http://generativelanguage.googleapis.com/v1beta/openai",
		"https://generativelanguage.googleapis.com:8443/v1beta/openai",
		"https://generativelanguage.googleapis.com/v1beta",
		"https://generativelanguage.googleapis.com/v1beta/openai?route=other",
		"https://generativelanguage.googleapis.com/v1beta/openai#other",
		":invalid",
	} {
		t.Run(base, func(t *testing.T) {
			if got := normalizeGeminiOpenAICompatPayload(base, input); !bytes.Equal(got, input) {
				t.Fatal("unrelated endpoint payload changed")
			}
		})
	}
}

func TestNormalizeGeminiOpenAICompatPayloadNonObjectUnchanged(t *testing.T) {
	for _, input := range [][]byte{nil, []byte(`invalid`), []byte(`[]`), []byte(`null`), []byte(`"store"`)} {
		if got := normalizeGeminiOpenAICompatPayload("https://generativelanguage.googleapis.com/v1beta/openai", input); !bytes.Equal(got, input) {
			t.Fatal("non-object payload changed")
		}
	}
}
