# AgentNativeDB

> Database + Agent Runtime + Knowledge Graph
>
> 以 Agent 为核心设计目标的数据库系统。

## 特性

- **Agent 原生数据管理** — 会话、记忆、决策作为一等公民
- **SQL 查询引擎** — 支持 SELECT/INSERT/UPDATE/DELETE、WHERE、ORDER BY、LIMIT、GROUP BY、聚合函数、JOIN、全文搜索
- **向量索引** — 自研 HNSW 实现，支持余弦/L2/点积距离
- **知识图谱** — 邻接表存储，BFS/K跳/最短路径查询
- **MCP Server** — 兼容 Model Context Protocol，可与 Cursor/Claude Desktop 集成
- **数据血缘** — 追踪数据来源和变换历史
- **多 Agent 协作** — 协作房间、消息传递、共享记忆
- **最小依赖** — 唯一外部依赖：BadgerDB（纯 Go KV 存储）
- **HTTP 客户端** — 支持命令历史、自动补全、颜色输出、多行SQL、JSON导出

## 快速开始

```bash
# 构建
make build

# 启动 HTTP API 服务
./bin/server -mode server

# 启动 MCP Server（stdio 传输）
./bin/server -mode mcp

# 交互式 SQL CLI（本地）
./bin/cli

# HTTP 客户端（连接远程服务器）
./bin/client -server localhost:8400

# 客户端功能演示
./bin/client -server localhost:8400 -format json sessions
./bin/client -server localhost:8400 export sessions sessions.json

# 运行测试
make test

# 基准测试
make bench

# 测试客户端
make test-client
```

## SQL 示例

```sql
-- 基础查询
SELECT * FROM agent_sessions WHERE state = 'active';

-- 聚合
SELECT agent_id, COUNT(*) as cnt FROM agent_sessions GROUP BY agent_id;

-- JOIN
SELECT s.agent_id, m.content
FROM agent_sessions s
JOIN agent_memories m ON s.id = m.session_id
WHERE m.importance > 0.7;

-- 排序分页
SELECT * FROM agent_memories ORDER BY importance DESC LIMIT 10;

-- 全文搜索索引（FULLTEXT 是倒排索引 INVERTED 的语法别名）
CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT);
CREATE FULLTEXT INDEX idx_docs_body ON docs(body);
SELECT * FROM docs WHERE MATCH(body) AGAINST ('agent memory');
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/sessions` | 创建会话 |
| GET | `/api/v1/sessions/{id}` | 获取会话 |
| PATCH | `/api/v1/sessions/{id}` | 更新会话 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| GET | `/api/v1/sessions` | 列出会话 |
| POST | `/api/v1/memories` | 存储记忆 |
| GET | `/api/v1/memories?session_id=` | 列出记忆 |
| POST | `/api/v1/decisions` | 记录决策 |
| GET | `/api/v1/decisions?session_id=` | 列出决策 |
| GET | `/api/v1/decisions/{id}/tree` | 决策树 |
| POST | `/api/v1/query` | SQL 查询 |
| GET | `/health` | 健康检查 |

## MCP 工具

| 工具 | 说明 |
|------|------|
| `query_sql` | 执行 SQL 查询 |
| `create_session` | 创建 Agent 会话 |
| `store_memory` | 存储 Agent 记忆 |
| `recall_memories` | 检索 Agent 记忆 |
| `record_decision` | 记录 Agent 决策 |

## 架构

```
┌─────────────────────────────────────────────┐
│              API Layer                      │
│   HTTP REST  │  MCP Server  │  CLI         │
├─────────────────────────────────────────────┤
│           Agent Runtime                     │
│  Session │ Memory │ Decision │ Coordinator │
├─────────────────────────────────────────────┤
│          Unified Query Layer                │
│   SQL Engine  │  Graph Query │ Vector Search│
├─────────────────────────────────────────────┤
│          Storage Engine                     │
│   BadgerDB  │  HNSW Index  │ Graph Store   │
└─────────────────────────────────────────────┘
```

## 项目结构

```
AgentNativeDB/
├── cmd/server/          HTTP + MCP 服务入口
├── cmd/cli/             交互式 SQL CLI（本地）
├── cmd/client/          HTTP 客户端（远程连接）
├── api/http/            RESTful API
├── api/mcp/             MCP Server
├── config/              配置管理
├── internal/
│   ├── storage/         存储引擎抽象 + LRU 缓存
│   ├── model/           数据模型
│   ├── util/            UUID v7
│   ├── agent/           Agent Runtime
│   ├── query/sql/       SQL 引擎
│   ├── vector/          HNSW 向量索引
│   ├── graph/           图存储
│   └── knowledge/       数据血缘
├── docs/                设计文档
├── test_client.sh       客户端测试脚本
└── config.example.json  示例配置文件
```

## 技术选型

| 组件 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.23+ | 单二进制部署，无运行时依赖 |
| KV 存储 | BadgerDB | 纯 Go 实现，无 CGO |
| 向量索引 | 自研 HNSW | 支持 cosine/l2/dot 距离 |
| 图存储 | 自研邻接表 | 基于 BadgerDB 持久化 |
| SQL 解析 | 自研递归下降 | 支持完整 SQL 子集 |

## 许可证

MIT
