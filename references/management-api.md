# Management API Reference

Comprehensive list of all management and monitoring endpoints for switchAILocal.

**Base URL**: `http://localhost:18080/v0/management`
**Auth**: Requires `X-Management-Key` header.

## 📊 Monitoring & Analytics

| Endpoint            | Method | Description                             |
| ------------------- | ------ | --------------------------------------- |
| `/analytics`        | GET    | Global performance statistics           |
| `/memory/stats`     | GET    | Stats for the memory and routing system |
| `/heartbeat/status` | GET    | Detailed health status of all providers |
| `/steering/rules`   | GET    | List currently active steering rules    |
| `/hooks/status`     | GET    | Current hook configurations and status  |

### Example: Check Analytics

```bash
curl http://localhost:18080/v0/management/analytics \
  -H "X-Management-Key: your-secret-key"
```

---

## ⚙️ Operations & Management

| Endpoint           | Method | Description                               |
| ------------------ | ------ | ----------------------------------------- |
| `/steering/reload` | POST   | Hot-reload routing and steering rules     |
| `/hooks/reload`    | POST   | Hot-reload all hook configurations        |
| `/config`          | GET    | Retrieve the current system configuration |
| `/config`          | PATCH  | Update specific fields in config.yaml     |

### Example: Reload Steering Rules

```bash
curl -X POST http://localhost:18080/v0/management/steering/reload \
  -H "X-Management-Key: your-secret-key"
```

---

## 🛠️ System Health

| Endpoint     | Method | Description                         |
| ------------ | ------ | ----------------------------------- |
| `/health`    | GET    | Basic server health check (no auth) |
| `/providers` | GET    | Detailed provider registration info |

## Management Dashboard

A full web-based dashboard is available at:
`http://localhost:18080/management`

Use this for visual monitoring, log tailing, and live config editing.
