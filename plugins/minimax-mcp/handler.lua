local Schema = require("schema")

local Plugin = {}

function Plugin:on_request(req)
    switchai.log("minimax-mcp hook triggered! Model=" .. tostring(req.model) .. " Provider=" .. tostring(req.provider))
    
    if req.provider ~= "minimax" and not string.match(string.lower(req.model or ""), "minimax") then
        switchai.log("Skipping minimax-mcp, not minimax provider/model")
        return nil
    end
    
    if not req.body then
        switchai.log("Skipping minimax-mcp, no body")
        return nil
    end

    local api_key = req.api_key
    if not api_key or api_key == "" then
        switchai.log("minimax-mcp error: MINIMAX_API_KEY could not be found via AIL AuthManager!")
        return nil
    end

    local env = {
        ["MINIMAX_API_KEY"] = api_key,
        ["MINIMAX_API_HOST"] = "https://api.minimax.io"
    }

    local prompt = "Please describe this image comprehensively."
    
    local new_body, err = switchai.minimax_native_vlm_extract(
        req.body, 
        api_key,
        "https://api.minimax.io",
        prompt
    )

    if err then
        switchai.log("VLM native extraction heavily failed: " .. err)
        return nil
    end

    if new_body then
        switchai.log("Intercepted Minimax Image! Generated OCR context natively.")
        req.body = new_body
        return req
    end

    return nil
end

return Plugin
