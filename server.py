#!/usr/bin/env python3
import sys
import json
import logging

# — send all logs to stderr
logging.basicConfig(
    stream=sys.stderr,
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s"
)
log = logging.getLogger(__name__)

# — describe your tools
TOOLS = [
    {
        "name": "hello",
        "description": "Say hello to someone",
        "inputSchema": {
            "type": "object",
            "properties": {
                "name": {
                    "type": "string",
                    "description": "Name to greet",
                    "default": "World"
                }
            }
        }
    }
]

def send_response(resp: dict):
    # exactly one JSON object per line on stdout
    sys.stdout.write(json.dumps(resp, separators=(",", ":")) + "\n")
    sys.stdout.flush()

def handle_initialize(req: dict):
    log.info("initialize()")
    return {"jsonrpc":"2.0","id":req["id"],"result":None}

def handle_tools_list(req: dict):
    log.info("tools/list()")
    return {"jsonrpc":"2.0","id":req["id"],"result":{"tools":TOOLS}}

def handle_call(req: dict):
    params = req.get("params", {})
    name = params.get("name")
    args = params.get("arguments", {}) or {}
    log.info("tools/call name=%s args=%r", name, args)

    if name == "hello":
        who = args.get("name", "World")
        return {
            "jsonrpc":"2.0",
            "id":req["id"],
            "result": {"result": f"Hello, {who}!"}
        }
    else:
        return {
            "jsonrpc":"2.0",
            "id":req["id"],
            "error":{"code":-32601,"message":"Tool not found"}
        }

def main():
    log.info("MCP stdio server starting")
    for raw in sys.stdin:
        line = raw.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            log.error("invalid JSON: %r", line)
            continue

        method = req.get("method")
        if method == "initialize":
            resp = handle_initialize(req)
        elif method == "tools/list":
            resp = handle_tools_list(req)
        elif method == "tools/call":
            resp = handle_call(req)
        else:
            log.warning("unknown method %r", method)
            resp = {
                "jsonrpc":"2.0",
                "id":req.get("id"),
                "error":{"code":-32601,"message":f"Unknown method {method}"}
            }
        send_response(resp)

if __name__ == "__main__":
    main()
