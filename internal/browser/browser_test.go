package browser

import (
	"testing"
)

func TestOpenURL_InvalidSchemes(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"file scheme", "file:///etc/passwd", true},
		{"ftp scheme", "ftp://example.com", true},
		{"command flag", "--flag=true", true},
		{"no scheme", "example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := OpenURL(tt.url); (err != nil) != tt.wantErr {
				t.Errorf("OpenURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
