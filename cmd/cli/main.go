package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/api/mcp"
	"github.com/startvibecoding/AgentNativeDB/config"
	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func main() {
	mode := flag.String("mode", "cli", "运行模式: mcp, cli")
	dataDir := flag.String("data", "./data", "数据目录")
	flag.Parse()

	cfg := config.Default()
	cfg.Storage.DataDir = *dataDir

	// 打开存储
	engine := badgerstore.New()
	opts := storage.Options{
		DataDir:          cfg.Storage.DataDir,
		SyncWrites:       cfg.Storage.SyncWrites,
		ValueLogFileSize: cfg.Storage.ValueLogFileSize,
		MemTableSize:     cfg.Storage.MemTableSize,
		NumMemTables:     cfg.Storage.NumMemTables,
	}
	if err := engine.Open(opts); err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer engine.Close()

	cache := storage.NewCache(512)
	sessionMgr := agent.NewSessionManager(engine, cache)
	memoryStore := agent.NewMemoryStore(engine, cache)
	decisionRecorder := agent.NewDecisionRecorder(engine, cache)

	switch *mode {
	case "mcp":
		runMCP(engine, sessionMgr, memoryStore, decisionRecorder)
	default:
		runCLI(engine)
	}
}

// runMCP 运行 MCP 模式（stdio 传输）
func runMCP(engine storage.Engine, session *agent.SessionManager, memory *agent.MemoryStore, decision *agent.DecisionRecorder) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	server := mcp.NewMCPServer(engine, session, memory, decision)
	if err := server.Run(context.Background()); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

// runCLI 运行交互式 SQL CLI
func runCLI(engine storage.Engine) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	fmt.Println("AgentNativeDB CLI v0.1.0")
	fmt.Println("输入 SQL 查询，'quit' 退出。")
	fmt.Println()

	executor := sql.NewExecutor(engine)
	planner := sql.NewPlanner()

	for {
		fmt.Print("andb> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "quit" || input == "exit" {
			break
		}
		if input == "help" {
			printHelp()
			continue
		}

		// 解析
		stmt, err := sql.Parse(input)
		if err != nil {
			fmt.Printf("解析错误: %v\n\n", err)
			continue
		}

		// 计划
		plan, err := planner.Plan(stmt)
		if err != nil {
			fmt.Printf("计划错误: %v\n\n", err)
			continue
		}

		// 执行
		result, err := executor.Execute(context.Background(), plan)
		if err != nil {
			fmt.Printf("执行错误: %v\n\n", err)
			continue
		}

		// 输出
		printResult(result)
	}

	fmt.Println("再见!")
}

func printHelp() {
	fmt.Println(`
支持的 SQL 语法:
  SELECT * FROM table
  SELECT col1, col2 FROM table WHERE col = 'value'
  SELECT col, COUNT(*) FROM table GROUP BY col
  SELECT * FROM table ORDER BY col DESC LIMIT 10
  INSERT INTO table (id, col) VALUES ('id1', 'val')
  UPDATE table SET col = 'new' WHERE id = 'id1'
  DELETE FROM table WHERE id = 'id1'

可用的表:
  agent_sessions, agent_memories, agent_decisions
  knowledge_entities, knowledge_relations, data_lineage

内置函数:
  COUNT(*), SUM(col), MIN(col), MAX(col), AVG(col)
  UPPER(s), LOWER(s), LENGTH(s), COALESCE(a, b)

命令:
  help   显示帮助
  quit   退出`)
}

func printResult(result *sql.Result) {
	if result == nil {
		fmt.Println("(空结果)")
		return
	}

	if result.RowsAffected > 0 {
		fmt.Printf("影响 %d 行\n\n", result.RowsAffected)
		return
	}

	if len(result.Rows) == 0 {
		fmt.Println("(无数据)")
		fmt.Println()
		return
	}

	// 计算列宽
	widths := make(map[string]int)
	for _, col := range result.Columns {
		widths[col] = len(col)
	}
	for _, row := range result.Rows {
		for _, col := range result.Columns {
			val := formatValue(row.Values[col])
			if len(val) > widths[col] {
				widths[col] = len(val)
			}
		}
	}
	// 限制最大列宽
	for col := range widths {
		if widths[col] > 40 {
			widths[col] = 40
		}
	}

	// 打印表头
	for _, col := range result.Columns {
		fmt.Printf("%-*s  ", widths[col], col)
	}
	fmt.Println()

	// 打印分隔线
	for _, col := range result.Columns {
		fmt.Printf("%s  ", strings.Repeat("-", widths[col]))
	}
	fmt.Println()

	// 打印数据行
	for _, row := range result.Rows {
		for _, col := range result.Columns {
			val := formatValue(row.Values[col])
			if len(val) > 40 {
				val = val[:37] + "..."
			}
			fmt.Printf("%-*s  ", widths[col], val)
		}
		fmt.Println()
	}

	fmt.Printf("(%d 行)\n\n", len(result.Rows))
}

func formatValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%.4f", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		data, _ := json.Marshal(val)
		return string(data)
	}
}
