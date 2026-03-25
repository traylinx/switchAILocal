# Plugin Development Guide

`switchAILocal` uses a robust, folder-based plugin architecture inspired by Kong Gateway. This system allows you to extend the core functionality using Lua scripts.

## 1. Architecture

Plugins are self-contained packages located in the `plugins/` directory. Each plugin MUST be its own directory.

### Directory Structure
```text
plugins/
  ├── my-cool-plugin/         <-- Plugin ID (Folder Name)
  │     ├── handler.lua       <-- The Logic (Hooks)
  │     ├── schema.lua        <-- Metadata & Config
  │     ├── README.md         <-- Entry point documentation
  │     ├── docs/             <-- Detailed documentation (Manuals, Guides)
  │     └── lib/              <-- Optional helper files
  │           └── utils.lua
```

### Naming Conventions
- **Plugin ID (Folder Name)**: Must be a "slug" (lowercase, alphanumeric, hyphens).
    - ✅ `my-plugin`
    - ✅ `security-filter-v2`
    - ❌ `MyPlugin` (No uppercase)
    - ❌ `my plugin` (No spaces)
- **Consistency**: The `name` field in `schema.lua` MUST match the Folder Name.

---

## 2. The Key Files

### `schema.lua`
Defines the identity of your plugin.

```lua
return {
    name = "my-cool-plugin",              -- MUST match folder name
    display_name = "My Cool Plugin",      -- Human readable name for logs
    version = "1.0.0",
}
```

### `handler.lua`
Contains the runtime logic. It must return a table implementing the hook interface.

```lua
local Plugin = {}

-- Hook: called before request is sent to LLM
function Plugin:on_request(req)
    -- req = { model="", messages={}, body="..." }
    
    switchai.log("Processing request...")
    
    -- Example: Force a specific model
    if req.model == "auto" then
        req.model = "switchai-fast"
        return req -- Return modified request
    end
    
    return nil -- Return nil to pass through unchanged
end

return Plugin
```

---

## 3. Host API (`switchai` module)

The Lua sandbox provides a `switchai` global command for interacting with the host.

| Function                  | Description                                                                         |
| :------------------------ | :---------------------------------------------------------------------------------- |
| `switchai.log(msg)`       | Writes looking to the application server logs (INFO level).                         |
| `switchai.classify(text)` | Uses the configured 'Router Model' (LLM) to classify intent. Returns a JSON string. |

---

## 4. Helper Libraries

You can use `require()` to load other Lua files within your plugin folder.

**Example**: `plugins/my-plugin/lib/utils.lua`
```lua
return {
    hello = function() return "world" end
}
```

**Usage in `handler.lua`**:
```lua
local utils = require("lib.utils")
-- utils.hello() -> "world"
```

## 5. Best Practices

1.  **Keep it Fast**: `on_request` runs for *every* request. Avoid heavy computation.
2.  **Use `switchai.classify` sparingly**: It triggers an LLM call, which adds latency. Use regex (Reflex) where possible.
3.  **Fail Safe**: If your plugin errors, it should gracefully return `nil` or handle the error, rather than crashing the request.
