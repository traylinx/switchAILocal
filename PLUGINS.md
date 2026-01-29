# Lua Plugin System Manual

The Lua Plugin System allows you to intercept and modify requests and responses in real-time. This is the foundation of the **Intelligent Routing (Cortex)** engine.

## 1. Activation

The plugin system is **strictly explicit**. No plugins are loaded unless their unique ID is listed in your `config.yaml`.

```yaml
plugin:
  enabled: true
  plugin-dir: "./plugins"
  # List the unique 'name' (IDs) of plugins to enable
  enabled-plugins:
    - "sebastian-interceptor"
```

## 2. Plugin Structure & Metadata

Every `.lua` file in your plugin directory must define its identity at the top of the file. This allows the system to verify the plugin before loading it.

```lua
name = "my-unique-plugin-id"              -- Mandatory: Slug-style ID (no spaces, e.g. "my-plugin")
display_name = "My Awesome Plugin Name"    -- Optional: Human-readable name for logs
```

> [!IMPORTANT]
> The `name` must be a clean string without spaces or special characters (use hyphens or underscores). This ID is used in `config.yaml` to enable the plugin.

## 3. Global Hooks

### `on_request(req)`
Triggered whenever a request reaches the server.
- **Parameters**: `req` (Table)
  - `model`: The model identifier (e.g., "gpt-4o")
  - `provider`: The target provider (e.g., "openai")
  - `body`: The raw JSON request body (String)
- **Returns**: 
  - `req` (Modified Table) to apply changes.
  - `nil` to skip modification.

## 4. The `switchai` Host API (The Bridge)

Since plugins run in a secure sandbox, they cannot access the network or disk directly. Use the `switchai` bridge for advanced features:

- `switchai.log(message)`: Logs a message to the main server console.
- `switchai.classify(prompt)`: Sends a classification request to the configured **Router Model**. Returns `(json_string, error)`.

## 5. Example: Sebastian's Interceptor

**File**: `./plugins/sebastian_demo.lua`
```lua
name = "sebastian-interceptor"
display_name = "Sebastian's Greeting Interceptor"

function on_request(req)
    switchai.log("Intercepting: " .. req.model)

    if req.body then
        -- Simple pattern match to replace "content": "..."
        local new_body = string.gsub(req.body, '("content":%s*")[^"]*', '%1Hi i\'m Sebastian')
        
        if new_body ~= req.body then
            req.body = new_body
            switchai.log("Modified body successfully!")
            return req
        end
    end
    return nil
end
```

## 5. Security & Isolation

- **No Network/Disk**: Plugins cannot perform external network calls or read files.
- **Timeouts**: Script execution is bound by the parent request's context timeout.
- **Read-Only**: Parts of the internal Go environment are explicitly NOT exposed to ensure stability.
