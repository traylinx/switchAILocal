package browser

import (
	"testing"
)

func TestOpenURL_InvalidSchemes(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"File Scheme", "file:///etc/passwd"},
		{"FTP Scheme", "ftp://example.com"},
		{"Command Injection via Flag", "--flag"},
		{"No Scheme", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenURL(tt.url)
			if err == nil {
				t.Errorf("expected error for URL %q, got nil", tt.url)
			}
			expectedErr := "invalid url scheme: only http and https are allowed"
			if err != nil && err.Error() != expectedErr {
				t.Errorf("expected error message %q, got %q", expectedErr, err.Error())
			}
		})
	}
}
