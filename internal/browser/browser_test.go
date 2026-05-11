package browser

import (
	"testing"
)

func TestOpenURL_InvalidSchemes(t *testing.T) {
	tests := []struct {
		name    string
		rawurl  string
		wantErr bool
	}{
		{
			name:    "file scheme",
			rawurl:  "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "ftp scheme",
			rawurl:  "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "no scheme",
			rawurl:  "example.com",
			wantErr: true,
		},
		{
			name:    "javascript pseudo-protocol",
			rawurl:  "javascript:alert(1)",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenURL(tt.rawurl)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
