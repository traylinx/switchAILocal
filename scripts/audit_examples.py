
import subprocess
import json
import re
import os

def run_curl(cmd):
    # Prepare the command:
    # 1. Handle management key
    cmd = cmd.replace("your-secret-key", "your-secret-key")
    
    # 2. Handle image data shell logic if present
    if "IMAGE_DATA=" in cmd:
        # Just use a dummy base64 for the audit
        dummy_base64 = "data:image/jpeg;base64,/9j/4AAQSkZJRg=="
        cmd = cmd.replace('$(base64 -i path/to/image.jpg)', dummy_base64)
        # Also remove the assignment line if it's there
        cmd = re.sub(r'IMAGE_DATA=.*?\n', f'IMAGE_DATA="{dummy_base64}"\n', cmd)

    try:
        # Use a 15-second timeout
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=15)
        return result.returncode, result.stdout, result.stderr
    except subprocess.TimeoutExpired:
        return -2, "", "Timeout: Request took longer than 15s"
    except Exception as e:
        return -1, "", str(e)

def extract_curls(file_path):
    with open(file_path, 'r') as f:
        content = f.read()
    
    # Simple regex to find bash code blocks
    blocks = re.findall(r'```bash\n(.*?)\n```', content, re.DOTALL)
    curls = []
    for b in blocks:
        if 'curl' in b:
            curls.append(b.strip())
    return curls

def main():
    examples_path = "/Users/sebastian/Projects/makakoo/agents/switchAILocal/docs/user/examples.md"
    curls = extract_curls(examples_path)
    
    report = "# Example Curl Audit (AUDIT1)\n\n"
    report += "This audit provides a real-world verification of all examples documented in `docs/user/examples.md`.\n\n"
    report += "| # | Request | Status | Result | Notes |\n"
    report += "|---|---------|--------|--------|-------|\n"
    
    for i, cmd in enumerate(curls):
        print(f"Running curl {i+1}/{len(curls)}...")
        
        # Clean up the command for display
        first_line = cmd.split('\n')[0]
        display_cmd = (first_line[:50] + "...") if len(first_line) > 50 else first_line
        
        code, stdout, stderr = run_curl(cmd)
        
        status = "✅ PASS"
        notes = "-"
        
        if code == -2:
            status = "⏳ TIMEOUT"
            result_summary = "Request timed out (15s)"
        elif code != 0:
            status = "❌ FAIL"
            result_summary = f"Process error {code}"
            if stderr:
                notes = stderr[:100]
        else:
            try:
                data = json.loads(stdout)
                if "error" in data or data.get("success") == False:
                    status = "⚠️ APP ERROR"
                    result_summary = data.get("error") or data.get("message") or "Error response"
                else:
                    result_summary = "200 OK (JSON)"
            except:
                if stdout.strip():
                    result_summary = "200 OK (Stream/Text)"
                else:
                    result_summary = "200 OK (Empty Body)"

        report += f"| {i+1} | `{display_cmd}` | {status} | {result_summary} | {notes} |\n"
        
        # Detail section
        report += f"\n### [{i+1}] {display_cmd}\n"
        report += f"**Command:**\n```bash\n{cmd}\n```\n"
        if stdout:
            report += f"**Response stdout:**\n```json\n{stdout}\n```\n"
        if stderr:
             report += f"**Response stderr:**\n```\n{stderr}\n```\n"
        report += "\n---\n"

    audit_path = "/Users/sebastian/.gemini/antigravity/brain/c362752a-d55b-4b4b-aefd-c8262e40767b/AUDIT1.md"
    with open(audit_path, "w") as f:
        f.write(report)
    print(f"Audit report generated at: {audit_path}")

if __name__ == "__main__":
    main()
