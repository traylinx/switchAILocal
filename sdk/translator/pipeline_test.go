package translator

import (
	"context"
	"testing"
)

func TestPipeline_TranslateRequest(t *testing.T) {
	reg := NewRegistry()
	pipeline := NewPipeline(reg)

	// Register a mock translator
	reg.Register(Format("A"), Format("B"), func(model string, raw []byte, stream bool) []byte {
		return append([]byte("translated:"), raw...)
	}, ResponseTransform{})

	// Add middleware
	pipeline.UseRequest(func(ctx context.Context, req RequestEnvelope, next RequestHandler) (RequestEnvelope, error) {
		req.Body = append([]byte("mw:"), req.Body...)
		return next(ctx, req)
	})

	req := RequestEnvelope{
		Format: Format("A"),
		Body:   []byte("original"),
	}

	result, err := pipeline.TranslateRequest(context.Background(), Format("A"), Format("B"), req)
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	expected := "translated:mw:original"
	if string(result.Body) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result.Body))
	}
}

func TestPipeline_TranslateResponse(t *testing.T) {
	reg := NewRegistry()
	pipeline := NewPipeline(reg)

	// Register a mock response translator
	reg.Register(Format("A"), Format("B"), nil, ResponseTransform{
		NonStream: func(ctx context.Context, model string, origReq, req, raw []byte, param *any) string {
			return "resp:" + string(raw)
		},
	})

	pipeline.UseResponse(func(ctx context.Context, resp ResponseEnvelope, next ResponseHandler) (ResponseEnvelope, error) {
		resp.Body = append([]byte("mw:"), resp.Body...)
		return next(ctx, resp)
	})

	resp := ResponseEnvelope{
		Format: Format("A"),
		Body:   []byte("original"),
	}

	result, err := pipeline.TranslateResponse(context.Background(), Format("A"), Format("B"), resp, nil, nil, nil)
	if err != nil {
		t.Fatalf("TranslateResponse failed: %v", err)
	}

	expected := "resp:mw:original"
	if string(result.Body) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result.Body))
	}
}
