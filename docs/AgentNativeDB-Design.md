# AgentNativeDB 开发计划 v3

> AgentNativeDB = Database + Agent Runtime + Knowledge Graph
>
> 不只是"能被 Agent 调用的数据库"，而是"以 Agent 为核心设计目标的数据库系统"。
>
> **语言：Go | 最小依赖 | BadgerDB 为存储内核**

---

## 一、项目概述

AgentNativeDB 是面向 AI Agent 原生的下一代数据库系统，将 Agent 的会话、记忆、任务、决策作为原生数据管理，并实现结构化数据、向量嵌入与知识图谱的统一存储。

### 核心目标

1. **Agent 作为一等公民**：Agent 的会话、记忆、任务、决策都是数据库的原生数据
2. **数据与知识融合**：结构化数据 + 向量嵌入 + 知识图谱 统一存储
3. **可审计的 Agent 行为**：所有 Agent 操作都有血缘追踪和决策记录
4. **自适应存储**：根据数据特征和查询模式自动选择最优存储格式
5. **多 Agent 协作**：原生支持多 Agent 会话、任务分发、结果聚合

### 设计原则

- **渐进式构建**：先跑通核心链路，再叠加能力层
- **最小依赖**：能用标准库解决的不引入第三方；必须引入的选纯 Go 实现
- **接口先行**：先定义 API 和数据模型，再实现内部引擎
- **测试驱动**：每个模块必须有单元测试、并发测试和基准测试

---

## 二、技术选型

### 2.1 实现语言：Go

| 维度 | 评估 |
|------|------|
| 编译与部署 | 单二进制，无运行时依赖，交叉编译简单 |
| 并发模型 | goroutine + channel，天然适合数据库并发场景 |
| 开发效率 | 语法简洁，团队上手快，编译速度快 |
| 标准库 | `net/http`、`encoding/json`、`sync`、`context` 等覆盖大量需求 |
| 生态验证 | BadgerDB、CockroachDB、etcd、InfluxDB 等验证了 Go 做存储的可行性 |

### 2.2 依赖策略

**核心原则：标准库优先，纯 Go 优先，能自研的不引入外部包。**

#### 允许的外部依赖（仅 2 个）

| 依赖 | 版本 | 用途 | 选择理由 |
|------|------|------|----------|
| `github.com/dgraph-io/badger/v4` | v4 | KV 存储引擎 | 纯 Go 实现，无 CGO，内置 LSM-tree、WAL、MVCC 事务 |
| `golang.org/x/exp` | latest | 泛型辅助（slices、maps） | Go 官方扩展库，Go 1.21+ 部分已进标准库 |

#### 使用标准库替代的组件

| 需求 | 不引入 | 使用 |
|------|--------|------|
| HTTP Server | gin、echo | `net/http` + 轻量路由封装 |
| SQL 解析 | vitess、pg_query | 自研递归下降解析器（`text/scanner` + 手写 AST） |
| 图查询解析 | 自研 Cypher 子集解析器 | 同上 |
| 向量索引 | go-hnsw | 自研 HNSW（基于 `math/rand` + `sort`） |
| 序列化 | protobuf、msgpack | `encoding/json`（外部 API）+ `encoding/binary`（内部二进制） |
| UUID | google/uuid | 自研 UUID v7（基于 `crypto/rand` + 时间戳） |
| LRU 缓存 | groupcache | 自研（`sync.RWMutex` + `container/list`） |
| 日志 | zap、zerolog | `log/slog`（Go 1.21+ 标准库） |
| 测试断言 | testify | 标准 `testing` 包 + 自定义断言辅助函数 |
| CLI 框架 | cobra | `flag` 标准库 + 自定义命令路由 |
| 配置管理 | viper | `encoding/json` 读取配置文件 |

#### 依赖树总览

```
agentnativedb/
├── github.com/dgraph-io/badger/v4   ← KV 存储（唯一的外部核心依赖）
│   └── github.com/dgraph-io/ristretto  ← BadgerDB 内部缓存（传递依赖）
├── golang.org/x/exp                  ← 泛型工具（可选，Go 1.21+ 可减少使用）
└── 标准库                             ← 其余全部
```

---

## 三、整体架构

### 3.1 分层架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        AgentNativeDB Architecture                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      API Layer (Phase 8)                         │  │
│  │      net/http Server  │  MCP Server  │  CLI  │  Go SDK           │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      Agent Runtime Layer (Phase 3)               │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐│  │
│  │  │   Session   │ │   Memory    │ │  Decision   │ │   Skill     ││  │
│  │  │   Manager   │ │   Store     │ │  Recorder   │ │   Registry  ││  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘│  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                │  │
│  │  │  Tool Call  │ │  Multi-Agent│ │   Task      │                │  │
│  │  │  Executor   │ │ Coordinator │ │   Queue     │                │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘                │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      Unified Query Layer (Phase 2)               │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐│  │
│  │  │    SQL      │ │  Graph      │ │  Vector     │ │  Natural    ││  │
│  │  │   Engine    │ │   Query     │ │   Search    │ │  Language    ││  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘│  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      Knowledge Layer (Phase 4)                   │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐│  │
│  │  │  Knowledge  │ │  Ontology   │ │  Data       │ │  Embedding  ││  │
│  │  │   Graph     │ │   Store     │ │  Lineage    │ │   Space     ││  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘│  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      Storage Engine Layer (Phase 1)              │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐│  │
│  │  │  BadgerDB   │ │  自研 HNSW  │ │  自研 LRU   │ │  自研       ││  │
│  │  │  (KV Store) │ │  (Vector)   │ │  (Cache)    │ │  (Graph)    ││  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘│  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Go Module 结构

```
agentnativedb/
├── go.mod
├── go.sum
├── main.go                          # 入口
├── cmd/
│   ├── server/                      # 数据库服务端
│   │   └── main.go
│   └── cli/                         # 命令行工具
│       └── main.go
├── internal/
│   ├── storage/                     # 存储引擎层
│   │   ├── engine.go                # StorageEngine 接口定义
│   │   ├── badger/                  # BadgerDB 实现
│   │   │   ├── store.go
│   │   │   ├── tx.go
│   │   │   └── store_test.go
│   │   ├── cache.go                 # LRU 缓存
│   │   └── cache_test.go
│   ├── vector/                      # 向量索引
│   │   ├── hnsw.go                  # HNSW 实现
│   │   ├── distance.go              # 距离计算（cosine/l2/dot）
│   │   ├── hnsw_test.go
│   │   └── benchmark_test.go
│   ├── graph/                       # 图存储
│   │   ├── store.go                 # 邻接表 + 倒排索引
│   │   ├── traversal.go             # BFS/DFS 遍历
│   │   └── store_test.go
│   ├── query/                       # 查询引擎
│   │   ├── sql/
│   │   │   ├── lexer.go             # 词法分析
│   │   │   ├── parser.go            # 语法分析
│   │   │   ├── ast.go               # AST 节点定义
│   │   │   ├── planner.go           # 查询计划
│   │   │   ├── executor.go          # 执行引擎
│   │   │   ├── lexer_test.go
│   │   │   └── parser_test.go
│   │   ├── graph/
│   │   │   ├── parser.go            # 图查询解析
│   │   │   └── executor.go
│   │   └── vector/
│   │       └── executor.go
│   ├── agent/                       # Agent Runtime
│   │   ├── session.go               # 会话管理
│   │   ├── memory.go                # 记忆存储
│   │   ├── decision.go              # 决策记录
│   │   ├── session_test.go
│   │   └── memory_test.go
│   ├── knowledge/                   # 知识图谱
│   │   ├── entity.go
│   │   ├── relation.go
│   │   ├── ontology.go
│   │   └── lineage.go
│   ├── model/                       # 数据模型
│   │   ├── session.go
│   │   ├── memory.go
│   │   ├── decision.go
│   │   ├── entity.go
│   │   ├── relation.go
│   │   ├── types.go                 # 通用类型（UUID、Timestamp 等）
│   │   └── types_test.go
│   └── util/                        # 工具库
│       ├── uuid.go                  # UUID v7 生成
│       ├── uuid_test.go
│       ├── json.go                  # JSON 辅助
│       └── assert_test.go           # 测试断言辅助
├── api/                             # API 层
│   ├── http/
│   │   ├── router.go                # 路由定义
│   │   ├── handler_session.go
│   │   ├── handler_memory.go
│   │   ├── handler_decision.go
│   │   ├── handler_query.go
│   │   ├── middleware.go            # 日志、认证、panic 恢复
│   │   └── router_test.go
│   └── mcp/                         # MCP Server（Phase 8）
│       └── server.go
├── config/
│   ├── config.go                    # 配置加载
│   └── default.go                   # 默认配置
└── docs/
    └── AgentNativeDB-Design.md
```

---

## 四、核心接口定义

### 4.1 存储引擎接口

```go
// internal/storage/engine.go

package storage

import "context"

// Engine 定义存储引擎的核心接口
type Engine interface {
    // 生命周期
    Open(opts Options) error
    Close() error

    // 读写操作
    Get(ctx context.Context, key []byte) ([]byte, error)
    Set(ctx context.Context, key, value []byte) error
    Delete(ctx context.Context, key []byte) error

    // 范围扫描
    Scan(ctx context.Context, start, end []byte, opts ScanOptions) Iterator

    // 事务
    NewTransaction(update bool) Transaction

    // 前缀查询（用于按表/类型分区）
    PrefixScan(ctx context.Context, prefix []byte, opts ScanOptions) Iterator

    // 批量写入
    BatchWrite(ctx context.Context, ops []WriteOp) error
}

// Transaction 定义事务接口
type Transaction interface {
    Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	Discard()
}

// Iterator 定义迭代器接口
type Iterator interface {
    Item() (key, value []byte)
    Next() bool
    Valid() bool
    Close() error
}

// ScanOptions 扫描选项
type ScanOptions struct {
    Limit   int
    Reverse bool
}

// WriteOp 批量写入操作
type WriteOp struct {
    Type    OpType // Put / Delete
    Key     []byte
    Value   []byte
}
```

### 4.2 数据模型

```go
// internal/model/types.go

package model

import (
    "encoding/json"
    "time"
)

// UUID 是 UUID v7 字符串（时间有序）
type UUID string

// Timestamp 统一时间戳
type Timestamp = time.Time

// SessionState 会话状态
type SessionState string

const (
    SessionActive    SessionState = "active"
    SessionPaused    SessionState = "paused"
    SessionCompleted SessionState = "completed"
    SessionFailed    SessionState = "failed"
)

// MemoryType 记忆类型
type MemoryType string

const (
    MemoryShortTerm MemoryType = "short_term"
    MemoryLongTerm  MemoryType = "long_term"
    MemoryWorking   MemoryType = "working"
)

// DecisionType 决策类型
type DecisionType string

const (
    DecisionReasoning DecisionType = "reasoning"
    DecisionToolCall  DecisionType = "tool_call"
    DecisionPlanning  DecisionType = "planning"
    DecisionReflection DecisionType = "reflection"
)
```

```go
// internal/model/session.go

package model

type AgentSession struct {
    ID        UUID              `json:"id"`
    AgentID   string            `json:"agent_id"`
    State     SessionState      `json:"state"`
    Context   map[string]any    `json:"context,omitempty"`
    Metadata  map[string]any    `json:"metadata,omitempty"`
    CreatedAt Timestamp         `json:"created_at"`
    UpdatedAt Timestamp         `json:"updated_at"`
}
```

```go
// internal/model/memory.go

package model

type MemoryEntry struct {
    ID           UUID       `json:"id"`
    SessionID    UUID       `json:"session_id"`
    Type         MemoryType `json:"type"`
    Content      string     `json:"content"`
    Embedding    []float32  `json:"-"`              // 不序列化到 JSON，单独存储
    Importance   float32    `json:"importance"`
    AccessCount  uint32     `json:"access_count"`
    Associations []UUID     `json:"associations,omitempty"`
    CreatedAt    Timestamp  `json:"created_at"`
    AccessedAt   Timestamp  `json:"accessed_at"`
}
```

```go
// internal/model/decision.go

package model

import "encoding/json"

type Decision struct {
    ID          UUID          `json:"id"`
    SessionID   UUID          `json:"session_id"`
    ParentID    *UUID         `json:"parent_id,omitempty"`
    Type        DecisionType  `json:"type"`
    Input       json.RawMessage `json:"input"`
    Output      json.RawMessage `json:"output"`
    Reasoning   string        `json:"reasoning,omitempty"`
    ToolsUsed   []string      `json:"tools_used,omitempty"`
    DurationMs  uint64        `json:"duration_ms"`
    TokenUsage  *TokenUsage   `json:"token_usage,omitempty"`
    CreatedAt   Timestamp     `json:"created_at"`
}

type TokenUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

```go
// internal/model/entity.go

package model

import "encoding/json"

type Entity struct {
    ID         UUID            `json:"id"`
    Type       string          `json:"type"`
    Name       string          `json:"name"`
    Properties json.RawMessage `json:"properties"`
    Embedding  []float32       `json:"-"`
    Source     string          `json:"source,omitempty"`
    Confidence float32         `json:"confidence"`
    CreatedAt  Timestamp       `json:"created_at"`
    UpdatedAt  Timestamp       `json:"updated_at"`
}

type Relation struct {
    ID         UUID            `json:"id"`
    Type       string          `json:"type"`
    SourceID   UUID            `json:"source_id"`
    TargetID   UUID            `json:"target_id"`
    Properties json.RawMessage `json:"properties"`
    Weight     float32         `json:"weight"`
    CreatedAt  Timestamp       `json:"created_at"`
}

type DataLineage struct {
    DataID          UUID             `json:"data_id"`
    SourceType      string           `json:"source_type"`
    SourceIDs       []UUID           `json:"source_ids"`
    Transformations []Transformation `json:"transformations"`
    AgentDecisions  []UUID           `json:"agent_decisions,omitempty"`
    CreatedAt       Timestamp        `json:"created_at"`
}

type Transformation struct {
    Type   string         `json:"type"`
    Params map[string]any `json:"params"`
}
```

### 4.3 Key 编码方案

BadgerDB 按 key 字节序排列，合理编码 key 可实现高效前缀扫描和范围查询。

```go
// internal/storage/keyencode.go

package storage

import "encoding/binary"

// Key 编码规则：
//   [表前缀 1字节][字段类型 1字节][字段值 变长]
//
// 表前缀定义：
//   0x01 = agent_sessions
//   0x02 = agent_memories
//   0x03 = agent_decisions
//   0x04 = knowledge_entities
//   0x05 = knowledge_relations
//   0x06 = data_lineage
//   0x10 = vector_index
//   0x20 = graph_adjacency
//   0xFF = system_metadata

const (
    PrefixSession  byte = 0x01
    PrefixMemory   byte = 0x02
    PrefixDecision byte = 0x03
    PrefixEntity   byte = 0x04
    PrefixRelation byte = 0x05
    PrefixLineage  byte = 0x06
    PrefixVector   byte = 0x10
    PrefixGraph    byte = 0x20
    PrefixSystem   byte = 0xFF
)

// EncodeKey 编码 key: [prefix][id]
func EncodeKey(prefix byte, id string) []byte {
    key := make([]byte, 1+len(id))
    key[0] = prefix
    copy(key[1:], id)
    return key
}

// EncodeIndexKey 编码索引 key: [prefix][field][id]
// 用于二级索引，如 agent_id 索引 session
func EncodeIndexKey(prefix byte, field string, id string) []byte {
    key := make([]byte, 1+len(field)+1+len(id))
    key[0] = prefix
    copy(key[1:], field)
    key[1+len(field)] = 0x00 // 分隔符
    copy(key[2+len(field):], id)
    return key
}

// EncodeVectorKey 编码向量存储 key: [0x10][维度标记][id]
func EncodeVectorKey(indexName string, id string) []byte {
    key := make([]byte, 1+len(indexName)+1+len(id))
    key[0] = PrefixVector
    copy(key[1:], indexName)
    key[1+len(indexName)] = 0x00
    copy(key[2+len(indexName):], id)
    return key
}

// Float32sToBytes 将 float32 切片转为字节（用于向量存储）
func Float32sToBytes(floats []float32) []byte {
    bytes := make([]byte, len(floats)*4)
    for i, f := range floats {
        binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
    }
    return bytes
}

// BytesToFloat32s 将字节转回 float32 切片
func BytesToFloat32s(bytes []byte) []float32 {
    floats := make([]float32, len(bytes)/4)
    for i := range floats {
        floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(bytes[i*4:]))
    }
    return floats
}
```

---

## 五、数据模型与系统表

### 5.1 系统表 Schema

```sql
-- Agent 系统表
CREATE SYSTEM TABLE agent_sessions (
    id STRING PRIMARY KEY,
    agent_id STRING NOT NULL,
    state STRING NOT NULL,
    context JSON,
    metadata JSON,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_sessions_agent ON agent_sessions(agent_id);
CREATE INDEX idx_sessions_state ON agent_sessions(state);

CREATE SYSTEM TABLE agent_memories (
    id STRING PRIMARY KEY,
    session_id STRING NOT NULL,
    type STRING NOT NULL,
    content TEXT NOT NULL,
    embedding BLOB,               -- 向量以二进制存储，维度由索引定义
    importance FLOAT NOT NULL,
    access_count INT NOT NULL DEFAULT 0,
    associations JSON,
    created_at TIMESTAMP NOT NULL,
    accessed_at TIMESTAMP NOT NULL,
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id)
);

CREATE INDEX idx_memories_session ON agent_memories(session_id);
CREATE INDEX idx_memories_type ON agent_memories(type);

CREATE SYSTEM TABLE agent_decisions (
    id STRING PRIMARY KEY,
    session_id STRING NOT NULL,
    parent_id STRING,
    type STRING NOT NULL,
    input JSON NOT NULL,
    output JSON NOT NULL,
    reasoning TEXT,
    tools_used JSON,
    duration_ms INT NOT NULL,
    token_usage JSON,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id),
    FOREIGN KEY (parent_id) REFERENCES agent_decisions(id)
);

CREATE INDEX idx_decisions_session ON agent_decisions(session_id);
CREATE INDEX idx_decisions_parent ON agent_decisions(parent_id);

-- 知识图谱表
CREATE SYSTEM TABLE knowledge_entities (
    id STRING PRIMARY KEY,
    type STRING NOT NULL,
    name STRING NOT NULL,
    properties JSON NOT NULL,
    embedding BLOB,
    source STRING,
    confidence FLOAT NOT NULL DEFAULT 1.0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_entities_type ON knowledge_entities(type);
CREATE INDEX idx_entities_name ON knowledge_entities(name);

CREATE SYSTEM TABLE knowledge_relations (
    id STRING PRIMARY KEY,
    type STRING NOT NULL,
    source_id STRING NOT NULL,
    target_id STRING NOT NULL,
    properties JSON NOT NULL,
    weight FLOAT NOT NULL DEFAULT 1.0,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (source_id) REFERENCES knowledge_entities(id),
    FOREIGN KEY (target_id) REFERENCES knowledge_entities(id)
);

CREATE INDEX idx_relations_source ON knowledge_relations(source_id);
CREATE INDEX idx_relations_target ON knowledge_relations(target_id);
CREATE INDEX idx_relations_type ON knowledge_relations(type);

-- 数据血缘表
CREATE SYSTEM TABLE data_lineage (
    data_id STRING PRIMARY KEY,
    source_type STRING NOT NULL,
    source_ids JSON NOT NULL,
    transformations JSON NOT NULL,
    agent_decisions JSON,
    created_at TIMESTAMP NOT NULL
);
```

### 5.2 向量维度策略

向量维度**不硬编码**，由创建索引时指定：

```sql
-- 创建向量索引时指定维度
CREATE VECTOR INDEX idx_mem_embedding
    ON agent_memories(embedding)
    WITH (dimension = 1536, metric = 'cosine');

-- 支持不同模型的维度
CREATE VECTOR INDEX idx_entity_embedding
    ON knowledge_entities(embedding)
    WITH (dimension = 768, metric = 'l2');
```

支持的距离度量（自研实现）：
- `cosine` — 余弦相似度（默认，适合文本嵌入）
- `l2` — 欧几里得距离
- `dot` — 点积

---

## 六、并发与事务模型

### 6.1 BadgerDB 事务

BadgerDB 内置 MVCC 事务支持，直接使用：

```go
// 只读事务（快照隔离）
err := db.View(func(txn *badger.Txn) error {
    item, err := txn.Get(key)
    if err != nil {
        return err
    }
    return item.Value(func(val []byte) error {
        // 处理 val
        return nil
    })
})

// 读写事务
err := db.Update(func(txn *badger.Txn) error {
    if err := txn.Set(key, value); err != nil {
        return err
    }
    return nil
})
```

### 6.2 并发控制策略

| 场景 | 策略 | 说明 |
|------|------|------|
| 读-读 | 无锁 | BadgerDB MVCC 天然支持 |
| 读-写 | 无锁 | 快照隔离，读不阻塞写 |
| 写-写 | 乐观锁 | BadgerDB 事务冲突检测，冲突时重试 |
| 批量写入 | WriteBatch | BadgerDB WriteBatch API，原子提交 |
| 向量索引更新 | sync.RWMutex | 写入加写锁，搜索加读锁 |
| 图结构更新 | sync.RWMutex | 同上 |

### 6.3 读写路径

```
写入路径：
Client → Agent API → Model.Encode() → BadgerDB.Update() → WAL + MemTable → SST

读取路径：
Client → Agent API → Cache.Get() → [命中] → 返回
                           ↓ [未命中]
                    BadgerDB.View() → Decode() → Cache.Set() → 返回
```

---

## 七、查询语言设计

### 7.1 分阶段查询能力

**Phase 1 — Go 原生 API**
```go
// 创建会话
session, err := agent.CreateSession(ctx, &model.AgentSession{
    AgentID:  "agent-001",
    Metadata: map[string]any{"model": "gpt-4"},
})

// 记录记忆
mem, err := agent.StoreMemory(ctx, &model.MemoryEntry{
    SessionID: session.ID,
    Type:      model.MemoryLongTerm,
    Content:   "用户偏好中文回复",
    Importance: 0.8,
})

// 语义检索
memories, err := agent.RecallMemories(ctx, RecallOptions{
    Query:     "用户偏好",
    SessionID: session.ID,
    TopK:      5,
})
```

**Phase 2 — SQL 子集**
```sql
-- 基础 CRUD
INSERT INTO agent_memories (id, session_id, type, content, importance)
VALUES ('mem-001', 'sess-001', 'long_term', '用户偏好中文回复', 0.8);

-- 查询
SELECT * FROM agent_memories
WHERE session_id = 'sess-001' AND type = 'long_term'
ORDER BY importance DESC
LIMIT 10;

-- 聚合
SELECT type, COUNT(*) as count, AVG(importance) as avg_importance
FROM agent_memories
GROUP BY type;
```

**Phase 4 — 图查询（简化 Cypher 子集）**
```sql
MATCH (d:Device {id: 'pump-03'})-[:HAS_ALARM]->(a:Alarm)
WHERE a.time > now() - 24h
RETURN d.name, a.type, a.severity;
```

**Phase 5 — 向量搜索**
```sql
SELECT content, embedding <-> '[0.1, 0.2, ...]' AS distance
FROM agent_memories
WHERE session_id = 'sess-001'
ORDER BY distance
LIMIT 10;
```

**Phase 5 — 混合查询**
```sql
SELECT m.content, k.name, v.distance
FROM agent_memories m
JOIN knowledge_entities k ON m.association_id = k.id
VECTOR JOIN v ON m.embedding <-> v.query_embedding < 0.3
WHERE m.session_id = 'sess-001'
ORDER BY v.distance
LIMIT 20;
```

### 7.2 Agent 内置函数（Phase 3+）

```sql
-- Agent 记忆检索（语义搜索封装）
SELECT agent_recall(
    query := 'pump-03 的历史故障模式',
    session_id := 'session-123',
    top_k := 5
);

-- Agent 决策记录
SELECT agent_record_decision(
    session_id := 'session-123',
    type := 'anomaly_detection',
    input := '{"device": "pump-03"}',
    output := '{"anomaly": true}',
    reasoning := '温度超过阈值'
);
```

> **注意**：`agent_reason()` 等需要调用外部 LLM 的函数推迟到 Phase 9，需额外实现成本控制、超时、重试机制。

---

## 八、开发阶段规划

### Phase 1: 存储引擎与基础架构（6 周）

**目标**：基于 BadgerDB 构建核心存储层，实现数据读写链路。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 1.1 项目初始化 | Go module、目录结构、Makefile、CI（GitHub Actions） | 项目骨架 | 0.5 周 |
| 1.2 存储抽象层 | `Engine` 接口、BadgerDB 实现、Key 编码方案 | storage 包 | 1 周 |
| 1.3 数据模型 | model 包（Session/Memory/Decision/Entity/Relation）、序列化 | model 包 | 0.5 周 |
| 1.4 UUID v7 | 基于 `crypto/rand` + 时间戳的 UUID v7 生成 | util 包 | 0.5 周 |
| 1.5 Agent CRUD | Session/Memory/Decision 的增删改查、前缀扫描、索引维护 | agent 包 | 1.5 周 |
| 1.6 LRU 缓存 | 基于 `sync.RWMutex` + `container/list` 的 LRU 缓存 | storage/cache.go | 0.5 周 |
| 1.7 测试 | 单元测试、并发读写测试、崩溃恢复测试 | 测试套件 | 1.5 周 |

**里程碑**：可通过 Go API 完成 Agent 会话创建、记忆存储与检索、决策记录。
**验收标准**：
- 1000 并发 goroutine 读写无数据丢失
- `go test -race` 无竞态检测告警
- 进程强杀后重启数据完整
- 基准测试：单次写入 < 1ms，单次读取 < 0.5ms

---

### Phase 2: 查询引擎与 HTTP API（6 周）

**目标**：实现 SQL 解析器和 HTTP API。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 2.1 SQL Lexer | 基于 `text/scanner` 的词法分析器，支持关键字/标识符/字面量 | sql/lexer.go | 1 周 |
| 2.2 SQL Parser | 递归下降语法分析器，支持 SELECT/INSERT/UPDATE/DELETE | sql/parser.go + ast.go | 2 周 |
| 2.3 查询计划器 | 查询计划生成、谓词下推、索引选择 | sql/planner.go | 1 周 |
| 2.4 执行引擎 | 数据合并、过滤、投影、排序、聚合（count/sum/min/max/avg） | sql/executor.go | 1 周 |
| 2.5 HTTP API | 基于 `net/http` 的 RESTful API、路由、中间件（日志/panic 恢复） | api/http/ | 1 周 |

**里程碑**：可通过 HTTP API 执行 SQL 查询。
**验收标准**：
- 支持 SELECT/INSERT/UPDATE/DELETE 基本语法
- 支持 WHERE、ORDER BY、LIMIT、GROUP BY
- 简单查询 P99 < 10ms
- SQL 解析器测试覆盖率 > 90%

---

### Phase 3: Agent Runtime（4 周）

**目标**：实现完整的 Agent 运行时管理。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 3.1 Session Manager | 会话状态机、创建/恢复/暂停/完成、超时清理 | agent/session.go | 1 周 |
| 3.2 Memory Store | 短期记忆（滑窗）、长期记忆（向量索引）、工作记忆（共享） | agent/memory.go | 1.5 周 |
| 3.3 Decision Recorder | 决策树记录、工具调用记录、推理链、耗时统计 | agent/decision.go | 1 周 |
| 3.4 Agent API 端点 | HTTP 端点封装 Agent 操作 | api/http/handler_agent.go | 0.5 周 |

**里程碑**：Agent 可通过 API 创建会话、存储/检索记忆、记录决策链。
**验收标准**：
- 多轮对话场景记忆检索准确率 > 80%
- 决策树可完整回溯
- 会话超时自动清理

---

### Phase 4: 向量索引与搜索（4 周）

**目标**：自研 HNSW 向量索引，支持语义搜索。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 4.1 距离计算 | cosine、l2、dot 距离函数，SIMD 优化（可选） | vector/distance.go | 0.5 周 |
| 4.2 HNSW 索引 | 跳表图结构、插入/搜索/删除、可配置参数（M、efConstruction、efSearch） | vector/hnsw.go | 2 周 |
| 4.3 持久化 | HNSW 索引序列化到 BadgerDB、启动时加载 | vector/persist.go | 1 周 |
| 4.4 基准测试 | 万级/十万级向量的插入和搜索性能 | vector/benchmark_test.go | 0.5 周 |

**里程碑**：支持可配置维度的向量相似度搜索。
**验收标准**：
- 10 万向量（1536 维）top-10 搜索 < 50ms
- 召回率 > 95%（与暴力搜索对比）
- 支持增量插入和删除

---

### Phase 5: 知识图谱层（4 周）

**目标**：实现知识图谱存储和图查询。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 5.1 图存储 | 邻接表 + 倒排索引，基于 BadgerDB 持久化 | graph/store.go | 1.5 周 |
| 5.2 图遍历 | BFS/DFS、最短路径、K 跳邻居 | graph/traversal.go | 1 周 |
| 5.3 图查询解析 | 简化 Cypher 解析器（MATCH/WHERE/RETURN） | query/graph/parser.go | 1 周 |
| 5.4 数据血缘 | 血缘追踪、变更历史 | knowledge/lineage.go | 0.5 周 |

**里程碑**：支持知识图谱 CRUD 和图模式查询。
**验收标准**：
- 10 万节点图谱 2 跳查询 < 50ms
- 图遍历无无限循环

---

### Phase 6: 混合查询与 RAG（4 周）

**目标**：实现 SQL + 向量 + 图谱的混合查询。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 6.1 向量 SQL 扩展 | `<->` 距离运算符、ORDER BY distance | query/vector/executor.go | 1 周 |
| 6.2 混合查询计划 | 多模态查询优化、代价模型 | query/planner.go | 1.5 周 |
| 6.3 RAG 基础 | 文档分块、嵌入存储、语义检索、上下文组装 | agent/rag.go | 1.5 周 |

**里程碑**：支持混合查询。
**验收标准**：万级数据集混合查询 P99 < 100ms。

---

### Phase 7: 自适应存储（4 周）

**目标**：根据数据特征自动优化存储策略。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 7.1 Data Profiler | 数据分布、基数、大小分析 | storage/profiler.go | 1 周 |
| 7.2 Query Analyzer | 查询模式统计 | storage/analyzer.go | 1 周 |
| 7.3 Format Decider | 存储格式推荐 | storage/decider.go | 1 周 |
| 7.4 Dynamic Converter | 后台格式转换 | storage/converter.go | 1 周 |

**里程碑**：存储引擎自动优化。
**验收标准**：转换后特定工作负载查询性能提升 > 30%。

---

### Phase 8: 多 Agent 协作与安全（4 周）

**目标**：实现多 Agent 协作和权限控制。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 8.1 Multi-Agent Coordinator | 协作会话、任务分发、结果聚合、共享记忆 | agent/coordinator.go | 1.5 周 |
| 8.2 Task Queue | 优先级队列、依赖管理、重试、超时 | agent/taskqueue.go | 1 周 |
| 8.3 Permission Manager | Agent 角色、细粒度权限 | agent/permission.go | 1 周 |
| 8.4 Audit Logger | 操作日志、合规报告 | agent/audit.go | 0.5 周 |

---

### Phase 9: 外部集成（4 周）

**目标**：提供 MCP、CLI 和 Go SDK。

| 任务 | 内容 | 产出 | 周期 |
|------|------|------|------|
| 9.1 MCP Server | MCP 协议实现、工具注册 | api/mcp/server.go | 1.5 周 |
| 9.2 CLI 工具 | 基于 `flag` 的命令行工具 | cmd/cli/ | 1 周 |
| 9.3 Go SDK | 独立的客户端库包 | sdk/ | 1 周 |
| 9.4 文档 | API 文档、使用指南、示例 | docs/ | 0.5 周 |

---

### Phase 10: 高级功能（持续迭代）

| 功能 | 说明 | 优先级 |
|------|------|--------|
| agent_reason() SQL 函数 | 查询中调用 LLM 推理 | P2 |
| 自然语言查询 | NL → SQL 翻译 | P2 |
| 分布式支持 | 数据分片、副本同步（Raft） | P3 |
| 流式查询 | 实时数据流处理 | P3 |
| SIMD 向量计算 | 利用 Go 汇编优化距离计算 | P3 |

---

## 九、测试策略

### 9.1 测试金字塔

```
                    ┌───────────┐
                    │  E2E 测试  │  ← MCP 工具集成测试
                    │  (少量)    │
                ┌───┴───────────┴───┐
                │    集成测试        │  ← 多模块协作测试
                │    (适量)          │
            ┌───┴───────────────────┴───┐
                │        单元测试            │  ← 每个模块的核心逻辑
                │        (大量)              │
        ┌───┴───────────────────────────┴───┐
        │           并发测试                  │  ← goroutine 竞态检测
        │           (核心模块)               │
        └───────────────────────────────────┘
```

### 9.2 测试工具

| 测试类型 | 工具 | 说明 |
|----------|------|------|
| 单元测试 | `testing` 标准库 | `_test.go` 文件 |
| 表驱动测试 | `testing` + 匿名结构体 | Go 惯用模式 |
| 并发测试 | `go test -race` | 内置竞态检测器 |
| 并发压力测试 | `sync.WaitGroup` + 多 goroutine | 自行编写 |
| 基准测试 | `testing.B` | `go test -bench` |
| 临时目录 | `testing.T.TempDir()` | 测试隔离 |
| 断言辅助 | 自研 `internal/util/assert_test.go` | 仅测试代码使用 |

### 9.3 关键测试场景

```go
// 1. 并发读写无竞态
func TestConcurrentReadWrite(t *testing.T) {
    store := openTestStore(t)
    var wg sync.WaitGroup
    for i := 0; i < 1000; i++ {
        wg.Add(2)
        go func(i int) {
            defer wg.Done()
            store.Set(ctx, key(i), val(i))
        }(i)
        go func(i int) {
            defer wg.Done()
            store.Get(ctx, key(i))
        }(i)
    }
    wg.Wait()
}

// 2. 崩溃恢复
func TestCrashRecovery(t *testing.T) {
    dir := t.TempDir()
    store := openStore(dir)
    store.Set(ctx, []byte("key"), []byte("value"))
    store.Close() // 模拟正常关闭

    store2 := openStore(dir) // 重新打开
    val, _ := store2.Get(ctx, []byte("key"))
    assertEqual(t, val, []byte("value"))
}

// 3. 事务隔离
func TestTransactionIsolation(t *testing.T) {
    store := openTestStore(t)
    // 事务内写入，外部不可见
    tx := store.NewTransaction(true)
    tx.Set([]byte("key"), []byte("new"))

    val, _ := store.Get(ctx, []byte("key"))
    assertNotEqual(t, val, []byte("new")) // 未提交不可见

    tx.Commit()
    val, _ = store.Get(ctx, []byte("key"))
    assertEqual(t, val, []byte("new")) // 提交后可见
}
```

---

## 十、配置设计

```go
// config/config.go

package config

import (
    "encoding/json"
    "os"
    "time"
)

type Config struct {
    Server   ServerConfig   `json:"server"`
    Storage  StorageConfig  `json:"storage"`
    Agent    AgentConfig    `json:"agent"`
    Vector   VectorConfig   `json:"vector"`
    Log      LogConfig      `json:"log"`
}

type ServerConfig struct {
    Host         string        `json:"host"`
    Port         int           `json:"port"`
    ReadTimeout  time.Duration `json:"read_timeout"`
    WriteTimeout time.Duration `json:"write_timeout"`
}

type StorageConfig struct {
    DataDir          string `json:"data_dir"`
    ValueLogFileSize int64  `json:"value_log_file_size"`
    MemTableSize     int64  `json:"mem_table_size"`
    NumMemTables     int    `json:"num_mem_tables"`
    SyncWrites       bool   `json:"sync_writes"`
    CacheSizeMB      int    `json:"cache_size_mb"`
}

type AgentConfig struct {
    SessionTimeout    time.Duration `json:"session_timeout"`
    MaxMemories       int           `json:"max_memories"`
    ShortTermWindow   int           `json:"short_term_window"`
    CleanupInterval   time.Duration `json:"cleanup_interval"`
}

type VectorConfig struct {
    DefaultDimension  int     `json:"default_dimension"`
    DefaultMetric     string  `json:"default_metric"`
    HNSWM             int     `json:"hnsw_m"`
    HNSWEfConstruction int    `json:"hnsw_ef_construction"`
    HNSWEfSearch      int     `json:"hnsw_ef_search"`
}

type LogConfig struct {
    Level  string `json:"level"`
    Format string `json:"format"` // "text" or "json"
}

// Load 从 JSON 文件加载配置
func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return Default(), nil // 返回默认配置
    }
    cfg := Default()
    if err := json.Unmarshal(data, cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}

// Default 返回默认配置
func Default() *Config {
    return &Config{
        Server: ServerConfig{
            Host:         "0.0.0.0",
            Port:         8400,
            ReadTimeout:  30 * time.Second,
            WriteTimeout: 30 * time.Second,
        },
        Storage: StorageConfig{
            DataDir:          "./data",
            ValueLogFileSize: 1 << 26, // 64MB
            MemTableSize:     1 << 24, // 16MB
            NumMemTables:     3,
            SyncWrites:       true,
            CacheSizeMB:      256,
        },
        Agent: AgentConfig{
            SessionTimeout:  24 * time.Hour,
            MaxMemories:     10000,
            ShortTermWindow: 50,
            CleanupInterval: 1 * time.Hour,
        },
        Vector: VectorConfig{
            DefaultDimension:   1536,
            DefaultMetric:      "cosine",
            HNSWM:              16,
            HNSWEfConstruction: 200,
            HNSWEfSearch:       64,
        },
        Log: LogConfig{
            Level:  "info",
            Format: "text",
        },
    }
}
```

---

## 十一、关键里程碑时间线

```
Week 1-6    ████████████████████  Phase 1: 存储引擎与基础架构
Week 7-12   ████████████████████  Phase 2: 查询引擎与 HTTP API
Week 13-16  ████████████████      Phase 3: Agent Runtime
Week 17-20  ████████████████      Phase 4: 向量索引与搜索
Week 21-24  ████████████████      Phase 5: 知识图谱层
Week 25-28  ████████████████      Phase 6: 混合查询与 RAG
Week 29-32  ████████████████      Phase 7: 自适应存储
Week 33-36  ████████████████      Phase 8: 多 Agent 协作与安全
Week 37-40  ████████████████      Phase 9: 外部集成
Week 41+    ────────────────────  Phase 10: 高级功能（持续迭代）
```

**核心链路预计周期**：40 周（约 10 个月）
**最小可用版本（Phase 1-3）**：16 周（约 4 个月）

---

## 十二、v0.1 最小可用版本（MVP）

Phase 1-3 完成后即为 MVP：

### MVP 能力

- ✅ Agent 会话创建、恢复、状态管理
- ✅ 短期/长期/工作记忆的存储与检索
- ✅ 决策链记录与查询
- ✅ SQL 查询（SELECT/INSERT/UPDATE/DELETE）
- ✅ HTTP API
- ✅ 崩溃恢复与数据持久化
- ✅ 并发安全

### MVP 不具备的能力

- ❌ 向量搜索（Phase 4）
- ❌ 知识图谱（Phase 5）
- ❌ 混合查询（Phase 6）
- ❌ 自适应存储（Phase 7）
- ❌ 多 Agent 协作（Phase 8）
- ❌ MCP/CLI/SDK（Phase 9）

### MVP 使用场景

1. **单 Agent 记忆管理**：Agent 通过 API 存储和检索对话记忆
2. **决策审计**：记录 Agent 的工具调用和推理链，用于调试和分析
3. **结构化数据存储**：替代 SQLite 作为 Agent 的本地数据存储

---

## 十三、未来演进方向

### 分布式架构（Phase 10+）

当前设计为单节点架构。未来分布式演进路径：

- **数据分片**：基于 agent_id 的一致性哈希分片
- **副本同步**：Raft 共识协议（可参考 etcd 的 Raft 实现）
- **读写分离**：主节点写入，只读副本处理查询

### 与现有系统集成

| 系统 | 集成方式 |
|------|----------|
| LangChain | Go SDK 作为 Memory 和 VectorStore 后端 |
| AutoGen | 作为 Agent 消息存储 |
| Cursor/Claude | 通过 MCP Server 集成 |

---

## 十四、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 自研 HNSW 质量 | 向量搜索召回率 | 严格基准测试，与暴力搜索对比验证 |
| SQL 解析器完整性 | 兼容性 | 先支持 SQL 子集，渐进扩展 |
| BadgerDB 大 Value 性能 | 向量存储场景 | 大 Value 分片存储或独立文件 |
| 单点故障 | 可用性 | MVP 阶段接受，Phase 10+ 实现分布式 |
| Go GC 延迟 | 尾延迟 | 控制堆上对象数量，复用 buffer |

---

> **AgentNativeDB** — 以 Agent 为核心设计目标的数据库系统。
>
> Go 实现 | 最小依赖 | 首个 MVP 预计 4 个月可用。
