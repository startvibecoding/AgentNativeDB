package agentnativedb_test

import (
	"os"
	"testing"

	. "github.com/startvibecoding/AgentNativeDB/sdk"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}

func TestDB_OpenClose(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestDB_Session(t *testing.T) {
	db := setupTestDB(t)

	// 创建
	sess, err := db.CreateSession("agent-001", map[string]any{"model": "gpt-4"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID() == "" {
		t.Fatal("expected non-empty ID")
	}
	if sess.AgentID() != "agent-001" {
		t.Fatalf("expected agent-001, got %s", sess.AgentID())
	}
	if sess.State() != "active" {
		t.Fatalf("expected active, got %s", sess.State())
	}

	// 获取
	got, err := db.GetSession(sess.ID())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID() != sess.ID() {
		t.Fatalf("ID mismatch")
	}

	// 列出
	sessions, err := db.ListSessions("agent-001")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// 关闭
	db.CloseSession(sess.ID())
	s, _ := db.GetSession(sess.ID())
	if s.State() != "completed" {
		t.Fatalf("expected completed, got %s", s.State())
	}
}

func TestDB_Memory(t *testing.T) {
	db := setupTestDB(t)

	sess, _ := db.CreateSession("agent-001")

	// 存储
	mem, err := db.StoreMemory(sess.ID(), "用户偏好中文回复", LongTerm, 0.8)
	if err != nil {
		t.Fatalf("store memory: %v", err)
	}
	if mem.Content() != "用户偏好中文回复" {
		t.Fatalf("content mismatch")
	}

	// 存储更多
	db.StoreMemory(sess.ID(), "短期记忆", ShortTerm, 0.3)
	db.StoreMemory(sess.ID(), "工作上下文", Working, 1.0)

	// 检索所有
	all, err := db.RecallMemories(sess.ID(), nil)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(all))
	}

	// 按类型检索
	lt := LongTerm
	longTerm, _ := db.RecallMemories(sess.ID(), &lt)
	if len(longTerm) != 1 {
		t.Fatalf("expected 1 long-term, got %d", len(longTerm))
	}

	// 带 embedding 存储
	embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	db.StoreMemory(sess.ID(), "带向量的记忆", LongTerm, 0.9, embedding)
}

func TestDB_Decision(t *testing.T) {
	db := setupTestDB(t)

	sess, _ := db.CreateSession("agent-001")

	// 记录决策
	dec, err := db.RecordDecision(sess.ID(), ToolCall,
		map[string]any{"tool": "search", "query": "test"},
		map[string]any{"results": []string{"a", "b"}},
		"需要搜索相关信息",
	)
	if err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if dec.Type() != ToolCall {
		t.Fatalf("expected tool_call, got %s", dec.Type())
	}

	// 记录子决策
	child, _ := db.RecordDecision(sess.ID(), Reasoning,
		"分析搜索结果", "选择结果 a",
		"因为相关性更高",
	)

	// 决策树
	tree, err := db.GetDecisionTree(dec.ID())
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}
	if tree.Decision.ID != dec.ID() {
		t.Fatalf("root ID mismatch")
	}
	_ = child
}

func TestDB_SQL(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试数据
	db.CreateSession("agent-001")
	db.CreateSession("agent-002")

	// SELECT
	result, err := db.Query("SELECT * FROM agent_sessions")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// WHERE
	result, _ = db.Query("SELECT * FROM agent_sessions WHERE agent_id = 'agent-001'")
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// COUNT
	result, _ = db.Query("SELECT COUNT(*) FROM agent_sessions")
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// INSERT
	result, err = db.Query("INSERT INTO agent_sessions (id, agent_id, state) VALUES ('s3', 'agent-003', 'active')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if result.RowsAffected != 1 {
		t.Fatalf("expected 1 affected, got %d", result.RowsAffected)
	}

	// INSERT 自动生成 ID
	result, err = db.Query("INSERT INTO agent_sessions (agent_id, state) VALUES ('agent-004', 'active')")
	if err != nil {
		t.Fatalf("insert auto id: %v", err)
	}

	// 验证
	result, _ = db.Query("SELECT * FROM agent_sessions")
	if len(result.Rows) != 4 {
		t.Fatalf("expected 4 rows after inserts, got %d", len(result.Rows))
	}
}

func TestDB_Vector(t *testing.T) {
	db := setupTestDB(t)

	// 创建索引
	err := db.CreateIndex("test", 4, "cosine")
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	// 插入
	db.InsertVector("test", "v1", []float32{1, 0, 0, 0})
	db.InsertVector("test", "v2", []float32{0, 1, 0, 0})
	db.InsertVector("test", "v3", []float32{0.7, 0.7, 0, 0})

	// 搜索
	results, err := db.SearchVector("test", []float32{1, 0, 0, 0}, 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "v1" {
		t.Fatalf("expected v1 nearest, got %s", results[0].ID)
	}

	// 删除
	db.DeleteVector("test", "v1")
	results, _ = db.SearchVector("test", []float32{1, 0, 0, 0}, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(results))
	}
}

func TestDB_Graph(t *testing.T) {
	db := setupTestDB(t)

	// 添加节点
	db.AddNode("a", "Person", "Alice")
	db.AddNode("b", "Person", "Bob")
	db.AddNode("c", "Person", "Charlie")

	// 添加边
	db.AddEdge("e1", "KNOWS", "a", "b")
	db.AddEdge("e2", "KNOWS", "b", "c")

	// 邻居
	neighbors, _ := db.GetNeighbors("b", "both")
	if len(neighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(neighbors))
	}

	// 最短路径
	path, _ := db.ShortestPath("a", "c")
	if len(path) != 3 {
		t.Fatalf("expected path length 3, got %d: %v", len(path), path)
	}

	// K 跳
	twoHop, _ := db.KHopNeighbors("a", 2)
	if len(twoHop) != 1 || twoHop[0].ID != "c" {
		t.Fatalf("expected [c] at 2-hop, got %v", twoHop)
	}
}

func TestDB_Collaboration(t *testing.T) {
	db := setupTestDB(t)

	// 创建房间
	room, err := db.CreateRoom("dev-team", "agent-001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	// 发送消息
	db.SendMessage(room.ID, "agent-001", "大家好")
	db.SendMessage(room.ID, "agent-001", "开始工作")

	// 获取消息
	msgs, _ := db.GetMessages(room.ID)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// 共享记忆
	db.ShareMemory(room.ID, "agent-001", "重要发现", 0.9)
}

func TestDB_TaskQueue(t *testing.T) {
	db := setupTestDB(t)

	// 入队
	task, err := db.EnqueueTask("agent-001", "analyze", 1, map[string]any{"data": "test"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// 出队
	dequeued, _ := db.DequeueTask()
	if dequeued.ID != task.ID {
		t.Fatalf("task ID mismatch")
	}

	// 完成
	db.CompleteTask(task.ID, map[string]any{"result": "done"})
}

func TestDB_Lineage(t *testing.T) {
	db := setupTestDB(t)

	db.RecordLineage("raw-1", "raw", nil)
	db.RecordLineage("raw-2", "raw", nil)
	db.RecordLineage("derived-1", "derived", []string{"raw-1", "raw-2"})
	db.RecordLineage("final-1", "agent_generated", []string{"derived-1"})

	tree, err := db.TraceLineage("final-1", 3)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if tree.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", tree.Depth())
	}
}

func TestDB_UUID(t *testing.T) {
	id := UUID()
	if len(id) != 36 {
		t.Fatalf("expected UUID length 36, got %d", len(id))
	}
}

func TestDB_FullWorkflow(t *testing.T) {
	db := setupTestDB(t)

	// 1. 创建会话
	sess, _ := db.CreateSession("analyst-001")

	// 2. 存储记忆
	db.StoreMemory(sess.ID(), "用户需要分析销售数据", LongTerm, 0.9)
	db.StoreMemory(sess.ID(), "数据来源: Q1 报表", ShortTerm, 0.5)

	// 3. 记录决策
	dec, _ := db.RecordDecision(sess.ID(), Planning,
		"分析销售数据", "先筛选再聚合",
		"数据量大，需要先过滤",
	)

	// 4. SQL 查询
	result, _ := db.Query("SELECT * FROM agent_memories WHERE session_id = '" + sess.ID() + "'")
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(result.Rows))
	}

	// 5. 向量搜索
	db.CreateIndex("memories", 4)
	db.InsertVector("memories", "m1", []float32{0.1, 0.2, 0.3, 0.4})
	db.SearchVector("memories", []float32{0.1, 0.2, 0.3, 0.4}, 1)

	// 6. 知识图谱
	db.AddNode("product-A", "Product", "Widget A")
	db.AddNode("customer-B", "Customer", "Acme Corp")
	db.AddEdge("order-1", "ORDERS", "customer-B", "product-A")

	// 7. 数据血缘
	db.RecordLineage("report-1", "derived", []string{"q1-data"})

	// 8. 关闭会话
	db.CloseSession(sess.ID())

	_ = dec
}
