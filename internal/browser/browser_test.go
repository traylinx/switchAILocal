package browser

import (
	"testing"
)

func TestOpenURL_InvalidScheme(t *testing.T) {
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
		"smb://server/share",
	}

	for _, url := range invalidURLs {
		err := OpenURL(url)
		if err == nil {
			t.Errorf("Expected error for URL %s, got nil", url)
		}
		expectedErr := "invalid URL scheme: only http:// and https:// are allowed"
		if err != nil && err.Error() != expectedErr {
			t.Errorf("Expected error message %q, got %q", expectedErr, err.Error())
		}
	}
}
