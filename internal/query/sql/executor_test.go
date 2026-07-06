package sql

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func setupTestDB(t *testing.T) *Executor {
	t.Helper()
	engine := storage.NewTestEngine(t)

	ctx := context.Background()

	// 插入测试数据
	sessions := []map[string]any{
		{"id": "s1", "agent_id": "agent-001", "state": "active"},
		{"id": "s2", "agent_id": "agent-001", "state": "completed"},
		{"id": "s3", "agent_id": "agent-002", "state": "active"},
	}
	for _, s := range sessions {
		data, _ := json.Marshal(s)
		key := storage.EncodeKey(storage.PrefixSession, s["id"].(string))
		engine.Set(ctx, key, data)
	}

	memories := []map[string]any{
		{"id": "m1", "session_id": "s1", "type": "short_term", "content": "hello", "importance": 0.3},
		{"id": "m2", "session_id": "s1", "type": "long_term", "content": "world", "importance": 0.8},
		{"id": "m3", "session_id": "s2", "type": "short_term", "content": "foo", "importance": 0.5},
		{"id": "m4", "session_id": "s2", "type": "long_term", "content": "bar", "importance": 0.9},
	}
	for _, m := range memories {
		data, _ := json.Marshal(m)
		key := storage.EncodeKey(storage.PrefixMemory, m["id"].(string))
		engine.Set(ctx, key, data)
	}

	return NewExecutor(engine)
}

func queryHelper(t *testing.T, executor *Executor, sql string) *Result {
	t.Helper()
	stmt, err := Parse(sql)
	if err != nil {
		t.Fatalf("parse %q: %v", sql, err)
	}
	planner := NewPlanner()
	plan, err := planner.Plan(stmt)
	if err != nil {
		t.Fatalf("plan %q: %v", sql, err)
	}
	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute %q: %v", sql, err)
	}
	return result
}

func TestExecutor_SelectAll(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions")

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectWithWhere(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions WHERE state = 'active'")

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 active sessions, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectColumns(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT id, agent_id FROM agent_sessions WHERE id = 's1'")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if row.Values["id"] != "s1" {
		t.Fatalf("expected id=s1, got %v", row.Values["id"])
	}
}

func TestExecutor_SelectLimit(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions LIMIT 2")

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectOrderBy(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions ORDER BY id ASC")

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}

	ids := []string{
		result.Rows[0].Values["id"].(string),
		result.Rows[1].Values["id"].(string),
		result.Rows[2].Values["id"].(string),
	}

	if ids[0] >= ids[1] || ids[1] >= ids[2] {
		t.Fatalf("expected ascending order, got %v", ids)
	}
}

func TestExecutor_SelectOrderByDesc(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions ORDER BY id DESC")

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}

	ids := []string{
		result.Rows[0].Values["id"].(string),
		result.Rows[1].Values["id"].(string),
		result.Rows[2].Values["id"].(string),
	}

	if ids[0] <= ids[1] || ids[1] <= ids[2] {
		t.Fatalf("expected descending order, got %v", ids)
	}
}

func TestExecutor_SelectLimitOffset(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions ORDER BY id ASC LIMIT 1 OFFSET 1")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// 第二个（按 id 排序）
	if result.Rows[0].Values["id"] != "s2" {
		t.Fatalf("expected s2, got %v", result.Rows[0].Values["id"])
	}
}

func TestExecutor_SelectMemories(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_memories WHERE type = 'long_term'")

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 long_term memories, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectComparison(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_memories WHERE importance > 0.5")

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows with importance > 0.5, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectMultipleConditions(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_memories WHERE type = 'short_term' AND importance > 0.3")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0].Values["id"] != "m3" {
		t.Fatalf("expected m3, got %v", result.Rows[0].Values["id"])
	}
}

func TestExecutor_SelectOrCondition(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions WHERE agent_id = 'agent-001' OR agent_id = 'agent-002'")

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectNotEqual(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT * FROM agent_sessions WHERE state != 'active'")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 non-active session, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectCount(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT COUNT(*) FROM agent_sessions")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	count, ok := result.Rows[0].Values["COUNT(*)"].(int64)
	if !ok {
		t.Fatalf("expected int64 count, got %T: %v", result.Rows[0].Values["COUNT(*)"], result.Rows[0].Values["COUNT(*)"])
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestExecutor_SelectCountWithAlias(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT COUNT(*) as total FROM agent_sessions")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	total, ok := result.Rows[0].Values["total"].(int64)
	if !ok {
		t.Fatalf("expected int64 total, got %T", result.Rows[0].Values["total"])
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
}

func TestExecutor_SelectGroupBy(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT agent_id, COUNT(*) as cnt FROM agent_sessions GROUP BY agent_id")

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectGroupByHaving(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT agent_id, COUNT(*) as cnt FROM agent_sessions GROUP BY agent_id HAVING cnt > 1")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 group with cnt > 1, got %d", len(result.Rows))
	}
}

func TestExecutor_SelectSumAvg(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT SUM(importance) as total, AVG(importance) as avg_imp FROM agent_memories")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	total, ok := result.Rows[0].Values["total"].(float64)
	if !ok {
		t.Fatalf("expected float64 total, got %T", result.Rows[0].Values["total"])
	}
	// 0.3 + 0.8 + 0.5 + 0.9 = 2.5
	if total < 2.49 || total > 2.51 {
		t.Fatalf("expected total ~2.5, got %f", total)
	}
}

func TestExecutor_SelectMinMax(t *testing.T) {
	executor := setupTestDB(t)
	result := queryHelper(t, executor, "SELECT MIN(importance) as min_imp, MAX(importance) as max_imp FROM agent_memories")

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	minVal, ok := result.Rows[0].Values["min_imp"].(float64)
	if !ok {
		t.Fatalf("expected float64 min, got %T", result.Rows[0].Values["min_imp"])
	}
	if minVal < 0.29 || minVal > 0.31 {
		t.Fatalf("expected min ~0.3, got %f", minVal)
	}

	maxVal := result.Rows[0].Values["max_imp"].(float64)
	if maxVal < 0.89 || maxVal > 0.91 {
		t.Fatalf("expected max ~0.9, got %f", maxVal)
	}
}

func TestExecutor_EndToEndWorkflow(t *testing.T) {
	executor := setupTestDB(t)

	// 查询活跃会话
	result := queryHelper(t, executor, "SELECT id, agent_id FROM agent_sessions WHERE state = 'active' ORDER BY id ASC")
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 active sessions, got %d", len(result.Rows))
	}

	// 统计各类型记忆数量
	result = queryHelper(t, executor, "SELECT type, COUNT(*) as cnt FROM agent_memories GROUP BY type")
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 memory types, got %d", len(result.Rows))
	}

	// 查询高重要性记忆
	result = queryHelper(t, executor, "SELECT id, content, importance FROM agent_memories WHERE importance > 0.7 ORDER BY importance DESC")
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 high-importance memories, got %d", len(result.Rows))
	}
}

func BenchmarkExecutor_SelectAll(b *testing.B) {
	engine := storage.NewTestEngine(b)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		data, _ := json.Marshal(map[string]any{"id": "s" + string(rune(i)), "agent_id": "agent-001", "state": "active"})
		key := storage.EncodeKey(storage.PrefixSession, "s"+string(rune(i)))
		engine.Set(ctx, key, data)
	}

	executor := NewExecutor(engine)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stmt, _ := Parse("SELECT * FROM agent_sessions WHERE state = 'active'")
		plan, _ := NewPlanner().Plan(stmt)
		executor.Execute(ctx, plan)
	}
}
