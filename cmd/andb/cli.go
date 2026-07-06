package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger" // 注册 badger 引擎
)

func runCLI(args []string) {
	fs := flag.NewFlagSet("cli", flag.ExitOnError)
	dataDir := fs.String("data", "./data", "数据目录")
	fs.Parse(args)

	engine, err := storage.CreateEngine(storage.Options{
		Backend:     storage.BackendBadger,
		DataDir:     *dataDir,
		SyncWrites:  false,
		CacheSizeMB: 256,
		BackendOpts: map[string]any{
			"value_log_file_size": int64(64 << 20),
			"mem_table_size":      int64(16 << 20),
			"num_mem_tables":      3,
		},
	})
	if err != nil {
		log.Fatalf("create storage engine: %v", err)
	}
	defer engine.Close()

	executor := sql.NewExecutor(engine)
	if err := executor.Init(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "init executor: %v\n", err)
		os.Exit(1)
	}
	planner := executor.Planner()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	fmt.Println("AgentNativeDB CLI v0.1.0")
	fmt.Println("输入 SQL 查询，'quit' 退出。")
	fmt.Println()

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
			cliPrintHelp()
			continue
		}

		stmt, err := sql.Parse(input)
		if err != nil {
			fmt.Printf("解析错误: %v\n\n", err)
			continue
		}

		plan, err := planner.Plan(stmt)
		if err != nil {
			fmt.Printf("计划错误: %v\n\n", err)
			continue
		}

		result, err := executor.Execute(context.Background(), plan)
		if err != nil {
			fmt.Printf("执行错误: %v\n\n", err)
			continue
		}

		cliPrintResult(result)
	}

	fmt.Println("再见!")
}

func cliPrintHelp() {
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

索引语法:
  CREATE INDEX name ON table(col) [USING HASH|BTREE|INVERTED]
  DROP INDEX name
  SHOW INDEXES [FROM table]

内置函数:
  COUNT(*), SUM(col), MIN(col), MAX(col), AVG(col)
  UPPER(s), LOWER(s), LENGTH(s), COALESCE(a, b)

命令:
  help   显示帮助
  quit   退出`)
}

func cliPrintResult(result *sql.Result) {
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

	widths := make(map[string]int)
	for _, col := range result.Columns {
		widths[col] = len(col)
	}
	for _, row := range result.Rows {
		for _, col := range result.Columns {
			val := cliFormatValue(row.Values[col])
			if len(val) > widths[col] {
				widths[col] = len(val)
			}
		}
	}
	for col := range widths {
		if widths[col] > 40 {
			widths[col] = 40
		}
	}

	for _, col := range result.Columns {
		fmt.Printf("%-*s  ", widths[col], col)
	}
	fmt.Println()

	for _, col := range result.Columns {
		fmt.Printf("%s  ", strings.Repeat("-", widths[col]))
	}
	fmt.Println()

	for _, row := range result.Rows {
		for _, col := range result.Columns {
			val := cliFormatValue(row.Values[col])
			if len(val) > 40 {
				val = val[:37] + "..."
			}
			fmt.Printf("%-*s  ", widths[col], val)
		}
		fmt.Println()
	}

	fmt.Printf("(%d 行)\n\n", len(result.Rows))
}

func cliFormatValue(v any) string {
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
