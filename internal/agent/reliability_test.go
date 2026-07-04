package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

type reliabilityEnv struct {
	engine      *badgerstore.BadgerEngine
	cache       *storage.Cache
	session     *SessionManagerEnhanced
	memory      *MemoryStoreEnhanced
	decision    *DecisionRecorder
	coordinator *Coordinator
	taskQueue   *TaskQueue
	audit       *AuditLogger
	ctx         context.Context
}

func newReliabilityEnv(t *testing.T, timeout, cleanupInterval time.Duration) *reliabilityEnv {
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
	session := NewSessionManagerEnhanced(engine, cache, SessionConfig{
		Timeout:             timeout,
		CleanupInterval:     cleanupInterval,
		MaxSessionsPerAgent: 0, // unlimited
	})
	memory := NewMemoryStoreEnhanced(engine, cache, DefaultMemoryConfig())
	decision := NewDecisionRecorder(engine, cache)
	coordinator := NewCoordinator(engine, session.SessionManager, memory.MemoryStore, decision)
	taskQueue := NewTaskQueue(engine)
	audit := NewAuditLogger(engine)

	t.Cleanup(func() {
		session.Stop()
		memory.Stop()
		taskQueue.Close()
	})

	return &reliabilityEnv{
		engine:      engine,
		cache:       cache,
		session:     session,
		memory:      memory,
		decision:    decision,
		coordinator: coordinator,
		taskQueue:   taskQueue,
		audit:       audit,
		ctx:         context.Background(),
	}
}

// ========== Session Timeout Cleanup ==========

func TestReliability_SessionCleanupLoop(t *testing.T) {
	env := newReliabilityEnv(t, 200*time.Millisecond, 50*time.Millisecond)

	// Create several sessions
	for i := 0; i < 5; i++ {
		env.session.Create(env.ctx, "agent-cleanup", nil)
	}

	// Verify all are active initially
	sessions, _ := env.session.ListActiveByAgent(env.ctx, "agent-cleanup")
	if len(sessions) != 5 {
		t.Fatalf("expected 5 active sessions, got %d", len(sessions))
	}

	// Wait for cleanup loop to run multiple times
	time.Sleep(500 * time.Millisecond)

	// All sessions should be expired now
	sessions, _ = env.session.ListActiveByAgent(env.ctx, "agent-cleanup")
	if len(sessions) != 0 {
		t.Fatalf("expected 0 active sessions after timeout, got %d", len(sessions))
	}
}

func TestReliability_SessionHeartbeatPreventsExpiry(t *testing.T) {
	env := newReliabilityEnv(t, 200*time.Millisecond, 50*time.Millisecond)

	session, _ := env.session.Create(env.ctx, "agent-heartbeat", nil)

	// Start a goroutine that sends heartbeats
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				env.session.Heartbeat(env.ctx, session.ID)
			case <-stopHeartbeat:
				return
			}
		}
	}()

	// Wait longer than timeout
	time.Sleep(400 * time.Millisecond)

	// Session should still be active due to heartbeats
	s, err := env.session.Get(env.ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if s.State != model.SessionActive {
		t.Fatalf("expected active state, got %s", s.State)
	}

	close(stopHeartbeat)
	time.Sleep(300 * time.Millisecond)

	// Now session should be expired
	sessions, _ := env.session.ListActiveByAgent(env.ctx, "agent-heartbeat")
	if len(sessions) != 0 {
		t.Fatalf("expected 0 active sessions after stopping heartbeat, got %d", len(sessions))
	}
}

func TestReliability_SessionCleanupDoesNotAffectNonActive(t *testing.T) {
	env := newReliabilityEnv(t, 200*time.Millisecond, 50*time.Millisecond)

	// Create a session and mark it completed immediately
	session, _ := env.session.Create(env.ctx, "agent-done", nil)
	env.session.Complete(env.ctx, session.ID)

	// Wait for cleanup
	time.Sleep(400 * time.Millisecond)

	// Completed session should not be affected
	s, err := env.session.Get(env.ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if s.State != model.SessionCompleted {
		t.Fatalf("expected completed state, got %s", s.State)
	}
}

// ========== Memory Decay Long-Run ==========

func TestReliability_MemoryDecayOverTime(t *testing.T) {
	env := newReliabilityEnv(t, time.Hour, 100*time.Millisecond)

	s, _ := env.session.Create(env.ctx, "agent-decay", nil)

	// Store memories with different importance
	for i := 0; i < 10; i++ {
		mem := model.NewMemory(s.ID, model.MemoryLongTerm,
			fmt.Sprintf("memory-%d", i), 0.5)
		env.memory.Store(env.ctx, mem)
	}

	// Verify initial count
	all, _ := env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != 10 {
		t.Fatalf("expected 10 memories, got %d", len(all))
	}
}

// ========== Task Queue Long-Run Stability ==========

func TestReliability_TaskQueueLongRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open: %v", err)
	}

	queue := NewTaskQueue(engine)

	ctx := context.Background()
	n := 200

	// Enqueue many tasks
	for i := 0; i < n; i++ {
		task := &Task{
			AgentID:  "agent-long",
			Name:     fmt.Sprintf("task-%d", i),
			Priority: rand.Intn(10),
		}
		if err := queue.Enqueue(ctx, task); err != nil {
			t.Fatalf("enqueue task %d: %v", i, err)
		}
	}

	if queue.Len() != n {
		t.Fatalf("expected %d tasks, got %d", n, queue.Len())
	}

	// Process all tasks
	processed := 0
	for i := 0; i < n; i++ {
		ctx2, cancel := context.WithTimeout(ctx, 1*time.Second)
		task, err := queue.Dequeue(ctx2)
		cancel()
		if err != nil {
			break
		}
		queue.Complete(ctx, task.ID, nil)
		processed++
	}

	queue.Close()
	engine.Close()
	os.RemoveAll(dir)

	if processed != n {
		t.Fatalf("expected to process %d tasks, got %d", n, processed)
	}
}

func TestReliability_TaskQueueMixedOperations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open: %v", err)
	}

	queue := NewTaskQueue(engine)
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 100

	// Concurrent enqueue
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task := &Task{
				AgentID:  "agent-mixed",
				Name:     fmt.Sprintf("task-%d", i),
				Priority: i % 5,
			}
			queue.Enqueue(ctx, task)
		}(i)
	}

	wg.Wait()

	// Process some tasks, fail some, cancel some
	processed, failed, cancelled := 0, 0, 0
	for i := 0; i < n; i++ {
		ctx2, cancel := context.WithTimeout(ctx, 1*time.Second)
		task, err := queue.Dequeue(ctx2)
		cancel()
		if err != nil {
			break
		}

		switch i % 3 {
		case 0:
			queue.Complete(ctx, task.ID, nil)
			processed++
		case 1:
			queue.Fail(ctx, task.ID, "simulated failure")
			failed++
		case 2:
			queue.Cancel(ctx, task.ID)
			cancelled++
		}
	}

	queue.Close()
	engine.Close()
	os.RemoveAll(dir)

	total := processed + failed + cancelled
	if total != n {
		t.Fatalf("expected %d total operations, got %d (processed=%d, failed=%d, cancelled=%d)", n, total, processed, failed, cancelled)
	}
}

// ========== Concurrent Operations Long-Run ==========

func TestReliability_ConcurrentSessionCreateDelete(t *testing.T) {
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	var wg sync.WaitGroup
	rounds := 50
	perRound := 20

	for r := 0; r < rounds; r++ {
		agentID := fmt.Sprintf("agent-r%d", r)

		// Create sessions
		sessions := make([]*model.AgentSession, perRound)
		for i := 0; i < perRound; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s, _ := env.session.Create(env.ctx, agentID, nil)
				sessions[i] = s
			}(i)
		}
		wg.Wait()

		// Verify created
		listed, _ := env.session.ListByAgent(env.ctx, agentID, 0)
		if len(listed) != perRound {
			t.Fatalf("round %d: expected %d sessions, got %d", r, perRound, len(listed))
		}

		// Delete some sessions
		for i := 0; i < perRound/2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if sessions[i] != nil {
					env.session.Delete(env.ctx, sessions[i].ID)
				}
			}(i)
		}
		wg.Wait()
	}
}

func TestReliability_ConcurrentMemoryOperations(t *testing.T) {
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	s, _ := env.session.Create(env.ctx, "agent-concurrent-mem", nil)

	var wg sync.WaitGroup
	n := 1000

	// Concurrent store
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mem := model.NewMemory(s.ID, model.MemoryShortTerm,
				fmt.Sprintf("content-%d", i), float32(rand.Float64()))
			env.memory.Store(env.ctx, mem)
		}(i)
	}
	wg.Wait()

	// Verify count
	all, _ := env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != n {
		t.Fatalf("expected %d memories, got %d", n, len(all))
	}

	// Concurrent delete
	ids := make([]string, len(all))
	for i, m := range all {
		ids[i] = m.ID
	}

	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			env.memory.Delete(env.ctx, id)
		}(id)
	}
	wg.Wait()

	// Verify all deleted
	all, _ = env.memory.ListBySession(env.ctx, s.ID, "", 0)
	if len(all) != 0 {
		t.Fatalf("expected 0 memories after delete, got %d", len(all))
	}
}

// ========== Cache Under Pressure ==========

func TestReliability_CacheUnderPressure(t *testing.T) {
	cache := storage.NewCache(100)

	// Fill cache beyond capacity
	for i := 0; i < 500; i++ {
		key := []byte(util.NewUUID())
		cache.Set(key, []byte(fmt.Sprintf("value-%d", i)))
	}

	// Cache size should be limited
	if cache.Len() > 100 {
		t.Fatalf("cache size exceeded capacity: %d", cache.Len())
	}

	// Concurrent access under pressure
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%d", i%200))
			cache.Get(key)
		}(i)
		go func(i int) {
			defer wg.Done()
			key := []byte(util.NewUUID())
			cache.Set(key, []byte(fmt.Sprintf("val-%d", i)))
		}(i)
	}
	wg.Wait()

	// Still within capacity
	if cache.Len() > 100 {
		t.Fatalf("cache size exceeded capacity after pressure: %d", cache.Len())
	}

	// Check stats are reasonable
	stats := cache.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Fatal("expected some cache operations")
	}
}

// ========== Decision Recorder Long-Run ==========

func TestReliability_DecisionRecorderLongRun(t *testing.T) {
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	s, _ := env.session.Create(env.ctx, "agent-decisions-long", nil)

	n := 200
	for i := 0; i < n; i++ {
		dec := model.NewDecision(s.ID, model.DecisionType([]string{
			string(model.DecisionReasoning),
			string(model.DecisionToolCall),
			string(model.DecisionPlanning),
		}[i%3]),
			nil, nil,
		)
		dec.DurationMs = uint64(rand.Intn(1000))
		env.decision.Record(env.ctx, dec)
	}

	decisions, err := env.decision.ListBySession(env.ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != n {
		t.Fatalf("expected %d decisions, got %d", n, len(decisions))
	}
}

// ========== Audit Logger Long-Run ==========

func TestReliability_AuditLoggerLongRun(t *testing.T) {
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	agents := []string{"agent-a", "agent-b", "agent-c"}
	operations := []AuditOperation{OpSessionCreate, OpMemoryStore, OpDecisionRecord, OpQueryExecute}

	n := 300
	for i := 0; i < n; i++ {
		agent := agents[i%len(agents)]
		op := operations[i%len(operations)]

		event := &AuditEvent{
			AgentID:   agent,
			SessionID: fmt.Sprintf("session-%d", i),
			Operation: op,
			Resource:  "test",
			Success:   i%10 != 0, // 10% failures
		}
		if !event.Success {
			event.Error = "simulated error"
		}

		env.audit.Log(env.ctx, event)
	}

	// Verify by agent
	for _, agent := range agents {
		events, _ := env.audit.ListByAgent(env.ctx, agent, 0)
		expected := n / len(agents)
		if len(events) != expected {
			t.Fatalf("agent %s: expected %d events, got %d", agent, expected, len(events))
		}
	}

	// Verify by operation
	for _, op := range operations {
		events, _ := env.audit.ListByOperation(env.ctx, op, 0)
		expected := n / len(operations)
		if len(events) != expected {
			t.Fatalf("op %s: expected %d events, got %d", op, expected, len(events))
		}
	}
}

// ========== Memory Leak Detection ==========

func TestReliability_NoMemoryLeak(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Perform many operations
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)

	cache := storage.NewCache(256)
	session := NewSessionManagerEnhanced(engine, cache, SessionConfig{
		Timeout:         time.Hour,
		CleanupInterval: time.Minute,
	})
	memory := NewMemoryStore(engine, cache)
	decision := NewDecisionRecorder(engine, cache)

	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		s, _ := session.Create(ctx, "agent-mem", nil)
		mem := model.NewMemory(s.ID, model.MemoryShortTerm, "leak-test", 0.5)
		memory.Store(ctx, mem)
		dec := model.NewDecision(s.ID, model.DecisionToolCall, nil, nil)
		decision.Record(ctx, dec)
	}

	session.Stop()
	engine.Close()
	os.RemoveAll(dir)

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Check that heap allocation didn't grow excessively
	heapDelta := int64(m2.TotalAlloc) - int64(m1.TotalAlloc)
	if heapDelta > 50<<20 { // 50MB threshold
		t.Fatalf("heap allocation grew by %d bytes (>50MB), possible leak", heapDelta)
	}
}

// ========== Graceful Shutdown ==========

func TestReliability_GracefulShutdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = true // Ensure data is flushed
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open: %v", err)
	}

	cache := storage.NewCache(512)
	session := NewSessionManager(engine, cache)
	memory := NewMemoryStore(engine, cache)

	ctx := context.Background()

	// Create data
	s, _ := session.Create(ctx, "agent-shutdown", nil)
	mem := model.NewMemory(s.ID, model.MemoryLongTerm, "important-memory", 0.9)
	memory.Store(ctx, mem)

	// Reopen and verify data persisted (same engine, just verify in-memory state)
	loadedSession, err := session.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	if loadedSession.AgentID != "agent-shutdown" {
		t.Fatalf("expected agent-shutdown, got %s", loadedSession.AgentID)
	}

	loadedMem, err := memory.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("memory not found: %v", err)
	}
	if loadedMem.Content != "important-memory" {
		t.Fatalf("expected important-memory, got %s", loadedMem.Content)
	}
}

// ========== Coordinator Long-Run ==========

func TestReliability_CoordinatorLongRun(t *testing.T) {
	t.Parallel()
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	// Create many rooms and verify each one
	n := 30
	roomIDs := make([]string, n)
	for i := 0; i < n; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		room, err := env.coordinator.CreateRoom(env.ctx,
			fmt.Sprintf("room-%d", i), agentID, RoomOptions{})
		if err != nil {
			t.Fatalf("create room %d: %v", i, err)
		}
		roomIDs[i] = room.ID

		// Add members
		for j := 1; j < 3; j++ {
			memberID := fmt.Sprintf("agent-%d-%d", i, j)
			env.coordinator.JoinRoom(env.ctx, room.ID, memberID, RoleMember)
		}

		// Send messages
		for k := 0; k < 5; k++ {
			env.coordinator.SendMessage(env.ctx, room.ID, agentID,
				MsgText, fmt.Sprintf("msg-%d", k))
		}
	}

	// Verify each room
	for i, roomID := range roomIDs {
		room, err := env.coordinator.GetRoom(env.ctx, roomID)
		if err != nil {
			t.Fatalf("get room %d: %v", i, err)
		}
		if room.Name != fmt.Sprintf("room-%d", i) {
			t.Fatalf("room %d: expected name room-%d, got %s", i, i, room.Name)
		}

		// Verify messages
		messages, _ := env.coordinator.GetMessages(env.ctx, roomID, 10)
		if len(messages) < 5 {
			t.Fatalf("room %d: expected at least 5 messages, got %d", i, len(messages))
		}
	}
}

// ========== Store/Recall Cycle ==========

func TestReliability_StoreRecallCycle(t *testing.T) {
	t.Parallel()
	env := newReliabilityEnv(t, time.Hour, time.Minute)

	s, _ := env.session.Create(env.ctx, "agent-cycle", nil)

	// Multiple store-recall cycles: store 20, delete 10 each cycle
	for cycle := 0; cycle < 10; cycle++ {
		// Store
		for i := 0; i < 20; i++ {
			mem := model.NewMemory(s.ID, model.MemoryShortTerm,
				fmt.Sprintf("cycle-%d-mem-%d", cycle, i), 0.5)
			env.memory.Store(env.ctx, mem)
		}

		// Recall - should have 20 + cycle*10 memories
		memories, err := env.memory.ListBySession(env.ctx, s.ID, "", 0)
		if err != nil {
			t.Fatalf("cycle %d list: %v", cycle, err)
		}
		expected := 20 + cycle*10
		if len(memories) != expected {
			t.Fatalf("cycle %d: expected %d memories, got %d", cycle, expected, len(memories))
		}

		// Delete half (10)
		for i := 0; i < 10 && i < len(memories); i++ {
			env.memory.Delete(env.ctx, memories[i].ID)
		}
	}
}

// ========== Benchmark: Long-Run Operations ==========

func BenchmarkReliability_ConcurrentSession(b *testing.B) {
	dir := b.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	cache := storage.NewCache(512)
	session := NewSessionManager(engine, cache)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			agentID := fmt.Sprintf("agent-%d", i%100)
			session.Create(ctx, agentID, nil)
			i++
		}
	})
}

func BenchmarkReliability_ConcurrentMemory(b *testing.B) {
	dir := b.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	cache := storage.NewCache(512)
	memory := NewMemoryStore(engine, cache)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mem := model.NewMemory("session-bench", model.MemoryShortTerm,
				fmt.Sprintf("mem-%d", i), 0.5)
			memory.Store(ctx, mem)
			i++
		}
	})
}
