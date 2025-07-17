# UTCP

Universal Tool Calling Protocol (UTCP) reference implementation.

UTCP is an attempt to standardise how a client can discover and call
"tools" (or APIs) no matter what transport they live behind.  Each
provider describes a transport (HTTP, CLI, GraphQL and so on) and a set
of tools exposed by that transport.  This repository contains a small
Go client that can load provider definitions, register them at runtime
and then invoke the discovered tools.
