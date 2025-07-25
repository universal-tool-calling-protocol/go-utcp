#!/usr/bin/env python3
import sys
import json
import logging
import time
from typing import Any, Dict, List, Optional

# — send all logs to stderr
logging.basicConfig(
    stream=sys.stderr,
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s"
)
log = logging.getLogger(__name__)

# — tool registry
TOOLS: List[Dict[str, Any]] = [
    {
        "name": "hello",
        "description": "Say hello to someone",
        "input_schema": {
            "type": "object",
            "properties": {
                "name": {"type": "string", "description": "Name to greet", "default": "World"}
            },
            "required": []
        }
    },
    {
        "name": "call_stream",
        "description": "Stream count messages from 1 to n",
        "input_schema": {
            "type": "object",
            "properties": {
                "count": {"type": "integer", "description": "Number of messages to stream", "default": 5},
                "delay": {"type": "number", "description": "Delay in seconds between messages", "default": 1}
            },
            "required": []
        }
    }
]

# — helper: send JSON-RPC response

def send_response(resp: Dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(resp, separators=(",", ":")) + "\n")
    sys.stdout.flush()

# — capabilities handshake
def handle_initialize(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("initialize()")
    result = {"mcp_version": "1.0", "capabilities": {"chat": True, "streaming": True, "tool_calls": True, "shutdown": True}}
    return {"jsonrpc": "2.0", "id": req["id"], "result": result}

# — return available tools
def handle_tools_list(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("tools/list()")
    return {"jsonrpc": "2.0", "id": req["id"], "result": {"tools": TOOLS}}

def send_line(obj):
    print(json.dumps(obj), flush=True)

def handle_call_stream(req):
    args = req.get("params", {}).get("arguments", {})
    count = int(args.get("count", 5))
    delay = float(args.get("delay", 1))

    # Send streaming chunks as notifications (no "id")
    for i in range(count):
        send_line({
            "jsonrpc": "2.0",
            "method": "chunk",
            "params": {
                "result": f"Chunk {i+1} of {count}"
            }
        })
        time.sleep(delay)

    # Final result (only one with "id")
    send_line({
        "jsonrpc": "2.0",
        "id": req["id"],
        "result": {
            "result": "Stream complete"
        }
    })

# — call a tool synchronously or streaming
def handle_tool_call(req: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    params = req.get("params", {}) or {}
    name = params.get("name")
    args = params.get("arguments", {}) or {}
    log.info("tools/call name=%s args=%r", name, args)

    if name == "hello":
        who = args.get("name", "World")
        return {"jsonrpc": "2.0", "id": req["id"], "result": {"result": f"Hello, {who}!"}}

    elif name == "call_stream":
        handle_call_stream(req)

    else:
        return {"jsonrpc": "2.0", "id": req["id"], "error": {"code": -32601, "message": "Tool not found"}}

# — chat interaction handler
def handle_chat_send(req: Dict[str, Any]) -> Dict[str, Any]:
    message = req.get("params", {}).get("message", {})
    log.info("chat/send message=%r", message)
    response = {"role": "assistant", "content": f"Echo: {message.get('content', '')}"}
    return {"jsonrpc": "2.0", "id": req["id"], "result": {"message": response}}

# — graceful shutdown
def handle_shutdown(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("shutdown() requested")
    return {"jsonrpc": "2.0", "id": req.get("id"), "result": None}

METHOD_MAP = {"initialize": handle_initialize, "tools/list": handle_tools_list, "tools/call": handle_tool_call, "chat/send": handle_chat_send, "shutdown": handle_shutdown}

if __name__ == "__main__":
    log.info("MCP stdio server starting with streaming tool support")
    for raw in sys.stdin:
        line = raw.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            handler = METHOD_MAP.get(req.get("method"))
            resp = handler(req) if handler else {"jsonrpc": "2.0", "id": req.get("id"), "error": {"code": -32601, "message": f"Unknown method {req.get('method')}"}}
        except json.JSONDecodeError:
            log.error("invalid JSON: %r", line)
            continue
        if resp is not None:
            send_response(resp)
