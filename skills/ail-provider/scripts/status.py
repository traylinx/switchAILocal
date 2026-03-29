#!/usr/bin/env python3
import os
import json
import urllib.request
from utils import get_ail_dir, get_api_key

def check_status():
    ail_dir = get_ail_dir()
    if not ail_dir:
        print("🔴 STATUS: switchAILocal directory not found.")
        print("  Please ensure it is installed correctly.")
        return

    api_key = get_api_key()
    req = urllib.request.Request("http://localhost:18080/v1/models")
    req.add_header("Authorization", f"Bearer {api_key}")
    
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            if resp.getcode() == 200:
                data = json.loads(resp.read().decode())
                model_count = len(data.get("data", []))
                print(f"🟢 STATUS: RUNNING ({model_count} models available on port 18080)")
            else:
                print(f"🟠 STATUS: UNKNOWN (HTTP {resp.getcode()})")
    except urllib.error.HTTPError as e:
        if e.code == 401:
            print(f"🟢 STATUS: RUNNING (But unauthorized - check API key in config.yaml)")
        else:
            print(f"🟠 STATUS: UNKNOWN (HTTP {e.code})")
    except Exception as e:
        print("🔴 STATUS: OFFLINE (Could not connect to localhost:18080)")
        print("  Start it by running: `ail start`")

def check_claude_config():
    settings_path = os.path.expanduser("~/.claude/settings.json")
    if not os.path.exists(settings_path):
        print("🔘 CLAUDE CODE: Default settings (no settings.json found)")
        return
        
    try:
        with open(settings_path, "r") as f:
            settings = json.load(f)
            providers = settings.get("providerSettings", {})
            for pk, pv in providers.items():
                if isinstance(pv, dict):
                    m = pv.get("model", "")
                    if m:
                        if pk == "anthropic" and m.startswith("gemini") or m.startswith("switch") or m.startswith("claude"):
                            print(f"🔵 CLAUDE CODE MODEL: {m} (routed via switchAILocal)")
                        else:
                            print(f"🔘 CLAUDE CODE MODEL: {m} (Provider: {pk})")
    except Exception as e:
        print(f"🔘 CLAUDE CODE: Error reading settings.json: {e}")

if __name__ == "__main__":
    print("─── switchAILocal Health Check ───")
    check_status()
    print("\n─── Current Settings ───")
    check_claude_config()
