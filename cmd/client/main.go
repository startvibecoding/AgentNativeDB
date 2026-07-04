package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/startvibecoding/AgentNativeDB/config"
)

// Client HTTP客户端
type Client struct {
	baseURL    string
	httpClient *http.Client
	outputFmt  string // json, table
	verbose    bool
}

// API响应结构
type apiResponse struct {
	OK     bool            `json:"ok"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ANSI颜色常量
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorBold    = "\033[1m"
)

func main() {
	cfgPath := flag.String("config", "", "配置文件路径")
	serverAddr := flag.String("server", "", "服务器地址 (host:port)")
	outputFmt := flag.String("format", "table", "输出格式: table, json")
	verbose := flag.Bool("verbose", false, "详细输出")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		cfg = config.Default()
	}

	// 确定服务器地址
	addr := *serverAddr
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	// 创建客户端
	client := &Client{
		baseURL: fmt.Sprintf("http://%s", addr),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		outputFmt: *outputFmt,
		verbose:   *verbose,
	}

	// 检查服务器连接
	if err := client.checkHealth(); err != nil {
		printError(fmt.Sprintf("无法连接到服务器 %s", addr), err)
		os.Exit(1)
	}

	printSuccess(fmt.Sprintf("已连接到 AgentNativeDB 服务器: %s", addr))
	fmt.Println("输入 'help' 查看可用命令，'quit' 退出。")
	fmt.Println()

	// 运行交互式CLI
	if err := runCLI(client); err != nil {
		printError("CLI错误", err)
		os.Exit(1)
	}
}

// printSuccess 打印成功信息
func printSuccess(msg string) {
	fmt.Printf("%s✓ %s%s\n", colorGreen, msg, colorReset)
}

// printError 打印错误信息
func printError(msg string, err error) {
	fmt.Printf("%s✗ %s: %v%s\n", colorRed, msg, err, colorReset)
}

// printWarning 打印警告信息
func printWarning(msg string) {
	fmt.Printf("%s⚠ %s%s\n", colorYellow, msg, colorReset)
}

// printInfo 打印信息
func printInfo(msg string) {
	fmt.Printf("%sℹ %s%s\n", colorCyan, msg, colorReset)
}

// printHeader 打印标题
func printHeader(msg string) {
	fmt.Printf("%s%s%s\n", colorBold, msg, colorReset)
}

// checkHealth 检查服务器健康状态
func (c *Client) checkHealth() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回状态码: %d", resp.StatusCode)
	}

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("服务器返回错误: %s", result.Error)
	}

	return nil
}

// runCLI 运行交互式CLI
func runCLI(client *Client) error {
	// 设置readline
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[36mandb-client>\033[0m ",
		HistoryFile:     "/tmp/andb_client_history",
		AutoComplete:    getAutoCompleter(),
		InterruptPrompt: "^C",
		EOFPrompt:       "quit",
	})
	if err != nil {
		return err
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil { // io.EOF, readline.ErrInterrupt
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// 处理多行输入（以分号结尾）
		for !strings.HasSuffix(input, ";") && !isCommand(input) {
			rl.SetPrompt("\033[33m  ...\033[0m ")
			nextLine, err := rl.Readline()
			if err != nil {
				break
			}
			if strings.TrimSpace(nextLine) == "" {
				break
			}
			input += " " + strings.TrimSpace(nextLine)
		}
		rl.SetPrompt("\033[36mandb-client>\033[0m ")

		// 执行命令
		if err := executeCommand(client, input); err != nil {
			printError("命令执行失败", err)
		}
	}

	fmt.Println("再见!")
	return nil
}

// isCommand 判断是否是命令（非SQL）
func isCommand(input string) bool {
	commands := []string{"help", "health", "quit", "exit", "sessions", "session", "create-session",
		"memories", "store-memory", "decisions", "decision-tree", "query", "sql",
		"update-session", "delete-session", "delete-memory", "delete-decision",
		"export", "import", "clear", "status", "version"}
	
	args := strings.Fields(input)
	if len(args) == 0 {
		return false
	}
	
	cmd := strings.ToLower(args[0])
	for _, c := range commands {
		if cmd == c {
			return true
		}
	}
	return false
}

// getAutoCompleter 获取自动补全器
func getAutoCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("help"),
		readline.PcItem("health"),
		readline.PcItem("quit"),
		readline.PcItem("exit"),
		readline.PcItem("clear"),
		readline.PcItem("status"),
		readline.PcItem("version"),
		readline.PcItem("sessions"),
		readline.PcItem("session"),
		readline.PcItem("create-session"),
		readline.PcItem("update-session"),
		readline.PcItem("delete-session"),
		readline.PcItem("memories"),
		readline.PcItem("store-memory"),
		readline.PcItem("delete-memory"),
		readline.PcItem("decisions"),
		readline.PcItem("decision-tree"),
		readline.PcItem("delete-decision"),
		readline.PcItem("query"),
		readline.PcItem("sql"),
		readline.PcItem("export"),
		readline.PcItem("import"),
	)
}

// executeCommand 执行命令
func executeCommand(client *Client, input string) error {
	args := strings.Fields(input)
	if len(args) == 0 {
		return nil
	}

	command := strings.ToLower(args[0])

	switch command {
	case "quit", "exit":
		fmt.Println("再见!")
		os.Exit(0)

	case "help":
		printHelp()

	case "clear":
		fmt.Print("\033[H\033[2J")

	case "status":
		return client.showStatus()

	case "version":
		printVersion()

	case "health":
		if err := client.checkHealth(); err != nil {
			printError("健康检查失败", err)
		} else {
			printSuccess("服务器状态: 正常")
		}

	case "query", "sql":
		if len(args) < 2 {
			printWarning("用法: query <SQL语句>")
			return nil
		}
		sql := strings.Join(args[1:], " ")
		return client.executeQuery(sql)

	case "sessions":
		return client.listSessions()

	case "session":
		if len(args) < 2 {
			printWarning("用法: session <id>")
			return nil
		}
		return client.getSession(args[1])

	case "create-session":
		if len(args) < 2 {
			printWarning("用法: create-session <agent_id> [metadata_json]")
			return nil
		}
		agentID := args[1]
		var metadata map[string]any
		if len(args) > 2 {
			metadataStr := strings.Join(args[2:], " ")
			if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
				return fmt.Errorf("解析metadata失败: %v", err)
			}
		}
		return client.createSession(agentID, metadata)

	case "update-session":
		if len(args) < 3 {
			printWarning("用法: update-session <id> <state> [context_json]")
			return nil
		}
		id := args[1]
		state := args[2]
		var context map[string]any
		if len(args) > 3 {
			contextStr := strings.Join(args[3:], " ")
			if err := json.Unmarshal([]byte(contextStr), &context); err != nil {
				return fmt.Errorf("解析context失败: %v", err)
			}
		}
		return client.updateSession(id, state, context)

	case "delete-session":
		if len(args) < 2 {
			printWarning("用法: delete-session <id>")
			return nil
		}
		return client.deleteSession(args[1])

	case "memories":
		if len(args) < 2 {
			printWarning("用法: memories <session_id> [limit]")
			return nil
		}
		sessionID := args[1]
		limit := 0
		if len(args) > 2 {
			fmt.Sscanf(args[2], "%d", &limit)
		}
		return client.listMemories(sessionID, limit)

	case "store-memory":
		if len(args) < 4 {
			printWarning("用法: store-memory <session_id> <type> <content> [importance]")
			return nil
		}
		sessionID := args[1]
		memType := args[2]
		content := args[3]
		var importance float32
		if len(args) > 4 {
			fmt.Sscanf(args[4], "%f", &importance)
		}
		return client.storeMemory(sessionID, memType, content, importance)

	case "delete-memory":
		if len(args) < 2 {
			printWarning("用法: delete-memory <id>")
			return nil
		}
		return client.deleteMemory(args[1])

	case "decisions":
		if len(args) < 2 {
			printWarning("用法: decisions <session_id> [limit]")
			return nil
		}
		sessionID := args[1]
		limit := 0
		if len(args) > 2 {
			fmt.Sscanf(args[2], "%d", &limit)
		}
		return client.listDecisions(sessionID, limit)

	case "decision-tree":
		if len(args) < 2 {
			printWarning("用法: decision-tree <decision_id>")
			return nil
		}
		return client.getDecisionTree(args[1])

	case "delete-decision":
		if len(args) < 2 {
			printWarning("用法: delete-decision <id>")
			return nil
		}
		return client.deleteDecision(args[1])

	case "export":
		if len(args) < 3 {
			printWarning("用法: export <type> <filename>")
			printInfo("类型: sessions, memories, decisions")
			return nil
		}
		return client.exportData(args[1], args[2])

	default:
		printWarning(fmt.Sprintf("未知命令: %s", command))
		fmt.Println("输入 'help' 查看可用命令")
	}

	return nil
}

// printHelp 打印帮助信息
func printHelp() {
	printHeader("AgentNativeDB Client 命令帮助")
	fmt.Println()
	printHeader("通用命令:")
	fmt.Println("  help                    显示此帮助信息")
	fmt.Println("  health                  检查服务器健康状态")
	fmt.Println("  status                  显示服务器状态")
	fmt.Println("  version                 显示版本信息")
	fmt.Println("  clear                   清屏")
	fmt.Println("  quit/exit               退出客户端")
	fmt.Println()
	printHeader("SQL查询:")
	fmt.Println("  query <SQL语句>          执行SQL查询")
	fmt.Println("  sql <SQL语句>            执行SQL查询 (别名)")
	fmt.Println()
	printHeader("会话管理:")
	fmt.Println("  sessions                列出所有会话")
	fmt.Println("  session <id>            获取指定会话")
	fmt.Println("  create-session <agent_id> [metadata_json]  创建新会话")
	fmt.Println("  update-session <id> <state> [context_json] 更新会话")
	fmt.Println("  delete-session <id>     删除会话")
	fmt.Println()
	printHeader("记忆管理:")
	fmt.Println("  memories <session_id> [limit]              列出会话记忆")
	fmt.Println("  store-memory <session_id> <type> <content> [importance]  存储记忆")
	fmt.Println("  delete-memory <id>      删除记忆")
	fmt.Println()
	printHeader("决策管理:")
	fmt.Println("  decisions <session_id> [limit]             列出会话决策")
	fmt.Println("  decision-tree <decision_id>                获取决策树")
	fmt.Println("  delete-decision <id>    删除决策")
	fmt.Println()
	printHeader("数据导出:")
	fmt.Println("  export <type> <filename>  导出数据到文件")
	fmt.Println("    类型: sessions, memories, decisions")
	fmt.Println()
	printHeader("示例:")
	fmt.Println("  query SELECT * FROM agent_sessions")
	fmt.Println("  create-session agent-001 {\"env\": \"test\"}")
	fmt.Println("  store-memory sess-123 short_term \"用户偏好设置\" 0.8")
	fmt.Println("  memories sess-123 10")
	fmt.Println("  export sessions sessions.json")
}

// printVersion 打印版本信息
func printVersion() {
	printHeader("AgentNativeDB Client")
	fmt.Println("版本: 1.0.0")
	fmt.Println("构建时间: 2026-07-04")
	fmt.Println("Go版本: 1.23+")
}

// showStatus 显示服务器状态
func (c *Client) showStatus() error {
	// 获取健康状态
	healthResp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer healthResp.Body.Close()

	var healthResult apiResponse
	if err := json.NewDecoder(healthResp.Body).Decode(&healthResult); err != nil {
		return err
	}

	printHeader("服务器状态")
	if healthResult.OK {
		printSuccess("健康状态: 正常")
	} else {
		printError("健康状态: 异常", fmt.Errorf("%s", healthResult.Error))
	}

	// 获取会话数量
	sessionsResp, err := c.httpClient.Get(c.baseURL + "/api/v1/sessions")
	if err == nil {
		defer sessionsResp.Body.Close()
		var sessionsResult apiResponse
		if err := json.NewDecoder(sessionsResp.Body).Decode(&sessionsResult); err == nil && sessionsResult.OK {
			var sessions []any
			json.Unmarshal(sessionsResult.Data, &sessions)
			fmt.Printf("会话数量: %d\n", len(sessions))
		}
	}

	return nil
}

// executeQuery 执行SQL查询
func (c *Client) executeQuery(sql string) error {
	if c.verbose {
		printInfo(fmt.Sprintf("执行SQL: %s", sql))
	}

	body := map[string]string{"sql": sql}
	data, _ := json.Marshal(body)

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v1/query",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	// 解析查询结果
	var queryResult struct {
		Columns      []string         `json:"columns"`
		Rows         []map[string]any `json:"rows"`
		RowsAffected int64            `json:"rows_affected"`
	}
	if err := json.Unmarshal(result.Data, &queryResult); err != nil {
		return err
	}

	// 输出结果
	if c.outputFmt == "json" {
		return printJSON(queryResult)
	}
	printQueryResult(queryResult.Columns, queryResult.Rows, queryResult.RowsAffected)
	return nil
}

// printJSON 打印JSON格式
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// printQueryResult 打印查询结果
func printQueryResult(columns []string, rows []map[string]any, rowsAffected int64) {
	if rowsAffected > 0 {
		printSuccess(fmt.Sprintf("影响 %d 行", rowsAffected))
		return
	}

	if len(rows) == 0 {
		printInfo("(无数据)")
		return
	}

	// 计算列宽
	widths := make(map[string]int)
	for _, col := range columns {
		widths[col] = len(col)
	}
	for _, row := range rows {
		for _, col := range columns {
			val := formatValue(row[col])
			if len(val) > widths[col] {
				widths[col] = len(val)
			}
		}
	}
	// 限制最大列宽
	for col := range widths {
		if widths[col] > 50 {
			widths[col] = 50
		}
	}

	// 打印表头
	fmt.Printf("%s", colorBold)
	for _, col := range columns {
		fmt.Printf("%-*s  ", widths[col], col)
	}
	fmt.Printf("%s\n", colorReset)

	// 打印分隔线
	for _, col := range columns {
		fmt.Printf("%s%s%s  ", colorCyan, strings.Repeat("-", widths[col]), colorReset)
	}
	fmt.Println()

	// 打印数据行
	for i, row := range rows {
		if i%2 == 0 {
			fmt.Printf("%s", colorWhite)
		} else {
			fmt.Printf("%s", colorReset)
		}
		for _, col := range columns {
			val := formatValue(row[col])
			if len(val) > 50 {
				val = val[:47] + "..."
			}
			fmt.Printf("%-*s  ", widths[col], val)
		}
		fmt.Printf("%s\n", colorReset)
	}

	fmt.Printf("\n%s(%d 行)%s\n", colorYellow, len(rows), colorReset)
}

// formatValue 格式化值
func formatValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
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

// listSessions 列出会话
func (c *Client) listSessions() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/sessions")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	var sessions []map[string]any
	if err := json.Unmarshal(result.Data, &sessions); err != nil {
		return err
	}

	if len(sessions) == 0 {
		printInfo("(无会话)")
		return nil
	}

	if c.outputFmt == "json" {
		return printJSON(sessions)
	}

	printHeader(fmt.Sprintf("会话列表 (%d 个)", len(sessions)))
	for i, session := range sessions {
		fmt.Printf("%s%d.%s ID: %s%v%s, Agent: %v, State: %s%v%s\n",
			colorCyan, i+1, colorReset,
			colorGreen, session["id"], colorReset,
			session["agent_id"],
			colorYellow, session["state"], colorReset)
	}
	fmt.Println()
	return nil
}

// getSession 获取会话
func (c *Client) getSession(id string) error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/sessions/" + id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	if c.outputFmt == "json" {
		var session any
		json.Unmarshal(result.Data, &session)
		return printJSON(session)
	}

	var session map[string]any
	if err := json.Unmarshal(result.Data, &session); err != nil {
		return err
	}

	printHeader("会话详情")
	fmt.Printf("ID:        %s%v%s\n", colorGreen, session["id"], colorReset)
	fmt.Printf("Agent:     %v\n", session["agent_id"])
	fmt.Printf("State:     %s%v%s\n", colorYellow, session["state"], colorReset)
	fmt.Printf("Created:   %v\n", session["created_at"])
	fmt.Printf("Updated:   %v\n", session["updated_at"])
	if ctx, ok := session["context"].(map[string]any); ok && len(ctx) > 0 {
		fmt.Printf("Context:   %v\n", ctx)
	}
	fmt.Println()
	return nil
}

// createSession 创建会话
func (c *Client) createSession(agentID string, metadata map[string]any) error {
	body := map[string]any{
		"agent_id": agentID,
	}
	if metadata != nil {
		body["metadata"] = metadata
	}
	data, _ := json.Marshal(body)

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v1/sessions",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	var session map[string]any
	if err := json.Unmarshal(result.Data, &session); err != nil {
		return err
	}

	printSuccess("会话创建成功")
	fmt.Printf("ID:    %s%v%s\n", colorGreen, session["id"], colorReset)
	fmt.Printf("Agent: %v\n", session["agent_id"])
	fmt.Println()
	return nil
}

// updateSession 更新会话
func (c *Client) updateSession(id, state string, context map[string]any) error {
	body := map[string]any{
		"state": state,
	}
	if context != nil {
		body["context"] = context
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest("PATCH", c.baseURL+"/api/v1/sessions/"+id, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	printSuccess("会话更新成功")
	return nil
}

// deleteSession 删除会话
func (c *Client) deleteSession(id string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/sessions/"+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	printSuccess("会话删除成功")
	return nil
}

// listMemories 列出记忆
func (c *Client) listMemories(sessionID string, limit int) error {
	url := fmt.Sprintf("%s/api/v1/memories?session_id=%s", c.baseURL, sessionID)
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	var memories []map[string]any
	if err := json.Unmarshal(result.Data, &memories); err != nil {
		return err
	}

	if len(memories) == 0 {
		printInfo("(无记忆)")
		return nil
	}

	if c.outputFmt == "json" {
		return printJSON(memories)
	}

	printHeader(fmt.Sprintf("记忆列表 (%d 个)", len(memories)))
	for i, memory := range memories {
		fmt.Printf("%s%d.%s ID: %s%v%s, Type: %s%v%s, Importance: %s%.2f%s\n",
			colorCyan, i+1, colorReset,
			colorGreen, memory["id"], colorReset,
			colorYellow, memory["type"], colorReset,
			colorMagenta, memory["importance"], colorReset)
		if content, ok := memory["content"].(string); ok {
			if len(content) > 100 {
				content = content[:97] + "..."
			}
			fmt.Printf("   内容: %s\n", content)
		}
	}
	fmt.Println()
	return nil
}

// storeMemory 存储记忆
func (c *Client) storeMemory(sessionID, memType, content string, importance float32) error {
	body := map[string]any{
		"session_id": sessionID,
		"type":       memType,
		"content":    content,
	}
	if importance > 0 {
		body["importance"] = importance
	}
	data, _ := json.Marshal(body)

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v1/memories",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	var memory map[string]any
	if err := json.Unmarshal(result.Data, &memory); err != nil {
		return err
	}

	printSuccess("记忆存储成功")
	fmt.Printf("ID:   %s%v%s\n", colorGreen, memory["id"], colorReset)
	fmt.Printf("类型: %v\n", memory["type"])
	fmt.Println()
	return nil
}

// deleteMemory 删除记忆
func (c *Client) deleteMemory(id string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/memories/"+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	printSuccess("记忆删除成功")
	return nil
}

// listDecisions 列出决策
func (c *Client) listDecisions(sessionID string, limit int) error {
	url := fmt.Sprintf("%s/api/v1/decisions?session_id=%s", c.baseURL, sessionID)
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	var decisions []map[string]any
	if err := json.Unmarshal(result.Data, &decisions); err != nil {
		return err
	}

	if len(decisions) == 0 {
		printInfo("(无决策)")
		return nil
	}

	if c.outputFmt == "json" {
		return printJSON(decisions)
	}

	printHeader(fmt.Sprintf("决策列表 (%d 个)", len(decisions)))
	for i, decision := range decisions {
		fmt.Printf("%s%d.%s ID: %s%v%s, Type: %s%v%s\n",
			colorCyan, i+1, colorReset,
			colorGreen, decision["id"], colorReset,
			colorYellow, decision["type"], colorReset)
		if reasoning, ok := decision["reasoning"].(string); ok && reasoning != "" {
			if len(reasoning) > 100 {
				reasoning = reasoning[:97] + "..."
			}
			fmt.Printf("   推理: %s\n", reasoning)
		}
		if tools, ok := decision["tools_used"].([]any); ok && len(tools) > 0 {
			fmt.Printf("   工具: %v\n", tools)
		}
	}
	fmt.Println()
	return nil
}

// getDecisionTree 获取决策树
func (c *Client) getDecisionTree(id string) error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/decisions/" + id + "/tree")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	if c.outputFmt == "json" {
		return printJSON(result.Data)
	}

	printHeader("决策树")
	data, _ := json.MarshalIndent(result.Data, "", "  ")
	fmt.Println(string(data))
	fmt.Println()
	return nil
}

// deleteDecision 删除决策
func (c *Client) deleteDecision(id string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/decisions/"+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	printSuccess("决策删除成功")
	return nil
}

// exportData 导出数据
func (c *Client) exportData(dataType, filename string) error {
	var url string
	switch dataType {
	case "sessions":
		url = c.baseURL + "/api/v1/sessions"
	case "memories":
		url = c.baseURL + "/api/v1/memories?session_id=all"
	case "decisions":
		url = c.baseURL + "/api/v1/decisions?session_id=all"
	default:
		return fmt.Errorf("不支持的导出类型: %s", dataType)
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result apiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}

	// 格式化JSON
	var data any
	if err := json.Unmarshal(result.Data, &data); err != nil {
		return err
	}

	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, formatted, 0644); err != nil {
		return err
	}

	printSuccess(fmt.Sprintf("数据已导出到: %s", filename))
	return nil
}
