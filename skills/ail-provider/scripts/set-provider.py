#!/usr/bin/env python3
import os
import sys
import json
import urllib.request
from utils import get_api_key

def update_shell_profile(api_key):
    """Update ~/.zshrc or ~/.bashrc to force Claude Code to use our proxy."""
    shell_rc = os.path.expanduser("~/.zshrc")
    if not os.path.exists(shell_rc) and os.path.exists(os.path.expanduser("~/.bashrc")):
        shell_rc = os.path.expanduser("~/.bashrc")

    if not os.path.exists(shell_rc):
        return

    # Read current lines
    try:
        with open(shell_rc, "r") as f:
            lines = f.readlines()
            
        # Filter out old switchAILocal entries
        new_lines = []
        for line in lines:
            if "ANTHROPIC_BASE_URL" in line and ("18080" in line or "switchAILocal" in line):
                continue
            if "ANTHROPIC_API_KEY" in line and ("sk-test" in line or "switchAILocal" in line):
                continue
            if "# switchAILocal provider" in line:
                continue
            new_lines.append(line)

        # Append new config
        new_lines.append("\n# switchAILocal provider config (managed by ail-provider skill)\n")
        new_lines.append('export ANTHROPIC_BASE_URL="http://localhost:18080/api/provider/anthropic"\n')
        new_lines.append(f'export ANTHROPIC_API_KEY="{api_key}"\n')

        with open(shell_rc, "w") as f:
            f.writelines(new_lines)
            
        print(f"✅ Shell profile updated: {shell_rc}")
    except Exception as e:
        print(f"⚠️ Warning: Could not update shell profile: {e}")


def update_claude_settings(model_id):
    """Update ~/.claude/settings.json to use the given model."""
    settings_path = os.path.expanduser("~/.claude/settings.json")
    
    # Ensure directory exists
    os.makedirs(os.path.dirname(settings_path), exist_ok=True)
    
    settings = {}
    if os.path.exists(settings_path):
        try:
            with open(settings_path, "r") as f:
                settings = json.load(f)
        except Exception:
            pass

    if "providerSettings" not in settings:
        settings["providerSettings"] = {}

    anthropic = settings["providerSettings"].get("anthropic", {})
    anthropic["model"] = model_id
    settings["providerSettings"]["anthropic"] = anthropic

    try:
        with open(settings_path, "w") as f:
            json.dump(settings, f, indent=2)
        print(f"✅ Claude Code settings updated to use model: {model_id}")
    except Exception as e:
        print(f"❌ Error updating {settings_path}: {e}")
        return False
    return True

def verify_connection(api_key, model_id):
    """Send a quick test request to ensure it works."""
    url = "http://localhost:18080/v1/chat/completions"
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}"
    }
    payload = json.dumps({
        "model": model_id,
        "messages": [{"role": "user", "content": "Say 'OK' if you receive this."}],
        "max_tokens": 10
    }).encode("utf-8")

    req = urllib.request.Request(url, data=payload, headers=headers)
    print(f"Testing connection to switchAILocal with model {model_id}...")
    
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read().decode())
            content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
            print(f"✅ Connection Verified! Model responded: {content.strip()}")
            return True
    except Exception as e:
        print(f"❌ Verification failed: {e}")
        return False

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python3 set-provider.py <model-id>")
        print("Example: python3 set-provider.py geminicli:gemini-2.5-pro")
        sys.exit(1)

    target_model = sys.argv[1]
    api_key = get_api_key()

    success = update_claude_settings(target_model)
    if success:
        update_shell_profile(api_key)
        verify_connection(api_key, target_model)
        
        print("\n🎉 SETUP COMPLETE!")
        print(f"All requests will now route through http://localhost:18080 using {target_model}.")
        print("NOTE: You may need to restart your terminal or run `source ~/.zshrc` for shell changes to take effect.")
