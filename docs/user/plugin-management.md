# Plugin Management Guide

For users of `switchAILocal`, managing plugins is straightforward and secure. The system uses an **Explicit Enrollment** model, meaning no code runs unless you say so.

## 1. Enabling Plugins

Plugins are managed in your `config.yaml` file under the `plugin` section.

```yaml
plugin:
  enabled: true             # Master switch for the engine
  plugin-dir: "./plugins"   # Directory where plugins live
  
  # The Allow-List:
  enabled-plugins:
    - "sebastian-interceptor"
    - "cortex-router"
```

### How it works:
1.  **ID Matching**: The string in `enabled-plugins` must match the **Folder Name** of the plugin in your `plugins/` directory.
2.  **Order Matters**: Plugins are executed in the order they are listed if they attach to the same hook (though currently order is not strictly guaranteed, usually it's iteration order, future versions might enforce priority).
3.  **Secure by Default**: If a plugin exists in the folder but is NOT in this list, it is ignored given the strict security policy.

## 2. Installing New Plugins

To install a plugin (e.g., `community-plugin-v1`):

1.  **Download/Copy** the plugin folder into your `./plugins/` directory.
    - Result: `./plugins/community-plugin-v1/`
2.  **Verify** the folder contains `handler.lua` and `schema.lua`.
3.  **Edit `config.yaml`**:
    ```yaml
    enabled-plugins:
      - "sebastian-interceptor"
      - "community-plugin-v1"  <-- Add this line
    ```
4.  **Restart** the server.

## 3. Troubleshooting

- **"Plugin not loading"**:
    - Check the logs (`server.log`) for `loading plugin: ...`.
    - Ensure the Folder Name matches the ID in `config.yaml` exactly.
    - Ensure `plugin.enabled` is `true`.
- **"Invalid directory name"**:
    - Plugin folder names must be lowercase and use hyphens (slugs). Rename the folder if it has spaces or uppercase letters.
