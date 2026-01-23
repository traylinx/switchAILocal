# switchAILocal Examples

This directory contains examples demonstrating how to extend and integrate with switchAILocal.

## 📁 Structure

| Directory                                                               | Description                                            |
| ----------------------------------------------------------------------- | ------------------------------------------------------ |
| **[middleware/privacy-firewall](./middleware/privacy-firewall/)**       | Lua plugin that redacts PII from prompts               |
| **[agent/active-coder](./agent/active-coder/)**                         | Simple agent that generates and executes Python code   |
| **[agent/bridge-executor](./agent/bridge-executor/)**                   | WebSocket Relay API client (the "real" Bridge pattern) |
| **[configuration/resilient-router](./configuration/resilient-router/)** | Tiered failover configuration example                  |
| **[advanced/custom-provider](./advanced/custom-provider/)**             | Go SDK extension for custom providers                  |
| **[legacy/translator](./legacy/translator/)**                           | Translation library usage (deprecated)                 |

## 🚀 Quick Start

### 1. Privacy Firewall (Lua Plugin)

Intercepts requests and redacts emails/phone numbers before they leave your machine.

```bash
cd middleware/privacy-firewall
# Copy config to server root and start switchAILocal
```

### 2. Simple Coder (Agent)

Generates Python code from natural language and executes it locally.

```bash
cd agent/active-coder
pip install -r requirements.txt
python coder.py "Calculate the 20th Fibonacci number"
```

### 3. Bridge Executor (WebSocket)

Demonstrates the WebSocket Relay API used by Gemini CLI and Claude Code.

```bash
cd agent/bridge-executor
pip install -r requirements.txt
python executor.py models
```

### 4. Resilient Router (Config)

Shows a tiered failover strategy: Local → Cheap Cloud → Premium Cloud.

```bash
cd configuration/resilient-router
# Use this config.yaml with your switchAILocal server
```
