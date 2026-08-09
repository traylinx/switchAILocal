package management

import "testing"

func TestBuildCallbackRedirectTarget(t *testing.T) {
	tests := []struct {
		name       string
		targetBase string
		rawQuery   string
		want       string
	}{
		{
			name:       "absolute localhost",
			targetBase: "http://localhost:8080/callback",
			want:       "http://localhost:8080/callback",
		},
		{
			name:       "relative target",
			targetBase: "/callback",
			want:       "/callback",
		},
		{
			name:       "preserves existing query",
			targetBase: "/callback?source=web",
			rawQuery:   "code=a%2Fb&state=x",
			want:       "/callback?source=web&code=a%2Fb&state=x",
		},
		{
			name:       "places query before fragment",
			targetBase: "http://127.0.0.1:8080/callback#done",
			rawQuery:   "code=a%2Fb&state=x",
			want:       "http://127.0.0.1:8080/callback?code=a%2Fb&state=x#done",
		},
		{
			name:       "preserves local userinfo",
			targetBase: "http://user:pass@localhost:8080/callback",
			want:       "http://user:pass@localhost:8080/callback",
		},
		{
			name:       "query cannot change target host",
			targetBase: "/callback",
			rawQuery:   "next=https%3A%2F%2Fevil.example",
			want:       "/callback?next=https%3A%2F%2Fevil.example",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildCallbackRedirectTarget(tc.targetBase, tc.rawQuery)
			if err != nil {
				t.Fatalf("buildCallbackRedirectTarget() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("buildCallbackRedirectTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildCallbackRedirectTargetRejectsUntrustedTargets(t *testing.T) {
	tests := []struct {
		name       string
		targetBase string
	}{
		{name: "remote HTTPS host", targetBase: "https://evil.example/callback"},
		{name: "network path reference", targetBase: "//evil.example/callback"},
		{name: "localhost prefix confusion", targetBase: "https://localhost.evil.example/callback"},
		{name: "unsafe scheme", targetBase: "javascript:alert(1)"},
		{name: "relative path without root", targetBase: "callback"},
		{name: "malformed URL", targetBase: "http://[::1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := buildCallbackRedirectTarget(tc.targetBase, "code=secret"); err == nil {
				t.Fatalf("buildCallbackRedirectTarget() = %q, want rejection", got)
			}
		})
	}
}
