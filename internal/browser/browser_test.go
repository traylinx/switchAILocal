package browser

import (
	"strings"
	"testing"
)

func TestOpenURL_RejectsInvalidScheme(t *testing.T) {
	tests := []struct {
		url     string
		wantErr string
	}{
		{"file:///etc/passwd", "invalid URL scheme: file"},
		{"ftp://example.com/file", "invalid URL scheme: ftp"},
		{"://invalid", "invalid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := OpenURL(tt.url)
			if err == nil {
				t.Errorf("OpenURL(%q) expected error, got nil", tt.url)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("OpenURL(%q) error = %v, want to contain %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
