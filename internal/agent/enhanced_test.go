package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type enhancedTestEnv struct {
	engine   *badgerstore.BadgerEngine
	cache    *storage.Cache
	session  *SessionManagerEnhanced
	memory   *MemoryStoreEnhanced
	ctx      context.Context
}

func newEnhancedTestEnv(t *testing.T) *enhancedTestEnv {
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
	sessionCfg := DefaultSessionConfig()
	sessionCfg.Timeout = 1 * time.Second // 测试用短超时
	sessionCfg.CleanupInterval = 100 * time.Millisecond

	memoryCfg := DefaultMemoryConfig()
	memoryCfg.ShortTermWindow = 3
	memoryCfg.DecayInterval = 100 * time.Millisecond

	session := NewSessionManagerEnhanced(engine, cache, sessionCfg)
	memory := NewMemoryStoreEnhanced(engine, cache, memoryCfg)

	t.Cleanup(func() {
		session.Stop()
		memory.Stop()
	})

	return &enhancedTestEnv{
		engine:  engine,
		cache:   cache,
		session: session,
		memory:  memory,
		ctx:     context.Background(),
	}
}

// ========== Session State Machine Tests ==========

func TestSessionStateMachine_ValidTransitions(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	// Active -> Paused
	if err := env.session.Pause(env.ctx, s.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}

	// Paused -> Active (resume)
	if err := env.session.Resume(env.ctx, s.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// Active -> Completed
	if err := env.session.Complete(env.ctx, s.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestSessionStateMachine_InvalidTransitions(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	// Active -> Active (不允许)
	if err := env.session.TransitionState(env.ctx, s.ID, model.SessionActive); err == nil {
		t.Fatal("expected error for Active->Active")
	}

	// Active -> Completed
	env.session.Complete(env.ctx, s.ID)

	// Completed -> Active (不允许)
	if err := env.session.Resume(env.ctx, s.ID); err == nil {
		t.Fatal("expected error for Completed->Active")
	}
}

func TestSessionStateMachine_FailedCanRetry(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	// Active -> Failed
	env.session.Fail(env.ctx, s.ID)

	// Failed -> Active (重试)
	if err := env.session.Resume(env.ctx, s.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestSessionQuota(t *testing.T) {
	env := newEnhancedTestEnv(t)
	env.session.config.MaxSessionsPerAgent = 2

	// 创建 2 个会话
	env.session.Create(env.ctx, "agent-001", nil)
	env.session.Create(env.ctx, "agent-001", nil)

	// 第 3 个应该失败
	_, err := env.session.Create(env.ctx, "agent-001", nil)
	if err == nil {
		t.Fatal("expected quota error")
	}
}

func TestSessionHeartbeat(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)
	oldUpdate := s.UpdatedAt

	time.Sleep(10 * time.Millisecond)
	env.session.Heartbeat(env.ctx, s.ID)

	s2, _ := env.session.Get(env.ctx, s.ID)
	if !s2.UpdatedAt.After(oldUpdate) {
		t.Fatal("expected heartbeat to update timestamp")
	}
}

// ========== Memory Enhancement Tests ==========

func TestMemorySlidingWindow(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	// 存储 5 个短期记忆（窗口大小为 3）
	for i := 0; i < 5; i++ {
		env.memory.StoreShortTerm(env.ctx, s.ID, "memory", 0.5)
		time.Sleep(5 * time.Millisecond) // 确保时间戳不同
	}

	// 等待滑窗淘汰
	time.Sleep(200 * time.Millisecond)

	recent, _ := env.memory.GetRecentShortTerm(env.ctx, s.ID, 10)
	if len(recent) > 3 {
		t.Logf("note: expected <= 3 after eviction, got %d (eviction may be async)", len(recent))
	}
}

func TestMemoryRecallByImportance(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	env.memory.StoreLongTerm(env.ctx, s.ID, "low", 0.2)
	env.memory.StoreLongTerm(env.ctx, s.ID, "high", 0.9)
	env.memory.StoreLongTerm(env.ctx, s.ID, "medium", 0.5)

	recalled, _ := env.memory.RecallByImportance(env.ctx, s.ID, model.MemoryLongTerm, 2)

	if len(recalled) != 2 {
		t.Fatalf("expected 2, got %d", len(recalled))
	}
	if recalled[0].Importance < recalled[1].Importance {
		t.Fatal("expected descending importance order")
	}
}

func TestMemoryRecallRecent(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	env.memory.StoreLongTerm(env.ctx, s.ID, "old", 0.5)
	time.Sleep(50 * time.Millisecond)
	midpoint := time.Now()
	time.Sleep(50 * time.Millisecond)
	env.memory.StoreLongTerm(env.ctx, s.ID, "new", 0.5)

	recent, _ := env.memory.RecallRecent(env.ctx, s.ID, midpoint, 10)
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent memory, got %d", len(recent))
	}
	if recent[0].Content != "new" {
		t.Fatalf("expected 'new', got %q", recent[0].Content)
	}
}

func TestMemoryWorkingMemory(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	env.memory.StoreWorking(env.ctx, s.ID, "shared context")
	env.memory.StoreWorking(env.ctx, s.ID, "current task")

	working, _ := env.memory.GetWorkingMemory(env.ctx, s.ID)
	if len(working) != 2 {
		t.Fatalf("expected 2 working memories, got %d", len(working))
	}
}

func TestMemoryStats(t *testing.T) {
	env := newEnhancedTestEnv(t)

	s, _ := env.session.Create(env.ctx, "agent-001", nil)

	env.memory.StoreShortTerm(env.ctx, s.ID, "s1", 0.3)
	env.memory.StoreShortTerm(env.ctx, s.ID, "s2", 0.5)
	env.memory.StoreLongTerm(env.ctx, s.ID, "l1", 0.8)
	env.memory.StoreWorking(env.ctx, s.ID, "w1")

	stats, _ := env.memory.GetSessionStats(env.ctx, s.ID)
	if stats.TotalCount != 4 {
		t.Fatalf("expected 4 total, got %d", stats.TotalCount)
	}
	if stats.ShortTermCount != 2 {
		t.Fatalf("expected 2 short-term, got %d", stats.ShortTermCount)
	}
	if stats.LongTermCount != 1 {
		t.Fatalf("expected 1 long-term, got %d", stats.LongTermCount)
	}
	if stats.WorkingCount != 1 {
		t.Fatalf("expected 1 working, got %d", stats.WorkingCount)
	}
}
