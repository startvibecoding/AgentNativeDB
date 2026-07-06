// 国际化模块 — 中文 / English

const messages = {
  zh: {
    // 导航
    'nav.dashboard': '仪表盘',
    'nav.tables': '表管理',
    'nav.sql': 'SQL 查询',
    'nav.sessions': '会话',
    'nav.memories': '记忆',
    'nav.decisions': '决策',
    'nav.vectors': '向量',
    'nav.aiNative': 'AI Native',
    'nav.aiDashboard': 'AI 仪表盘',
    'ai.title': 'AI Native 运行中心',
    'ai.overview': '运行概览',
    'ai.tutorial': '快速开始 (可复制给 AI)',
    'ai.tab.cli': 'CLI / 嵌入式 SDK',
    'ai.tab.http': 'HTTP / MCP',
    'ai.tutorial.cli': `# AgentNativeDB CLI / 嵌入式 SDK 快速开始

## 1. 启动交互式 CLI

\`\`\`bash
./bin/andb cli              # 本地 BadgerDB,数据目录默认 ./data
./bin/andb cli -data ./mydb # 指定数据目录
\`\`\`

在 CLI 里可以直接执行 SQL:
\`\`\`sql
CREATE TABLE users (id STRING PRIMARY KEY, name VARCHAR(100));
INSERT INTO users (id, name) VALUES ('u1', 'Alice');
SELECT * FROM users;
\`\`\`

## 2. 嵌入式 SDK (Go)

\`\`\`go
import agentnativedb "github.com/startvibecoding/AgentNativeDB"

db, err := agentnativedb.Open("./data")
if err != nil { panic(err) }
defer db.Close()

// 会话
sess, _ := db.CreateSession("my-agent", map[string]any{"task": "coding"})

// 记忆
mem, _ := db.StoreMemory(sess.ID(), "用户偏好简体中文", agentnativedb.LongTerm, 0.9)

// 决策
_, _ = db.RecordDecision(sess.ID(), agentnativedb.Reasoning,
  "如何优化 SQL", "增加索引", "分析了执行计划")

// 向量 + payload
db.CreateIndex("docs", 128, "cosine")
db.InsertVectorWithPayload("docs", "doc-1",
  []float32{0.1, 0.2, ...},
  []byte(\`{"title":"示例文档"}\`))
results, _ := db.SearchVector("docs", []float32{0.1, 0.2, ...}, 5)

// SQL
result, _ := db.Query("SELECT * FROM sessions WHERE state = 'active'")
\`\`\`

## 完整调用链路

1. 创建 Session
2. 运行中持续写入 Memory + Decision
3. 需要时通过向量索引检索相关 Memory
4. 任务结束 CloseSession
5. 用 SQL 做离线统计/审计
`,
    'ai.tutorial.http': `# AgentNativeDB MCP over HTTP 接入指南(给 AI 看)

这是面向另一个 AI 客户端的接入文档。AgentNativeDB 在启动 HTTP 服务后,会在 \`POST /mcp\` 暴露一个 MCP(Model Context Protocol)JSON-RPC 端点,任何支持 MCP HTTP 传输的 AI 都可以直接调用。

## 1. 启动服务

\`\`\`bash
./bin/andb server
# 默认监听 0.0.0.0:8400
# MCP 端点: POST http://localhost:8400/mcp
\`\`\`

## 2. 通信协议

- 传输: HTTP POST
- Content-Type: application/json
- 消息格式: JSON-RPC 2.0
- 端点: \`http://{host}:{port}/mcp\`

## 3. 标准握手(initialize)

\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "your-ai-client", "version": "1.0.0"}
    }
  }'
\`\`\`

返回示例:
\`\`\`json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {"tools": {}},
    "serverInfo": {"name": "AgentNativeDB", "version": "0.1.0"}
  }
}
\`\`\`

## 4. 获取可用 tools

\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
\`\`\`

返回 5 个 tools:
- \`query_sql\`: 执行 SQL(SELECT/INSERT/UPDATE/DELETE 等)
- \`create_session\`: 创建会话,需要 agent_id
- \`store_memory\`: 存储记忆,需要 session_id + content
- \`recall_memories\`: 检索记忆,需要 session_id
- \`record_decision\`: 记录决策,需要 session_id + type + input + output

## 5. 调用 tool 示例

### 执行 SQL
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "query_sql",
      "arguments": {
        "sql": "SELECT * FROM sessions WHERE state = '\''active'\'' LIMIT 10"
      }
    }
  }'
\`\`\`

### 创建会话
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "create_session",
      "arguments": {"agent_id": "my-agent"}
    }
  }'
\`\`\`

### 存储记忆
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "store_memory",
      "arguments": {
        "session_id": "SESS_ID",
        "content": "用户偏好简体中文",
        "type": "long_term",
        "importance": 0.9
      }
    }
  }'
\`\`\`

### 检索记忆
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 6,
    "method": "tools/call",
    "params": {
      "name": "recall_memories",
      "arguments": {"session_id": "SESS_ID", "type": "long_term", "limit": 10}
    }
  }'
\`\`\`

### 记录决策
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 7,
    "method": "tools/call",
    "params": {
      "name": "record_decision",
      "arguments": {
        "session_id": "SESS_ID",
        "type": "reasoning",
        "input": {"question": "如何优化 SQL"},
        "output": {"answer": "增加索引"},
        "reasoning": "分析了执行计划"
      }
    }
  }'
\`\`\`

## 6. 给 AI 的调用模板

当 AI 需要操作 AgentNativeDB 时,按下面 JSON-RPC 模板构造请求:

\`\`\`json
{
  "jsonrpc": "2.0",
  "id": "<递增整数或字符串>",
  "method": "tools/call",
  "params": {
    "name": "<tool_name>",
    "arguments": {
      // 根据 tools/list 返回的 inputSchema 填写
    }
  }
}
\`\`\`

所有 tool 调用结果都在 \`result.content[0].text\` 中,错误时 \`result.isError\` 为 true。

## 7. stdio 模式(可选)

如果你的 AI 客户端只支持 stdio MCP(如 Cursor / Claude Desktop),则使用:

\`\`\`bash
./bin/andb server -mode mcp
\`\`\`

此时每行输入输出都是一个 JSON-RPC 对象,方法同上。
`,
    'ai.copy': '复制 Markdown',
    'ai.stats.activeSessions': '活跃会话',
    'ai.stats.totalMemories': '总记忆数',
    'ai.stats.totalDecisions': '总决策数',
    'ai.stats.avgDecisionTime': '平均决策耗时',
    'ai.stats.topAgents': 'Top Agents',
    'ai.stats.recentDecisions': '最近决策',
    'ai.stats.memoryByType': '记忆类型分布',

    // 状态
    'status.connected': '已连接',
    'status.disconnected': '未连接',
    'status.ok': '运行中',
    'status.loading': '加载中…',

    // 通用
    'common.refresh': '刷新',
    'common.create': '创建',
    'common.cancel': '取消',
    'common.confirm': '确认',
    'common.delete': '删除',
    'common.close': '关闭',
    'common.add': '添加',
    'common.execute': '执行',
    'common.executing': '执行中…',
    'common.total': '共',
    'common.units.items': '项',
    'common.units.rows': '行',
    'common.units.cols': '列',
    'common.units.times': '次',
    'common.units.ms': 'ms',
    'common.empty': '暂无数据',
    'common.noSelection': '选择查看详情',
    'common.filter': '过滤…',
    'common.version': '版本',
    'common.language': '语言',

    // Dashboard
    'dashboard.title': '概览',
    'dashboard.tables': '数据表',
    'dashboard.sessions': '会话',
    'dashboard.memories': '记忆',
    'dashboard.decisions': '决策',
    'dashboard.serverStatus': '服务器状态',
    'dashboard.engine': '引擎',
    'dashboard.engineValue': 'BadgerDB',
    'dashboard.protocol': '协议',
    'dashboard.protocolValue': 'HTTP REST',
    'dashboard.tablesEmpty': '暂无数据表',
    'dashboard.tablesHint': '使用 SQL CREATE TABLE 创建',
    'dashboard.quickActions': '快速操作',
    'dashboard.action.sql': 'SQL 查询',
    'dashboard.action.tables': '管理表',
    'dashboard.action.sessions': '会话',
    'dashboard.action.memories': '记忆',

    // Tables
    'tables.create': '创建表',
    'tables.createTitle': '创建新表',
    'tables.tableName': '表名',
    'tables.columns': '列定义',
    'tables.addColumn': '添加列',
    'tables.column': '列名',
    'tables.primaryKey': '主键',
    'tables.structure': '表结构',
    'tables.data': '数据',
    'tables.dataHint': '最多 100 行',
    'tables.dataEmpty': '表中暂无数据',
    'tables.editSchema': '编辑表结构',
    'tables.schemaEditTitle': '编辑表结构',
    'tables.saveSchema': '保存更改',
    'tables.addCol': '添加列',
    'tables.dropCol': '删除',
    'tables.modifyCol': '修改',
    'tables.backToList': '返回列表',
    'tables.operation': '操作',
    'tables.edit': '编辑',
    'tables.where': 'WHERE (可选)',
    'tables.order': 'ORDER BY (可选)',
    'tables.pageSize': '每页行数',
    'tables.prevPage': '上一页',
    'tables.nextPage': '下一页',
    'tables.insertRow': '添加行',
    'tables.saveNewRow': '保存新增',
    'tables.cancelInsert': '取消新增',
    'tables.dropConfirm': '确认删除',
    'tables.dropMessage': '确定要删除表',
    'tables.dropHint': '吗？此操作不可恢复。',
    'tables.selectHint': '选择左侧的表查看详情',
    'tables.actions': '操作',
    'tables.viewData': '查看数据',
    'tables.editData': '编辑数据',
    'tables.viewDataTitle': '查看数据',
    'tables.editDataTitle': '编辑数据',
    'tables.backToList': '返回列表',
    'tables.operation': '操作',
    'tables.edit': '编辑',
    'tables.indexes': '索引',
    'tables.indexName': '索引名',
    'tables.indexColumn': '索引列',
    'tables.indexType': '索引类型',
    'tables.indexTypes': '索引类型',
    'tables.createIndex': '新建索引',
    'tables.dropIndex': '删除索引',
    'tables.noIndexes': '暂无索引',
    'tables.indexExists': '索引已存在',
    'tables.indexCreated': '索引创建成功',
    'tables.fulltextIndex': '全文索引',

    // SQL
    'sql.editor': 'SQL 编辑器',
    'sql.shortcut': '⌘ Enter 执行',
    'sql.placeholder': '输入 SQL 语句…',
    'sql.result': '查询结果',
    'sql.affected': '影响',
    'sql.success': '语句执行成功',
    'sql.emptyResult': '查询结果为空',
    'sql.history': '执行历史',
    'sql.historyEmpty': '暂无历史',
    'sql.presets.tables': '所有表',
    'sql.presets.indexes': '所有索引',
    'sql.presets.sessions': '查看会话',
    'sql.presets.memories': '查看记忆',
    'sql.presets.decisions': '查看决策',
    'sql.presets.countSessions': '统计会话',
    'sql.presets.countMemories': '统计记忆',
    'sql.presets.fulltext': '全文搜索',

    // Sessions
    'sessions.create': '创建会话',
    'sessions.createTitle': '创建新会话',
    'sessions.agentId': 'Agent ID',
    'sessions.agentIdHint': '例如: assistant-01',
    'sessions.detail': '会话详情',
    'sessions.id': 'ID',
    'sessions.agent': 'Agent',
    'sessions.state': '状态',
    'sessions.createdAt': '创建时间',
    'sessions.updatedAt': '更新时间',
    'sessions.actions': '操作',
    'sessions.activate': '激活',
    'sessions.pause': '暂停',
    'sessions.complete': '完成',
    'sessions.context': '上下文',
    'sessions.metadata': '元数据',
    'sessions.empty': '暂无会话',
    'sessions.selectHint': '选择会话查看详情',
    'sessions.filterHint': '按 Agent 过滤…',

    // Memories
    'memories.add': '添加记忆',
    'memories.addTitle': '添加新记忆',
    'memories.type': '记忆类型',
    'memories.type.short': '短期',
    'memories.type.long': '长期',
    'memories.type.working': '工作',
    'memories.content': '内容',
    'memories.contentPlaceholder': '输入记忆内容…',
    'memories.importance': '重要度',
    'memories.accessCount': '访问',
    'memories.empty': '暂无记忆',
    'memories.selectHint': '请先选择一个会话',
    'memories.selectSession': '选择会话…',

    // Decisions
    'decisions.tree': '决策树',
    'decisions.detail': '决策详情',
    'decisions.type': '类型',
    'decisions.type.reasoning': '推理',
    'decisions.type.tool_call': '工具调用',
    'decisions.type.planning': '规划',
    'decisions.type.reflection': '反思',
    'decisions.duration': '耗时',
    'decisions.createdAt': '创建时间',
    'decisions.reasoning': '推理过程',
    'decisions.toolsUsed': '使用工具',
    'decisions.input': '输入',
    'decisions.output': '输出',
    'decisions.empty': '暂无决策',
    'decisions.selectHint': '请先选择一个会话',
    'decisions.selectSession': '选择会话…',
    'decisions.viewTree': '查看树',
  },

  en: {
    // Navigation
    'nav.dashboard': 'Dashboard',
    'nav.tables': 'Tables',
    'nav.sql': 'SQL',
    'nav.sessions': 'Sessions',
    'nav.memories': 'Memories',
    'nav.decisions': 'Decisions',
    'nav.vectors': 'Vectors',
    'nav.aiNative': 'AI Native',
    'nav.aiDashboard': 'AI Dashboard',
    'ai.title': 'AI Native Dashboard',
    'ai.overview': 'Overview',
    'ai.tutorial': 'Quick Start (Copy for AI)',
    'ai.tab.cli': 'CLI / Embedded SDK',
    'ai.tab.http': 'HTTP / MCP',
    'ai.tutorial.cli': `# AgentNativeDB CLI / Embedded SDK Quick Start

## 1. Start Interactive CLI

\`\`\`bash
./bin/andb cli              # local BadgerDB, default ./data
./bin/andb cli -data ./mydb # custom data directory
\`\`\`

Run SQL in CLI:
\`\`\`sql
CREATE TABLE users (id STRING PRIMARY KEY, name VARCHAR(100));
INSERT INTO users (id, name) VALUES ('u1', 'Alice');
SELECT * FROM users;
\`\`\`

## 2. Embedded SDK (Go)

\`\`\`go
import agentnativedb "github.com/startvibecoding/AgentNativeDB"

db, err := agentnativedb.Open("./data")
if err != nil { panic(err) }
defer db.Close()

// Session
sess, _ := db.CreateSession("my-agent", map[string]any{"task": "coding"})

// Memory
mem, _ := db.StoreMemory(sess.ID(), "User prefers Chinese", agentnativedb.LongTerm, 0.9)

// Decision
_, _ = db.RecordDecision(sess.ID(), agentnativedb.Reasoning,
  "How to optimize SQL", "Add index", "Analyzed execution plan")

// Vector with payload
db.CreateIndex("docs", 128, "cosine")
db.InsertVectorWithPayload("docs", "doc-1",
  []float32{0.1, 0.2, ...},
  []byte("{\"title\":\"Example doc\"}"))
results, _ := db.SearchVector("docs", []float32{0.1, 0.2, ...}, 5)

// SQL
result, _ := db.Query("SELECT * FROM sessions WHERE state = 'active'")
\`\`\`

## Typical Flow

1. Create Session
2. Write Memory + Decision during execution
3. Retrieve relevant Memory via vector search when needed
4. CloseSession when done
5. Run SQL for offline analysis/audit
`,
    'ai.tutorial.http': `# AgentNativeDB MCP over HTTP Guide (for AI clients)

This guide is meant to be shared with another AI client. Once AgentNativeDB HTTP server is running, it exposes an MCP (Model Context Protocol) JSON-RPC endpoint at \`POST /mcp\`. Any MCP-compatible AI can call it directly.

## 1. Start the service

\`\`\`bash
./bin/andb server
# default: 0.0.0.0:8400
# MCP endpoint: POST http://localhost:8400/mcp
\`\`\`

## 2. Protocol

- Transport: HTTP POST
- Content-Type: application/json
- Message format: JSON-RPC 2.0
- Endpoint: \`http://{host}:{port}/mcp\`

## 3. Handshake (initialize)

\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "your-ai-client", "version": "1.0.0"}
    }
  }'
\`\`\`

Response:
\`\`\`json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {"tools": {}},
    "serverInfo": {"name": "AgentNativeDB", "version": "0.1.0"}
  }
}
\`\`\`

## 4. List available tools

\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
\`\`\`

Returns 5 tools:
- \`query_sql\`: Execute SQL (SELECT/INSERT/UPDATE/DELETE, etc.)
- \`create_session\`: Create session, requires agent_id
- \`store_memory\`: Store memory, requires session_id + content
- \`recall_memories\`: Recall memories, requires session_id
- \`record_decision\`: Record decision, requires session_id + type + input + output

## 5. Tool call examples

### Execute SQL
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "query_sql",
      "arguments": {
        "sql": "SELECT * FROM sessions WHERE state = '\''active'\'' LIMIT 10"
      }
    }
  }'
\`\`\`

### Create session
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "create_session",
      "arguments": {"agent_id": "my-agent"}
    }
  }'
\`\`\`

### Store memory
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "store_memory",
      "arguments": {
        "session_id": "SESS_ID",
        "content": "User prefers Chinese",
        "type": "long_term",
        "importance": 0.9
      }
    }
  }'
\`\`\`

### Recall memories
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 6,
    "method": "tools/call",
    "params": {
      "name": "recall_memories",
      "arguments": {"session_id": "SESS_ID", "type": "long_term", "limit": 10}
    }
  }'
\`\`\`

### Record decision
\`\`\`bash
curl -X POST http://localhost:8400/mcp \  -H 'Content-Type: application/json' \  -d '{
    "jsonrpc": "2.0",
    "id": 7,
    "method": "tools/call",
    "params": {
      "name": "record_decision",
      "arguments": {
        "session_id": "SESS_ID",
        "type": "reasoning",
        "input": {"question": "How to optimize SQL"},
        "output": {"answer": "Add index"},
        "reasoning": "Analyzed execution plan"
      }
    }
  }'
\`\`\`

## 6. Template for AI clients

When the AI needs to operate AgentNativeDB, construct a request like this:

\`\`\`json
{
  "jsonrpc": "2.0",
  "id": "<incremental integer or string>",
  "method": "tools/call",
  "params": {
    "name": "<tool_name>",
    "arguments": {
      // Fill according to the inputSchema returned by tools/list
    }
  }
}
\`\`\`

All tool results are in \`result.content[0].text\`; errors set \`result.isError\` to true.

## 7. stdio mode (optional)

If the AI client only supports stdio MCP (e.g. Cursor / Claude Desktop), use:

\`\`\`bash
./bin/andb server -mode mcp
\`\`\`

Each line of stdin/stdout is a JSON-RPC object; methods are the same as above.
`,
    'ai.copy': 'Copy Markdown',
    'ai.stats.activeSessions': 'Active Sessions',
    'ai.stats.totalMemories': 'Total Memories',
    'ai.stats.totalDecisions': 'Total Decisions',
    'ai.stats.avgDecisionTime': 'Avg Decision Time',
    'ai.stats.topAgents': 'Top Agents',
    'ai.stats.recentDecisions': 'Recent Decisions',
    'ai.stats.memoryByType': 'Memory by Type',

    // Status
    'status.connected': 'Connected',
    'status.disconnected': 'Disconnected',
    'status.ok': 'Running',
    'status.loading': 'Loading…',

    // Common
    'common.refresh': 'Refresh',
    'common.create': 'Create',
    'common.cancel': 'Cancel',
    'common.confirm': 'Confirm',
    'common.delete': 'Delete',
    'common.close': 'Close',
    'common.add': 'Add',
    'common.execute': 'Run',
    'common.executing': 'Running…',
    'common.total': 'Total',
    'common.units.items': '',
    'common.units.rows': 'rows',
    'common.units.cols': 'cols',
    'common.units.times': 'times',
    'common.units.ms': 'ms',
    'common.empty': 'No data yet',
    'common.noSelection': 'Select to view details',
    'common.filter': 'Filter…',
    'common.version': 'Version',
    'common.language': 'Language',

    // Dashboard
    'dashboard.title': 'Overview',
    'dashboard.tables': 'Tables',
    'dashboard.sessions': 'Sessions',
    'dashboard.memories': 'Memories',
    'dashboard.decisions': 'Decisions',
    'dashboard.serverStatus': 'Server Status',
    'dashboard.engine': 'Engine',
    'dashboard.engineValue': 'BadgerDB',
    'dashboard.protocol': 'Protocol',
    'dashboard.protocolValue': 'HTTP REST',
    'dashboard.tablesEmpty': 'No tables yet',
    'dashboard.tablesHint': 'Use SQL CREATE TABLE to create',
    'dashboard.quickActions': 'Quick Actions',
    'dashboard.action.sql': 'SQL Query',
    'dashboard.action.tables': 'Manage Tables',
    'dashboard.action.sessions': 'Sessions',
    'dashboard.action.memories': 'Memories',

    // Tables
    'tables.create': 'Create Table',
    'tables.createTitle': 'Create New Table',
    'tables.tableName': 'Table Name',
    'tables.columns': 'Column Definitions',
    'tables.addColumn': 'Add Column',
    'tables.column': 'Column Name',
    'tables.primaryKey': 'Primary Key',
    'tables.structure': 'Schema',
    'tables.data': 'Data',
    'tables.dataHint': 'Max 100 rows',
    'tables.dataEmpty': 'Table is empty',
    'tables.editSchema': 'Edit Schema',
    'tables.schemaEditTitle': 'Edit Table Schema',
    'tables.saveSchema': 'Save Changes',
    'tables.addCol': 'Add Column',
    'tables.dropCol': 'Drop',
    'tables.modifyCol': 'Modify',
    'tables.backToList': 'Back to List',
    'tables.operation': 'Action',
    'tables.edit': 'Edit',
    'tables.where': 'WHERE (optional)',
    'tables.order': 'ORDER BY (optional)',
    'tables.pageSize': 'Page size',
    'tables.prevPage': 'Previous',
    'tables.nextPage': 'Next',
    'tables.insertRow': 'Add Row',
    'tables.saveNewRow': 'Save New',
    'tables.cancelInsert': 'Cancel Add',
    'tables.dropConfirm': 'Confirm Delete',
    'tables.dropMessage': 'Are you sure you want to delete table',
    'tables.dropHint': '? This cannot be undone.',
    'tables.selectHint': 'Select a table from the sidebar',
    'tables.actions': 'Actions',
    'tables.viewData': 'View Data',
    'tables.editData': 'Edit Data',
    'tables.viewDataTitle': 'View Data',
    'tables.editDataTitle': 'Edit Data',
    'tables.backToList': 'Back to List',
    'tables.operation': 'Action',
    'tables.edit': 'Edit',
    'tables.indexes': 'Indexes',
    'tables.indexName': 'Index Name',
    'tables.indexColumn': 'Index Column',
    'tables.indexType': 'Index Type',
    'tables.indexTypes': 'Index Types',
    'tables.createIndex': 'Create Index',
    'tables.dropIndex': 'Drop Index',
    'tables.noIndexes': 'No indexes',
    'tables.indexExists': 'Index already exists',
    'tables.indexCreated': 'Index created',
    'tables.fulltextIndex': 'Full-text',

    // SQL
    'sql.editor': 'SQL Editor',
    'sql.shortcut': '⌘ Enter to run',
    'sql.placeholder': 'Enter SQL statement…',
    'sql.result': 'Results',
    'sql.affected': 'Affected',
    'sql.success': 'Statement executed successfully',
    'sql.emptyResult': 'No results',
    'sql.history': 'History',
    'sql.historyEmpty': 'No history yet',
    'sql.presets.tables': 'All Tables',
    'sql.presets.indexes': 'All Indexes',
    'sql.presets.sessions': 'Sessions',
    'sql.presets.memories': 'Memories',
    'sql.presets.decisions': 'Decisions',
    'sql.presets.countSessions': 'Count Sessions',
    'sql.presets.countMemories': 'Count Memories',
    'sql.presets.fulltext': 'Full-text Search',

    // Sessions
    'sessions.create': 'New Session',
    'sessions.createTitle': 'Create New Session',
    'sessions.agentId': 'Agent ID',
    'sessions.agentIdHint': 'e.g. assistant-01',
    'sessions.detail': 'Session Detail',
    'sessions.id': 'ID',
    'sessions.agent': 'Agent',
    'sessions.state': 'State',
    'sessions.createdAt': 'Created',
    'sessions.updatedAt': 'Updated',
    'sessions.actions': 'Actions',
    'sessions.activate': 'Activate',
    'sessions.pause': 'Pause',
    'sessions.complete': 'Complete',
    'sessions.context': 'Context',
    'sessions.metadata': 'Metadata',
    'sessions.empty': 'No sessions yet',
    'sessions.selectHint': 'Select a session to view details',
    'sessions.filterHint': 'Filter by Agent…',

    // Memories
    'memories.add': 'Add Memory',
    'memories.addTitle': 'Add New Memory',
    'memories.type': 'Memory Type',
    'memories.type.short': 'Short-term',
    'memories.type.long': 'Long-term',
    'memories.type.working': 'Working',
    'memories.content': 'Content',
    'memories.contentPlaceholder': 'Enter memory content…',
    'memories.importance': 'Importance',
    'memories.accessCount': 'Accessed',
    'memories.empty': 'No memories yet',
    'memories.selectHint': 'Select a session first',
    'memories.selectSession': 'Select session…',

    // Decisions
    'decisions.tree': 'Decision Tree',
    'decisions.detail': 'Decision Detail',
    'decisions.type': 'Type',
    'decisions.type.reasoning': 'Reasoning',
    'decisions.type.tool_call': 'Tool Call',
    'decisions.type.planning': 'Planning',
    'decisions.type.reflection': 'Reflection',
    'decisions.duration': 'Duration',
    'decisions.createdAt': 'Created',
    'decisions.reasoning': 'Reasoning',
    'decisions.toolsUsed': 'Tools Used',
    'decisions.input': 'Input',
    'decisions.output': 'Output',
    'decisions.empty': 'No decisions yet',
    'decisions.selectHint': 'Select a session first',
    'decisions.selectSession': 'Select session…',
    'decisions.viewTree': 'View Tree',
  },
};

// 简单的 i18n 实现
export function createI18n(initialLocale = 'zh') {
  let _locale = $state(initialLocale);

  return {
    get locale() { return _locale; },
    set locale(v) { _locale = v; },
    t(key) {
      return messages[_locale]?.[key] ?? messages.zh[key] ?? key;
    },
    toggle() {
      _locale = _locale === 'zh' ? 'en' : 'zh';
    },
  };
}
