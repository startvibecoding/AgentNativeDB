package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/startvibecoding/AgentNativeDB/config"
)

// ANSI 颜色
const (
	cliColorReset  = "\033[0m"
	cliColorRed    = "\033[31m"
	cliColorGreen  = "\033[32m"
	cliColorYellow = "\033[33m"
	cliColorCyan   = "\033[36m"
	cliColorWhite  = "\033[37m"
	cliColorBold   = "\033[1m"
)

type clientAPIResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type httpClient struct {
	baseURL    string
	httpClient *http.Client
	outputFmt  string
}

func runClient(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	cfgPath := fs.String("config", "", "配置文件路径")
	serverAddr := fs.String("server", "", "服务器地址 (host:port)")
	outputFmt := fs.String("format", "table", "输出格式: table, json")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		cfg = config.Default()
	}

	addr := *serverAddr
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	c := &httpClient{
		baseURL:    fmt.Sprintf("http://%s", addr),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		outputFmt:  *outputFmt,
	}

	if err := c.checkHealth(); err != nil {
		fmt.Fprintf(os.Stderr, "%s无法连接到服务器 %s: %v%s\n", cliColorRed, addr, err, cliColorReset)
		os.Exit(1)
	}

	fmt.Printf("%s✓ 已连接到 %s%s\n", cliColorGreen, addr, cliColorReset)
	fmt.Println("输入 'help' 查看命令，'quit' 退出。")
	fmt.Println()

	clientRunInteractive(c)
}

func (c *httpClient) checkHealth() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	var r clientAPIResponse
	return json.NewDecoder(resp.Body).Decode(&r)
}

func clientRunInteractive(c *httpClient) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for {
		fmt.Print("andb-client> ")
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()
		input = strings.TrimSpace(input)
		if input == "quit" || input == "exit" {
			break
		}
		if input == "" {
			continue
		}

		clientExecCommand(c, input)
	}
	fmt.Println("再见!")
}

func clientExecCommand(c *httpClient, input string) {
	args := strings.Fields(input)
	if len(args) == 0 {
		return
	}

	switch strings.ToLower(args[0]) {
	case "help":
		clientPrintHelp()
	case "health":
		if err := c.checkHealth(); err != nil {
			fmt.Printf("%s✗ 健康检查失败: %v%s\n", cliColorRed, err, cliColorReset)
		} else {
			fmt.Printf("%s✓ 服务器正常%s\n", cliColorGreen, cliColorReset)
		}
	case "query", "sql":
		if len(args) < 2 {
			fmt.Println("用法: query <SQL语句>")
			return
		}
		c.executeQuery(strings.Join(args[1:], " "))
	case "sessions":
		c.listSessions()
	default:
		fmt.Printf("未知命令: %s\n", args[0])
	}
}

func (c *httpClient) executeQuery(sql string) {
	data, _ := json.Marshal(map[string]string{"sql": sql})
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/query", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", cliColorRed, err, cliColorReset)
		return
	}
	defer resp.Body.Close()

	var r clientAPIResponse
	json.NewDecoder(resp.Body).Decode(&r)
	if !r.OK {
		fmt.Printf("%s错误: %s%s\n", cliColorRed, r.Error, cliColorReset)
		return
	}

	var result struct {
		Columns []string         `json:"columns"`
		Rows    []map[string]any `json:"rows"`
	}
	json.Unmarshal(r.Data, &result)

	if len(result.Rows) == 0 {
		fmt.Println("(无数据)")
		return
	}

	// 简单表格输出
	widths := make(map[string]int)
	for _, col := range result.Columns {
		widths[col] = len(col)
	}
	for _, row := range result.Rows {
		for _, col := range result.Columns {
			val := fmt.Sprintf("%v", row[col])
			if len(val) > widths[col] {
				widths[col] = len(val)
			}
		}
	}

	for _, col := range result.Columns {
		fmt.Printf("%-*s  ", widths[col], col)
	}
	fmt.Println()
	for _, row := range result.Rows {
		for _, col := range result.Columns {
			fmt.Printf("%-*s  ", widths[col], fmt.Sprintf("%v", row[col]))
		}
		fmt.Println()
	}
	fmt.Printf("(%d 行)\n", len(result.Rows))
}

func (c *httpClient) listSessions() {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/sessions")
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", cliColorRed, err, cliColorReset)
		return
	}
	defer resp.Body.Close()

	var r clientAPIResponse
	json.NewDecoder(resp.Body).Decode(&r)
	if !r.OK {
		fmt.Printf("%s错误: %s%s\n", cliColorRed, r.Error, cliColorReset)
		return
	}

	var sessions []map[string]any
	json.Unmarshal(r.Data, &sessions)

	if len(sessions) == 0 {
		fmt.Println("(无会话)")
		return
	}

	for i, s := range sessions {
		fmt.Printf("%s%d.%s ID: %s%v%s Agent: %v State: %s%v%s\n",
			cliColorCyan, i+1, cliColorReset,
			cliColorGreen, s["id"], cliColorReset,
			s["agent_id"],
			cliColorYellow, s["state"], cliColorReset)
	}
}

func clientPrintHelp() {
	fmt.Println(`命令:
  help              显示帮助
  health            检查服务器健康
  sessions          列出所有会话
  query <SQL>       执行 SQL 查询
  quit              退出`)
}
