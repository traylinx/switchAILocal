#!/usr/bin/env python3
from collections import defaultdict
from utils import fetch_models

def print_models_table():
    models = fetch_models()
    if models is None:
        print("🔴 ERROR: Failed to connect to switchAILocal on http://localhost:18080")
        print("Please run `ail start` to start the server.")
        return

    print("─── switchAILocal Models ───")
    print(f"Total available models: {len(models)}")
    print("\n| Provider | Model ID |")
    print("|---|---|")

    groups = defaultdict(list)
    for m in models:
        mid = m.get('id', '')
        owner = m.get('owned_by', 'unknown').upper()
        if ':' in mid:
            prefix = mid.split(':')[0]
            groups[prefix.upper()].append(mid)
        else:
            groups[owner].append(mid)

    for provider in sorted(groups.keys()):
        model_list = sorted(groups[provider])
        for model in model_list:
            # Highlight recommended models with an emoji
            rec = ""
            if model in ["geminicli:gemini-2.5-pro", "switchai:switchai-fast", "switchai:switchai-reasoner"]:
                rec = " ⭐"
            elif model.startswith("auto"):
                rec = " 🧠 (Auto-router)"
                
            print(f"| {provider} | `{model}`{rec} |")

if __name__ == "__main__":
    print_models_table()
