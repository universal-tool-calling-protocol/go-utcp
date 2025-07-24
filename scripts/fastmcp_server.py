from fastmcp import FastMCP
mcp = FastMCP("Demo")

@mcp.tool
def hello(name: str="World"):
    return f"Hello, {name}!"

if __name__ == "__main__":
    mcp.run(transport="http", host="127.0.0.1", port=8008, path="/mcp")
