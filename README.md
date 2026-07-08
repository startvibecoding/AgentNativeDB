<p align="center">
  <img src="docs/assets/logo.png" alt="AgentNativeDB" width="128" height="128">
</p>

<h1 align="center">AgentNativeDB</h1>

<p align="center">
  <strong>🤖 Agent-Native Database — Sessions, Memory, Decisions as First-Class Citizens</strong>
</p>

<p align="center">
  A purpose-built database system designed from the ground up for AI agents.<br>
  Combines a SQL query engine, vector search (HNSW), graph store, knowledge/lineage tracking, and MCP server integration — all in a single Go binary.
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/andb-installer"><img src="https://img.shields.io/npm/dm/andb-installer.svg" alt="npm downloads"></a>
  <a href="https://pypi.org/project/andb-installer/"><img src="https://img.shields.io/pypi/v/andb-installer.svg" alt="PyPI version"></a>
  <a href="https://github.com/startvibecoding/AgentNativeDB/releases/latest"><img src="https://img.shields.io/github/release/startvibecoding/AgentNativeDB.svg" alt="GitHub release"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://goreportcard.com/report/github.com/startvibecoding/AgentNativeDB"><img src="https://goreportcard.com/badge/github.com/startvibecoding/AgentNativeDB" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/startvibecoding/AgentNativeDB"><img src="https://pkg.go.dev/badge/github.com/startvibecoding/AgentNativeDB?status.svg" alt="GoDoc"></a>
</p>

---

## ✨ Why AgentNativeDB?

**The Problem:** AI agents need persistent memory, structured data, vector search, and knowledge graphs — but stitching together multiple databases is fragile and complex.

**The Solution:** AgentNativeDB is the **agent-native database** that combines everything into a single binary. Sessions, memories, and decisions are first-class data types, not afterthoughts.

### 🎯 Key Highlights

| Feature | What It Means for You |
|---------|----------------------|
| **🤖 Agent-Native Schema** | Sessions, memories, decisions, and tasks are built-in table types with optimized storage |
| **⚡ SQL Query Engine** | Hand-written lexer → parser → planner → executor — SELECT/INSERT/UPDATE/DELETE, WHERE, JOIN, GROUP BY, ORDER BY, LIMIT, aggregations |
| **🔍 Vector Search** | Custom HNSW index with cosine/L2/dot-product distance — no third-party vector libraries |
| **🕸️ Graph Store** | Adjacency-list storage with BFS, K-hop, shortest-path queries — all persisted on BadgerDB |
| **📊 Data Lineage** | Track data provenance and transformation history through the knowledge layer |
| **🔗 MCP Server** | Model Context Protocol (stdio) — plug into Cursor, Claude Desktop, or any MCP-compatible client |
| **🌐 HTTP API** | RESTful endpoints for sessions, memories, decisions, and SQL queries |
| **💻 Interactive CLI** | Local SQL REPL with syntax highlighting, command history, and auto-completion |
| **🖥️ Web UI** | Svelte-based dashboard embedded into the binary — table management, data visualization, SQL editor |
| **📦 Pure Go** | Single binary, zero CGO, no external runtime dependencies — only BadgerDB (pure Go KV store) |

---

## 🚀 Get Started in 30 Seconds

```bash
# Install (pick one)
npm install -g andb-installer               # npm
pip install andb-installer                  # PyPI / pipx
go install github.com/startvibecoding/AgentNativeDB/cmd/andb@latest  # Go

# Build from source
git clone https://github.com/startvibecoding/AgentNativeDB.git
cd AgentNativeDB
make build
```

**Supported Platforms:** Linux (x86_64, arm64), macOS (x86_64, arm64), Windows (x86_64)

### Run

```bash
# HTTP server (default: 0.0.0.0:8400)
./bin/andb server

# MCP server (stdio transport)
./bin/andb server -mode mcp

# Interactive SQL CLI (local)
./bin/andb cli

# HTTP client (connect to remote server)
./bin/andb client -server localhost:8400

# Version
./bin/andb version
```

**Uninstall:**

```bash
npm uninstall -g andb-installer
pip uninstall andb-installer
```

---

## 🎮 SQL Query Examples

```sql
-- Create tables
CREATE TABLE agent_sessions (
  id VARCHAR(64) PRIMARY KEY,
  agent_id VARCHAR(64),
  state VARCHAR(32),
  created_at INTEGER
);

CREATE TABLE agent_memories (
  id VARCHAR(64) PRIMARY KEY,
  session_id VARCHAR(64),
  content TEXT,
  importance FLOAT
);

-- Basic queries
SELECT * FROM agent_sessions WHERE state = 'active';

-- Aggregation
SELECT agent_id, COUNT(*) as cnt FROM agent_sessions GROUP BY agent_id;

-- JOIN
SELECT s.agent_id, m.content
FROM agent_sessions s
JOIN agent_memories m ON s.id = m.session_id
WHERE m.importance > 0.7;

-- Sorting and pagination
SELECT * FROM agent_memories ORDER BY importance DESC LIMIT 10;

-- Full-text search (INVERTED index)
CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT);
CREATE FULLTEXT INDEX idx_docs_body ON docs(body);
SELECT * FROM docs WHERE MATCH(body) AGAINST ('agent memory');

-- Vector search
CREATE TABLE embeddings (id VARCHAR(64) PRIMARY KEY, embedding FLOAT);
CREATE VECTOR INDEX idx_emb ON embeddings(embedding) WITH (dimensions=128);
SELECT * FROM vector_search(idx_emb, '[0.1, 0.2, ...]', 10);

-- Graph queries
CREATE GRAPH TABLE edges (src VARCHAR(64), dst VARCHAR(64));
SELECT * FROM graph_bfs(edges, 'node_001', 3);
SELECT * FROM graph_shortest_path(edges, 'node_001', 'node_010');
```

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────┐
│                  API Layer                       │
│   HTTP REST  │  MCP Server  │  CLI  │  Web UI   │
├─────────────────────────────────────────────────┤
│              Agent Runtime                       │
│  Session │ Memory │ Decision │ Coordinator │ Audit│
├─────────────────────────────────────────────────┤
│             Unified Query Layer                  │
│   SQL Engine  │  Graph Query  │  Vector Search   │
├─────────────────────────────────────────────────┤
│              Storage Engine                      │
│   BadgerDB  │  HNSW Index  │  Graph Store       │
└─────────────────────────────────────────────────┘
```

### Project Structure

```
AgentNativeDB/
├── cmd/andb/                 # Single entry point (server, cli, client, version)
├── api/
│   ├── http/                 # RESTful HTTP API
│   └── mcp/                  # MCP Server (stdio transport)
├── config/                   # Configuration management
├── internal/
│   ├── storage/              # Storage engine abstraction + LRU cache
│   ├── storage/badger/       # BadgerDB implementation
│   ├── model/                # Core data types (Session, Memory, Decision, Entity)
│   ├── agent/                # Agent runtime (session, memory, decision, RAG, audit)
│   ├── query/sql/            # SQL engine (lexer → parser → planner → executor)
│   ├── query/sql/index/      # Secondary indexes (Hash, BTree, Inverted/FullText)
│   ├── query/graph/          # Graph query surface
│   ├── query/vector/         # Vector query surface
│   ├── vector/               # HNSW vector index
│   ├── graph/                # Graph store (adjacency list, BFS, K-hop)
│   ├── knowledge/            # Data lineage tracking
│   └── util/                 # UUID v7 generation
├── sdk/                      # Go SDK
├── ui/                       # Svelte + Vite Web UI (embedded into binary)
├── docs/                     # Design document
└── examples/                 # Example scripts
```

---

## 📚 API Reference

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/sessions` | Create session |
| GET | `/api/v1/sessions/{id}` | Get session |
| PATCH | `/api/v1/sessions/{id}` | Update session |
| DELETE | `/api/v1/sessions/{id}` | Delete session |
| GET | `/api/v1/sessions` | List sessions |
| POST | `/api/v1/memories` | Store memory |
| GET | `/api/v1/memories?session_id=` | List memories |
| POST | `/api/v1/decisions` | Record decision |
| GET | `/api/v1/decisions?session_id=` | List decisions |
| GET | `/api/v1/decisions/{id}/tree` | Decision tree |
| POST | `/api/v1/query` | SQL query |
| GET | `/health` | Health check |

### MCP Tools

| Tool | Description |
|------|-------------|
| `query_sql` | Execute SQL query |
| `create_session` | Create agent session |
| `store_memory` | Store agent memory |
| `recall_memories` | Retrieve agent memories |
| `record_decision` | Record agent decision |

---

## 🛠️ Built-in Commands

| Command | Description |
|---------|-------------|
| `./bin/andb server` | Start HTTP API server (default: 0.0.0.0:8400) |
| `./bin/andb server -mode mcp` | Start MCP server (stdio transport) |
| `./bin/andb cli` | Interactive SQL REPL (local database) |
| `./bin/andb client -server host:port` | HTTP client (connect to remote) |
| `./bin/andb version` | Show version |

---

## 🔧 Configuration

### Settings File

| Location | Platform | Scope |
|----------|----------|-------|
| `config.json` | All | Project-level configuration |

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8400
  },
  "storage": {
    "data_dir": "./data"
  }
}
```

### Command-Line Flags

```bash
./bin/andb server                    # Default: 0.0.0.0:8400
./bin/andb server -host 127.0.0.1    # Bind to localhost
./bin/andb server -port 9000         # Custom port
./bin/andb cli                       # Default data dir
./bin/andb cli -data /path/to/data   # Custom data dir
./bin/andb client -server host:port  # Connect to remote
```

---

## 🛠️ Development

```bash
# Build the Web UI (Vite) then the binary
make build

# Build only the Go binary (no UI rebuild)
go build -o bin/andb ./cmd/andb

# Run server
make run

# Run tests
make test

# Run tests with race detector
make race

# Benchmarks
make bench

# Lint / format / vet
make lint

# Full check (fmt + vet + test)
make check

# Clean build artifacts
make clean
```

---

## 📊 Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go 1.23+ | Single binary, no runtime deps, cross-compile |
| KV Store | BadgerDB | Pure Go, no CGO, built-in LSM-tree, WAL, MVCC |
| Vector Index | Custom HNSW | Supports cosine/L2/dot distance, zero external deps |
| Graph Store | Custom adjacency list | Persisted on BadgerDB |
| SQL Engine | Custom recursive descent | Supports full SQL subset |
| Web UI | Svelte + Vite | Embedded into binary at build time |
| Protocol | MCP (stdio) | Standard agent-tool integration protocol |

---

## 🤝 Contributing

We welcome contributions! See [AGENTS.md](AGENTS.md) for project conventions.

```bash
git clone https://github.com/startvibecoding/AgentNativeDB.git
cd AgentNativeDB
make build
make test
```

---

## 📄 License

MIT — see [LICENSE](LICENSE) for details.

---

<p align="center">
  <strong>Ready to build agent-native data infrastructure? ⭐ Star this repo and get started!</strong>
</p>
