package browser

import (
	"testing"
)

func TestOpenURL_InvalidSchemes(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://example.com"},
		{"no scheme", "example.com"},
		{"javascript scheme", "javascript:alert(1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenURL(tt.url)
			if err == nil {
				t.Errorf("OpenURL(%q) expected error, got nil", tt.url)
			}
		})
	}
}
