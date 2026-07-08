# AgentNativeDB

Agent-native database with SQL query engine, vector search (HNSW), graph store, and MCP server integration. Each wheel includes a platform-native `andb` binary.

## Installation

```bash
pip install andb-installer
```

Or with an isolated environment:

```bash
pipx install andb-installer
```

After installation, the `andb` command is available on your `PATH`.

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

-- Vector search
CREATE TABLE embeddings (id VARCHAR(64) PRIMARY KEY, embedding FLOAT);
CREATE VECTOR INDEX idx_emb ON embeddings(embedding) WITH (dimensions=128);
SELECT * FROM vector_search(idx_emb, '[0.1, 0.2, ...]', 10);
```

## Supported Platforms

- Linux x86_64, arm64
- macOS x86_64, arm64
- Windows x64

## Links

- **Homepage** — <https://github.com/startvibecoding/AgentNativeDB>
- **Documentation** — <https://github.com/startvibecoding/AgentNativeDB/tree/main/docs>
- **Issues** — <https://github.com/startvibecoding/AgentNativeDB/issues>
- **License** — MIT

## Uninstall

```bash
pip uninstall andb-installer

# or if you used pipx:
pipx uninstall andb-installer
```
