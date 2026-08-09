local Schema = require("schema")

local Plugin = {}

-- The Cortex generic answer prompt (packages/generic) asks the model to emit a
-- "classification_code" but never lists the allowed values, while the gateway
-- enforces profile.answer_schema()["properties"]["classification"]["enum"].
-- Without the vocabulary the model guesses (e.g. "FACT"), validation rejects it,
-- and the regeneration loop blows the 10s client read-lease. We supply the
-- vocabulary here, scoped strictly to requests that already carry the marker.
local STEER = table.concat({
    "Reasoning: low. ",
    "classification_code MUST be exactly one of ",
    "[\"SUPPORTED\",\"PARTIALLY_SUPPORTED\",\"CONFLICTING\",\"ABSTAINED\"]. ",
    "Use SUPPORTED when the evidence supports the answer; CONFLICTING when the ",
    "evidence conflicts; PARTIALLY_SUPPORTED when only partly supported. ",
    "When the evidence is insufficient to answer, abstain by returning exactly ",
    "{\"statements\": []} with no statement objects \226\128\148 never emit a statement ",
    "with an empty citation_ids list (each statement needs at least one citation). ",
    "Output JSON only \226\128\148 no analysis, no prose, no markdown.",
})

-- gpt-oss reasons verbosely by default (~600 tokens even for a one-line answer),
-- which pushes the generate+verify chain past the 10s client read-lease on a
-- cloud provider. "Reasoning: low" is the harmony control token; it cuts output
-- to ~300 tokens with no loss of correctness on these structured tasks.
local LEAN = "Reasoning: low. Output JSON only \226\128\148 no analysis, no prose, no markdown."

function Plugin:on_request(req)
    if not req.body then return req end
    -- Answer generation: supply the missing classification vocabulary.
    if string.find(req.body, "classification_code", 1, true) then
        req.body = switchai.json_inject(req.body, STEER)
    -- Grounding / verifier passes: just trim the reasoning budget.
    elseif string.find(req.body, "material statement", 1, true)
        or string.find(req.body, "Identify contradictions", 1, true) then
        req.body = switchai.json_inject(req.body, LEAN)
    end
    return req
end

return Plugin
