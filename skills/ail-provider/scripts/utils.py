import os
import sys
import json
import urllib.request
import urllib.error

def get_ail_dir():
    """Find the switchAILocal directory by checking common paths."""
    search_paths = [
        os.path.join(os.path.expanduser("~"), "Projects", "makakoo", "agents", "switchAILocal"),
        os.path.join(os.path.expanduser("~"), ".switchailocal", "repo")
    ]
    
    # Try looking for a git toplevel if we are inside a project (optional)
    try:
        import subprocess
        toplevel = subprocess.check_output(["git", "rev-parse", "--show-toplevel"], stderr=subprocess.DEVNULL).decode().strip()
        if toplevel:
            search_paths.append(toplevel)
    except:
        pass
        
    for path in search_paths:
        if os.path.exists(os.path.join(path, "config.yaml")):
            return path
            
    return None

def get_api_key():
    """Extract an active API key from config.yaml, or return the default fallback."""
    ail_dir = get_ail_dir()
    if not ail_dir:
        return "sk-test-123"
        
    config_path = os.path.join(ail_dir, "config.yaml")
    try:
        with open(config_path, "r") as f:
            for line in f:
                # Look for the first API key entry like: - "sk-..." or api-key: "..."
                if '"sk-' in line or "'sk-" in line:
                    parts = line.split('"')
                    if len(parts) >= 3:
                        return parts[1]
    except:
        pass
    
    return "sk-test-123"

def fetch_models():
    """Fetch the list of available models from the local proxy."""
    api_key = get_api_key()
    req = urllib.request.Request("http://localhost:18080/v1/models")
    req.add_header("Authorization", f"Bearer {api_key}")
    
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read().decode())
            return data.get("data", [])
    except urllib.error.HTTPError as e:
        if e.code == 401:
            print("🔴 ERROR: Unauthorized (401). Check API key in config.yaml.")
        return None
    except Exception as e:
        return None
