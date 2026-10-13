Enhancement: MCP server for AI clients

A new `mcp` HTTP service serves the user's files to AI clients over the Model Context Protocol, authenticated with app passwords of the new `mcp` client kind, which may be restricted to root folders.
It offers the tools the app password's permissions allow, filters listings by its roots, rate-limits calls and logs each one.
