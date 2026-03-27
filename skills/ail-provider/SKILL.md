---
name: ail-provider
description: |
  Configure Claude Code to route all API requests through switchAILocal, a local
  AI gateway that unifies Gemini, Claude, Ollama, OpenAI, Groq, Alibaba, Xiaomi,
  and other providers behind a single OpenAI-compatible endpoint. Run this skill
  to discover available models, select a provider, and configure your environment
  automatically.
allowed-tools:
  - Bash
  - Read
  - Write
  - AskUserQuestion

---

# switchAILocal Provider Setup

This skill configures Claude Code to use **switchAILocal** (`ail`) as its API provider.
Once configured, all your Claude Code requests route through `http://localhost:18080`,
which can forward to any of your configured AI providers (Gemini CLI, Ollama, Groq,
Xiaomi, Alibaba, switchAI Cloud, and more).

## Preamble (run first)

```bash
# ── 1. Locate switchAILocal ──────────────────────────────────────────
AIL_DIR=""
for _D in \
  "$HOME/Projects/makakoo/agents/switchAILocal" \
  "$HOME/.switchailocal/repo" \
  "$(git rev-parse --show-toplevel 2>/dev/null)"; do
  [ -f "$_D/config.yaml" ] && AIL_DIR="$_D" && break
done

if [ -z "$AIL_DIR" ]; then
  echo "AIL_NOT_FOUND"
else
  echo "AIL_DIR: $AIL_DIR"
fi

# ── 2. Check if switchAILocal server is running ──────────────────────
_API_KEY=$(grep -A0 '  - "' "$AIL_DIR/config.yaml" 2>/dev/null | head -1 | sed 's/.*"\(.*\)".*/\1/' || echo "sk-test-123")
_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" -m 3 http://localhost:18080/v1/models -H "Authorization: Bearer $_API_KEY" 2>/dev/null || echo "000")

if [ "$_HEALTH" = "200" ]; then
  echo "AIL_STATUS: RUNNING"
  echo "AIL_API_KEY: $_API_KEY"
else
  echo "AIL_STATUS: OFFLINE ($_HEALTH)"
fi

# ── 3. Fetch and group available models ──────────────────────────────
if [ "$_HEALTH" = "200" ]; then
  curl -s http://localhost:18080/v1/models -H "Authorization: Bearer $_API_KEY" 2>/dev/null | \
    python3 -c "
import json, sys
from collections import defaultdict
try:
    data = json.load(sys.stdin)
    models = data.get('data', [])
    groups = defaultdict(list)
    for m in models:
        mid = m.get('id', '')
        owner = m.get('owned_by', 'unknown').upper()
        # Detect provider from prefix
        if ':' in mid:
            prefix = mid.split(':')[0]
            groups[prefix.upper()].append(mid)
        else:
            groups[owner].append(mid)
    print(f'TOTAL_MODELS: {len(models)}')
    for provider in sorted(groups.keys()):
        model_list = sorted(groups[provider])
        print(f'  {provider} ({len(model_list)}): {\" | \".join(model_list[:8])}')
        if len(model_list) > 8:
            print(f'    ... and {len(model_list) - 8} more')
except Exception as e:
    print(f'PARSE_ERROR: {e}')
" 2>/dev/null || echo "MODEL_FETCH_FAILED"
fi

# ── 4. Check current Claude Code config ──────────────────────────────
_CLAUDE_SETTINGS="$HOME/.claude/settings.json"
if [ -f "$_CLAUDE_SETTINGS" ]; then
  _CURRENT_MODEL=$(python3 -c "
import json
with open('$_CLAUDE_SETTINGS') as f:
    s = json.load(f)
    p = s.get('providerSettings', {})
    for pk, pv in p.items():
        if isinstance(pv, dict):
            m = pv.get('model', '')
            if m: print(f'CURRENT: {m}')
" 2>/dev/null || echo "UNKNOWN")
  echo "$_CURRENT_MODEL"
else
  echo "CURRENT: default (no settings.json)"
fi
```

## After Preamble — Interactive Setup

Based on the preamble output, follow these steps:

### If `AIL_NOT_FOUND`
Tell the user: "switchAILocal config not found. Please provide the path to your switchAILocal directory."
Use AskUserQuestion to get the path, then re-read `config.yaml` from that location.

### If `AIL_STATUS: OFFLINE`
Tell the user: "switchAILocal server is not running on port 18080. Start it with:"
```bash
cd <AIL_DIR> && ail start
```
Then wait for the user to confirm it's running before proceeding.

### If `AIL_STATUS: RUNNING`

1. **Present the Model Wizard.** Format the model groups from the preamble output into
   a clean table, organized by provider. Highlight recommended models:
   - For **coding**: `geminicli:gemini-2.5-pro`, `ollama:kimi-k2.5:cloud`
   - For **fast tasks**: `switchai:switchai-fast`, `ollama:gpt-oss:20b-cloud`
   - For **reasoning**: `switchai:switchai-reasoner`, `xiaomi:mimo-v2-pro`

2. **Ask the user** via AskUserQuestion:
   > Which model do you want to use? Enter the full model ID from the list above
   > (e.g., `xiaomi:mimo-v2-pro` or `geminicli:gemini-2.5-pro`).

3. **Configure environment.** After the user selects a model, run:

```bash
# Write env vars to shell profile
SHELL_RC="$HOME/.zshrc"
[ -f "$HOME/.bashrc" ] && [ ! -f "$HOME/.zshrc" ] && SHELL_RC="$HOME/.bashrc"

# Remove any existing ail entries
grep -v "ANTHROPIC_BASE_URL.*switchAILocal\|ANTHROPIC_BASE_URL.*18080\|ANTHROPIC_API_KEY.*sk-test\|# switchAILocal" "$SHELL_RC" > "$SHELL_RC.ail-bak" 2>/dev/null
mv "$SHELL_RC.ail-bak" "$SHELL_RC"

# Append fresh config
cat >> "$SHELL_RC" << 'AILEOF'

# switchAILocal provider config (managed by ail-provider skill)
export ANTHROPIC_BASE_URL="http://localhost:18080/api/provider/anthropic"
export ANTHROPIC_API_KEY="AIL_API_KEY_PLACEHOLDER"
AILEOF

# Replace placeholder with actual key
sed -i '' "s/AIL_API_KEY_PLACEHOLDER/$_API_KEY/" "$SHELL_RC" 2>/dev/null || \
  sed -i "s/AIL_API_KEY_PLACEHOLDER/$_API_KEY/" "$SHELL_RC"

echo "✅ Shell profile updated: $SHELL_RC"
```

4. **Update Claude Code settings.** Write the selected model to `~/.claude/settings.json`:

```bash
python3 << 'PYEOF'
import json, os, sys

settings_path = os.path.expanduser("~/.claude/settings.json")
model = "SELECTED_MODEL"  # Claude: replace with the user's chosen model

if os.path.exists(settings_path):
    with open(settings_path) as f:
        settings = json.load(f)
else:
    settings = {}

# Update model in provider settings
if "providerSettings" not in settings:
    settings["providerSettings"] = {}

# Set the model for the Anthropic provider
anthropic = settings["providerSettings"].get("anthropic", {})
anthropic["model"] = model
settings["providerSettings"]["anthropic"] = anthropic

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)

print(f"✅ Claude Code model set to: {model}")
PYEOF
```

5. **Verify the connection.** Run a test request:

```bash
curl -s http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $_API_KEY" \
  -d '{"model":"SELECTED_MODEL","messages":[{"role":"user","content":"Say hello in one word"}]}' | \
  python3 -c "
import json, sys
try:
    r = json.load(sys.stdin)
    content = r['choices'][0]['message']['content']
    print(f'✅ VERIFIED — Response: {content[:80]}')
except Exception as e:
    print(f'❌ VERIFY FAILED: {e}')
"
```

If verification succeeds, tell the user:
> ✅ **switchAILocal is configured!** All your Claude Code requests now route through
> `http://localhost:18080` using model `SELECTED_MODEL`.
>
> You can change models anytime by running this skill again or by editing
> `~/.claude/settings.json`.

## Changing Models Later

To switch models without re-running the full wizard:

1. List available models: `curl -s http://localhost:18080/v1/models -H "Authorization: Bearer sk-test-123" | python3 -c "import json,sys; [print(m['id']) for m in json.load(sys.stdin)['data']]"`
2. Edit `~/.claude/settings.json` and change the `model` field
3. Restart Claude Code
