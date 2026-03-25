# LUA Plugin System

`switchAILocal` features a LUA-based plugin system that allows you to intercept and modify API requests and responses at runtime without re-compiling the binary.

## Enabling Plugins

Plugins are disabled by default. Enable them in your `config.yaml`:

```yaml
plugin:
  enabled: true
  plugin-dir: "./plugins"
```

Create the `./plugins` directory and place your `.lua` files there. All files ending in `.lua` will be automatically loaded on startup.

## Available Hooks

The engine looks for specific global functions in your LUA scripts.

### 1. `on_request(data)`

Called after the incoming OpenAI-compatible request is parsed but before it is sent to the provider.

- **Argument**: `data` (table) - The request payload (e.g., `messages`, `model`, `temperature`).
- **Return**: `data` (table) - The modified payload. Return `nil` to make no changes.

#### Example: Inject a System Message
```lua
function on_request(data)
    if data.messages then
        table.insert(data.messages, 1, {
            role = "system",
            content = "You are a helpful assistant. Always mention LUA."
        })
    end
    return data
end
```

### 2. `on_response(data)`

Called after the provider returns a response but before it is sent back to the client.

- **Argument**: `data` (table) - The response payload (e.g., `choices`, `usage`).
- **Return**: `data` (table) - The modified payload. Return `nil` to make no changes.

#### Example: Redact Content
```lua
function on_response(data)
    if data.choices and data.choices[1].message then
        local content = data.choices[1].message.content
        data.choices[1].message.content = content:gsub("secret", "[REDACTED]")
    end
    return data
end
```

## Global Variables and Modules

Each plugin runs in a sandbox with the following modules pre-loaded:
- `base`
- `table`
- `string`
- `math`

## Performance Note

LUA scripts are executed for every request. Keep your logic lightweight to avoid introducing latency. The engine uses a pool of LUA states to handle concurrent requests efficiently.
