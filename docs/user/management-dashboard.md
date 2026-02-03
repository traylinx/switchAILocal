# Management Dashboard V2

The **switchAILocal Management Dashboard** is a modern, single-page React-based interface that provides an intuitive way to configure your AI providers, manage model routing, and adjust system settings—no YAML editing required.

Access the dashboard at: `http://localhost:18080/management`

## 🌟 Key Features

The V2 dashboard is a complete architectural rewrite designed for performance, security, and ease of use:

- **Modern React UI**: Responsive design built with React 18 and Lucide icons.
- **Zero Dependencies**: A self-contained 226 KB HTML file that works offline with zero external network requests.
- **Optimistic UI**: Real-time updates with immediate visual feedback.
- **Universal Provider Support**: Visual configuration for 15+ CLI, Local, and Cloud providers.
- **Visual Routing**: Manage model mappings via a clean table interface.
- **Secure Authentication**: Integrated management key support with URL parameter convenience (`?key=...`).

---

## 🛠️ Dashboard Components

### 1. Provider Hub
Configure and monitor your AI connections in real-time.

- **CLI Tools**: Connect to Gemini CLI, Claude CLI, Codex, and Vibe. Use your existing premium subscriptions for FREE.
- **Local Models**: Auto-discovery for Ollama and LM Studio.
- **Cloud APIs**: Setup Traylinx, Google, Anthropic, and Groq with secure API key masks.
- **Connection Testing**: Integrated "Test Connection" buttons to verify setup before saving.

### 2. Model Routing
Manage how clients request models without manual YAML edits.

-   **Model Switching**: Map standardized names (e.g., `gpt-4`) to specific backends (e.g., `claudecli:v3`).
-   **Alias Management**: Create short, memorable aliases for long provider strings.
-   **Live Updates**: Mappings are applied instantly without server restarts.

### 3. System Settings
Control the core proxy behavior.

-   **Debug Mode**: One-click toggle for verbose server logging.
-   **Proxy URL**: Configure the external access URL for remote clients.
-   **System Info**: Real-time view of server host, port, and security (TLS) status.

---

## 🚀 Building & Development

The Management UI resides in the `frontend/` directory and compiles to a single, inlined HTML file.

### Quick Build
Use the unified script to build the UI:
```bash
./ail_ui.sh
```

### Manual Build
```bash
cd frontend
npm install
npm run build
```
**Output**: `static/management.html` (~226 KB, self-contained)

### Development Mode
```bash
cd frontend
npm run dev
```
The dev server runs on `http://localhost:5173` and proxies requests to the SwitchAILocal backend.

---

## 🔬 Architecture Details

-   **Framework**: React 18 + Vite
-   **State Management**: Zustand (minimal boilerplate, high performance)
-   **Data Fetching**: SWR (automatic polling, caching, and revalidation)
-   **Styling**: Modern Vanilla CSS with system-wide variables
-   **Inlining**: `vite-plugin-singlefile` bundles all CSS, JS, and Icons into one HTML file.

---

*The SwitchAILocal Management Dashboard: Empowering agents with a human-grade interface.* 🚀
