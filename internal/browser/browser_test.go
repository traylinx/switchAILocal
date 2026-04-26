package browser

import (
	"testing"
)

func TestOpenURL_Negative(t *testing.T) {
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
		"smb://server/share",
		"gopher://example.com",
	}

	for _, rawurl := range invalidURLs {
		t.Run(rawurl, func(t *testing.T) {
			err := OpenURL(rawurl)
			if err == nil {
				t.Errorf("expected error for unsupported scheme in URL %q, got nil", rawurl)
			}
		})
	}
}
