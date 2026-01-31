# Cortex Intelligent Router: Technical Manual (Deep Dive)

The **Cortex Intelligent Router** is the cognitive nucleus of `switchAILocal`. It transforms the system from a passive proxy into an active decision-maker that understands user intent and optimizes model selection "in the moment."

---

## 1. Request Interception Logic

Cortex only activates when a user explicitly requests an abstract model ID. It intercepts requests where:
- `model == "auto"`
- `model == "cortex"`

If any other specific model ID is provided (e.g., `gemini:pro`), Cortex steps aside and allows the standard routing logic to proceed.

---

## 2. Tier 0: Payload Pre-Processing

Before classification, Cortex extracts the core content. 
- It looks for the `"content"` field within the JSON request body.
- If it's a standard chat request, it parses the messages to find the latest user prompt.
- **Special Case**: It checks if the payload is an **Image Generation** request (contains `prompt` but no `messages`). If so, it immediately routes to the `image_gen` expert and sets the internal operation metadata to `images_generations`.

---

## 3. Tier 1: The Reflex Tier (Regex-Based)

This tier runs at near-zero latency using Lua string matching. It acts as a safety and efficiency filter.

### A. Visual Input Detection
Cortex scans the entire raw request body for visual markers:
- **Pattern**: `\"image_url\"` or `\"type\"%s*:%s*\"image\"`
- **Action**: Routes to the `vision` expert immediately.

### B. PII (Personally Identifiable Information) Detection
To ensure data privacy, Cortex checks for email patterns.
- **Pattern**: `[%w%.%_%-]+@[%w%.%_%-]+%.%a%a+`
- **Action**: Routes to the `secure` expert (intended to be a local or highly trusted model).

### C. Code Detection
Cortex looks for common programming markers:
- ` ``` ` (Markdown code blocks)
- `def%s+[%w_]+` (Python function definitions)
- `function%s*[%w_]*%(` (Javascript/Lua function definitions)
- `class%s+[%w_]+` (Class definitions)
- **Action**: Routes to the `coding` expert.

### D. Context Length Awareness
If the prompt text exceeds **4,000 characters**, Cortex preemptively routes it to the `long_ctx` expert to avoid context window overflows on smaller models.

---

## 4. Tier 2: The Cognitive Tier (LLM-Based)

If no Reflex rules trigger, Cortex engages the **Router Model** (configured via `intelligence.router-model`).

### The Classification Call
Cortex calls the internal `switchai.classify(text)` function. The Go host wraps your prompt in a hidden system instruction that demands a JSON response in this exact format:
```json
{
  "intent": "coding | reasoning | creative | image_generation | audio_transcription | audio_speech",
  "complexity": "simple | complex"
}
```

### The Decision Matrix Logic
Upon receiving the classification, Cortex maps the intent to your `config.yaml` matrix:

| Intent                | Complexity | Matrix Key Used |
| :-------------------- | :--------- | :-------------- |
| `coding`              | Any        | `coding`        |
| `reasoning`           | `complex`  | `reasoning`     |
| `creative`            | Any        | `creative`      |
| `image_generation`    | Any        | `image_gen`     |
| `audio_transcription` | Any        | `transcription` |
| `audio_speech`        | Any        | `speech`        |
| *Else*                | *Else*     | `fast`          |

---

## 5. Metadata & Operation Overrides

Cortex doesn't just change the model; it also adjusts the **Operation Metadata**. This ensures that if a user sends a chat-like prompt that is classified as `image_generation`, the system knows to call the image generation endpoint of the target provider rather than the chat completion endpoint.

- `image_generation` -> `operation: images_generations`
- `audio_transcription` -> `operation: audio_transcriptions`
- `audio_speech` -> `operation: audio_speech`

---

## 6. Implementation Reference (Lua)

The logic is contained within `/plugins/cortex-router/handler.lua`. 

### State Management
Cortex uses a **Host-Side Cache** (managed in Go) to remember classification results for identical prompts. This prevents redundant (and potentially expensive/slow) calls to the Router Model for repeated queries.

### Safety: Infinite Loop Protection
Cortex uses a `skip_lua` flag when calling the Router Model. This ensures that the classification request itself doesn't trigger another round of Cortex analysis, preventing recursive loops.
