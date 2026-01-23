package translator

import (
	"context"
	"testing"
)

func TestRegistry_RegisterAndTranslate(t *testing.T) {
	reg := NewRegistry()

	// request transform
	reg.Register(Format("A"), Format("B"), func(model string, raw []byte, stream bool) []byte {
		return []byte("req_trans")
	}, ResponseTransform{})

	if res := reg.TranslateRequest(Format("A"), Format("B"), "model", []byte("orig"), false); string(res) != "req_trans" {
		t.Errorf("Expected req_trans, got %s", string(res))
	}

	// fallback
	if res := reg.TranslateRequest(Format("C"), Format("D"), "model", []byte("orig"), false); string(res) != "orig" {
		t.Errorf("Expected orig, got %s", string(res))
	}
}

func TestRegistry_ResponseTransformers(t *testing.T) {
	reg := NewRegistry()

	respTrans := ResponseTransform{
		NonStream: func(ctx context.Context, model string, origReq, req, raw []byte, param *any) string {
			return "resp_trans"
		},
		TokenCount: func(ctx context.Context, count int64) string {
			return "tokens"
		},
	}
	reg.Register(Format("A"), Format("B"), nil, respTrans)

	if !reg.HasResponseTransformer(Format("A"), Format("B")) {
		t.Error("Should have response transformer")
	}

	if val := reg.TranslateNonStream(context.Background(), Format("A"), Format("B"), "m", nil, nil, []byte("raw"), nil); val != "resp_trans" {
		t.Errorf("Expected resp_trans, got %s", val)
	}

	if val := reg.TranslateTokenCount(context.Background(), Format("A"), Format("B"), 100, []byte("raw")); val != "tokens" {
		t.Errorf("Expected tokens, got %s", val)
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Exercise default registry wrappers
	Register(Format("X"), Format("Y"), func(m string, r []byte, s bool) []byte { return []byte("default") }, ResponseTransform{})

	if res := TranslateRequest(Format("X"), Format("Y"), "m", nil, false); string(res) != "default" {
		t.Errorf("Default registry failed")
	}

	if !HasResponseTransformer(Format("X"), Format("Y")) {
		t.Error("Default registry should have transformer")
	}
}
