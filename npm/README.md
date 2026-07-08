# andb-installer

AgentNativeDB — Agent-native database with SQL query engine, vector search (HNSW), graph store, and MCP server integration.

## Installation

```bash
npm install -g andb-installer
```

## Quick Start

```bash
# Start HTTP API server (default: 0.0.0.0:8400)
andb server

# Start MCP server (stdio transport for Cursor/Claude Desktop)
andb server -mode mcp

# Interactive SQL REPL (local database)
andb cli

# Connect to remote server
andb client -server localhost:8400
```

## Features

- 🤖 **Agent-Native Schema** — Sessions, memories, decisions as first-class data types
- ⚡ **SQL Query Engine** — SELECT/INSERT/UPDATE/DELETE, WHERE, JOIN, GROUP BY, ORDER BY, LIMIT, aggregations
- 🔍 **Vector Search** — Custom HNSW index with cosine/L2/dot-product distance
- 🕸️ **Graph Store** — Adjacency-list with BFS, K-hop, shortest-path queries
- 📊 **Data Lineage** — Track data provenance through the knowledge layer
- 🔗 **MCP Server** — Model Context Protocol (stdio) for Cursor, Claude Desktop
- 🌐 **HTTP API** — RESTful endpoints for sessions, memories, decisions, SQL queries
- 💻 **Interactive CLI** — Local SQL REPL with syntax highlighting
- 🖥️ **Web UI** — Svelte dashboard embedded into the binary
- 📦 **Pure Go** — Single binary, zero CGO, only BadgerDB as dependency

## SQL Examples

```sql
-- Create agent session table
CREATE TABLE agent_sessions (
  id VARCHAR(64) PRIMARY KEY,
  agent_id VARCHAR(64),
  state VARCHAR(32)
);

-- Query with JOIN
SELECT s.agent_id, m.content
FROM agent_sessions s
JOIN agent_memories m ON s.id = m.session_id;

-- Full-text search
CREATE FULLTEXT INDEX idx_body ON docs(body);
SELECT * FROM docs WHERE MATCH(body) AGAINST ('agent memory');
```

## More Information

- **GitHub**: [github.com/startvibecoding/AgentNativeDB](https://github.com/startvibecoding/AgentNativeDB)
- **Documentation**: [docs](https://github.com/startvibecoding/AgentNativeDB/tree/main/docs)

## Uninstall

```bash
npm uninstall -g andb-installer
```

## License

MIT
