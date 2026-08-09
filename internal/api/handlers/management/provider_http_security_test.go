package management

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
)

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn providerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func postProviderJSON(t *testing.T, path string, handler gin.HandlerFunc, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	router := gin.New()
	router.POST(path, handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestParseProviderHTTPURLAllowsHTTPPrivateNetworks(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:11434",
		"https://[::1]/v1/models?tenant=local",
		"http://10.0.0.5:8080/models",
		"https://192.168.1.25/provider",
		"http://169.254.169.254/latest/meta-data/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			parsed, err := parseProviderHTTPURL(rawURL, "testUrl")
			require.NoError(t, err)
			assert.Equal(t, rawURL, parsed.String())
		})
	}
}

func TestParseProviderHTTPURLRejectsAmbiguousOrUnsafeSyntax(t *testing.T) {
	for _, tc := range []struct {
		name, rawURL, want string
	}{
		{name: "bad scheme", rawURL: "file:///etc/passwd", want: "http or https"},
		{name: "missing host", rawURL: "http:///models", want: "include a host"},
		{name: "userinfo", rawURL: "https://user:secret@example.com/models", want: "userinfo"},
		{name: "malformed", rawURL: "http://[::1", want: "malformed"},
		{name: "fragment", rawURL: "https://example.com/models#ignored", want: "fragment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseProviderHTTPURL(tc.rawURL, "testUrl")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseProviderHTTPURLErrorDoesNotEchoMalformedCredentials(t *testing.T) {
	_, err := parseProviderHTTPURL("http://user:secret@[::1", "testUrl")
	require.ErrorContains(t, err, "malformed")
	assert.NotContains(t, err.Error(), "user")
	assert.NotContains(t, err.Error(), "secret")
}

func TestDiscoverModelsRejectsEveryUserControlledURLBeforeNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	badURLs := []struct {
		name, value string
	}{
		{name: "bad scheme", value: "file:///etc/passwd"},
		{name: "missing host", value: "http:///models"},
		{name: "userinfo", value: "http://user:secret@example.com/models"},
		{name: "malformed", value: "http://[::1"},
		{name: "fragment", value: "http://example.com/models#ignored"},
		{name: "empty", value: "   "},
	}
	for _, field := range []string{"modelsUrl", "baseUrl", "proxyUrl"} {
		for _, bad := range badURLs {
			t.Run(field+" "+bad.name, func(t *testing.T) {
				payload := map[string]any{field: bad.value}
				if field == "proxyUrl" {
					payload["modelsUrl"] = "http://127.0.0.1:1/models"
				}
				response := postProviderJSON(t, "/discover", h.DiscoverModels, payload)
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			})
		}
	}
}

func TestInferProviderModelsURLPreservesPathsAndChoosesEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, baseURL, want string
	}{
		{name: "standard", baseURL: "https://example.com/v1/?tenant=local", want: "https://example.com/v1/models?tenant=local"},
		{name: "ollama name", baseURL: "http://example.com/ollama/", want: "http://example.com/ollama/api/tags"},
		{name: "ollama port", baseURL: "http://127.0.0.1:11434", want: "http://127.0.0.1:11434/api/tags"},
		{name: "encoded path", baseURL: "https://example.com/a%2Fb", want: "https://example.com/a%2Fb/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelsURL, err := inferProviderModelsURL(tc.baseURL)
			require.NoError(t, err)
			assert.Equal(t, tc.want, modelsURL.String())
		})
	}
}

func TestProviderHTTPClientUsesConfiguredProxy(t *testing.T) {
	var targetHits, proxyHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		assert.Equal(t, target.URL+"/", r.URL.String())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	response, err := providerHTTPClient(0, proxyURL).Get(target.URL)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.EqualValues(t, 1, proxyHits.Load())
	assert.Zero(t, targetHits.Load())
}

func TestProviderHTTPClientSupportsWrappedDefaultTransport(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()
	var hits atomic.Int32
	http.DefaultTransport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		hits.Add(1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	response, err := providerHTTPClient(0, nil).Get("http://example.com")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.EqualValues(t, 1, hits.Load())
}

func TestProviderUsesOpenAIBearerAuthChecksHostname(t *testing.T) {
	for _, tc := range []struct {
		rawURL string
		want   bool
	}{
		{rawURL: "https://openai.com/v1", want: true},
		{rawURL: "https://api.openai.com/v1", want: true},
		{rawURL: "https://OPENAI.COM/v1", want: true},
		{rawURL: "https://openai.com.attacker.example/v1", want: false},
		{rawURL: "https://attacker.example/openai.com", want: false},
	} {
		t.Run(tc.rawURL, func(t *testing.T) {
			providerURL, err := url.Parse(tc.rawURL)
			require.NoError(t, err)
			assert.Equal(t, tc.want, providerUsesOpenAIBearerAuth(providerURL))
		})
	}
}

func TestDiscoverModelsAllowsLoopbackInferenceAndValidatedRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"local-model"}]}`))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/models", http.StatusFound)
	}))
	defer redirect.Close()

	h := &Handler{}
	for _, payload := range []map[string]any{
		{"baseUrl": target.URL},
		{"modelsUrl": redirect.URL},
	} {
		response := postProviderJSON(t, "/discover", h.DiscoverModels, payload)
		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), "local-model")
	}
}

func TestProviderRedirectPolicyRevalidatesEveryHop(t *testing.T) {
	for _, tc := range []struct {
		name, rawURL string
		viaCount     int
		wantErr      bool
	}{
		{name: "loopback allowed", rawURL: "http://127.0.0.1:11434/models"},
		{name: "bad scheme", rawURL: "file:///etc/passwd", wantErr: true},
		{name: "missing host", rawURL: "http:///models", wantErr: true},
		{name: "userinfo", rawURL: "http://user:secret@example.com/models", wantErr: true},
		{name: "redirect limit", rawURL: "https://example.com/models", viaCount: 10, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := url.Parse(tc.rawURL)
			require.NoError(t, err)
			err = validateProviderRedirect(&http.Request{URL: parsed}, make([]*http.Request, tc.viaCount))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProviderRedirectRejectsUserinfoWithoutContactingTarget(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()
	targetURL, err := url.Parse(target.URL)
	require.NoError(t, err)
	targetURL.User = url.UserPassword("user", "secret")

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL.String(), http.StatusFound)
	}))
	defer redirect.Close()
	client := providerHTTPClient(0, nil)
	request, err := http.NewRequest(http.MethodGet, redirect.URL, nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	require.ErrorContains(t, err, "userinfo")
	assert.Zero(t, targetHits.Load())
}

func TestProviderConnectionRejectsEveryConfiguredURLBeforeNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{cfg: &config.Config{}}
	badURLs := []struct {
		name, value string
	}{
		{name: "bad scheme", value: "file:///etc/passwd"},
		{name: "missing host", value: "http:///models"},
		{name: "userinfo", value: "http://user:secret@example.com/models"},
		{name: "malformed", value: "http://[::1"},
		{name: "fragment", value: "http://example.com/models#ignored"},
		{name: "empty", value: "   "},
	}
	for _, field := range []string{"base-url", "models-url", "proxy-url"} {
		for _, bad := range badURLs {
			t.Run(field+" "+bad.name, func(t *testing.T) {
				payload := TestProviderRequest{
					ProviderID: "openai",
					Category:   "cloud",
					Config: map[string]interface{}{
						"api-key": "test-key",
						field:     bad.value,
					},
				}
				response := postProviderJSON(t, "/test", h.TestProvider, payload)
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), "invalid_url")
			})
		}
	}

	for _, bad := range badURLs {
		t.Run("local base-url "+bad.name, func(t *testing.T) {
			payload := TestProviderRequest{
				ProviderID: "ollama",
				Category:   "local",
				Config: map[string]interface{}{
					"base-url": bad.value,
				},
			}
			response := postProviderJSON(t, "/test", h.TestProvider, payload)
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "invalid_url")
		})
	}
}

func TestProviderConnectionAllowsLoopbackForLocalAndCloudProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	h := &Handler{cfg: &config.Config{}}

	local := TestProviderRequest{
		ProviderID: "ollama",
		Category:   "local",
		Config:     map[string]interface{}{"base-url": server.URL},
	}
	response := postProviderJSON(t, "/test", h.TestProvider, local)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"status":"success"`)

	cloud := TestProviderRequest{
		ProviderID: "openai",
		Category:   "cloud",
		Config: map[string]interface{}{
			"api-key":    "test-key",
			"base-url":   server.URL,
			"models-url": server.URL + "/models",
		},
	}
	response = postProviderJSON(t, "/test", h.TestProvider, cloud)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"status":"success"`)
}
