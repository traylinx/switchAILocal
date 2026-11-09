# Lua Plugin System

switchAILocal can run sandboxed Lua plugins on every request. A plugin can rewrite the model, pin a provider, or edit the JSON body before routing (`on_request`), and observe successful non-streaming responses (`on_response`). The bundled **Cortex Router** plugin uses this to implement `model: "auto"`.

Full manual: [docs-site/advanced/lua-plugins.mdx](docs-site/advanced/lua-plugins.mdx) (published as "Lua Plugin System" in the docs site).

## Enable

```yaml
plugin:
  enabled: true
  plugin-dir: "./plugins"   # no default; set it explicitly
  enabled-plugins:          # only listed plugins are loaded
    - "cortex-router"
```

Plugins are loaded once at startup; restart after changing them. In the Docker image the bundled plugins are at `/app/plugins`.

## Layout

```
plugins/
└── my-plugin/          # lower-case slug, must equal schema.name
    ├── schema.lua      # returns { name = "my-plugin", display_name = "...", version = "..." }
    └── handler.lua     # returns a table with on_request / on_response methods
```

```lua
-- handler.lua
local Plugin = {}

function Plugin:on_request(req)
    -- req.model, req.provider (only for "provider:model" requests),
    -- req.body (JSON string), req.metadata, req.api_key (when known)
    if req.provider and req.provider ~= "" then return nil end
    if #req.body > 200000 then
        req.model = "deepseek-v4-pro"
        return req          -- model / provider / body / metadata are read back
    end
    return nil              -- nil = unchanged
end

function Plugin:on_response(res)
    -- observation only: return value is ignored, not called for streams
    switchai.log("served " .. tostring(res.model) .. " via " .. tostring(res.provider))
end

return Plugin
```

## Host API (summary)

- Core: `switchai.log`, `switchai.classify` (returns raw router-model text), `switchai.json_decode`, `switchai.json_encode`, `switchai.json_inject`, `switchai.get_cache` / `switchai.set_cache`, `switchai.config` (`router_model`, `router_fallback`, `skills_path`, `matrix`), `switchai.scan_skills`, `switchai.exec`, `switchai.mcp_call`.
- Intelligence (needs `intelligence.enabled: true`): `get_available_models`, `get_dynamic_matrix`, `is_model_available`, `get_skills`, `embed`, `cosine_similarity`, `semantic_match_intent`, `match_skill`, `cache_lookup`, `cache_store`, `cache_metrics`, `parse_confidence`, `verify_intent`, `evaluate_response`, `get_cascade_metrics`, `get_intelligence_metrics`, `record_feedback`.

## Sandbox

- Libraries: base, `table`, `string`, `math`, `package`. No `io`, no `os.execute`; `os` only has `date` and `time`. `dofile` / `loadfile` are removed.
- `switchai.exec` allowlist: `ls`, `cat`, `echo`, `date`, `git` (read-only subcommands: `status`, `diff`, `log`, `show`, `branch`, `ls-files`, `grep`), `whoami`, `pwd`, `grep`. 5 s timeout, 8 KB output cap.
- `switchai.mcp_call` launchers: `uvx`, `npx`, `node`, `python3`. 30 s timeout.
- Plugins see request bodies and provider API keys. Only enable plugins you have read.

## Bundled plugins

| Plugin | Purpose |
|---|---|
| `cortex-router` | Routes `auto` / `cortex` requests (tiered classification, or `intelligence.matrix.auto` when set). 21 skills in `plugins/cortex-router/skills/`. See [docs-site/intelligent-systems/cortex-router.mdx](docs-site/intelligent-systems/cortex-router.mdx). |
| `cortex-answer-schema` | Narrow compatibility shim for one answer-schema prompt format; inert for other traffic. See [docs-site/advanced/cortex-answer-schema.mdx](docs-site/advanced/cortex-answer-schema.mdx) and [plugins/cortex-answer-schema/README.md](plugins/cortex-answer-schema/README.md). |
| `minimax-mcp` | Replaces image content with a text description for MiniMax models. |
