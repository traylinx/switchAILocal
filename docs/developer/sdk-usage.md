# Embedding switchAILocal (Go SDK)

You can embed the `switchAILocal` proxy into your own Go applications. This allows you to provide OpenAI/Gemini/Claude compatibility to your users without requiring them to run a separate proxy process.

## Install & Import

```bash
go get github.com/traylinx/switchAILocal/sdk/switchailocal
```

```go
import (
    "context"
    "errors"
    "time"

    "github.com/traylinx/switchAILocal/internal/config"
    "github.com/traylinx/switchAILocal/sdk/switchailocal"
)
```

Note the module path is `github.com/traylinx/switchAILocal`.


## Minimal Embed

```go
cfg, err := config.LoadConfig("config.yaml")
if err != nil { panic(err) }

svc, err := switchailocal.NewBuilder().
    WithConfig(cfg).
    WithConfigPath("config.yaml"). // absolute or working-dir relative
    Build()
if err != nil { panic(err) }

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
    panic(err)
}
```

The service manages config/auth watching, background token refresh, and graceful shutdown. Cancel the context to stop it.

## Server Options (middleware, routes, logs)

The server accepts options via `WithServerOptions`:

```go
svc, _ := switchailocal.NewBuilder().
  WithConfig(cfg).
  WithConfigPath("config.yaml").
  WithServerOptions(
    // Add global middleware
    switchailocal.WithMiddleware(func(c *gin.Context) { c.Header("X-Embed", "1"); c.Next() }),
    // Tweak gin engine early (CORS, trusted proxies, etc.)
    switchailocal.WithEngineConfigurator(func(e *gin.Engine) { e.ForwardedByClientIP = true }),
    // Add your own routes after defaults
    switchailocal.WithRouterConfigurator(func(e *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
      e.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
    }),
    // Override request log writer/dir
    switchailocal.WithRequestLoggerFactory(func(cfg *config.Config, cfgPath string) logging.RequestLogger {
      return logging.NewFileRequestLogger(true, "logs", filepath.Dir(cfgPath))
    }),
  ).
  Build()
```

These options mirror the internals used by the CLI server.

## Management API (when embedded)

- Management endpoints are mounted only when `remote-management.secret-key` is set in `config.yaml`.
- Remote access additionally requires `remote-management.allow-remote: true`.
- See MANAGEMENT_API.md for endpoints. Your embedded server exposes them under `/v0/management` on the configured port.

## Using the Core Auth Manager

The service uses a core `auth.Manager` for selection, execution, and auto‑refresh. When embedding, you can provide your own manager to customize transports or hooks:

```go
core := coreauth.NewManager(coreauth.NewFileStore(cfg.AuthDir), nil, nil)
core.SetRoundTripperProvider(myRTProvider) // per‑auth *http.Transport

svc, _ := switchailocal.NewBuilder().
    WithConfig(cfg).
    WithConfigPath("config.yaml").
    WithCoreAuthManager(core).
    Build()
```

Implement a custom per‑auth transport:

```go
type myRTProvider struct{}
func (myRTProvider) RoundTripperFor(a *coreauth.Auth) http.RoundTripper {
    if a == nil || a.ProxyURL == "" { return nil }
    u, _ := url.Parse(a.ProxyURL)
    return &http.Transport{ Proxy: http.ProxyURL(u) }
}
```

Programmatic execution is available on the manager:

```go
// Non‑streaming
resp, err := core.Execute(ctx, []string{"gemini"}, req, opts)

// Streaming
chunks, err := core.ExecuteStream(ctx, []string{"gemini"}, req, opts)
for ch := range chunks { /* ... */ }
```

Note: Built‑in provider executors are wired automatically when you run the `Service`. If you want to use `Manager` stand‑alone without the HTTP server, you must register your own executors that implement `auth.ProviderExecutor`.

## Custom Client Sources

Replace the default loaders if your creds live outside the local filesystem:

```go
type memoryTokenProvider struct{}
func (p *memoryTokenProvider) Load(ctx context.Context, cfg *config.Config) (*switchailocal.TokenClientResult, error) {
    // Populate from memory/remote store and return counts
    return &switchailocal.TokenClientResult{}, nil
}

svc, _ := switchailocal.NewBuilder().
  WithConfig(cfg).
  WithConfigPath("config.yaml").
  WithTokenClientProvider(&memoryTokenProvider{}).
  WithAPIKeyClientProvider(switchailocal.NewAPIKeyClientProvider()).
  Build()
```

## Hooks

Observe lifecycle without patching internals:

```go
hooks := switchailocal.Hooks{
  OnBeforeStart: func(cfg *config.Config) { log.Infof("starting on :%d", cfg.Port) },
  OnAfterStart:  func(s *switchailocal.Service) { log.Info("ready") },
}
svc, _ := switchailocal.NewBuilder().WithConfig(cfg).WithConfigPath("config.yaml").WithHooks(hooks).Build()
```

## Shutdown

`Run` defers `Shutdown`, so cancelling the parent context is enough. To stop manually:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = svc.Shutdown(ctx)
```

## Notes

- Hot reload: changes to `config.yaml` and `auths/` are picked up automatically.
- Request logging can be toggled at runtime via the Management API.
- Gemini Web features (`gemini-web.*`) are honored in the embedded server.
