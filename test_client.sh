#!/bin/bash

# 测试客户端脚本

set -e

echo "=== 测试 AgentNativeDB Client ==="

# 检查二进制是否存在
if [ ! -f "./bin/andb" ]; then
    echo "错误: 二进制不存在，请先运行 'make build'"
    exit 1
fi

# 检查服务器是否运行
echo "1. 检查服务器连接..."
if ! ./bin/andb client -server localhost:8400 health 2>/dev/null; then
    echo "警告: 服务器未运行，跳过测试"
    echo "请先启动服务器: ./bin/andb server"
    exit 0
fi

echo "2. 测试版本命令..."
./bin/andb version

echo "3. 测试创建会话..."
./bin/andb client -server localhost:8400 create-session test-agent-001

echo "4. 测试列出会话..."
./bin/andb client -server localhost:8400 sessions

echo "5. 测试SQL查询..."
./bin/andb client -server localhost:8400 query "SELECT * FROM agent_sessions"

echo "6. 测试导出功能..."
./bin/andb client -server localhost:8400 export sessions /tmp/test_sessions.json
if [ -f "/tmp/test_sessions.json" ]; then
    echo "导出文件内容:"
    cat /tmp/test_sessions.json
fi

echo ""
echo "=== 测试完成 ==="
