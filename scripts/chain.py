# server.py
from fastmcp import FastMCP

mcp = FastMCP("Demo MCP Server")

@mcp.tool()
def hello(name: str) -> dict:
    """Returns a greeting message for a given name."""
    return {"message": f"Hello, {name}!"}

@mcp.tool()
def process(message: str) -> dict:
    """Transforms input text to uppercase."""
    return {"result": message.upper()}

if __name__ == "__main__":
    mcp.run(transport="http", host="0.0.0.0", port=8002)
