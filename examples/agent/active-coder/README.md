# Simple Coder Example

This example demonstrates a basic **agent loop**: Ask an LLM to generate code, extract it, and execute it locally.

> ⚠️ **Note**: This example performs direct local execution using Python's `subprocess`. It does **NOT** use the switchAILocal Bridge Agent WebSocket API. For Bridge Agent examples, see `examples/agent/bridge-executor/`.

## How it Works

1. **Ask**: Send a task description to the LLM via switchAILocal.
2. **Extract**: Parse the response for a Python code block.
3. **Confirm**: Display the code and ask the user for confirmation.
4. **Execute**: Run the code locally and display the output.

## Usage

```bash
# Install dependencies
pip install -r requirements.txt

# Run with a task
python coder.py "Calculate the 20th Fibonacci number"

# Use a different model
SWITCHAI_MODEL=gpt-4o python coder.py "List files in current directory"
```

## Configuration

| Environment Variable | Default                                     | Description  |
| -------------------- | ------------------------------------------- | ------------ |
| `SWITCHAI_URL`       | `http://localhost:8081/v1/chat/completions` | API endpoint |
| `SWITCHAI_MODEL`     | `gemini-2.5-flash`                          | Model to use |

## Security Warning

This example executes LLM-generated code directly on your machine. Always review the generated code before confirming execution!
