# AGENTS.md

## Project Snapshot

AgentNativeDB is a **Go 1.23+** agent-native database system combining a SQL query engine, vector search (HNSW), graph store, knowledge graph, and MCP server integration. Built on BadgerDB (pure Go, no CGO). The binary is called `andb`.

## Important Directories

| Path | Contents |
|------|----------|
| `cmd/andb/` | Single entry point: `main.go` dispatches to `server`, `cli`, `client`, `version` subcommands |
| `api/http/` | RESTful HTTP API router and integration tests |
| `api/mcp/` | MCP Server (Model Context Protocol) for Cursor/Claude Desktop |
| `config/` | Configuration management, reads `config.example.json` |
| `internal/storage/` | Storage engine abstraction (`Engine` interface) + LRU cache + key encoding + table manager |
| `internal/storage/badger/` | BadgerDB implementation of the `Engine` interface |
| `internal/model/` | Core data types: Session, Memory, Decision, Entity, types.go (helpers) |
| `internal/agent/` | Agent runtime: session manager, memory manager, decision tracking, RAG, coordinator, audit, permissions |
| `internal/query/sql/` | SQL engine: lexer, parser (recursive descent), AST, planner, executor, stats |
| `internal/query/sql/index/` | Secondary indexes: Hash, BTree, Inverted (with gse Chinese tokenization) |
| `internal/vector/` | HNSW vector index: distance functions (cosine/l2/dot), store |
| `internal/graph/` | Graph store: adjacency list, BFS, K-hop, shortest path |
| `internal/knowledge/` | Data lineage tracking |
| `internal/util/` | UUID v7 generation |
| `sdk/` | Go SDK (`agentnativedb.go` + tests) |
| `docs/` | Design document (`AgentNativeDB-Design.md`) |
| `examples/` | Example scripts and client demo |

## Architecture

Four-layer design:
1. **API Layer** — HTTP REST (`api/http/`), MCP Server (`api/mcp/`), CLI (`cmd/andb/cli.go`)
2. **Agent Runtime** — Session, Memory, Decision, Coordinator (`internal/agent/`)
3. **Query Layer** — SQL Engine, Graph Query, Vector Search (`internal/query/`, `internal/graph/`, `internal/vector/`)
4. **Storage Engine** — BadgerDB via `storage.Engine` interface, with LRU cache and key-prefix encoding

Key patterns:
- **Interface-based**: `storage.Engine` abstracts the KV store; all agent managers accept `Engine` via constructor injection
- **Key encoding**: 1-byte table prefix (e.g. `0x01`=sessions, `0x02`=memories) + field values (`internal/storage/keyencode.go`)
- **Custom SQL**: Hand-written recursive descent parser, no third-party SQL library
- **Custom HNSW**: Hand-written vector index, no third-party vector library
- **Minimal deps**: Only BadgerDB and gse (Chinese tokenizer) are external dependencies

## Build / Test / Run Commands

```bash
# Build the binary
make build              # → bin/andb

# Run server (HTTP on 0.0.0.0:8400)
./bin/andb server

# Run MCP server (stdio transport)
./bin/andb server -mode mcp

# Run interactive SQL CLI (local)
./bin/andb cli

# Run HTTP client (remote)
./bin/andb client -server localhost:8400

# Run all tests (with race detector)
make test               # go test -v -race -count=1 ./...

# Run benchmarks
make bench              # go test -bench=. -benchmem ./...

# Lint (fmt + vet)
make lint

# Format only
make fmt                # go fmt ./...

# Vet only
make vet                # go vet ./...

# Generate coverage report
make cover              # → coverage.html

# Full check (fmt + vet + test)
make check

# Clean build artifacts and data
make clean              # rm -rf bin/ data/
```

## Coding Conventions

- **Language**: Chinese is the primary language for comments, error messages, and documentation. Keep all new comments and user-facing strings in Chinese.
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` for error wrapping.
- **Testing**: Tests use `t.TempDir()` for BadgerDB instances (never write to `./data` in tests). Standard `testing` package, no test frameworks.
- **Naming**: Standard Go conventions. Interfaces are single-method or small. Exported types use PascalCase. Unexported types use camelCase.
- **No CGO**: The project must remain buildable without CGO. Do not add dependencies that require CGO.
- **Module path**: `github.com/startvibecoding/AgentNativeDB`
- **Binary name**: `andb`
- **JSON tags**: All exported struct fields have `json:"..."` tags for serialization.
- **Table prefixes**: When adding new table types, add a new `Prefix*` constant in `internal/storage/keyencode.go` following the existing byte-prefix scheme.
- **SQL support**: The SQL engine is hand-written (lexer → parser → planner → executor). When adding SQL features, update all four stages.

## Agent Must NOT

- Do **not** add CGO-dependent dependencies — the project must build with `CGO_ENABLED=0`.
- Do **not** delete or rename existing `Prefix*` constants in `keyencode.go` — these are part of the on-disk format.
- Do **not** modify the `storage.Engine` interface without updating all implementations (currently `internal/storage/badger/store.go`).
- Do **not** hardcode data paths — always use the `Options.DataDir` field or `config.json` settings.
- Do **not** introduce test frameworks beyond the standard `testing` package.
- Do **not** use `sudo`, `su`, or any privilege escalation.
- Do **not** modify `.git` history or force-push.
- Do **not** commit `bin/` or `data/` directories.
