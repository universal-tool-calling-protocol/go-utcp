#!/usr/bin/env python3
import sys
import json
import logging
from typing import Any, Dict, List

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
    }
]

# — helper: send JSON-RPC response
def send_response(resp: Dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(resp, separators=(",", ":")) + "\n")
    sys.stdout.flush()

# — capabilities handshake
def handle_initialize(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("initialize()")
    result = {
        "mcp_version": "1.0",
        "capabilities": {
            "chat": True,
            "streaming": True,
            "tool_calls": True,
            "shutdown": True
        }
    }
    return {"jsonrpc": "2.0", "id": req["id"], "result": result}

# — return available tools
def handle_tools_list(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("tools/list()")
    return {"jsonrpc": "2.0", "id": req["id"], "result": {"tools": TOOLS}}

# — call a tool synchronously
def handle_tool_call(req: Dict[str, Any]) -> Dict[str, Any]:
    params = req.get("params", {}) or {}
    name = params.get("name")
    args = params.get("arguments", {}) or {}
    log.info("tools/call name=%s args=%r", name, args)

    if name == "hello":
        who = args.get("name", "World")
        # Return under 'result' key to match MCP consumer expectations
        return {"jsonrpc": "2.0", "id": req["id"], "result": {"result": f"Hello, {who}!"}}
    else:
        return {
            "jsonrpc": "2.0",
            "id": req["id"],
            "error": {"code": -32601, "message": "Tool not found"}
        }

# — chat interaction handler
def handle_chat_send(req: Dict[str, Any]) -> Dict[str, Any]:
    params = req.get("params", {}) or {}
    context = params.get("context", {})
    message = params.get("message", {})
    log.info("chat/send context=%r message=%r", context, message)

    # stub: echo back
    response = {
        "role": "assistant",
        "content": f"Echo: {message.get('content', '')}"
    }
    return {"jsonrpc": "2.0", "id": req["id"], "result": {"message": response}}

# — graceful shutdown
def handle_shutdown(req: Dict[str, Any]) -> Dict[str, Any]:
    log.info("shutdown() requested")
    return {"jsonrpc": "2.0", "id": req.get("id"), "result": None}

# — dispatcher
METHOD_MAP = {
    "initialize": handle_initialize,
    "tools/list": handle_tools_list,
    "tools/call": handle_tool_call,
    "chat/send": handle_chat_send,
    "shutdown": handle_shutdown
}

# — main loop
if __name__ == "__main__":
    log.info("MCP stdio server starting (v1.0)")
    for raw in sys.stdin:
        line = raw.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            method = req.get("method")
            handler = METHOD_MAP.get(method)
            if handler:
                resp = handler(req)
            else:
                log.warning("unknown method %r", method)
                resp = {"jsonrpc": "2.0", "id": req.get("id"), "error": {"code": -32601, "message": f"Unknown method {method}"}}
        except json.JSONDecodeError:
            log.error("invalid JSON: %r", line)
            continue
        send_response(resp)
