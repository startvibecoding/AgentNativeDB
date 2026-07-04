# AgentNativeDB Client

HTTP客户端，用于连接AgentNativeDB服务器。

## 构建

```bash
make client
```

或构建所有二进制：

```bash
make build
```

## 使用方法

### 启动客户端

```bash
# 使用默认配置连接
./bin/client

# 指定服务器地址
./bin/client -server localhost:8400

# 使用配置文件
./bin/client -config config.json

# 指定输出格式
./bin/client -format json

# 详细输出模式
./bin/client -verbose
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-config` | 配置文件路径 | - |
| `-server` | 服务器地址 (host:port) | 0.0.0.0:8400 |
| `-format` | 输出格式 (table, json) | table |
| `-verbose` | 详细输出模式 | false |

### 可用命令

#### 通用命令
```
help                    显示帮助信息
health                  检查服务器健康状态
status                  显示服务器状态
version                 显示版本信息
clear                   清屏
quit/exit               退出客户端
```

#### SQL查询
```
query <SQL语句>          执行SQL查询
sql <SQL语句>            执行SQL查询 (别名)
```

#### 会话管理
```
sessions                列出所有会话
session <id>            获取指定会话
create-session <agent_id> [metadata_json]  创建新会话
update-session <id> <state> [context_json] 更新会话
delete-session <id>     删除会话
```

#### 记忆管理
```
memories <session_id> [limit]              列出会话记忆
store-memory <session_id> <type> <content> [importance]  存储记忆
delete-memory <id>      删除记忆
```

#### 决策管理
```
decisions <session_id> [limit]             列出会话决策
decision-tree <decision_id>                获取决策树
delete-decision <id>    删除决策
```

#### 数据导出
```
export <type> <filename>  导出数据到文件
  类型: sessions, memories, decisions
```

### 示例

```bash
# 检查服务器状态
andb-client> health

# 显示服务器详细状态
andb-client> status

# 创建会话
andb-client> create-session agent-001 {"env": "test"}

# 列出会话
andb-client> sessions

# 更新会话状态
andb-client> update-session sess-123 active {"key": "value"}

# 存储记忆
andb-client> store-memory sess-123 short_term "用户偏好设置" 0.8

# 查询记忆
andb-client> memories sess-123 10

# 执行SQL查询
andb-client> query SELECT * FROM agent_sessions

# 多行SQL输入
andb-client> SELECT s.agent_id, m.content
  ... FROM agent_sessions s
  ... JOIN agent_memories m ON s.id = m.session_id
  ... WHERE m.importance > 0.7;

# 导出数据
andb-client> export sessions sessions.json
andb-client> export memories memories.json

# JSON格式输出
andb-client> -format json sessions
```

## 特性

### 命令历史
- 使用上下箭头键浏览历史命令
- 历史记录保存在 `/tmp/andb_client_history`

### 自动补全
- 支持命令自动补全
- 按 Tab 键补全命令

### 多行输入
- 支持多行SQL语句输入
- 以分号结尾或使用空行结束

### 颜色输出
- 成功信息: 绿色
- 错误信息: 红色
- 警告信息: 黄色
- 信息提示: 青色
- 表格标题: 粗体

### 输出格式
- `table`: 表格格式（默认）
- `json`: JSON格式

## 配置

客户端使用与服务器相同的配置格式。默认配置：

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8400
  }
}
```

## 开发

### 项目结构

```
cmd/
├── cli/          # 本地SQL CLI
├── client/       # HTTP客户端
└── server/       # HTTP服务器
```

### 依赖

- `github.com/chzyer/readline` - 命令行编辑库

### API端点

客户端连接到服务器的以下API端点：

- `GET /health` - 健康检查
- `POST /api/v1/sessions` - 创建会话
- `GET /api/v1/sessions` - 列出会话
- `GET /api/v1/sessions/{id}` - 获取会话
- `PATCH /api/v1/sessions/{id}` - 更新会话
- `DELETE /api/v1/sessions/{id}` - 删除会话
- `POST /api/v1/memories` - 存储记忆
- `GET /api/v1/memories` - 列出记忆
- `GET /api/v1/memories/{id}` - 获取记忆
- `DELETE /api/v1/memories/{id}` - 删除记忆
- `POST /api/v1/decisions` - 记录决策
- `GET /api/v1/decisions` - 列出决策
- `GET /api/v1/decisions/{id}` - 获取决策
- `DELETE /api/v1/decisions/{id}` - 删除决策
- `GET /api/v1/decisions/{id}/tree` - 获取决策树
- `POST /api/v1/query` - 执行SQL查询

## 故障排除

### 连接失败
```
✗ 无法连接到服务器 localhost:8400: connection refused
```
- 确保服务器已启动
- 检查服务器地址和端口是否正确
- 检查防火墙设置

### 认证失败
```
✗ 服务器返回错误: unauthorized
```
- 检查配置文件中的认证信息
- 确保有足够的权限

### 查询错误
```
✗ 查询错误: parse error: ...
```
- 检查SQL语法
- 参考支持的SQL语法文档
