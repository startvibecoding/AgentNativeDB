# AGENTS.md

## Project Snapshot

AgentNativeDB is a **Go 1.23+** agent-native database system combining a SQL query engine, vector search (HNSW), graph store, knowledge/lineage tracking, and MCP server integration. It ships with a Svelte-based Web UI embedded into a single binary. Built on BadgerDB (pure Go, no CGO). The binary is called `andb`.

Module path: `github.com/startvibecoding/AgentNativeDB`
External runtime deps: `github.com/dgraph-io/badger/v4`.

## Important Directories

| Path | Contents |
|------|----------|
| `cmd/andb/` | Single entry point: `main.go` dispatches to `server`, `cli`, `client`, `version` subcommands (`server.go`, `cli.go`, `client.go`) |
| `api/http/` | RESTful HTTP API router (`router.go`) and integration tests |
| `api/mcp/` | MCP Server (Model Context Protocol, stdio transport) for Cursor / Claude Desktop |
| `config/` | Configuration management; reads `config.example.json` |
| `internal/storage/` | Storage engine abstraction (`Engine` interface), LRU cache, key encoding, table manager |
| `internal/storage/badger/` | BadgerDB implementation of the `Engine` interface |
| `internal/model/` | Core data types: `Session`, `Memory`, `Decision`, `Entity`, plus `types.go` helpers |
| `internal/agent/` | Agent runtime: session, memory (+enhanced), decision, coordinator, RAG, audit, permissions, task queue |
| `internal/query/sql/` | SQL engine: lexer, parser (recursive descent), AST, planner, executor, stats |
| `internal/query/sql/index/` | Secondary indexes: Hash, BTree, Inverted (with built-in full-text tokenization) |
| `internal/query/graph/` | Graph query surface exposed through the query layer |
| `internal/query/vector/` | Vector query surface exposed through the query layer |
| `internal/vector/` | HNSW vector index: distance functions (cosine / l2 / dot), store |
| `internal/graph/` | Graph store: adjacency list, BFS, K-hop, shortest path |
| `internal/knowledge/` | Data lineage tracking (`lineage.go`) |
| `internal/util/` | UUID v7 generation |
| `sdk/` | Go SDK (`agentnativedb.go` + tests) |
| `ui/` | Svelte + Vite Web UI (built into `ui/dist/`, embedded into the `andb` binary) |
| `docs/` | Design document (`AgentNativeDB-Design.md`) and client guide (`client.md`) |
| `examples/` | Example scripts (`basic/`, `client_demo.sh`) |

## Architecture

Four-layer design:
1. **API Layer** — HTTP REST (`api/http/`), MCP Server (`api/mcp/`), CLI (`cmd/andb/cli.go`), Web UI (`ui/`)
2. **Agent Runtime** — Session, Memory, Decision, Coordinator, Audit, Permission, TaskQueue, RAG (`internal/agent/`)
3. **Query Layer** — SQL Engine, Graph Query, Vector Search (`internal/query/{sql,graph,vector}/`, `internal/graph/`, `internal/vector/`)
4. **Storage Engine** — BadgerDB via `storage.Engine` interface, with LRU cache and key-prefix encoding

Key patterns:
- **Interface-based**: `storage.Engine` abstracts the KV store; agent managers accept `Engine` via constructor injection.
- **Key encoding**: 1-byte table prefix (e.g. `0x01`=sessions, `0x02`=memories) + field values (`internal/storage/keyencode.go`).
- **Custom SQL**: Hand-written recursive descent parser; no third-party SQL library. Supported data types: `INT`, `INTEGER`, `VARCHAR`, `TEXT`, `STRING`, `FLOAT`, `BOOL`, `BOOLEAN`.
- **Custom HNSW**: Hand-written vector index; no third-party vector library.
- **Minimal deps**: Only BadgerDB is a core external dependency.
- **Single binary**: The Web UI is built with Vite then embedded into the Go binary during `make build`.

## Build / Test / Run Commands

```bash
# Build the Web UI (Vite) then the binary
make build              # runs `ui-build` then `go build -o bin/andb ./cmd/andb`

# Build only the Web UI
make ui-build           # cd ui && npm run build

# Run server (HTTP on 0.0.0.0:8400 by default)
make run                # build + ./bin/andb server
./bin/andb server

# Run MCP server (stdio transport)
./bin/andb server -mode mcp

# Run interactive SQL CLI (local, opens local BadgerDB)
make run-cli
./bin/andb cli
./bin/andb cli -data /path/to/data

# Run HTTP client (remote)
make run-client
./bin/andb client -server localhost:8400

# Version
./bin/andb version

# Tests (with race detector)
make test               # go test -v -race -count=1 ./...
make race               # go test -race -count=1 ./...

# Benchmarks
make bench              # go test -bench=. -benchmem ./...

# Lint / format / vet
make lint               # fmt + vet
make fmt                # go fmt ./...
make vet                # go vet ./...

# Coverage report
make cover              # → coverage.html

# Full check (fmt + vet + test)
make check

# Dependencies
make tidy               # go mod tidy

# Clean build artifacts and data
make clean              # rm -rf bin/ data/
```

Note: `make build` requires Node/npm because it invokes `npm run build` inside `ui/`. If you only need to iterate on Go code, run `go build ./cmd/andb` directly, but the resulting binary may lack the latest UI assets.

## Coding Conventions

- **Language**: Chinese is the primary language for comments, error messages, CLI help, and documentation. Keep new comments and user-facing strings in Chinese.
- **Error wrapping**: Use `fmt.Errorf("上下文: %w", err)` for error wrapping.
- **Testing**: Tests use `t.TempDir()` for BadgerDB instances (never write to `./data` in tests). Use the standard `testing` package; no third-party test frameworks.
- **Naming**: Standard Go conventions. Interfaces are single-method or small. Exported types use PascalCase; unexported types use camelCase.
- **No CGO**: The project must remain buildable with `CGO_ENABLED=0`. Do not add dependencies that require CGO.
- **Module path**: `github.com/startvibecoding/AgentNativeDB`
- **Binary name**: `andb`
- **JSON tags**: All exported struct fields have `json:"..."` tags for serialization.
- **Table prefixes**: When adding new table types, add a new `Prefix*` constant in `internal/storage/keyencode.go` following the existing byte-prefix scheme. Never reuse a byte.
- **SQL support**: The SQL engine is hand-written (lexer → parser → planner → executor). When adding SQL features, update all four stages plus tests.
- **UI assets**: Do not hand-edit files under `ui/dist/`; regenerate via `make ui-build`.

## Agent Must NOT

- Do **not** add CGO-dependent dependencies — the project must build with `CGO_ENABLED=0`.
- Do **not** delete or rename existing `Prefix*` constants in `keyencode.go` — these are part of the on-disk format.
- Do **not** modify the `storage.Engine` interface without updating all implementations (currently `internal/storage/badger/`).
- Do **not** hardcode data paths — always use the `Options.DataDir` field or `config.json` settings.
- Do **not** introduce test frameworks beyond the standard `testing` package.
- Do **not** commit `bin/`, `data/`, `ui/node_modules/`, or `ui/dist/` directories.
- Do **not** use `sudo`, `su`, or any privilege escalation.
- Do **not** modify `.git` history or force-push.
