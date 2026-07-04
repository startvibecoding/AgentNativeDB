# andb

## 项目概述

AgentNativeDB 是一个以 Agent 为核心设计目标的数据库系统，单二进制部署（Go 1.23+，无 CGO）。
模块路径：`github.com/startvibecoding/AgentNativeDB`

核心技术栈：
- KV 存储：BadgerDB（纯 Go）
- 向量索引：自研 HNSW（cosine/l2/dot 距离）
- SQL 解析：自研递归下降解析器
- 二级索引：Hash（等值）、BTree（范围）、Inverted（全文检索，gse 中文分词）
- 图存储：自研邻接表（基于 BadgerDB 持久化）
- 依赖仅 BadgerDB + gse

## 构建与运行

```bash
# 构建
make build              # → bin/andb

# 运行测试（含竞态检测）
make test               # go test -v -race -count=1 ./...

# 基准测试
make bench              # go test -bench=. -benchmem ./...

# 代码检查
make lint               # go fmt + go vet

# 覆盖率
make cover              # → coverage.html

# 清理
make clean              # rm -rf bin/ data/
```

## 二进制子命令

```bash
andb server   [flags]   # HTTP/MCP 服务
andb cli      [flags]   # 交互式 SQL CLI（本地）
andb client   [flags]   # HTTP 客户端（远程）
andb version            # 版本号
andb help               # 帮助
```

### server 子命令

```bash
andb server                        # 默认 HTTP 服务 (0.0.0.0:8400)
andb server -config config.json    # 指定配置文件
andb server -mode mcp              # MCP Server（stdio 传输，用于 Cursor/Claude Desktop）
```

flags：
- `-config string` — 配置文件路径
- `-mode string` — 运行模式：`server`（默认）、`mcp`

### cli 子命令

```bash
andb cli                           # 本地直连 ./data
andb cli -data /path/to/data       # 指定数据目录
```

flags：
- `-data string` — 数据目录（默认 `./data`）

CLI 内置命令：`help`、`quit`/`exit`

### client 子命令

```bash
andb client -server localhost:8400              # 连接远程服务器
andb client -server localhost:8400 -format json # JSON 输出
andb client -config config.json                 # 从配置读取地址
```

flags：
- `-config string` — 配置文件路径
- `-server string` — 服务器地址 (host:port)
- `-format string` — 输出格式：`table`（默认）、`json`

client 交互命令：`help`、`health`、`sessions`、`query <SQL>`、`quit`/`exit`

## SQL 语法

### DDL

```sql
-- 建表
CREATE TABLE users (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(64),
    age INT,
    score FLOAT,
    active BOOL
)

CREATE TABLE IF NOT EXISTS users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64))

-- 删表
DROP TABLE users
DROP TABLE IF EXISTS users

-- 改表
ALTER TABLE users ADD COLUMN email VARCHAR(128)
ALTER TABLE users DROP COLUMN email
ALTER TABLE users MODIFY COLUMN name VARCHAR(128)

-- 查看表
SHOW TABLES
DESCRIBE users        -- 或 DESC users
```

列类型：`INT`、`FLOAT`、`VARCHAR(n)`、`TEXT`、`BOOL`

### 二级索引

```sql
-- 创建索引
CREATE INDEX idx_name ON users(name) USING HASH       -- 等值查询
CREATE INDEX idx_age ON users(age) USING BTREE        -- 范围/排序查询
CREATE INDEX idx_content ON docs(body) USING INVERTED -- 全文检索
CREATE UNIQUE INDEX idx_email ON users(email) USING HASH
CREATE INDEX IF NOT EXISTS idx_name ON users(name) USING HASH

-- 删除索引
DROP INDEX idx_name
DROP INDEX IF EXISTS idx_name

-- 查看索引
SHOW INDEXES
SHOW INDEXES FROM users
```

索引类型选择规则：
- `col = literal` → 优先 HASH，可回退 BTREE
- `col <op> literal`（`<`, `<=`, `>`, `>=`）→ 优先 BTREE
- `BETWEEN` → 优先 BTREE
- `MATCH(col) AGAINST(...)` → 必须 INVERTED
- Planner 自动选择最优索引

### DML

```sql
-- 插入
INSERT INTO users (id, name, age) VALUES ('u1', 'alice', 30)

-- 更新
UPDATE users SET age = 31 WHERE id = 'u1'

-- 删除
DELETE FROM users WHERE id = 'u1'
```

### 查询

```sql
-- 基础查询
SELECT * FROM users WHERE age > 20
SELECT name, age FROM users WHERE active = TRUE

-- 别名
SELECT name AS n, age AS a FROM users

-- 运算符：=, !=, <>, <, <=, >, >=, AND, OR, NOT, LIKE, IN, BETWEEN, IS NULL, IS NOT NULL

-- 排序
SELECT * FROM users ORDER BY age DESC
SELECT * FROM users ORDER BY age ASC, name DESC

-- 分页
SELECT * FROM users LIMIT 10
SELECT * FROM users LIMIT 10 OFFSET 20

-- 聚合
SELECT COUNT(*) FROM users
SELECT name, COUNT(*) as cnt FROM users GROUP BY name
SELECT name, AVG(score) as avg_score FROM users GROUP BY name HAVING AVG(score) > 80
-- 聚合函数：COUNT(*), SUM(col), MIN(col), MAX(col), AVG(col)
-- 支持 COUNT(DISTINCT col)

-- JOIN
SELECT s.agent_id, m.content
FROM agent_sessions s
JOIN agent_memories m ON s.id = m.session_id
WHERE m.importance > 0.7

-- 全文检索（中文，需先创建 INVERTED 索引）
SELECT * FROM docs WHERE MATCH(body) AGAINST ('数据库 向量')
```

### 内置函数

- 聚合：`COUNT(*)`, `COUNT(DISTINCT col)`, `SUM(col)`, `MIN(col)`, `MAX(col)`, `AVG(col)`
- 字符串：`UPPER(s)`, `LOWER(s)`, `LENGTH(s)`
- 通用：`COALESCE(a, b)`

### 内置表

| 表名 | 前缀 | 用途 |
|------|------|------|
| `agent_sessions` | `0x01` | Agent 会话 |
| `agent_memories` | `0x02` | Agent 记忆 |
| `agent_decisions` | `0x03` | Agent 决策 |
| `knowledge_entities` | `0x04` | 知识实体 |
| `knowledge_relations` | `0x05` | 知识关系 |
| `data_lineage` | `0x06` | 数据血缘 |

用户表前缀从 `0x30` 开始（`CREATE TABLE` 时自动分配）。

### agent_sessions 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR | UUID v7 主键 |
| agent_id | VARCHAR | Agent 标识 |
| state | VARCHAR | active/paused/completed/failed |
| context | TEXT | JSON 上下文 |
| metadata | TEXT | JSON 元数据 |
| created_at | TEXT | 创建时间 |
| updated_at | TEXT | 更新时间 |

### agent_memories 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR | UUID v7 主键 |
| session_id | VARCHAR | 关联会话 |
| type | VARCHAR | short_term/long_term/working |
| content | TEXT | 记忆内容 |
| importance | FLOAT | 重要度 0.0-1.0 |
| access_count | INT | 访问次数 |
| associations | TEXT | 关联 ID 列表 |
| created_at | TEXT | 创建时间 |
| accessed_at | TEXT | 最后访问时间 |

### agent_decisions 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR | UUID v7 主键 |
| session_id | VARCHAR | 关联会话 |
| parent_id | VARCHAR | 父决策 ID（决策树） |
| type | VARCHAR | reasoning/tool_call/planning/reflection |
| input | TEXT | JSON 输入 |
| output | TEXT | JSON 输出 |
| reasoning | TEXT | 推理过程 |
| tools_used | TEXT | 使用的工具列表 |
| duration_ms | INT | 耗时（毫秒） |
| token_usage | TEXT | Token 用量 |
| created_at | TEXT | 创建时间 |

## HTTP API

默认地址：`0.0.0.0:8400`

所有响应格式：
```json
{"ok": true, "data": ...}
{"ok": false, "error": "错误信息"}
```

### Health

```
GET /health
→ {"ok": true, "data": {"status": "ok", "time": "..."}}
```

### Sessions

```
POST /api/v1/sessions
Body: {"agent_id": "agent-001", "metadata": {"env": "test"}}

GET /api/v1/sessions?agent_id=xxx&limit=10

GET /api/v1/sessions/{id}

PATCH /api/v1/sessions/{id}
Body: {"state": "paused", "context": {"key": "value"}}

DELETE /api/v1/sessions/{id}
```

### Memories

```
POST /api/v1/memories
Body: {"session_id": "sess-123", "type": "short_term", "content": "用户偏好", "importance": 0.8}

GET /api/v1/memories?session_id=xxx&type=short_term&limit=10

GET /api/v1/memories/{id}

DELETE /api/v1/memories/{id}
```

### Decisions

```
POST /api/v1/decisions
Body: {
  "session_id": "sess-123",
  "type": "reasoning",
  "input": {...},
  "output": {...},
  "reasoning": "因为...",
  "tools_used": ["query_sql"],
  "duration_ms": 150
}

GET /api/v1/decisions?session_id=xxx&limit=10

GET /api/v1/decisions/{id}

GET /api/v1/decisions/{id}/tree    -- 决策树

DELETE /api/v1/decisions/{id}
```

### SQL 查询

```
POST /api/v1/query
Body: {"sql": "SELECT * FROM users WHERE age > 20"}
→ {"ok": true, "data": {"columns": ["id", "name", "age"], "rows": [...]}}
```

## MCP Server

传输方式：stdio（JSON-RPC 2.0）

启动：`andb server -mode mcp`

协议版本：`2024-11-05`

### 工具列表

| 工具 | 必填参数 | 可选参数 | 说明 |
|------|----------|----------|------|
| `query_sql` | `sql: string` | — | 执行 SQL 查询 |
| `create_session` | `agent_id: string` | — | 创建 Agent 会话 |
| `store_memory` | `session_id: string, content: string` | `type: string` (short_term/long_term/working, 默认 short_term), `importance: number` (0.0-1.0, 默认 0.5) | 存储记忆 |
| `recall_memories` | `session_id: string` | `type: string`, `limit: integer` (默认 10) | 检索记忆 |
| `record_decision` | `session_id: string, type: string, input: object, output: object` | `reasoning: string` | 记录决策 |

决策类型（type）：`reasoning`、`tool_call`、`planning`、`reflection`

## 向量搜索

通过 Go SDK 使用（非 SQL 直接访问）：

```go
vs := vector.NewVectorStore(engine)
vs.CreateIndex("my_index", 1536, "cosine")  // 维度, 距离度量
vs.Insert("my_index", "id1", []float32{0.1, 0.2, ...})
results, _ := vs.Search("my_index", queryVec, 10)  // topK=10
```

距离度量：`cosine`、`l2`、`dot`

HNSW 参数（配置文件）：
- `hnsw_m`: 16（默认）— 每层最大连接数
- `hnsw_ef_construction`: 200（默认）— 构建时搜索宽度
- `hnsw_ef_search`: 64（默认）— 查询时搜索宽度
- `default_dimension`: 1536（默认）
- `default_metric`: "cosine"（默认）

## 图存储

通过 Go SDK 使用：

```go
gs := graph.NewGraphStore(engine)

// 节点
gs.AddNode(ctx, &graph.Node{ID: "n1", Type: "person", Name: "Alice"})
gs.GetNode(ctx, "n1")
gs.DeleteNode(ctx, "n1")  // 自动删除关联边
gs.FindNodesByType(ctx, "person", 100)
gs.FindNodesByName(ctx, "Alice", 10)

// 边
gs.AddEdge(ctx, &graph.Edge{ID: "e1", Type: "knows", FromID: "n1", ToID: "n2", Weight: 0.9})
gs.GetEdges(ctx, "n1")         // 所有关联边
gs.GetOutEdges(ctx, "n1")      // 出边
gs.GetInEdges(ctx, "n1")       // 入边
gs.GetNeighbors(ctx, "n1", graph.DirBoth)  // 邻居节点
gs.DeleteEdge(ctx, "e1")

// 图算法
gs.KHopNeighbors(ctx, "n1", 3, graph.DirBoth)  // K 跳邻居
gs.ShortestPath(ctx, "n1", "n5", graph.DirBoth) // 最短路径 (BFS)
```

遍历方向：`DirOut`（出边）、`DirIn`（入边）、`DirBoth`（双向）

## 数据血缘

通过 Go SDK 使用：

```go
lt := knowledge.NewLineageTracker(engine)

// 记录血缘
lt.Record(ctx, &knowledge.Lineage{
    DataID:     "data-1",
    SourceType: "derived",
    SourceIDs:  []string{"data-a", "data-b"},
    Steps: []knowledge.Step{
        {Type: "transform", Operation: "merge", Timestamp: time.Now()},
    },
})

// 获取血缘
lineage, _ := lt.Get(ctx, "data-1")

// 追溯上游（递归）
tree, _ := lt.TraceUpstream(ctx, "data-1", 5)  // depth=5

// 按决策查询
lineages, _ := lt.ListByDecision(ctx, "decision-id")
```

## Agent Runtime 组件

### 会话管理 (SessionManager)

```go
mgr := agent.NewSessionManager(engine, cache)
sess, _ := mgr.Create(ctx, "agent-001", map[string]any{"env": "test"})
sess, _ = mgr.Get(ctx, sess.ID)
mgr.Update(ctx, sess)
mgr.Delete(ctx, sess.ID)
sessions, _ := mgr.ListByAgent(ctx, "agent-001", 10)
sessions, _ = mgr.ListAll(ctx, 10)
```

增强版 `SessionManager`：支持超时清理、每 Agent 最大活跃会话数限制。

### 记忆管理 (MemoryStore)

```go
store := agent.NewMemoryStore(engine, cache)
mem, _ := store.Store(ctx, model.NewMemory(sessID, model.MemoryShortTerm, "内容", 0.8))
mem, _ = store.Get(ctx, mem.ID)
store.Delete(ctx, mem.ID)
mems, _ := store.ListBySession(ctx, sessID, model.MemoryShortTerm, 10)
```

增强版 `MemoryStoreEnhanced`：支持短期记忆滑窗淘汰、重要度衰减、工作记忆 TTL。

记忆类型：`short_term`、`long_term`、`working`

### 决策记录 (DecisionRecorder)

```go
rec := agent.NewDecisionRecorder(engine, cache)
d, _ := rec.Record(ctx, model.NewDecision(sessID, model.DecisionReasoning, inputJSON, outputJSON))
d, _ = rec.Get(ctx, d.ID)
rec.Delete(ctx, d.ID)
decisions, _ := rec.ListBySession(ctx, sessID, 10)
tree, _ := rec.BuildDecisionTree(ctx, d.ID)  // 构建决策树
```

决策类型：`reasoning`、`tool_call`、`planning`、`reflection`

### 多 Agent 协调 (Coordinator)

```go
coord := agent.NewCoordinator(engine, sessions, memory, decision)
room, _ := coord.CreateRoom(ctx, "room-1", "协作讨论")
coord.JoinRoom(ctx, "room-1", "agent-001")
coord.LeaveRoom(ctx, "room-1", "agent-001")
msg := coord.SendMessage(ctx, "room-1", "agent-001", "大家好")
messages, _ := coord.GetMessages(ctx, "room-1", 50)
```

### RAG 引擎

```go
rag := agent.NewRAGEngine(engine, memoryStore, vectorStore)
chunks, _ := rag.IngestDocument(ctx, sessID, &agent.Document{Content: "长文本...", Title: "文档"})
results, _ := rag.Retrieve(ctx, sessID, "查询文本", 5)  // topK=5
```

### 审计日志 (AuditLogger)

```go
audit := agent.NewAuditLogger(engine)
audit.Log(ctx, &agent.AuditEvent{AgentID: "a1", Action: "query", Resource: "users", Detail: "SELECT *"})
events, _ := audit.List(ctx, "a1", 100)
```

### 权限管理

操作类型（Action）：`read`、`write`、`delete`、`admin`、`*`（通配）
资源类型（Resource）：`session`、`memory`、`decision`、`room`、`task`、`query`、`audit`、`permission`、`*`（通配）

### 任务队列 (TaskQueue)

```go
q := agent.NewTaskQueue(engine)
task, _ := q.Enqueue(ctx, &agent.Task{Priority: 1, Payload: "..."})
task, _ = q.Dequeue(ctx)
```

优先级队列，支持并发安全。

## 配置文件

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8400,
    "read_timeout": 30000000000,
    "write_timeout": 30000000000
  },
  "storage": {
    "data_dir": "./data",
    "value_log_file_size": 67108864,
    "mem_table_size": 16777216,
    "num_mem_tables": 3,
    "sync_writes": true,
    "cache_size_mb": 256
  },
  "agent": {
    "session_timeout": 86400000000000,
    "max_memories": 10000,
    "short_term_window": 50,
    "cleanup_interval": 3600000000000
  },
  "vector": {
    "default_dimension": 1536,
    "default_metric": "cosine",
    "hnsw_m": 16,
    "hnsw_ef_construction": 200,
    "hnsw_ef_search": 64
  },
  "log": {
    "level": "info",
    "format": "text"
  }
}
```

日志级别：`debug`、`info`（默认）、`warn`、`error`
日志格式：`text`（默认）、`json`

## SDK 用法

```go
import agentnativedb "github.com/startvibecoding/AgentNativeDB/sdk"

db, _ := agentnativedb.Open("./mydata")
defer db.Close()

sess, _ := db.CreateSession("agent-001")
db.StoreMemory(sess.ID, "用户偏好中文", agentnativedb.LongTerm, 0.8)
memories, _ := db.RecallMemories(sess.ID, nil, 10)
result, _ := db.Query("SELECT * FROM agent_sessions")
```

## 存储 Key 编码

主键格式：`[1字节前缀][ID]`
索引格式：`[1字节前缀][字段名][0x00][ID]`
向量格式：`[0x10][索引名][0x00][ID]`

前缀定义：
- `0x01` agent_sessions
- `0x02` agent_memories
- `0x03` agent_decisions
- `0x04` knowledge_entities
- `0x05` knowledge_relations
- `0x06` data_lineage
- `0x10` vector_index
- `0x11` 用户表索引（Hash/BTree/Inverted）
- `0x20` graph_adjacency
- `0xFF` system_metadata
- `0x30+` 用户表（CREATE TABLE 时自动分配）

## 技术约束

- Go 1.23+，单二进制，无 CGO
- 存储：BadgerDB（纯 Go KV）
- 中文分词：gse
- 索引自动维护：INSERT/UPDATE/DELETE 自动更新二级索引
- Planner 自动选择索引：`col=literal`→Hash，范围→BTree，`MATCH`→Inverted
- UUID 使用 v7（时间有序）
- MCP 协议版本：2024-11-05
- 数据目录默认 `./data`，可通过配置或 `-data` 参数指定
