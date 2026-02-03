# Multimodal (Images)

Learn how to work with images using switchAILocal and OpenAI-compatible formats.

## 🖼️ Image Handling

Send images in your messages using base64 encoding or public URLs.

### Python Example

```python
import base64
from openai import OpenAI

client = OpenAI(base_url="http://localhost:18080/v1", api_key="sk-test-123")

def encode_image(path):
    with open(path, "rb") as f:
        return base64.b64encode(f.read()).decode('utf-8')

response = client.chat.completions.create(
    model="geminicli:gemini-2.5-pro",
    messages=[{
        "role": "user",
        "content": [
            {"type": "text", "text": "What's in this image?"},
            {
                "type": "image_url",
                "image_url": {"url": f"data:image/jpeg;base64,{encode_image('image.jpg')}"}
            }
        ]
    }]
)
print(response.choices[0].message.content)
```

### cURL Example

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image."},
          {
            "type": "image_url",
            "image_url": "https://example.com/image.jpg"
          }
        ]
      }
    ]
  }'
```

## Guidelines

1. **Provider Support**: Use `geminicli:`, `claudecli:`, or `vibe:` for the best multimodal support.
2. **Resolution**: Most providers work best with images under 2048x2048.
3. **Format**: JPEG, PNG, WEBP, and GIF are generally supported.
4. **Local Path**: CLI providers can also handle local paths directly if passed via `attachments`.
