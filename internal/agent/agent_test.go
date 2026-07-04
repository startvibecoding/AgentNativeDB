package agent

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type testEnv struct {
	engine  *badgerstore.BadgerEngine
	cache   *storage.Cache
	session *SessionManager
	memory  *MemoryStore
	decision *DecisionRecorder
	ctx     context.Context
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()

	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open engine: %v", err)
	}

	t.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})

	cache := storage.NewCache(512)

	return &testEnv{
		engine:   engine,
		cache:    cache,
		session:  NewSessionManager(engine, cache),
		memory:   NewMemoryStore(engine, cache),
		decision: NewDecisionRecorder(engine, cache),
		ctx:      context.Background(),
	}
}

// ========== Session Tests ==========

func TestSessionManager_Create(t *testing.T) {
	env := newTestEnv(t)

	s, err := env.session.Create(env.ctx, "agent-001", map[string]any{"model": "gpt-4"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if s.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if s.AgentID != "agent-001" {
		t.Fatalf("expected agent-001, got %s", s.AgentID)
	}
	if s.State != model.SessionActive {
		t.Fatalf("expected active state, got %s", s.State)
	}
}

func TestSessionManager_Get(t *testing.T) {
	env := newTestEnv(t)

	created, _ := env.session.Create(env.ctx, "agent-001", nil)
	fetched, err := env.session.Get(env.ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if fetched.ID != created.ID {
		t.Fatalf("ID mismatch: %s vs %s", fetched.ID, created.ID)
	}
	if fetched.AgentID != created.AgentID {
		t.Fatalf("AgentID mismatch: %s vs %s", fetched.AgentID, created.AgentID)
	}
}

func TestSessionManager_Update(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	s.State = model.SessionPaused
	s.Context["key"] = "value"

	err := env.session.Update(env.ctx, s)
	if err != nil {
		t.Fatalf("update session: %v", err)
	}

	fetched, _ := env.session.Get(env.ctx, s.ID)
	if fetched.State != model.SessionPaused {
		t.Fatalf("expected paused, got %s", fetched.State)
	}
	if fetched.Context["key"] != "value" {
		t.Fatalf("expected context key=value, got %v", fetched.Context)
	}
}

func TestSessionManager_Delete(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	err := env.session.Delete(env.ctx, s.ID)
	if err != nil {
		t.Fatalf("delete session: %v", err)
	}

	_, err = env.session.Get(env.ctx, s.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSessionManager_ListByAgent(t *testing.T) {
	env := newTestEnv(t)

	env.session.Create(env.ctx, "agent-001", nil)
	env.session.Create(env.ctx, "agent-001", nil)
	env.session.Create(env.ctx, "agent-002", nil)

	sessions, err := env.session.ListByAgent(env.ctx, "agent-001", 0)
	if err != nil {
		t.Fatalf("list by agent: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionManager_UpdateState(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	env.session.UpdateState(env.ctx, s.ID, model.SessionCompleted)

	fetched, _ := env.session.Get(env.ctx, s.ID)
	if fetched.State != model.SessionCompleted {
		t.Fatalf("expected completed, got %s", fetched.State)
	}
}

// ========== Memory Tests ==========

func TestMemoryStore_Store(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	m, err := env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryLongTerm, "用户偏好中文", 0.8))
	if err != nil {
		t.Fatalf("store memory: %v", err)
	}

	if m.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if m.Content != "用户偏好中文" {
		t.Fatalf("expected content, got %s", m.Content)
	}
}

func TestMemoryStore_Get(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	stored, _ := env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryLongTerm, "记忆内容", 0.5))

	fetched, err := env.memory.Get(env.ctx, stored.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if fetched.Content != "记忆内容" {
		t.Fatalf("expected '记忆内容', got %q", fetched.Content)
	}
}

func TestMemoryStore_WithEmbedding(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	m := model.NewMemory(s.ID, model.MemoryLongTerm, "带向量的记忆", 0.9)
	m.Embedding = []float32{0.1, 0.2, 0.3, 0.4, 0.5}

	stored, _ := env.memory.Store(env.ctx, m)

	// 获取带 embedding 的记忆
	fetched, err := env.memory.GetWithEmbedding(env.ctx, stored.ID)
	if err != nil {
		t.Fatalf("get with embedding: %v", err)
	}
	if len(fetched.Embedding) != 5 {
		t.Fatalf("expected 5-dim embedding, got %d", len(fetched.Embedding))
	}
	if fetched.Embedding[0] != 0.1 {
		t.Fatalf("expected 0.1, got %f", fetched.Embedding[0])
	}
}

func TestMemoryStore_ListBySession(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryShortTerm, "短期1", 0.5))
	env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryLongTerm, "长期1", 0.8))
	env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryShortTerm, "短期2", 0.3))

	// 列出所有
	all, _ := env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(all))
	}

	// 按类型过滤
	shortTerm, _ := env.memory.ListBySession(env.ctx, s.ID, model.MemoryShortTerm, 0)
	if len(shortTerm) != 2 {
		t.Fatalf("expected 2 short-term memories, got %d", len(shortTerm))
	}

	longTerm, _ := env.memory.ListBySession(env.ctx, s.ID, model.MemoryLongTerm, 0)
	if len(longTerm) != 1 {
		t.Fatalf("expected 1 long-term memory, got %d", len(longTerm))
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	m, _ := env.memory.Store(env.ctx, model.NewMemory(s.ID, model.MemoryLongTerm, "要删除的记忆", 0.5))

	env.memory.Delete(env.ctx, m.ID)

	_, err := env.memory.Get(env.ctx, m.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestMemoryStore_BatchStore(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	memories := []*model.MemoryEntry{
		model.NewMemory(s.ID, model.MemoryShortTerm, "批量1", 0.3),
		model.NewMemory(s.ID, model.MemoryShortTerm, "批量2", 0.5),
		model.NewMemory(s.ID, model.MemoryLongTerm, "批量3", 0.8),
	}

	err := env.memory.BatchStore(env.ctx, memories)
	if err != nil {
		t.Fatalf("batch store: %v", err)
	}

	all, _ := env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != 3 {
		t.Fatalf("expected 3 memories after batch, got %d", len(all))
	}
}

// ========== Decision Tests ==========

func TestDecisionRecorder_Record(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	d := model.NewDecision(s.ID, model.DecisionToolCall,
		json.RawMessage(`{"tool":"search","query":"test"}`),
		json.RawMessage(`{"results":[]}`),
	)
	d.Reasoning = "搜索测试数据"
	d.ToolsUsed = []string{"search"}
	d.DurationMs = 150

	recorded, err := env.decision.Record(env.ctx, d)
	if err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if recorded.ID == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestDecisionRecorder_DecisionTree(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	// 创建根决策
	root := model.NewDecision(s.ID, model.DecisionPlanning,
		json.RawMessage(`{"task":"analyze"}`),
		json.RawMessage(`{"plan":"step1,step2"}`),
	)
	root.DurationMs = 100
	root, _ = env.decision.Record(env.ctx, root)

	// 创建子决策
	child1 := model.NewDecision(s.ID, model.DecisionToolCall,
		json.RawMessage(`{"tool":"read"}`),
		json.RawMessage(`{"data":"..."}`),
	)
	child1.ParentID = &root.ID
	child1.DurationMs = 50
	child1, _ = env.decision.Record(env.ctx, child1)

	child2 := model.NewDecision(s.ID, model.DecisionReasoning,
		json.RawMessage(`{"context":"..."}`),
		json.RawMessage(`{"result":"ok"}`),
	)
	child2.ParentID = &root.ID
	child2.DurationMs = 30
	child2, _ = env.decision.Record(env.ctx, child2)

	// 构建决策树
	tree, err := env.decision.BuildDecisionTree(env.ctx, root.ID)
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}

	if tree.Decision.ID != root.ID {
		t.Fatalf("root ID mismatch")
	}
	if len(tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tree.Children))
	}

	totalDuration := tree.TotalDuration()
	if totalDuration != 180 {
		t.Fatalf("expected total duration 180, got %d", totalDuration)
	}
}

func TestDecisionRecorder_ListBySession(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	for i := 0; i < 5; i++ {
		d := model.NewDecision(s.ID, model.DecisionToolCall,
			json.RawMessage(`{}`),
			json.RawMessage(`{}`),
		)
		d.DurationMs = uint64(i * 10)
		env.decision.Record(env.ctx, d)
	}

	decisions, err := env.decision.ListBySession(env.ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("list by session: %v", err)
	}
	if len(decisions) != 5 {
		t.Fatalf("expected 5 decisions, got %d", len(decisions))
	}

	// 带 limit
	limited, _ := env.decision.ListBySession(env.ctx, s.ID, 3)
	if len(limited) != 3 {
		t.Fatalf("expected 3 decisions with limit, got %d", len(limited))
	}
}

// ========== Concurrent Tests ==========

func TestConcurrentSessionOperations(t *testing.T) {
	env := newTestEnv(t)

	var wg sync.WaitGroup
	n := 100

	// 并发创建
	sessions := make([]*model.AgentSession, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := env.session.Create(env.ctx, "agent-concurrent", nil)
			if err != nil {
				t.Errorf("create session %d: %v", i, err)
				return
			}
			sessions[i] = s
		}(i)
	}
	wg.Wait()

	// 并发读取
	for i := 0; i < n; i++ {
		if sessions[i] == nil {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			env.session.Get(env.ctx, id)
		}(sessions[i].ID)
	}
	wg.Wait()
}

func TestConcurrentMemoryOperations(t *testing.T) {
	env := newTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	var wg sync.WaitGroup
	n := 200

	// 并发存储
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := model.NewMemory(s.ID, model.MemoryShortTerm, "并发记忆", 0.5)
			env.memory.Store(env.ctx, m)
		}(i)
	}
	wg.Wait()

	// 验证总数
	all, _ := env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != n {
		t.Fatalf("expected %d memories, got %d", n, len(all))
	}
}
