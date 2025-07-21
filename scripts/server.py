#!/usr/bin/env python3
"""Example MCP server using the fastmcp library."""

import asyncio
import logging
from fastmcp import FastMCP


logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


server = FastMCP(name="demo")


@server.tool
def hello(name: str = "World") -> str:
    """Say hello to someone."""

    return f"Hello, {name}!"


@server.tool
async def call_stream(count: int = 5, delay: float = 1):
    """Stream count messages from 1 to n."""

    for i in range(count):
        yield f"Chunk {i + 1} of {count}"
        await asyncio.sleep(delay)
    return "Stream complete"


if __name__ == "__main__":
    logger.info("Starting FastMCP stdio server")
    server.run()
