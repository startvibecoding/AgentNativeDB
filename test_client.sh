#!/bin/bash

# 测试客户端脚本

set -e

echo "=== 测试 AgentNativeDB Client ==="

# 检查客户端二进制是否存在
if [ ! -f "./bin/client" ]; then
    echo "错误: 客户端二进制不存在，请先运行 'make client'"
    exit 1
fi

# 检查服务器是否运行
echo "1. 检查服务器连接..."
if ! ./bin/client -server localhost:8400 health 2>/dev/null; then
    echo "警告: 服务器未运行，跳过测试"
    echo "请先启动服务器: ./bin/server"
    exit 0
fi

echo "2. 测试版本命令..."
./bin/client -server localhost:8400 version

echo "3. 测试状态命令..."
./bin/client -server localhost:8400 status

echo "4. 测试创建会话..."
./bin/client -server localhost:8400 create-session test-agent-001

echo "5. 测试列出会话..."
./bin/client -server localhost:8400 sessions

echo "6. 测试存储记忆..."
SESSION_ID=$(./bin/client -server localhost:8400 sessions 2>/dev/null | grep "ID:" | head -1 | awk '{print $2}' | sed 's/\x1b\[[0-9;]*m//g')
if [ -n "$SESSION_ID" ]; then
    ./bin/client -server localhost:8400 store-memory "$SESSION_ID" short_term "测试记忆内容" 0.9
    ./bin/client -server localhost:8400 memories "$SESSION_ID"
fi

echo "7. 测试SQL查询..."
./bin/client -server localhost:8400 query "SELECT * FROM agent_sessions"

echo "8. 测试JSON格式输出..."
./bin/client -server localhost:8400 -format json sessions

echo "9. 测试导出功能..."
./bin/client -server localhost:8400 export sessions /tmp/test_sessions.json
if [ -f "/tmp/test_sessions.json" ]; then
    echo "导出文件内容:"
    cat /tmp/test_sessions.json
fi

echo ""
echo "=== 测试完成 ==="
