package browser

import (
	"testing"
)

func TestOpenURL_Security(t *testing.T) {
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com/file",
		"javascript:alert(1)",
		"data:text/html,<html>",
		"smb://server/share",
	}

	for _, u := range invalidURLs {
		err := OpenURL(u)
		if err == nil {
			t.Errorf("Expected OpenURL to fail for invalid scheme: %s", u)
		}
	}
}
