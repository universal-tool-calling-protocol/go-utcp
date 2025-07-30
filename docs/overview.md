# Project Overview

The **go-utcp** library implements the Universal Tool Calling Protocol (UTCP) in Go.
It provides a client that can interact with many types of tool providers such as
HTTP servers, command line programs, gRPC services and more.

UTCP defines a simple format for describing tools that can be invoked through a
variety of transports. Providers expose a "manual" with a list of tools and their
parameters. The client loads these manuals and calls the tools using the
appropriate transport.

The library includes:

- Client and transport implementations
- Provider implementations for common protocols
- Utilities for converting OpenAPI specs to UTCP manuals
- In-memory repository and search strategy for discovered tools
- Example programs demonstrating different transports

For more details on how to use the client see the README and the example
programs under the `examples/` directory.
