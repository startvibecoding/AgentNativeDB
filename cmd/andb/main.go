package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "server", "mcp":
		runServer(os.Args[2:])
	case "cli":
		runCLI(os.Args[2:])
	case "client":
		runClient(os.Args[2:])
	case "version":
		fmt.Printf("AgentNativeDB %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`AgentNativeDB — Agent 原生数据库

用法:
  andb server [flags]     启动 HTTP/MCP 服务
  andb cli   [flags]      交互式 SQL CLI（本地）
  andb client [flags]     HTTP 客户端（远程连接）
  andb version            显示版本

示例:
  andb server                    # 默认启动 HTTP 服务 (0.0.0.0:8400)
  andb server -mode mcp          # 启动 MCP Server（stdio）
  andb cli                       # 本地交互式 SQL
  andb cli -data /path/to/data   # 指定数据目录
  andb client -server localhost:8400   # 连接远程服务器`)
}
