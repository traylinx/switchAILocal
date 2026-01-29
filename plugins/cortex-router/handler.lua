-- handler.lua for cortex-router
local Schema = require("schema")

-- Configuration: Map intents to model aliases (should match config.yaml)
local config = {
    models = {
        coding    = "switchai-chat",
        reasoning = "switchai-reasoner",
        creative  = "switchai-chat",
        fast      = "switchai-fast",
        secure    = "switchai-fast",
        long_ctx  = "switchai-chat"
    }
}

-- Private Helper: Check for PII
local function has_pii(text)
    if string.match(text, "[%w%.%_%-]+@[%w%.%_%-]+%.%a%a+") then return true end
    return false
end

-- Private Helper: Check for code detection
local function is_coding_request(text)
    if string.match(text, "```") then return true end
    if string.match(text, "def%s+[%w_]+") then return true end
    if string.match(text, "function%s*[%w_]*%(") then return true end
    if string.match(text, "class%s+[%w_]+") then return true end
    return false
end

-- Private Helper: Parse JSON
local function parse_classification(json_str)
    local intent = string.match(json_str, '"intent"%s*:%s*"(.-)"')
    local complexity = string.match(json_str, '"complexity"%s*:%s*"(.-)"')
    return {
        intent = intent or "chat",
        complexity = complexity or "simple"
    }
end

local Plugin = {}

function Plugin:on_request(req)
    if req.model ~= "auto" and req.model ~= "cortex" then
        return nil 
    end

    local input = req.body or ""
    local prompt_content = input -- simple extraction simplification
    local content_match = string.match(input, '"content"%s*:%s*"(.-)"')
    if content_match then prompt_content = content_match end
    
    switchai.log("Analysing request...")

    -- TIER 1: REFLEX
    if has_pii(prompt_content) then
        switchai.log("Reflex: PII detected -> Secure Model")
        req.model = config.models.secure
        return req
    end

    if is_coding_request(prompt_content) then
        switchai.log("Reflex: Code detected -> Coding Model")
        req.model = config.models.coding
        return req
    end

    if #prompt_content > 4000 then
        switchai.log("Reflex: Long input -> Long Context Model")
        req.model = config.models.long_ctx
        return req
    end

    -- TIER 2: COGNITIVE
    switchai.log("Cognitive: Engaging Router Model...")
    local json_res, err = switchai.classify(prompt_content)
    if err then
        switchai.log("Classification Error: " .. err .. " -> Fallback Fast")
        req.model = config.models.fast
        return req
    end

    local decision = parse_classification(json_res)
    switchai.log(string.format("Decision: %s / %s", decision.intent, decision.complexity))

    if decision.intent == "coding" then
        req.model = config.models.coding
    elseif decision.intent == "reasoning" and decision.complexity == "complex" then
        req.model = config.models.reasoning
    elseif decision.intent == "creative" then
        req.model = config.models.creative
    else
        req.model = config.models.fast
    end

    switchai.log("Routing to: " .. req.model)
    return req
end

return Plugin
