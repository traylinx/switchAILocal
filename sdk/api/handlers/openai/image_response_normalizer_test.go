package openai

import (
	"encoding/json"
	"testing"
)

func TestNormalizeOpenAIImagesResponse_PreservesStandardDataArray(t *testing.T) {
	input := []byte(`{"created":123,"data":[{"url":"https://example.com/a.png"}]}`)
	got := normalizeOpenAIImagesResponse(input)
	if string(got) != string(input) {
		t.Fatalf("standard OpenAI response changed: got %s", got)
	}
}

func TestNormalizeOpenAIImagesResponse_ConvertsImageURLsObject(t *testing.T) {
	input := []byte(`{"id":"img_123","data":{"image_urls":["https://example.com/a.png","https://example.com/b.png"]}}`)
	got := normalizeOpenAIImagesResponse(input)

	var body struct {
		ID   string `json:"id"`
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal normalized response: %v\nbody=%s", err, got)
	}
	if body.ID != "img_123" {
		t.Fatalf("id=%q, want img_123", body.ID)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data len=%d, want 2; body=%s", len(body.Data), got)
	}
	if body.Data[0].URL != "https://example.com/a.png" || body.Data[1].URL != "https://example.com/b.png" {
		t.Fatalf("unexpected urls: %#v", body.Data)
	}
}

func TestNormalizeOpenAIImagesResponse_ConvertsBase64Object(t *testing.T) {
	input := []byte(`{"data":{"base64":"abc123"}}`)
	got := normalizeOpenAIImagesResponse(input)

	var body struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal normalized response: %v\nbody=%s", err, got)
	}
	if len(body.Data) != 1 || body.Data[0].B64JSON != "abc123" {
		t.Fatalf("unexpected b64_json data: %#v", body.Data)
	}
}

func TestNormalizeOpenAIImagesResponse_CombinesURLAndBase64Object(t *testing.T) {
	input := []byte(`{"data":{"url":"https://example.com/a.png","b64_json":"abc123","revised_prompt":"clean prompt"}}`)
	got := normalizeOpenAIImagesResponse(input)

	var body struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal normalized response: %v\nbody=%s", err, got)
	}
	if len(body.Data) != 1 {
		t.Fatalf("data len=%d, want 1; body=%s", len(body.Data), got)
	}
	if body.Data[0].URL != "https://example.com/a.png" || body.Data[0].B64JSON != "abc123" || body.Data[0].RevisedPrompt != "clean prompt" {
		t.Fatalf("unexpected combined data: %#v", body.Data[0])
	}
}

func TestNormalizeOpenAIImagesResponse_PreservesNestedImageRevisedPrompt(t *testing.T) {
	input := []byte(`{"data":{"images":[{"url":"https://example.com/a.png","revised_prompt":"nested prompt"}]}}`)
	got := normalizeOpenAIImagesResponse(input)

	var body struct {
		Data []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal normalized response: %v\nbody=%s", err, got)
	}
	if len(body.Data) != 1 || body.Data[0].URL != "https://example.com/a.png" || body.Data[0].RevisedPrompt != "nested prompt" {
		t.Fatalf("unexpected nested data: %#v", body.Data)
	}
}

func TestNormalizeOpenAIImagesResponse_LeavesUnknownShapeUntouched(t *testing.T) {
	input := []byte(`{"data":{"status":"queued"}}`)
	got := normalizeOpenAIImagesResponse(input)
	if string(got) != string(input) {
		t.Fatalf("unknown response shape changed: got %s", got)
	}
}
