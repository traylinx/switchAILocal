-- handler.lua for sebastian-interceptor
local Schema = require("schema")

local Plugin = {}

function Plugin:on_request(req)
    switchai.log("Intercepting: " .. (req.model or "unknown"))

    if req.body then
        -- Simple pattern match to replace "content": "..."
        local new_body = string.gsub(req.body, '("content":%s*")[^"]*', '%1Hi i\'m Sebastian')
        
        if new_body ~= req.body then
            req.body = new_body
            switchai.log("Modified body successfully!")
            return req
        end
    end
    return nil
end

return Plugin
