# fast_mcp_server.py
from fastmcp import FastMCP
import time

mcp = FastMCP("Demo 🚀")

@mcp.tool()
def hello(name: str) -> str:
    return f"Hello, {name}!"

@mcp.tool()
def handle_call_stream(count: int = 5, delay: float = 1.0):
    for i in range(count):
        yield f"Chunk {i+1} of {count}"
        time.sleep(delay)
    yield "Stream complete"

if __name__ == "__main__":
    mcp.run(
        transport="http",  # <- This is key
        host="127.0.0.1",
        port=8002,
        path="/mcp"
    )
