package store

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Keep HTTPS credential semantics while adapting to go-git's per-client options.
// The server is local and always rejects; no credentials or remote state leave tests.
func TestGitTokenStoreClientOptionsPreserveBasicAuth(t *testing.T) {
	for _, tc := range []struct{ name, user, password, wantUser string }{
		{"explicit", "operator", "synthetic-password", "operator"},
		{"default-user", "", "synthetic-token", "git"},
		{"anonymous", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type observed struct {
				user, password string
				present        bool
			}
			requests := make(chan observed, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u, p, ok := r.BasicAuth()
				requests <- observed{u, p, ok}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()
			store := NewGitTokenStore(server.URL+"/repository", tc.user, tc.password)
			store.SetBaseDir(filepath.Join(t.TempDir(), "repo", "auths"))
			if err := store.EnsureRepository(); err == nil {
				t.Fatal("rejecting remote unexpectedly succeeded")
			}
			select {
			case got := <-requests:
				if got.user != tc.wantUser || got.password != tc.password || got.present != (tc.wantUser != "") {
					t.Fatal("HTTP auth contract changed")
				}
			default:
				t.Fatal("client options did not reach HTTP transport")
			}
		})
	}
}
