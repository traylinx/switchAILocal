package browser

import (
	"strings"
	"testing"
)

func TestOpenURL_InvalidScheme(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name:    "file scheme",
			url:     "file:///etc/passwd",
			wantErr: "invalid URL scheme: file",
		},
		{
			name:    "ftp scheme",
			url:     "ftp://example.com/file",
			wantErr: "invalid URL scheme: ftp",
		},
		{
			name:    "javascript scheme",
			url:     "javascript:alert(1)",
			wantErr: "invalid URL scheme: javascript",
		},
		{
			name:    "no scheme",
			url:     "example.com",
			wantErr: "invalid URL scheme",
		},
		{
			name:    "empty scheme",
			url:     "://example.com",
			wantErr: "invalid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenURL(tt.url)
			if err == nil {
				t.Errorf("OpenURL() error = nil, wantErr %v", tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("OpenURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
