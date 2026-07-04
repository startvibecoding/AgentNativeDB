#!/bin/bash

# AgentNativeDB Client 使用示例

set -e

echo "=== AgentNativeDB Client 使用示例 ==="

# 检查二进制是否存在
if [ ! -f "./bin/andb" ]; then
    echo "错误: 二进制不存在，请先运行 'make build'"
    exit 1
fi

# 检查服务器是否运行
echo "1. 检查服务器连接..."
if ! ./bin/andb client -server localhost:8400 health 2>/dev/null; then
    echo "警告: 服务器未运行，正在启动..."
    ./bin/andb server > /tmp/server.log 2>&1 &
    sleep 2
    if ! ./bin/andb client -server localhost:8400 health 2>/dev/null; then
        echo "错误: 无法启动服务器"
        exit 1
    fi
fi

echo "2. 创建会话..."
./bin/andb client -server localhost:8400 create-session demo-agent-001

echo "3. 列出会话..."
./bin/andb client -server localhost:8400 sessions

echo "4. 存储记忆..."
SESSION_ID=$(./bin/andb client -server localhost:8400 sessions 2>/dev/null | grep "ID:" | head -1 | awk '{print $2}' | sed 's/\x1b\[[0-9;]*m//g')
if [ -n "$SESSION_ID" ]; then
    ./bin/andb client -server localhost:8400 store-memory "$SESSION_ID" short_term "用户喜欢简洁的界面" 0.8
    ./bin/andb client -server localhost:8400 store-memory "$SESSION_ID" long_term "用户是高级用户，喜欢命令行工具" 0.9
fi

echo "5. 查询记忆..."
if [ -n "$SESSION_ID" ]; then
    ./bin/andb client -server localhost:8400 memories "$SESSION_ID"
fi

echo "6. 执行SQL查询..."
./bin/andb client -server localhost:8400 query "SELECT * FROM agent_sessions"

echo "7. JSON格式输出..."
./bin/andb client -server localhost:8400 -format json sessions

echo "8. 导出数据..."
./bin/andb client -server localhost:8400 export sessions /tmp/demo_sessions.json
echo "导出文件: /tmp/demo_sessions.json"

echo ""
echo "=== 示例完成 ==="
