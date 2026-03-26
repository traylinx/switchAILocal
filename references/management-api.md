# Management & Telemetry API Reference

The management API is available on the same port as the main server. All endpoints require the management authentication middleware (configured via `MANAGEMENT_PASSWORD` env var or config).

> For dashboard UI access, navigate to `http://localhost:18080/management`.

## Auto-Routing Endpoints

### GET `/v0/management/autoroute/status`

Returns the current Lab telemetry snapshot.

**Response:**
```json
{
  "enabled": true,
  "active_weights": {
    "availability": 0.35,
    "quota": 0.25,
    "latency": 0.2,
    "success_rate": 0.2
  },
  "shadow_weights": {
    "availability": 0.36,
    "quota": 0.24,
    "latency": 0.21,
    "success_rate": 0.19
  },
  "active_hypothesis": true,
  "window_req_count": 42,
  "avg_prod_rqs": 0.82,
  "avg_shadow_rqs": 0.85
}
```

| Field | Description |
|-------|-------------|
| `active_weights` | Current production scoring weights |
| `shadow_weights` | Current Lab hypothesis weights |
| `active_hypothesis` | Whether the Lab is actively testing a hypothesis |
| `window_req_count` | Requests evaluated in the current adaptation window |
| `avg_prod_rqs` | Average RQS with production weights |
| `avg_shadow_rqs` | Average RQS with shadow weights |

### GET `/v0/management/autoroute/journal`

Returns the most recent routing decisions from the circular ring buffer.

**Response:**
```json
{
  "entries": [
    {
      "Timestamp": "2026-03-26T14:58:38.905Z",
      "RequestID": "6502391f",
      "Intent": "coding",
      "Complexity": 0.1,
      "ProdModel": "deepseek-v3.1:671b-cloud",
      "ProdTier": "free",
      "ProdLatency": 56406054347,
      "ProdSuccess": true,
      "ProdRQS": 0.8,
      "ShadowModel": "deepseek-v3.1:671b-cloud",
      "ShadowTier": "free",
      "ShadowExpectedRQS": 0.8,
      "WeightAvail": 0.35,
      "WeightQuota": 0.25,
      "WeightLatency": 0.2,
      "WeightSuccess": 0.2
    }
  ]
}
```

## Provider Health Endpoints

### GET `/v1/providers`

Returns a list of all registered providers and their current health status.

### GET `/v1/models`

Returns a list of all available models across all providers (OpenAI-compatible format).

## Configuration Endpoints

### GET `/v0/management/config`

Returns the current server configuration (sanitized, no API keys).

### POST `/v0/management/config`

Hot-reload configuration changes without restarting the server.

## Dashboard UI

Navigate to `http://localhost:18080/management` for the real-time dashboard. It includes:

- **Provider Health Cards**: Live status of all connected providers
- **Auto-Routing Card**: Active weights visualization, Lab status, shadow hypothesis
- **Live Routing Journal**: Table of recent routing decisions with RQS scores
- **System Metrics**: Request counts, error rates, latency percentiles
