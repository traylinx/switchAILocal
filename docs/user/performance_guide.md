# switchAILocal Performance & Production Guide

Welcome to the switchAILocal performance tuning guide. This document explains how to configure and operate the proxy in high-concurrency production environments.

By default, switchAILocal is configured to be as simple and transparent as possible (all production constraints are disabled). However, if you are exposing the proxy to multiple users, teams, or billing customers, you should enable the performance and stability features detailed below.

---

## 1. Enabling Performance Features

All performance features are configured in the `performance` block of your `config.yaml` file.

```yaml
performance:
  profiling-enabled: false
  pprof-port: 6060

  rate-limiter:
    enabled: true
    global-requests-per-second: 1000
    global-burst: 500
    per-key-requests-per-second: 60
    per-key-burst: 30

  load-shedding:
    enabled: true
    max-in-flight: 500
    retry-after-seconds: 5
```

---

## 2. Rate Limiting

The rate limiter protects the proxy from being overwhelmed by too many requests in a short time. It uses a high-performance, lock-free Token Bucket algorithm.

You can configure two different tiers of rate limits simultaneously:

### Global Limit (`global-requests-per-second`)

This defines the absolute maximum capacity of your switchAILocal instance. If this limit is exceeded, **all** new requests will be rejected, regardless of who made them.
- **Use case:** Protecting your underlying infrastructure/network from DDoS attacks or runaway scripts.

### Per-Key Limit (`per-key-requests-per-second`)

This defines the maximum capacity allocated to a *single API Key*.
- **Use case:** Ensuring "fair use" among different team members or customers. It prevents one noisy neighbor from eating up all your proxy's capacity.

**How Rejections Work:**
When a client hits a rate limit, switchAILocal immediately returns an `HTTP 429 Too Many Requests` status, along with an OpenAI-compatible JSON error message and a `Retry-After: 1` header.

---

## 3. Graceful Load Shedding

While Rate Limiting restricts the *rate* of incoming requests, **Load Shedding** restricts the *number of requests actively being processed at the exact same time* (concurrency).

```yaml
  load-shedding:
    enabled: true
    max-in-flight: 500
```

### Why do you need Load Shedding?

LLM queries can take a long time to return (sometimes 30+ seconds). If a user sends 1,000 requests, and each request takes 30 seconds to finish, those 1,000 requests will pile up, consuming RAM, open sockets, and database connections.

If this number grows unbounded, your server will eventually run out of memory (OOM) and crash, disrupting service for everyone.

### How it Works

With `max-in-flight: 500`, switchAILocal tracks exactly how many requests are currently waiting for an upstream provider (like OpenAI or Anthropic). The moment 501 concurrent requests hit the proxy, the 501st request is instantly rejected with an `HTTP 503 Service Unavailable` and a `Retry-After` header.

This guarantees that your proxy will **never** crash under load, degrading gracefully under extreme traffic spikes.

---

## 4. Observability & Dashboard

To properly tune the rates above, you need to know how the proxy is behaving.

### The Management Dashboard API

You can check the real-time health of the proxy by calling the Observability Dashboard endpoint.
Note: This endpoint requires your `management-secret-key` exactly as the other `/v0/management/` routes do.

```bash
curl -H "Authorization: Bearer <your-management-secret>" \
     "http://localhost:18080/v0/management/observability/dashboard"
```

**Example Response:**

```json
{
  "timestamp": "2026-03-29T08:45:33Z",
  "system": {
    "goroutines": 142,
    "heap_alloc_mb": 24.5,
    "heap_sys_mb": 64.0,
    "num_gc": 12,
    "go_version": "go1.22.0",
    "num_cpu": 16
  },
  "server": {
    "uptime": "45h12m14s"
  }
}
```

### Prometheus Metrics

If you enabled prometheus metrics (`observability.metrics.enabled: true` in your config), the following new metrics are available to build Grafana dashboards:

- `switchailocal_requests_in_flight` (Gauge): Exact current concurrency. Use this to tune `max-in-flight`.
- `switchailocal_rate_limited_total` (Counter): How many times the rate limiter rejected requests.
- `switchailocal_load_shed_total` (Counter): How many times the load shedder rejected requests.

### Deep Profiling (pprof)

If you are a developer chasing down a CPU bottleneck or memory leak, you can enable `profiling-enabled: true`. This turns on standard Go `pprof` debugging endpoints on port `6060`.

*Security Note: The profiling server is hard-bound to `127.0.0.1`. It can never be accessed from the public internet, even if your proxy is.*

```bash
# Capture a 30-second CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```
