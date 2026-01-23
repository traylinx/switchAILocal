-- Example LUA plugin for switchAILocal
-- This plugin demonstrates request/response modification capabilities

-- on_request is called before the request is sent to the provider
-- It receives a table with request data and can return modified data
function on_request(req)
    -- Add custom header
    if req.headers == nil then
        req.headers = {}
    end
    req.headers["X-LUA-Plugin"] = "enabled"
    req.headers["X-Request-Modified-At"] = os.date("%Y-%m-%d %H:%M:%S")
    
    -- Log the model being used (for debugging)
    if req.model then
        print("[LUA Plugin] Processing request for model: " .. req.model)
    end
    
    return req
end

-- on_response is called after receiving the response from the provider
-- It can modify the response before it's returned to the client
function on_response(resp)
    -- Add metadata about plugin processing
    if resp.metadata == nil then
        resp.metadata = {}
    end
    resp.metadata["lua_processed"] = true
    
    return resp
end

-- Helper function to log messages
function log_info(message)
    print("[LUA Plugin INFO] " .. message)
end
