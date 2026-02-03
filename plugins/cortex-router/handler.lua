--[[
================================================================================
CORTEX ROUTER - Phase 2 Handler
================================================================================

This handler implements intelligent multi-tier routing for switchAILocal.
It routes requests with model="auto" or model="cortex" to the optimal model
based on request content analysis.

ROUTING FLOW:
-------------
1. CACHE TIER (< 1ms)
   - Semantic cache lookup for similar previous queries
   - Requires: intelligence.semantic_cache.enabled

2. REFLEX TIER (< 1ms)
   - Fast regex-based pattern matching
   - Detects: PII, code blocks, visual input, long context
   - No external dependencies

3. SEMANTIC TIER (< 20ms)
   - Embedding-based intent matching
   - Requires: intelligence.embedding.enabled, intelligence.semantic_tier.enabled
   - Falls back to cognitive tier on low confidence

4. COGNITIVE TIER (200-500ms)
   - LLM-based classification
   - Uses router-model from config
   - Includes confidence scoring

5. VERIFICATION (< 100ms)
   - Cross-validates semantic and cognitive results
   - Escalates to reasoning model on disagreement

6. CASCADE (post-response)
   - Quality-based model escalation
   - Retries with better model if response quality is poor

LUA API FUNCTIONS (Phase 2):
----------------------------
switchai.get_dynamic_matrix()
  Returns: table {slot: {primary, fallbacks, score, reason}} or nil, error
  Purpose: Get auto-assigned model matrix based on discovered models

switchai.get_available_models()
  Returns: table [{id, provider, display_name, capabilities, is_available}] or nil, error
  Purpose: Get list of discovered models from all providers

switchai.is_model_available(model_id)
  Returns: boolean
  Purpose: Check if a specific model is available

switchai.embed(text)
  Returns: table [384 floats] or nil, error
  Purpose: Compute embedding vector for text

switchai.semantic_match_intent(text)
  Returns: table {intent, confidence, latency_ms} or nil, error
  Purpose: Match query to best intent using semantic similarity

switchai.match_skill(text)
  Returns: table {skill: {id, name, description, system_prompt}, confidence} or nil, error
  Purpose: Match query to best skill using semantic similarity

switchai.cache_lookup(query)
  Returns: table {decision, metadata} or nil, error
  Purpose: Look up cached routing decision

switchai.cache_store(query, decision, metadata)
  Returns: nil, error
  Purpose: Store routing decision in semantic cache

switchai.cache_metrics()
  Returns: table {hits, misses, size, hit_rate} or nil, error
  Purpose: Get cache performance metrics

switchai.parse_confidence(json_str)
  Returns: table {intent, complexity, confidence} or nil, error
  Purpose: Parse LLM classification response with confidence

switchai.verify_intent(tier1_intent, tier2_intent)
  Returns: boolean
  Purpose: Check if two intents are equivalent

switchai.evaluate_response(response, current_tier)
  Returns: table {should_cascade, next_tier, quality_score, reason, signals} or nil, error
  Purpose: Evaluate response quality for cascade decision

switchai.get_cascade_metrics()
  Returns: table or nil, error
  Purpose: Get cascade performance metrics

switchai.record_feedback(data)
  Returns: nil, error
  Purpose: Record routing feedback for learning

EXISTING LUA API FUNCTIONS (Phase 1):
-------------------------------------
switchai.classify(prompt)
  Returns: string (JSON) or nil, error
  Purpose: Classify prompt using router model

switchai.log(message)
  Purpose: Log message to server logs

switchai.config
  Fields: router_model, router_fallback, skills_path, matrix
  Purpose: Access intelligence configuration

switchai.get_cache(key) / switchai.set_cache(key, value)
  Purpose: Simple key-value cache

switchai.scan_skills(path)
  Returns: table {name: {name, description, required_capability, content}}
  Purpose: Scan SKILL.md files from directory

switchai.get_skills()
  Returns: table {id: {id, name, description, required_capability, system_prompt, has_embedding}}
  Purpose: Get skills from enhanced registry

switchai.json_inject(json_str, content)
  Returns: string (JSON)
  Purpose: Inject system message into chat payload

CONFIGURATION:
--------------
intelligence:
  enabled: true                    # Master switch
  router-model: "ollama:qwen:0.5b" # Classification model
  router-fallback: "openai:gpt-4o-mini"
  matrix:                          # Static capability matrix
    coding: "switchai-chat"
    reasoning: "switchai-reasoner"
    creative: "switchai-chat"
    fast: "switchai-fast"
    secure: "switchai-fast"
    vision: "switchai-chat"
  
  # Phase 2 features (all optional)
  discovery:
    enabled: true
  embedding:
    enabled: true
    model: "all-MiniLM-L6-v2"
  semantic_tier:
    enabled: true
    confidence_threshold: 0.85
  skill_matching:
    enabled: true
    confidence_threshold: 0.80
  semantic_cache:
    enabled: true
    similarity_threshold: 0.95
    max_size: 10000
  confidence:
    enabled: true
  verification:
    enabled: true
  cascade:
    enabled: true
    quality_threshold: 0.70
  feedback:
    enabled: true
    retention_days: 90

GRACEFUL DEGRADATION:
---------------------
- If intelligence.enabled: false, all Phase 2 Lua functions return errors
- Plugin falls back to static config.matrix for routing
- Each feature can be independently disabled
- Missing embedding model disables semantic tier and cache
- System continues to function with reduced capabilities

================================================================================
--]]

local Schema = require("schema")

-- Configuration: Map intents to model aliases (should match config.yaml)
-- This is the static fallback configuration used when dynamic matrix is unavailable
local config = {
    models = {
        coding        = switchai.config.matrix.coding        or "switchai-chat",
        reasoning     = switchai.config.matrix.reasoning     or "switchai-reasoner",
        creative      = switchai.config.matrix.creative      or "switchai-chat",
        fast          = switchai.config.matrix.fast          or "switchai-fast",
        secure        = switchai.config.matrix.secure        or "switchai-fast",
        long_ctx      = switchai.config.matrix.long_ctx      or "switchai-chat",
        image_gen     = switchai.config.matrix.image_gen     or error("config.intelligence.matrix.image_gen not configured"),
        transcription = switchai.config.matrix.transcription or error("config.intelligence.matrix.transcription not configured"),
        speech        = switchai.config.matrix.speech        or error("config.intelligence.matrix.speech not configured"),
        vision        = switchai.config.matrix.vision        or "switchai-chat",
        research      = switchai.config.matrix.research      or "switchai-chat",
        chat          = switchai.config.matrix.chat          or "switchai-fast",
        long_context  = switchai.config.matrix.long_context  or "switchai-chat"
    },
    -- Semantic tier confidence threshold (default 0.85)
    -- Queries with confidence >= this threshold are routed directly
    -- Lower confidence queries fall through to cognitive tier
    semantic_threshold = 0.85
}

--[[
get_model_for_capability(capability)
  
  Resolves a capability slot to an actual model ID.
  
  Priority:
  1. Dynamic matrix (if intelligence services enabled)
  2. Static config.models fallback
  
  Parameters:
    capability: string - The capability slot (coding, reasoning, etc.)
  
  Returns:
    string - The model ID to use for this capability
--]]

--[[
get_model_for_capability(capability)
  
  Resolves a capability slot to an actual model ID.
  
  Priority:
  1. Dynamic matrix (if intelligence services enabled)
  2. Static config.models fallback
  
  Parameters:
    capability: string - The capability slot (coding, reasoning, etc.)
  
  Returns:
    string - The model ID to use for this capability
--]]
local function get_model_for_capability(capability)
    -- Try dynamic matrix first (Phase 2)
    -- Returns nil if intelligence services disabled
    local matrix, err = switchai.get_dynamic_matrix()
    if matrix and matrix[capability] then
        local assignment = matrix[capability]
        -- Check if primary model is available
        if switchai.is_model_available(assignment.primary) then
            return assignment.primary
        end
        -- Try fallback models in order
        if assignment.fallbacks then
            for _, fallback in ipairs(assignment.fallbacks) do
                if switchai.is_model_available(fallback) then
                    switchai.log("Primary unavailable, using fallback: " .. fallback)
                    return fallback
                end
            end
        end
    end
    -- Fall back to static config (v1.0 behavior)
    return config.models[capability] or config.models.fast
end

--[[
REFLEX TIER HELPER FUNCTIONS
These functions provide fast pattern matching for obvious routing decisions.
They run in < 1ms and don't require any external services.
--]]

-- has_pii(text): Detect potential PII (email addresses)
-- Returns: boolean
-- Routes to: secure model
local function has_pii(text)
    if string.match(text, "[%w%.%_%-]+@[%w%.%_%-]+%.%a%a+") then return true end
    return false
end

-- is_coding_request(text): Detect code-related content
-- Returns: boolean
-- Routes to: coding model
local function is_coding_request(text)
    if string.match(text, "```") then return true end           -- Code blocks
    if string.match(text, "def%s+[%w_]+") then return true end  -- Python functions
    if string.match(text, "function%s*[%w_]*%(") then return true end  -- JS/Lua functions
    if string.match(text, "class%s+[%w_]+") then return true end  -- Class definitions
    return false
end

-- is_image_gen_request(input_str): Detect image generation payloads
-- Returns: boolean
-- Routes to: image_gen model
local function is_image_gen_request(input_str)
    -- Image generation has "prompt" but no "messages" array
    if string.match(input_str, '"prompt"%s*:%s*".-"') and not string.match(input_str, '"messages"%s*:%s*%[') then
        return true
    end
    return false
end

-- has_visual_input(input_str): Detect image content in chat
-- Returns: boolean
-- Routes to: vision model
local function has_visual_input(input_str)
    if string.match(input_str, '"image_url"') or string.match(input_str, '"type"%s*:%s*"image"') then
        return true
    end
    return false
end

--[[
intent_to_capability(intent)

Maps semantic/cognitive intent names to capability slots.
This allows the routing tiers to use consistent capability names.

Parameters:
  intent: string - The detected intent (coding, reasoning, etc.)

Returns:
  string - The capability slot name
--]]
local function intent_to_capability(intent)
    local mapping = {
        coding = "coding",
        reasoning = "reasoning",
        creative = "creative",
        fast = "fast",
        secure = "secure",
        vision = "vision",
        long_context = "long_ctx",
        image_generation = "image_gen",
        transcription = "transcription",
        speech = "speech",
        research = "research",
        chat = "chat"
    }
    return mapping[intent] or "fast"
end

--[[
finalize_request(req)

Ensures the request is properly formatted for the Go host.
Extracts provider prefix from model string if present.

Parameters:
  req: table - The request object

Returns:
  table - The finalized request
--]]
local function finalize_request(req)
    if not req.model then return req end
    
    -- Extract provider prefix if present (e.g., "ollama:llama3.2" -> provider="ollama", model="llama3.2")
    local prefix, model_only = string.match(req.model, "^([^:]+):(.+)$")
    if prefix and model_only then
        req.provider = prefix
        req.model = model_only
        switchai.log(string.format("Final Routing -> Provider: %s, Model: %s", prefix, model_only))
    else
        switchai.log("Final Routing -> Model: " .. req.model)
    end
    
    return req
end

--[[
================================================================================
PLUGIN IMPLEMENTATION
================================================================================
--]]

local Plugin = {}

--[[
Plugin:on_request(req)

Main request handler. Routes requests with model="auto" or model="cortex"
through the multi-tier routing system.

Parameters:
  req: table - The request object with fields:
    - model: string - The requested model (or "auto"/"cortex")
    - body: string - The request body (JSON)
    - metadata: table - Optional metadata

Returns:
  table - Modified request with routed model, or nil to pass through

Routing Tiers (in order):
  1. Reflex: Fast pattern matching (PII, code, visual, long input)
  2. Semantic: Embedding-based intent matching (if enabled)
  3. Cognitive: LLM-based classification with confidence scoring
  4. Verification: Cross-validation of semantic and cognitive results
--]]


local Plugin = {}

function Plugin:on_request(req)
    if req.model ~= "auto" and req.model ~= "cortex" then
        return nil 
    end

    -- Do not route if a specific provider is requested
    if req.provider and req.provider ~= "" then
        return nil
    end

    -- Initialize metadata if missing
    req.metadata = req.metadata or {}

    local input = req.body or ""
    
    -- Special routing for endpoint-specific payloads
    if is_image_gen_request(input) then
        switchai.log("Reflex: Image Generation Payload detected -> Image Model")
        req.model = get_model_for_capability("image_gen")
        req.metadata.operation = "images_generations"
        req.metadata.routing_tier = "reflex"
        return finalize_request(req)
    end

    local prompt_content = input
    -- Robust content extraction (handles escaped quotes and block style)
    local content_match = string.match(input, '"content"%s*:%s*"((.-)[^\\])"')
    if content_match then prompt_content = content_match end
    
    -- Strip outer markers if it's still JSON-like (complex extraction fallback)
    if string.sub(prompt_content, 1, 1) == "[" then
        -- It's an array (multimodal), extraction needs to be smarter or we use the whole body for cache
    end

    switchai.log("Analysing request...")

    -- ============================================
    -- TIER 0: SEMANTIC CACHE (Fastest)
    -- ============================================
    local cached_result, cache_err = switchai.cache_lookup(prompt_content)
    if cached_result and cached_result.decision then
        switchai.log("Semantic Cache: HIT -> Model: " .. cached_result.decision)
        req.model = cached_result.decision
        req.metadata.routing_tier = "cache"
        return finalize_request(req)
    elseif cache_err then
        switchai.log("Semantic Cache: " .. cache_err)
    end

    -- ============================================
    -- TIER 1: REFLEX (Fast pattern matching)
    -- ============================================
    if has_visual_input(input) then
         switchai.log("Reflex: Visual Input detected -> Vision Model")
         req.model = get_model_for_capability("vision")
         req.metadata.routing_tier = "reflex"
         return finalize_request(req)
    end

    if has_pii(prompt_content) then
        switchai.log("Reflex: PII detected -> Secure Model")
        req.model = get_model_for_capability("secure")
        req.metadata.routing_tier = "reflex"
        return finalize_request(req)
    end

    if is_coding_request(prompt_content) then
        switchai.log("Reflex: Code detected -> Coding Model")
        req.model = get_model_for_capability("coding")
        req.metadata.routing_tier = "reflex"
        return finalize_request(req)
    end

    if #prompt_content > 4000 then
        switchai.log("Reflex: Long input -> Long Context Model")
        req.model = get_model_for_capability("long_ctx")
        req.metadata.routing_tier = "reflex"
        return finalize_request(req)
    end

    -- ============================================
    -- TIER 2: SEMANTIC (Embedding-based matching)
    -- ============================================
    local semantic_result, semantic_err = switchai.semantic_match_intent(prompt_content)
    if semantic_result and semantic_result.confidence >= config.semantic_threshold then
        local capability = intent_to_capability(semantic_result.intent)
        switchai.log(string.format("Semantic: %s (%.2f confidence, %dms)", 
            semantic_result.intent, semantic_result.confidence, semantic_result.latency_ms or 0))
        req.model = get_model_for_capability(capability)
        req.metadata.routing_tier = "semantic"
        req.metadata.semantic_intent = semantic_result.intent
        req.metadata.semantic_confidence = semantic_result.confidence
        
        -- Try skill matching after semantic intent matching
        local skill_result, skill_err = switchai.match_skill(prompt_content)
        if skill_result and skill_result.confidence >= 0.80 then
            switchai.log(string.format("Skill Match: %s (%.2f confidence)", 
                skill_result.skill.name, skill_result.confidence))
            
            -- Augment prompt with skill system prompt
            req.body = switchai.json_inject(req.body, skill_result.skill.system_prompt)
            
            -- Add skill metadata to request
            req.metadata.matched_skill = skill_result.skill.name
            req.metadata.matched_skill_id = skill_result.skill.id
            req.metadata.skill_confidence = skill_result.confidence
        elseif skill_err then
            switchai.log("Skill Match: " .. skill_err)
        end
        
        return finalize_request(req)
    end

    -- ============================================
    -- TIER 3: COGNITIVE (LLM-based classification)
    -- ============================================
    switchai.log("Cognitive: Engaging Router Model...")
    local json_res, err = switchai.classify(prompt_content)
    if err then
        switchai.log("Classification Error: " .. err .. " -> Fallback Fast")
        req.model = get_model_for_capability("fast")
        req.metadata.routing_tier = "cognitive"
        req.metadata.routing_error = err
        return finalize_request(req)
    end

    local decision, parse_err = switchai.parse_confidence(json_res)
    if not decision then
        switchai.log("Parse Error: " .. (parse_err or "unknown") .. " -> Fallback Fast")
        req.model = get_model_for_capability("fast")
        return finalize_request(req)
    end

    switchai.log(string.format("Cognitive: %s / %s (confidence: %.2f)", 
        decision.intent, decision.complexity, decision.confidence))
    
    req.metadata.routing_tier = "cognitive"
    req.metadata.cognitive_intent = decision.intent
    req.metadata.cognitive_complexity = decision.complexity
    req.metadata.cognitive_confidence = decision.confidence

    -- Low confidence handling (escalate to reasoning)
    if decision.confidence < 0.60 then
        switchai.log("Cognitive: Low confidence, escalating to reasoning model")
        req.model = get_model_for_capability("reasoning")
        return finalize_request(req)
    end

    -- ============================================
    -- VERIFICATION (Consensus Check)
    -- ============================================
    -- Compare Cognitive decision against Semantic signal (if available)
    if semantic_result and semantic_result.intent then
        -- Only verify if semantic signal has reasonable relevance (> 0.60) to avoid noise from weak semantic matches
        -- Note: If it was >= config.semantic_threshold (0.85), we would have already routed.
        if semantic_result.confidence > 0.60 then
            local is_match = switchai.verify_intent(semantic_result.intent, decision.intent)
            if not is_match then
                switchai.log(string.format("Verification: Mismatch detected! Semantic=%s (%.2f) vs Cognitive=%s (%.2f)", 
                    semantic_result.intent, semantic_result.confidence, decision.intent, decision.confidence))
                
                switchai.log("Verification: Escalating to Reasoning model due to intent mismatch.")
                req.model = get_model_for_capability("reasoning")
                req.metadata.verification_mismatch = true
                req.metadata.verification_action = "escalate"
                req.metadata.semantic_intent = semantic_result.intent
                return finalize_request(req)
            else
                switchai.log("Verification: Consensus confirmed.")
                req.metadata.verification_match = true
            end
        end
    end

    -- Route based on intent
    if decision.intent == "coding" then
        req.model = get_model_for_capability("coding")
    elseif decision.intent == "image_generation" then
        req.model = get_model_for_capability("image_gen")
        req.metadata.operation = "images_generations"
    elseif decision.intent == "audio_transcription" then
        req.model = get_model_for_capability("transcription")
        req.metadata.operation = "audio_transcriptions"
    elseif decision.intent == "audio_speech" then
        req.model = get_model_for_capability("speech")
        req.metadata.operation = "audio_speech"
    elseif decision.intent == "reasoning" and decision.complexity == "complex" then
        req.model = get_model_for_capability("reasoning")
    elseif decision.intent == "creative" then
        req.model = get_model_for_capability("creative")
    elseif decision.intent == "research" then
        req.model = get_model_for_capability("research")
    else
        req.model = get_model_for_capability("fast")
    end

    -- Final routing decision
    switchai.log("Routing to: " .. req.model)
    
    -- Record routing decision metadata for feedback
    req.metadata.routing_start_time = os.time()
    
    return finalize_request(req)
end

--[[
================================================================================
ON_RESPONSE HOOK: Quality-based Cascading & Feedback
================================================================================

Plugin:on_response(res)

Post-response handler. Evaluates response quality and records feedback.

Parameters:
  res: table - The response object with fields:
    - body: string - The response body
    - model: string - The model that generated the response
    - metadata: table - Routing metadata from on_request
    - original_query: string - The original query (if available)
    - error: string - Error message (if any)

Returns:
  table - Modified response with cascade metadata, or nil

Features:
  1. Feedback Recording: Records routing decisions for learning
  2. Quality Evaluation: Detects quality signals in responses
  3. Cascade Triggering: Signals need for model escalation

Cascade Tiers:
  - fast: Quick responses, may have quality issues
  - standard: Balanced quality and speed
  - reasoning: High-quality, thorough responses

Quality Signals Detected:
  - Abrupt endings
  - Missing sections
  - Incomplete responses
  - Error patterns
--]]
function Plugin:on_response(res)
    -- Record feedback for this routing decision
    local metadata = res.metadata or {}
    local routing_tier = metadata.routing_tier or "unknown"
    local selected_model = res.model or "unknown"
    
    -- Calculate latency if start time was recorded
    local latency_ms = 0
    if metadata.routing_start_time then
        latency_ms = (os.time() - metadata.routing_start_time) * 1000
    end
    
    -- Determine intent from metadata
    local intent = metadata.semantic_intent or metadata.cognitive_intent or "unknown"
    
    -- Determine confidence
    local confidence = metadata.semantic_confidence or metadata.cognitive_confidence or 0.0
    
    -- Determine if cascade occurred
    local cascade_occurred = metadata.cascade_needed or false
    
    -- Determine success (assume success if we got a response)
    local success = res.body ~= nil and res.body ~= ""
    
    -- Build feedback record
    local feedback_data = {
        query = res.original_query or "",
        intent = intent,
        selected_model = selected_model,
        routing_tier = routing_tier,
        confidence = confidence,
        matched_skill = metadata.matched_skill or "",
        cascade_occurred = cascade_occurred,
        response_quality = metadata.cascade_quality_score or 0.0,
        latency_ms = latency_ms,
        success = success,
        error_message = res.error or "",
        metadata = {
            complexity = metadata.cognitive_complexity or "",
            verification_match = metadata.verification_match or false,
            verification_mismatch = metadata.verification_mismatch or false
        }
    }
    
    -- Record feedback (non-blocking, errors are logged)
    local _, feedback_err = switchai.record_feedback(feedback_data)
    if feedback_err then
        switchai.log("Feedback: " .. feedback_err)
    end
    
    -- 2. Store in Semantic Cache for tier-based decisions
    if routing_tier == "reflex" or routing_tier == "cognitive" or routing_tier == "semantic" then
        local decision = selected_model
        if res.provider then
             decision = res.provider .. ":" .. res.model
        end
        
        local cache_key = res.original_query or ""
        local content_match = string.match(cache_key, '"content"%s*:%s*"((.-)[^\\])"')
        if content_match then cache_key = content_match end

        switchai.log("Semantic Cache: Storing decision for " .. decision .. " with key: " .. cache_key)
        switchai.cache_store(cache_key, decision, {
            intent = intent,
            tier = routing_tier
        })
    end

    -- Skip cascade evaluation if not applicable
    if not res or not res.body then
        return nil
    end
    
    -- Only cascade for cognitive tier responses (semantic and reflex are already optimized)
    if routing_tier ~= "cognitive" then
        return nil
    end

    -- Get current capability/tier from the model used
    local current_tier = metadata.cascade_tier or "standard"
    
    -- Evaluate response quality
    local evaluation, err = switchai.evaluate_response(res.body, current_tier)
    if err then
        switchai.log("Cascade: " .. err)
        return nil
    end
    
    if not evaluation then
        return nil
    end

    -- Log quality assessment
    switchai.log(string.format("Cascade: Quality=%.2f, Tier=%s", 
        evaluation.quality_score, evaluation.current_tier))

    -- Check if cascade is needed
    if evaluation.should_cascade then
        switchai.log(string.format("Cascade: Triggering escalation to %s - %s", 
            evaluation.next_tier, evaluation.reason))
        
        -- Signal cascade needed
        res.metadata = res.metadata or {}
        res.metadata.cascade_needed = true
        res.metadata.cascade_next_tier = evaluation.next_tier
        res.metadata.cascade_reason = evaluation.reason
        res.metadata.cascade_quality_score = evaluation.quality_score
        
        -- Log detected signals
        if evaluation.signals then
            for _, signal in ipairs(evaluation.signals) do
                switchai.log(string.format("Cascade Signal: %s (%.2f) - %s", 
                    signal.type, signal.severity, signal.description))
            end
        end
    else
        switchai.log("Cascade: Response quality acceptable")
    end

    return res
end

return Plugin
