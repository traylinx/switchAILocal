"""
Bridge Executor Example
Demonstrates how to use the switchAILocal WebSocket Relay API to execute commands remotely.

This is the TRUE Bridge Agent pattern - executing tools via WebSocket, not direct subprocess.
"""

import asyncio
import json
import uuid
import sys
import websockets

# Configuration
WS_URL = "ws://localhost:8081/ws"
PROVIDER = "bridge"  # The provider identifier for bridge agent sessions

async def execute_http_request(method: str, url: str, body: dict = None):
    """
    Sends an HTTP request through the switchAILocal WebSocket relay.
    
    This demonstrates how agentic clients can use switchAILocal as a unified
    gateway - sending requests through WebSocket instead of direct HTTP.
    """
    request_id = str(uuid.uuid4())
    
    message = {
        "id": request_id,
        "type": "http_request",
        "payload": {
            "method": method,
            "url": url,
            "headers": {"Content-Type": ["application/json"]},
            "body": json.dumps(body) if body else ""
        }
    }
    
    print(f"🔌 Connecting to {WS_URL}...")
    
    try:
        async with websockets.connect(WS_URL) as ws:
            print(f"✅ Connected! Sending request to {url}...")
            
            # Send the request
            await ws.send(json.dumps(message))
            
            # Wait for response (handle streaming or single response)
            while True:
                response_raw = await asyncio.wait_for(ws.recv(), timeout=30)
                response = json.loads(response_raw)
                
                msg_type = response.get("type")
                msg_id = response.get("id")
                payload = response.get("payload", {})
                
                if msg_type == "http_response":
                    print(f"📨 Response (Status: {payload.get('status', 'N/A')}):")
                    body = payload.get("body", "")
                    try:
                        print(json.dumps(json.loads(body), indent=2))
                    except:
                        print(body)
                    return payload
                    
                elif msg_type == "stream_start":
                    print(f"📡 Stream started (Status: {payload.get('status', 200)})")
                    
                elif msg_type == "stream_chunk":
                    chunk = payload.get("data", "")
                    print(chunk, end="", flush=True)
                    
                elif msg_type == "stream_end":
                    print("\n📡 Stream ended.")
                    return None
                    
                elif msg_type == "error":
                    print(f"❌ Error: {payload.get('error', 'Unknown error')}")
                    return None
                    
                elif msg_type == "pong":
                    # Heartbeat response, ignore
                    continue
                    
                else:
                    print(f"⚠️ Unknown message type: {msg_type}")
                    
    except websockets.exceptions.ConnectionClosed as e:
        print(f"❌ Connection closed: {e}")
    except asyncio.TimeoutError:
        print("❌ Timeout waiting for response")
    except Exception as e:
        print(f"❌ Error: {e}")

async def main():
    if len(sys.argv) < 2:
        print("Bridge Executor Example")
        print("=" * 40)
        print("\nUsage:")
        print("  python executor.py chat \"Hello, world!\"")
        print("  python executor.py models")
        print("\nThis demonstrates the WebSocket Relay API.")
        sys.exit(0)
    
    command = sys.argv[1]
    
    if command == "models":
        # List available models via WebSocket
        await execute_http_request("GET", "/v1/models")
        
    elif command == "chat":
        if len(sys.argv) < 3:
            print("Usage: python executor.py chat \"Your message\"")
            sys.exit(1)
            
        prompt = sys.argv[2]
        body = {
            "model": "gemini-2.5-flash",
            "messages": [{"role": "user", "content": prompt}],
            "stream": False
        }
        await execute_http_request("POST", "/v1/chat/completions", body)
        
    else:
        print(f"Unknown command: {command}")
        print("Available commands: models, chat")

if __name__ == "__main__":
    asyncio.run(main())
