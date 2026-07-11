# Raft 集群模式使用指南

AgentNativeDB 内置基于 Raft 共识算法的高可用集群模式，支持多节点数据复制和自动故障转移。

## 特性

- **强一致性**：所有写操作通过 Raft 日志复制到多数节点后才提交
- **自动故障转移**：Leader 故障后自动选举新 Leader
- **读写分离**：写操作必须通过 Leader，读操作可在任意节点执行（可能略有延迟）
- **纯 Go 实现**：无 CGO 依赖，与现有 BadgerDB 存储无缝集成
- **HTTP 传输**：复用现有 HTTP 端口，无需额外端口

## 快速开始

### 1. 单机集群（测试用）

启动单节点集群并初始化：
```bash
./bin/andb server -cluster -node-id node1 -bootstrap
```

### 2. 三节点集群

准备三个节点，分别在三台服务器上启动：

**节点1（初始Leader）：**
```bash
./bin/andb server \
  -cluster \
  -node-id node1 \
  -raft-addr node1.example.com:8400 \
  -bootstrap \
  -data-dir ./data1
```

**节点2：**
```bash
./bin/andb server \
  -cluster \
  -node-id node2 \
  -raft-addr node2.example.com:8400 \
  -data-dir ./data2
```

启动后通过API添加节点2到集群：
```bash
curl -X POST http://node1.example.com:8400/api/v1/cluster/peers \
  -H "Content-Type: application/json" \
  -d '{"node_id":"node2","raft_addr":"node2.example.com:8400"}'
```

**节点3：**
```bash
./bin/andb server \
  -cluster \
  -node-id node3 \
  -raft-addr node3.example.com:8400 \
  -data-dir ./data3
```

添加节点3到集群：
```bash
curl -X POST http://node1.example.com:8400/api/v1/cluster/peers \
  -H "Content-Type: application/json" \
  -d '{"node_id":"node3","raft_addr":"node3.example.com:8400"}'
```

## 配置文件

在 `config.json` 中配置集群：

```json
{
  "cluster": {
    "enabled": true,
    "node_id": "node1",
    "raft_addr": "0.0.0.0:8400",
    "peers": {
      "node1": "node1.example.com:8400",
      "node2": "node2.example.com:8400",
      "node3": "node3.example.com:8400"
    },
    "bootstrap": false
  }
}
```

配置说明：
- `enabled`: 是否启用集群模式
- `node_id`: 当前节点唯一ID，集群内必须唯一
- `raft_addr`: Raft内部通信地址，节点间必须能互相访问
- `peers`: 集群节点地址映射
- `bootstrap`: 是否初始化新集群（仅在首次启动第一个节点时设为true）

## 集群管理API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/cluster/status` | 查询集群状态 |
| POST | `/api/v1/cluster/peers` | 添加节点到集群（仅Leader） |
| DELETE | `/api/v1/cluster/peers/{id}` | 从集群移除节点（仅Leader） |
| GET | `/health` | 健康检查，包含集群状态信息 |

### 查询集群状态示例
```bash
curl http://localhost:8400/api/v1/cluster/status
```

响应：
```json
{
  "ok": true,
  "data": {
    "node_id": "node1",
    "state": "leader",
    "term": 1,
    "commit_index": 42,
    "last_applied": 42,
    "leader_id": "node1",
    "peers": {
      "node1": "node1.example.com:8400",
      "node2": "node2.example.com:8400",
      "node3": "node3.example.com:8400"
    }
  }
}
```

## Raft 内部端点

Raft协议内部通信端点挂载在 `/raft/` 路径下，请勿手动调用：
- `POST /raft/vote` - 投票请求
- `POST /raft/append` - 日志复制/心跳
- `POST /raft/snapshot` - 快照安装（预留）

## 注意事项

1. **bootstrap 只执行一次**：仅在创建全新集群时在第一个节点使用 `-bootstrap` 参数，后续启动不需要
2. **节点数要求**：集群至少需要1个节点，生产环境推荐3或5个节点（奇数个）
3. **网络要求**：节点间网络必须可靠，延迟建议在100ms以内
4. **数据目录**：每个节点必须使用独立的数据目录
5. **写操作**：写操作必须发送到 Leader，Follower 收到写请求会返回 503 错误和当前 Leader 地址
6. **读一致性**：默认允许从 Follower 读取，可能存在短暂复制延迟；如需线性一致性读，请先查询 Leader 地址再读
