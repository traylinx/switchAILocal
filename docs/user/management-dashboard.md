# Enhanced Management Dashboard

The **switchAILocal Management Dashboard** provides a sophisticated graphical interface for configuring your AI providers, managing models, and monitoring system status.

Access the dashboard at: `http://localhost:18080/management`

## Features

### 1. Unified Provider Configuration
Manage all your AI providers (OpenAI, Gemini, Claude, Local models) from a single interface. The configuration modal is split into two tabs for better organization:

- **Basic Tab**: Essential settings like API Keys, Base URLs, and routing prefixes.
- **Advanced Tab**: Deep control over your configuration.

### 2. Advanced Configuration
The **Advanced** tab exposes powerful configuration options previously only available by editing `config.yaml` manually:

- **Custom Headers**: Add specific HTTP headers (e.g., for custom authentication or routing) using the Key-Value editor.
- **Excluded Models**: Define patterns to exclude specific models. Supports wildcards:
  - `*` (All models)
  - `prefix-*` (e.g., `gemini-2.5-*`)
  - `*-suffix` (e.g., `*-preview`)
  - `*substring*` (e.g., `*flash*`)
- **Proxy Override**: Set a specific SOCKS5/HTTP proxy for individual providers, overriding the global system proxy.

### 3. Model Discovery & Aliasing
Never manually copy-paste model names again.

- **Model Discovery**: Enter a `Models URL` in the advanced configuration to fetch the list of available models directly from the provider.
- **Model Aliases**: Map complex upstream model names to simple, client-friendly aliases.
  - *Example*: Map `meta-llama/llama-4-maverick-17b-128e-instruct` to `llama4`.
  - The dashboard provides a table interface to easily manage these mappings.

### 4. Connection Testing
Verify your configuration before saving. The "Test Connection" feature validates:
- API Key validity
- Base URL reachability
- Proxy connectivity
- Model availability

### 5. Hot Reload
All changes made in the dashboard are saved to `config.yaml` and applied immediately without requiring a server restart. Existing comments in your YAML file are preserved.

## Usage

1. **Open the Dashboard**: Navigate to `http://localhost:18080/management`.
2. **Select a Provider**: Click "Configure" on any provider card.
3. **Edit Settings**: Switch between Basic and Advanced tabs to modify settings.
4. **Discover Models**: (Optional) In the Advanced tab, click "Fetch Models" to browse available models from the provider.
5. **Test & Save**: Click "Test Connection" to verify, then "Save" to apply changes instantly.

---

## Building from Source

If you are developing `switchAILocal` or want to customize the dashboard, you can build it yourself:

1.  **Navigate to the root directory**.
2.  **Run the UI build script**:
    ```bash
    ./ail_ui.sh
    ```
3.  **Result**: The build process will generate `static/management.html` in the project root.
4.  **Verification**: The server will automatically pick up the new `management.html` from the `static/` directory when started.
