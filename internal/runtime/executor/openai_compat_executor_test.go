package executor

import (
	"bytes"
	"mime"
	"mime/multipart"
	"testing"
)

func TestRewriteMultipartModelField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "ail-transcribe"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("audio-bytes")); err != nil {
		t.Fatal(err)
	}
	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rewritten, rewrittenContentType, err := rewriteMultipartModelField(body.Bytes(), contentType, "whisper-large-v3-turbo")
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(rewrittenContentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(rewritten), params["boundary"])
	form, err := reader.ReadForm(1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}
	if got := form.Value["model"][0]; got != "whisper-large-v3-turbo" {
		t.Fatalf("model=%q, want whisper-large-v3-turbo", got)
	}
	if got := len(form.File["file"]); got != 1 {
		t.Fatalf("file count=%d, want 1", got)
	}
}
