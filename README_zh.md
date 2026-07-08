<p align="center">
  <img src="docs/assets/logo.png" alt="AgentNativeDB" width="128" height="128">
</p>

<h1 align="center">AgentNativeDB</h1>

<p align="center">
  <strong>🤖 Agent 原生数据库 — 会话、记忆、决策作为一等公民</strong>
</p>

<p align="center">
  以 Agent 为核心设计目标的数据库系统，从零构建。<br>
  融合 SQL 查询引擎、向量搜索（HNSW）、图存储、知识/血缘追踪和 MCP Server 集成 —— 打包为单个 Go 二进制文件。
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/andb-installer"><img src="https://img.shields.io/npm/dm/andb-installer.svg" alt="npm 下载量"></a>
  <a href="https://pypi.org/project/andb-installer/"><img src="https://img.shields.io/pypi/v/andb-installer.svg" alt="PyPI 版本"></a>
  <a href="https://github.com/startvibecoding/AgentNativeDB/releases/latest"><img src="https://img.shields.io/github/release/startvibecoding/AgentNativeDB.svg" alt="GitHub release"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://goreportcard.com/report/github.com/startvibecoding/AgentNativeDB"><img src="https://goreportcard.com/badge/github.com/startvibecoding/AgentNativeDB" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/startvibecoding/AgentNativeDB"><img src="https://pkg.go.dev/badge/github.com/startvibecoding/AgentNativeDB?status.svg" alt="GoDoc"></a>
</p>

---

## ✨ 为什么选择 AgentNativeDB？

**痛点：** AI Agent 需要持久记忆、结构化数据、向量搜索和知识图谱 —— 但把多个数据库拼凑在一起既脆弱又复杂。

**方案：** AgentNativeDB 是 **Agent 原生数据库**，将所有能力整合为单一二进制文件。会话、记忆和决策是内建的一等数据类型，而非后期拼接。

### 🎯 核心特性

| 特性 | 说明 |
|------|------|
| **🤖 Agent 原生 Schema** | 会话、记忆、决策、任务作为内建表类型，拥有优化存储 |
| **⚡ SQL 查询引擎** | 自研递归下降解析器 —— SELECT/INSERT/UPDATE/DELETE、WHERE、JOIN、GROUP BY、ORDER BY、LIMIT、聚合函数 |
| **🔍 向量搜索** | 自研 HNSW 索引，支持余弦/L2/点积距离 —— 零第三方向量库 |
| **🕸️ 图存储** | 邻接表存储，支持 BFS、K跳、最短路径查询 —— 全部持久化于 BadgerDB |
| **📊 数据血缘** | 通过知识层追踪数据来源和变换历史 |
| **🔗 MCP Server** | Model Context Protocol（stdio）—— 对接 Cursor、Claude Desktop 或任何 MCP 兼容客户端 |
| **🌐 HTTP API** | RESTful 端点，覆盖会话、记忆、决策和 SQL 查询 |
| **💻 交互式 CLI** | 本地 SQL REPL，支持语法高亮、命令历史和自动补全 |
| **🖥️ Web UI** | Svelte 构建的管理面板，内嵌于二进制文件 —— 表管理、数据可视化、SQL 编辑器 |
| **📦 纯 Go** | 单二进制部署，零 CGO，零外部运行时依赖 —— 唯一核心依赖：BadgerDB（纯 Go KV 存储） |

---

## 🚀 30 秒快速开始

```bash
# 安装（任选一种）
npm install -g andb-installer               # npm
pip install andb-installer                  # PyPI / pipx
go install github.com/startvibecoding/AgentNativeDB/cmd/andb@latest  # Go

# 从源码构建
git clone https://github.com/startvibecoding/AgentNativeDB.git
cd AgentNativeDB
make build
```

**支持平台：** Linux（x86_64、arm64）、macOS（x86_64、arm64）、Windows（x86_64）

### 运行

```bash
# HTTP 服务（默认 0.0.0.0:8400）
./bin/andb server

# MCP Server（stdio 传输）
./bin/andb server -mode mcp

# 交互式 SQL CLI（本地）
./bin/andb cli

# HTTP 客户端（连接远程服务器）
./bin/andb client -server localhost:8400

# 版本
./bin/andb version
```

**卸载：**

```bash
npm uninstall -g andb-installer
pip uninstall andb-installer
```

---

## 🎮 SQL 查询示例

```sql
-- 建表
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

-- 全文搜索（倒排索引）
CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT);
CREATE FULLTEXT INDEX idx_docs_body ON docs(body);
SELECT * FROM docs WHERE MATCH(body) AGAINST ('agent memory');

-- 向量搜索
CREATE TABLE embeddings (id VARCHAR(64) PRIMARY KEY, embedding FLOAT);
CREATE VECTOR INDEX idx_emb ON embeddings(embedding) WITH (dimensions=128);
SELECT * FROM vector_search(idx_emb, '[0.1, 0.2, ...]', 10);

-- 图查询
CREATE GRAPH TABLE edges (src VARCHAR(64), dst VARCHAR(64));
SELECT * FROM graph_bfs(edges, 'node_001', 3);
SELECT * FROM graph_shortest_path(edges, 'node_001', 'node_010');
```

---

## 🏗️ 架构

```
┌─────────────────────────────────────────────────┐
│                  API 层                          │
│   HTTP REST  │  MCP Server  │  CLI  │  Web UI   │
├─────────────────────────────────────────────────┤
│              Agent 运行时                        │
│  Session │ Memory │ Decision │ Coordinator │ Audit│
├─────────────────────────────────────────────────┤
│             统一查询层                            │
│   SQL 引擎  │  图查询  │  向量搜索               │
├─────────────────────────────────────────────────┤
│              存储引擎                            │
│   BadgerDB  │  HNSW 索引  │  图存储              │
└─────────────────────────────────────────────────┘
```

### 项目结构

```
AgentNativeDB/
├── cmd/andb/                 # 单一入口点（server、cli、client、version）
├── api/
│   ├── http/                 # RESTful HTTP API
│   └── mcp/                  # MCP Server（stdio 传输）
├── config/                   # 配置管理
├── internal/
│   ├── storage/              # 存储引擎抽象 + LRU 缓存
│   ├── storage/badger/       # BadgerDB 实现
│   ├── model/                # 核心数据类型（Session、Memory、Decision、Entity）
│   ├── agent/                # Agent 运行时（session、memory、decision、RAG、audit）
│   ├── query/sql/            # SQL 引擎（lexer → parser → planner → executor）
│   ├── query/sql/index/      # 二级索引（Hash、BTree、Inverted/FullText）
│   ├── query/graph/          # 图查询层
│   ├── query/vector/         # 向量查询层
│   ├── vector/               # HNSW 向量索引
│   ├── graph/                # 图存储（邻接表、BFS、K跳）
│   ├── knowledge/            # 数据血缘追踪
│   └── util/                 # UUID v7 生成
├── sdk/                      # Go SDK
├── ui/                       # Svelte + Vite Web UI（内嵌于二进制文件）
├── docs/                     # 设计文档
└── examples/                 # 示例脚本
```

---

## 📚 API 参考

### HTTP 端点

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

### MCP 工具

| 工具 | 说明 |
|------|------|
| `query_sql` | 执行 SQL 查询 |
| `create_session` | 创建 Agent 会话 |
| `store_memory` | 存储 Agent 记忆 |
| `recall_memories` | 检索 Agent 记忆 |
| `record_decision` | 记录 Agent 决策 |

---

## 🛠️ 内建命令

| 命令 | 说明 |
|------|------|
| `./bin/andb server` | 启动 HTTP API 服务（默认 0.0.0.0:8400） |
| `./bin/andb server -mode mcp` | 启动 MCP Server（stdio 传输） |
| `./bin/andb cli` | 交互式 SQL REPL（本地数据库） |
| `./bin/andb client -server host:port` | HTTP 客户端（连接远程） |
| `./bin/andb version` | 显示版本 |

---

## 🔧 配置

### 配置文件

| 位置 | 平台 | 范围 |
|------|------|------|
| `config.json` | 所有 | 项目级配置 |

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

### 命令行参数

```bash
./bin/andb server                    # 默认：0.0.0.0:8400
./bin/andb server -host 127.0.0.1    # 绑定本地
./bin/andb server -port 9000         # 自定义端口
./bin/andb cli                       # 默认数据目录
./bin/andb cli -data /path/to/data   # 自定义数据目录
./bin/andb client -server host:port  # 连接远程
```

---

## 🛠️ 开发

```bash
# 构建 Web UI（Vite）然后构建二进制文件
make build

# 仅构建 Go 二进制文件（不重建 UI）
go build -o bin/andb ./cmd/andb

# 启动服务
make run

# 运行测试
make test

# 带竞态检测的测试
make race

# 基准测试
make bench

# Lint / 格式化 / 静态分析
make lint

# 完整检查（fmt + vet + test）
make check

# 清理构建产物
make clean
```

---

## 📊 技术选型

| 组件 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.23+ | 单二进制部署，零运行时依赖 |
| KV 存储 | BadgerDB | 纯 Go 实现，无 CGO，内置 LSM-tree、WAL、MVCC |
| 向量索引 | 自研 HNSW | 支持 cosine/l2/dot 距离，零外部依赖 |
| 图存储 | 自研邻接表 | 基于 BadgerDB 持久化 |
| SQL 引擎 | 自研递归下降 | 支持完整 SQL 子集 |
| Web UI | Svelte + Vite | 构建时内嵌于二进制文件 |
| 协议 | MCP（stdio） | 标准 Agent-Tool 集成协议 |

---

## 🤝 贡献

欢迎贡献！项目约定参见 [AGENTS.md](AGENTS.md)。

```bash
git clone https://github.com/startvibecoding/AgentNativeDB.git
cd AgentNativeDB
make build
make test
```

---

## 📄 许可证

MIT —— 详见 [LICENSE](LICENSE)。

---

<p align="center">
  <strong>准备好构建 Agent 原生数据基础设施了吗？⭐ Star 本仓库，立即开始！</strong>
</p>
