package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultServer = "http://localhost:8400"
	queryEndpoint = "/api/v1/query"
	totalRows     = 1000
)

// API 响应结构
type apiResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// SQL 查询结果
type queryResult struct {
	Columns []string        `json:"columns"`
	Rows    []queryRow      `json:"rows"`
}

type queryRow struct {
	Values map[string]any `json:"values"`
}

var (
	serverURL string
	httpCli   = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	serverURL = defaultServer
	if len(os.Args) > 1 {
		serverURL = os.Args[1]
	}
	// 去掉末尾斜杠
	serverURL = strings.TrimRight(serverURL, "/")

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       AgentNativeDB Mock 数据生成器          ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Printf("服务器: %s\n", serverURL)
	fmt.Println()

	// 检查服务器连接
	if err := healthCheck(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法连接服务器: %v\n", err)
		fmt.Fprintf(os.Stderr, "   请先启动服务器: ./bin/andb server\n")
		os.Exit(1)
	}
	fmt.Println("✅ 服务器连接正常")

	// 创建表
	fmt.Println()
	fmt.Println("── 创建表 ──")
	createTables()

	// 插入数据
	fmt.Println()
	fmt.Printf("── 插入 %d 条 mock 数据 ──\n", totalRows)
	insertMockData()

	// 验证
	fmt.Println()
	fmt.Println("── 验证数据 ──")
	verifyData()

	fmt.Println()
	fmt.Println("✅ Mock 数据生成完成!")
}

// execSQL 执行 SQL 语句
func execSQL(sql string) (*queryResult, error) {
	body, _ := json.Marshal(map[string]string{"sql": sql})
	resp, err := httpCli.Post(serverURL+queryEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("SQL 执行失败: %s", apiResp.Error)
	}

	var result queryResult
	if len(apiResp.Data) > 0 {
		json.Unmarshal(apiResp.Data, &result)
	}
	return &result, nil
}

// healthCheck 健康检查
func healthCheck() error {
	resp, err := httpCli.Get(serverURL + "/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("状态码: %d", resp.StatusCode)
	}
	return nil
}

// createTables 建表
func createTables() {
	tables := []struct {
		name string
		sql  string
	}{
		{
			name: "mock_users",
			sql: `CREATE TABLE mock_users (
				id STRING PRIMARY KEY,
				username VARCHAR,
				email VARCHAR,
				age INT,
				score FLOAT,
				active BOOL,
				bio TEXT,
				level INTEGER,
				weight FLOAT,
				verified BOOLEAN
			)`,
		},
		{
			name: "mock_products",
			sql: `CREATE TABLE mock_products (
				id STRING PRIMARY KEY,
				name VARCHAR,
				price FLOAT,
				stock INT,
				on_sale BOOL,
				description TEXT,
				rating FLOAT,
				weight FLOAT,
				discontinued BOOLEAN,
				brand STRING
			)`,
		},
	}

	for _, t := range tables {
		// 先删除已存在的表（忽略错误）
		execSQL(fmt.Sprintf("DROP TABLE %s", t.name))

		_, err := execSQL(t.sql)
		if err != nil {
			fmt.Printf("  ❌ 创建表 %s 失败: %v\n", t.name, err)
			continue
		}
		fmt.Printf("  ✅ 表 %s 创建成功\n", t.name)
	}
}

// 随机数据生成工具
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

var firstNames = []string{
	"张", "李", "王", "赵", "刘", "陈", "杨", "黄", "吴", "周",
	"徐", "孙", "马", "朱", "胡", "郭", "何", "林", "罗", "高",
}

var lastNames = []string{
	"伟", "芳", "娜", "敏", "静", "丽", "强", "磊", "洋", "勇",
	"艳", "杰", "涛", "明", "超", "秀英", "华", "慧", "建华", "建国",
}

var domains = []string{
	"gmail.com", "outlook.com", "qq.com", "163.com", "yahoo.com",
	"icloud.com", "hotmail.com", "foxmail.com",
}

var productAdjs = []string{
	"高端", "轻薄", "智能", "经典", "旗舰", "超值", "专业", "迷你", "豪华", "简约",
}

var productNouns = []string{
	"笔记本电脑", "无线耳机", "机械键盘", "智能手表", "平板电脑",
	"蓝牙音箱", "摄像头", "显示器", "鼠标", "充电宝",
	"手机壳", "数据线", "路由器", "硬盘", "内存条",
}

var brands = []string{
	"TechPro", "NovaByte", "QuantumX", "SkyLine", "Zenith",
	"Pulse", "Echo", "Vertex", "Apex", "Orbit",
}

var bioTemplates = []string{
	"热爱编程的%s开发者，专注于%s领域",
	"全栈工程师，%d年经验，喜欢开源",
	"产品经理转型技术，对%s有深入研究",
	"自由职业者，远程工作%d年，热爱旅行",
	"学生党，正在学习%s，目标是全栈",
	"连续创业者，第%d次创业，专注于%s",
	"资深架构师，擅长%s和分布式系统",
	"技术博主，粉丝%d万，分享%s经验",
}

var techFields = []string{
	"人工智能", "区块链", "云计算", "大数据", "物联网",
	"前端开发", "后端开发", "DevOps", "安全", "游戏开发",
	"移动开发", "嵌入式", "数据科学", "机器学习", "深度学习",
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}

func randomEmail(username string) string {
	return fmt.Sprintf("%s@%s", username, domains[rng.Intn(len(domains))])
}

func randomName() string {
	return firstNames[rng.Intn(len(firstNames))] + lastNames[rng.Intn(len(lastNames))]
}

// fmtVal 格式化从 JSON 解析出来的数值（float64），整数去掉小数点
func fmtVal(v any) string {
	if v == nil {
		return "NULL"
	}
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return fmt.Sprintf("%d", int64(f))
		}
		return fmt.Sprintf("%.2f", f)
	}
	return fmt.Sprintf("%v", v)
}

func randomBio() string {
	tmpl := bioTemplates[rng.Intn(len(bioTemplates))]
	field := techFields[rng.Intn(len(techFields))]
	years := rng.Intn(15) + 1
	fans := rng.Intn(500) + 1
	return fmt.Sprintf(tmpl, field, years, fans, field)
}

func randomProductName() string {
	return productAdjs[rng.Intn(len(productAdjs))] + productNouns[rng.Intn(len(productNouns))]
}

// insertMockData 插入 mock 数据
func insertMockData() {
	start := time.Now()

	// ── mock_users: 逐条 INSERT ──
	fmt.Printf("  📝 插入 mock_users ... ")
	usersOK := 0
	for i := 0; i < totalRows; i++ {
		id := fmt.Sprintf("u_%04d", i)
		username := randomName() + "_" + randomString(4)
		email := randomEmail(username)
		age := rng.Intn(60) + 18
		score := fmt.Sprintf("%.2f", rng.Float64()*100)
		active := rng.Intn(2) == 1
		bio := strings.ReplaceAll(randomBio(), "'", "''")
		level := rng.Intn(100) + 1
		weight := fmt.Sprintf("%.1f", 45.0+rng.Float64()*60)
		verified := rng.Intn(3) == 0

		sql := fmt.Sprintf(
			"INSERT INTO mock_users (id, username, email, age, score, active, bio, level, weight, verified) VALUES ('%s', '%s', '%s', %d, %s, %t, '%s', %d, %s, %t)",
			id, username, email, age, score, active, bio, level, weight, verified,
		)
		if _, err := execSQL(sql); err != nil {
			if i < 3 {
				fmt.Printf("\n    ⚠️  第 %d 条失败: %v\n", i+1, err)
			}
			continue
		}
		usersOK++
		if (i+1)%200 == 0 {
			fmt.Printf("%d..", i+1)
		}
	}
	fmt.Printf(" 完成 (%d/%d)\n", usersOK, totalRows)

	// ── mock_products: 逐条 INSERT ──
	fmt.Printf("  📝 插入 mock_products ... ")
	productsOK := 0
	for i := 0; i < totalRows; i++ {
		id := fmt.Sprintf("p_%04d", i)
		name := randomProductName()
		price := fmt.Sprintf("%.2f", 9.99+rng.Float64()*9990)
		stock := rng.Intn(10000)
		onSale := rng.Intn(2) == 1
		desc := fmt.Sprintf("这是%s的详细描述，采用先进技术打造，品质卓越。编号: %s", name, randomString(6))
		desc = strings.ReplaceAll(desc, "'", "''")
		rating := fmt.Sprintf("%.1f", 1.0+rng.Float64()*4.0)
		weight := fmt.Sprintf("%.2f", 0.1+rng.Float64()*50)
		discontinued := rng.Intn(10) == 0
		brand := brands[rng.Intn(len(brands))]

		sql := fmt.Sprintf(
			"INSERT INTO mock_products (id, name, price, stock, on_sale, description, rating, weight, discontinued, brand) VALUES ('%s', '%s', %s, %d, %t, '%s', %s, %s, %t, '%s')",
			id, name, price, stock, onSale, desc, rating, weight, discontinued, brand,
		)
		if _, err := execSQL(sql); err != nil {
			if i < 3 {
				fmt.Printf("\n    ⚠️  第 %d 条失败: %v\n", i+1, err)
			}
			continue
		}
		productsOK++
		if (i+1)%200 == 0 {
			fmt.Printf("%d..", i+1)
		}
	}
	fmt.Printf(" 完成 (%d/%d)\n", productsOK, totalRows)

	elapsed := time.Since(start)
	total := usersOK + productsOK
	fmt.Printf("  ⏱  总耗时: %s (%.1f 条/秒)\n", elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds())
}

// verifyData 验证数据
func verifyData() {
	tables := []string{"mock_users", "mock_products"}
	for _, t := range tables {
		result, err := execSQL(fmt.Sprintf("SELECT COUNT(*) AS cnt FROM %s", t))
		if err != nil {
			fmt.Printf("  ❌ 查询 %s 失败: %v\n", t, err)
			continue
		}
		cnt := "?"
		if len(result.Rows) > 0 {
			if v, ok := result.Rows[0].Values["cnt"]; ok {
				cnt = fmt.Sprintf("%v", v)
			}
		}
		fmt.Printf("  📊 %-20s %s 条记录\n", t, cnt)
	}

	// 展示 mock_users 的前 3 条
	fmt.Println()
	fmt.Println("  📋 mock_users 示例 (前 3 条):")
	result, err := execSQL("SELECT id, username, email, age, score, active FROM mock_users LIMIT 3")
	if err != nil {
		fmt.Printf("    查询失败: %v\n", err)
		return
	}
	for _, row := range result.Rows {
		fmt.Printf("    %-8s %-16s %-28s %-4s %-8s %v\n",
			row.Values["id"], row.Values["username"], row.Values["email"],
			fmtVal(row.Values["age"]), fmtVal(row.Values["score"]), row.Values["active"],
		)
	}

	// 展示 mock_products 的前 3 条
	fmt.Println()
	fmt.Println("  📋 mock_products 示例 (前 3 条):")
	result, err = execSQL("SELECT id, name, price, stock, brand, rating FROM mock_products LIMIT 3")
	if err != nil {
		fmt.Printf("    查询失败: %v\n", err)
		return
	}
	for _, row := range result.Rows {
		fmt.Printf("    %-8s %-20s %-10s %-6s %-10s %v\n",
			row.Values["id"], row.Values["name"], fmtVal(row.Values["price"]),
			fmtVal(row.Values["stock"]), row.Values["brand"], fmtVal(row.Values["rating"]),
		)
	}

	// 展示表结构
	fmt.Println()
	for _, t := range []string{"mock_users", "mock_products"} {
		fmt.Printf("  🔍 DESCRIBE %s:\n", t)
		desc, err := execSQL(fmt.Sprintf("DESCRIBE %s", t))
		if err != nil {
			fmt.Printf("    查询失败: %v\n", err)
			continue
		}
		for _, row := range desc.Rows {
			fmt.Printf("    %-14s %-12s key=%v null=%v\n",
				row.Values["field"], row.Values["type"],
				row.Values["key"], row.Values["null"],
			)
		}
	}
}
