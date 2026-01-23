-- Privacy Firewall Plugin
-- Intercepts prompts and redacts suspected PII (Phone numbers and Emails)
-- Demonstrates: Interceptor Pattern using switchAILocal Lua Engine

-- Note: No external libraries needed. The 'data' argument is already a Lua table.

function on_request(data)
    local messages = data["messages"]
    
    -- Handle OpenAI-style messages array
    if messages ~= nil then
        for i, msg in ipairs(messages) do
            if msg["role"] == "user" then
                local content = msg["content"]
                
                -- Handle string content (legacy format)
                if type(content) == "string" then
                    messages[i]["content"] = redact_pii(content)
                
                -- Handle array content (new OpenAI format: [{type: "text", text: "..."}])
                elseif type(content) == "table" then
                    for j, part in ipairs(content) do
                        if part["type"] == "text" and type(part["text"]) == "string" then
                            content[j]["text"] = redact_pii(part["text"])
                        end
                    end
                end
            end
        end
    end

    -- Handle simple prompt string (legacy/completion API)
    if data["prompt"] ~= nil and type(data["prompt"]) == "string" then
        data["prompt"] = redact_pii(data["prompt"])
    end

    print("[Privacy Firewall] Request inspected and sanitized.")
    return data
end

function redact_pii(text)
    local original = text
    
    -- Redact Email addresses
    -- Pattern: word@word.word (handles dots, hyphens, underscores)
    text = string.gsub(text, "[%w%.%-_]+@[%w%.%-_]+%.%w+", "[EMAIL REDACTED]")
    
    -- Redact Phone Numbers (multiple formats)
    -- US Format: 555-123-4567
    text = string.gsub(text, "%d%d%d%-%d%d%d%-%d%d%d%d", "[PHONE REDACTED]")
    -- US Format: 555-1234
    text = string.gsub(text, "%d%d%d%-%d%d%d%d", "[PHONE REDACTED]")
    -- US Format: (555) 123-4567
    text = string.gsub(text, "%(%d%d%d%)%s*%d%d%d%-%d%d%d%d", "[PHONE REDACTED]")
    -- International: +1 555 123 4567
    text = string.gsub(text, "%+%d+%s+%d+%s+%d+%s+%d+", "[PHONE REDACTED]")
    
    if original ~= text then
        print("[Privacy Firewall] 🚨 PII Detected and Redacted!")
    end
    
    return text
end
