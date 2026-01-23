"""
Simple Coder Example
Demonstrates a basic agent loop: Ask LLM to generate code, extract it, and execute locally.

NOTE: This example does NOT use the Bridge Agent WebSocket API.
      For Bridge Agent examples, see examples/agent/bridge-executor/
"""

import os
import sys
import requests
import re
import subprocess

# Configuration (can be overridden via environment variables)
SERVER_URL = os.environ.get("SWITCHAI_URL", "http://localhost:8081/v1/chat/completions")
MODEL = os.environ.get("SWITCHAI_MODEL", "gemini-2.5-flash")

def ask_llm(task):
    """Asks the LLM to write a Python script for the task."""
    print(f"🤖 User Task: {task}")
    
    prompt = f"""
    You are a Simple Coder agent. 
    Your goal is to write a PYTHON script that solves the user's task.
    
    Rules:
    1. Output ONLY the python code inside a fenced code block: ```python ... ```.
    2. The code must be complete and runnable.
    3. Do not include explanations outside the code block.
    
    Task: {task}
    """

    payload = {
        "model": MODEL,
        "messages": [
            {"role": "user", "content": prompt}
        ],
        "temperature": 0.2
    }

    try:
        response = requests.post(SERVER_URL, json=payload, timeout=60)
        response.raise_for_status()
        return response.json()['choices'][0]['message']['content']
    except requests.exceptions.RequestException as e:
        print(f"❌ Error communicating with switchAILocal: {e}")
        sys.exit(1)

def extract_code(llm_response):
    """Extracts python code block from response."""
    # Flexible pattern: handles ```python, ```Python, ```python3, etc.
    match = re.search(r"```[Pp]ython[3]?\s*\n(.*?)\n```", llm_response, re.DOTALL)
    if match:
        return match.group(1)
    return None

def execute_code(code):
    """Executes the extracted code after user confirmation."""
    print("\n📝 Generated Code:")
    print("-" * 40)
    print(code)
    print("-" * 40)
    
    confirm = input("\n⚠️  Execute this code? (y/N): ")
    if confirm.lower() != 'y':
        print("🛑 Execution cancelled.")
        return

    print("\n🚀 Executing...")
    try:
        result = subprocess.run(
            [sys.executable, "-c", code], 
            capture_output=True, 
            text=True, 
            timeout=30
        )
        print("✅ Output:")
        print(result.stdout)
        if result.stderr:
            print("⚠️ Errors:")
            print(result.stderr)
    except subprocess.TimeoutExpired:
        print("💥 Execution timed out (30s limit).")
    except Exception as e:
        print(f"💥 Execution failed: {e}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python coder.py \"<your task>\"")
        print("Example: python coder.py \"Calculate the 10th Fibonacci number\"")
        print("\nEnvironment Variables:")
        print("  SWITCHAI_URL   - API endpoint (default: http://localhost:8081/v1/chat/completions)")
        print("  SWITCHAI_MODEL - Model to use (default: gemini-2.5-flash)")
        sys.exit(1)

    task = sys.argv[1]
    
    # 1. Ask LLM
    response = ask_llm(task)
    
    # 2. Extract Code
    code = extract_code(response)
    
    if not code:
        print("❌ No code block found in LLM response.")
        print("Raw response:", response)
        sys.exit(1)
        
    # 3. Execute
    execute_code(code)

if __name__ == "__main__":
    main()
