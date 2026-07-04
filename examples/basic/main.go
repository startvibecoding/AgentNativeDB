// AgentNativeDB SDK 基础使用示例
//
// 运行: go run ./examples/basic
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/startvibecoding/AgentNativeDB/sdk"
)

func main() {
	// 创建临时目录
	dir := "./example-data"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	// 打开数据库
	db, err := agentnativedb.Open(dir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== AgentNativeDB SDK 示例 ===")
	fmt.Println()

	// 1. 创建会话
	fmt.Println("1. 创建 Agent 会话")
	sess, _ := db.CreateSession("analyst-001", map[string]any{"model": "gpt-4"})
	fmt.Printf("   会话 ID: %s\n\n", sess.ID())

	// 2. 存储记忆
	fmt.Println("2. 存储 Agent 记忆")
	db.StoreMemory(sess.ID(), "用户偏好中文回复", agentnativedb.LongTerm, 0.9)
	db.StoreMemory(sess.ID(), "当前任务: 分析销售数据", agentnativedb.ShortTerm, 0.7)
	db.StoreMemory(sess.ID(), "数据来源: Q1 报表", agentnativedb.Working, 1.0)
	fmt.Println("   存储了 3 条记忆")
	fmt.Println()

	// 3. 检索记忆
	fmt.Println("3. 检索记忆")
	memories, _ := db.RecallMemories(sess.ID(), nil)
	for _, m := range memories {
		fmt.Printf("   [%s] %s (重要度: %.1f)\n", m.Type(), m.Content(), m.Importance())
	}
	fmt.Println()

	// 4. 记录决策
	fmt.Println("4. 记录 Agent 决策")
	dec, _ := db.RecordDecision(sess.ID(), agentnativedb.Planning,
		"分析销售数据", "先按区域筛选再聚合",
		"数据量大，需要先过滤",
	)
	fmt.Printf("   决策 ID: %s, 类型: %s\n\n", dec.ID(), dec.Type())

	// 5. SQL 查询
	fmt.Println("5. SQL 查询")
	result, _ := db.Query("SELECT id, agent_id, state FROM agent_sessions")
	fmt.Printf("   查询结果: %d 行\n", len(result.Rows))
	for _, row := range result.Rows {
		fmt.Printf("   %v\n", row.Values)
	}
	fmt.Println()

	// 6. 向量搜索
	fmt.Println("6. 向量搜索")
	db.CreateIndex("docs", 4, "cosine")
	db.InsertVector("docs", "doc-1", []float32{0.1, 0.2, 0.3, 0.4})
	db.InsertVector("docs", "doc-2", []float32{0.9, 0.1, 0.0, 0.0})
	db.InsertVector("docs", "doc-3", []float32{0.1, 0.2, 0.4, 0.3})

	results, _ := db.SearchVector("docs", []float32{0.1, 0.2, 0.3, 0.4}, 3)
	for _, r := range results {
		fmt.Printf("   %s: 距离=%.4f 相似度=%.4f\n", r.ID, r.Distance, r.Score)
	}
	fmt.Println()

	// 7. 知识图谱
	fmt.Println("7. 知识图谱")
	db.AddNode("product-A", "Product", "Widget A")
	db.AddNode("customer-B", "Customer", "Acme Corp")
	db.AddNode("order-1", "Order", "Order #001")
	db.AddEdge("e1", "ORDERS", "customer-B", "order-1")
	db.AddEdge("e2", "CONTAINS", "order-1", "product-A")

	path, _ := db.ShortestPath("customer-B", "product-A")
	fmt.Printf("   最短路径: %v\n", path)

	neighbors, _ := db.GetNeighbors("order-1")
	fmt.Printf("   order-1 的邻居: %d 个\n", len(neighbors))
	fmt.Println()

	// 8. 多 Agent 协作
	fmt.Println("8. 多 Agent 协作")
	room, _ := db.CreateRoom("analysis-team", "analyst-001")
	db.SendMessage(room.ID, "analyst-001", "开始分析")
	db.SendMessage(room.ID, "analyst-001", "发现异常模式")

	msgs, _ := db.GetMessages(room.ID)
	fmt.Printf("   房间消息: %d 条\n", len(msgs))
	fmt.Println()

	// 9. 数据血缘
	fmt.Println("9. 数据血缘")
	db.RecordLineage("q1-data", "raw", nil)
	db.RecordLineage("report-1", "derived", []string{"q1-data"})

	tree, _ := db.TraceLineage("report-1", 5)
	fmt.Printf("   血缘深度: %d\n", tree.Depth())
	fmt.Println()

	// 10. 关闭会话
	fmt.Println("10. 完成任务")
	db.CloseSession(sess.ID())
	fmt.Println("    会话已关闭")

	fmt.Println("\n=== 示例完成 ===")
}
