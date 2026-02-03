# Memory System Reference

Record routing decisions, track outcomes, and view analytics.

## Enable Memory

```yaml
# config.yaml
memory:
  enabled: true
  retention-days: 30
  storage-path: "./.memory"
```

## What Gets Recorded

Each routing decision includes:
- Timestamp
- Requested model
- Selected provider
- Request parameters
- Outcome (success/failure)
- Latency
- Error details (if failed)

## View Memory Stats

```bash
curl http://localhost:18080/v0/management/memory/stats \
  -H "X-Management-Key: your-key"
```

## View Analytics

```bash
curl http://localhost:18080/v0/management/analytics \
  -H "X-Management-Key: your-key"
```

Analytics include:
- Provider success rates
- Average latency per provider
- Common failure patterns
- Model availability trends

## View Routing History

```bash
curl http://localhost:18080/v0/management/memory/history \
  -H "X-Management-Key: your-key"
```

## Storage Management

- Records stored in JSON format in configured directory
- Automatic cleanup removes records older than `retention-days`
- Cleanup runs daily at midnight
- Storage usage reported in stats endpoint
