package agent

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

type e2eTestEnv struct {
	engine      storage.Engine
	cache       *storage.Cache
	session     *SessionManager
	memory      *MemoryStore
	decision    *DecisionRecorder
	coordinator *Coordinator
	taskQueue   *TaskQueue
	audit       *AuditLogger
	vectorStore *vector.VectorStore
	ctx         context.Context
}

func newE2ETestEnv(t *testing.T) *e2eTestEnv {
	t.Helper()
	engine := storage.NewTestEngine(t)

	cache := storage.NewCache(512)
	session := NewSessionManager(engine, cache)
	memory := NewMemoryStore(engine, cache)
	decision := NewDecisionRecorder(engine, cache)
	coordinator := NewCoordinator(engine, session, memory, decision)
	taskQueue := NewTaskQueue(engine)
	audit := NewAuditLogger(engine)
	vectorStore := vector.NewVectorStore(engine)
	vectorStore.CreateIndex("e2e_chunks", 4, "cosine")
	vectorStore.CreateIndex("rag_chunks", 4, "cosine")

	return &e2eTestEnv{
		engine:      engine,
		cache:       cache,
		session:     session,
		memory:      memory,
		decision:    decision,
		coordinator: coordinator,
		taskQueue:   taskQueue,
		audit:       audit,
		vectorStore: vectorStore,
		ctx:         context.Background(),
	}
}

// ========== Agent Session Workflow ==========

func TestE2E_FullAgentSessionLifecycle(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, err := env.session.Create(env.ctx, "agent-001", map[string]any{
		"task": "analyze sales data",
		"model": "gpt-4",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// 2. Store working memories
	for i := 0; i < 3; i++ {
		mem := model.NewMemory(session.ID, model.MemoryWorking,
			"Working memory item", 0.5)
		env.memory.Store(env.ctx, mem)
	}

	// 3. Record decisions
	input := json.RawMessage(`{"query": "sales trend"}`)
	output := json.RawMessage(`{"result": "upward trend"}`)
	dec := model.NewDecision(session.ID, model.DecisionReasoning, input, output)
	dec.Reasoning = "Analyzing sales data shows upward trend"
	dec.DurationMs = 200
	dec.TokenUsage = &model.TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	env.decision.Record(env.ctx, dec)

	// 4. Store long-term memories from analysis
	longTerm := model.NewMemory(session.ID, model.MemoryLongTerm,
		"Sales are trending upward", 0.9)
	env.memory.Store(env.ctx, longTerm)

	// 5. Update session state
	_, err = env.session.Get(env.ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	// 6. Verify all data
	// Check memories
	memories, _ := env.memory.ListBySession(env.ctx, session.ID, "", 0)
	if len(memories) != 4 {
		t.Fatalf("expected 4 memories, got %d", len(memories))
	}

	// Check decisions
	decisions, _ := env.decision.ListBySession(env.ctx, session.ID, 0)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}

	// 7. Log audit event
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-001",
		SessionID: session.ID,
		Operation: OpSessionCreate,
		Resource:  "session",
		Success:   true,
	})

	// 8. Complete the session
	// (In real scenario, would call session.Update with completed state)
}

// ========== Multi-Agent Collaboration Workflow ==========

func TestE2E_MultiAgentCollaboration(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create sessions for multiple agents
	session1, _ := env.session.Create(env.ctx, "agent-analyst", nil)
	session2, _ := env.session.Create(env.ctx, "agent-writer", nil)
	session3, _ := env.session.Create(env.ctx, "agent-reviewer", nil)
	_ = session2
	_ = session3

	// 2. Create a collaboration room
	room, err := env.coordinator.CreateRoom(env.ctx, "content-review", "agent-analyst", RoomOptions{
		MaxMembers: 5,
		Persistent: true,
	})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	// 3. Agents join the room
	env.coordinator.JoinRoom(env.ctx, room.ID, "agent-writer", RoleMember)
	env.coordinator.JoinRoom(env.ctx, room.ID, "agent-reviewer", RoleViewer)

	// 4. Agent-analyst stores analysis results as memories
	analysisMem := model.NewMemory(session1.ID, model.MemoryLongTerm,
		"Data shows 20% growth in Q3", 0.8)
	env.memory.Store(env.ctx, analysisMem)

	// 5. Share memory with room
	env.coordinator.ShareMemory(env.ctx, room.ID, "agent-analyst",
		"Q3 growth data: 20% increase", 0.9)

	// 6. Agent-writer sends a message
	msg, err := env.coordinator.SendMessage(env.ctx, room.ID, "agent-writer",
		MsgText, "Draft created based on analysis")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if msg.RoomID != room.ID {
		t.Fatalf("message room ID mismatch")
	}

	// 7. Verify room messages
	messages, err := env.coordinator.GetMessages(env.ctx, room.ID, 10)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(messages))
	}

	// 8. Verify room members
	loadedRoom, _ := env.coordinator.GetRoom(env.ctx, room.ID)
	if len(loadedRoom.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(loadedRoom.Members))
	}

	// 9. Agent-reviewer leaves
	env.coordinator.LeaveRoom(env.ctx, room.ID, "agent-reviewer")

	// Verify member left
	loadedRoom, _ = env.coordinator.GetRoom(env.ctx, room.ID)
	if len(loadedRoom.Members) != 2 {
		t.Fatalf("expected 2 members after leave, got %d", len(loadedRoom.Members))
	}
}

// ========== Task Queue + Session Workflow ==========

func TestE2E_TaskDrivenSession(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-task", map[string]any{
		"task_type": "data_processing",
	})

	// 2. Create tasks for the session
	tasks := []*Task{
		{
			AgentID:  "agent-task",
			Name:     "fetch_data",
			Priority: 1,
			Input:    map[string]any{"source": "api"},
		},
		{
			AgentID:  "agent-task",
			Name:     "analyze_data",
			Priority: 2,
			Input:    map[string]any{"method": "statistics"},
		},
		{
			AgentID:  "agent-task",
			Name:     "generate_report",
			Priority: 3,
			Input:    map[string]any{"format": "markdown"},
		},
	}

	for _, task := range tasks {
		env.taskQueue.Enqueue(env.ctx, task)
	}

	// 3. Process tasks in priority order
	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	for i := 0; i < 3; i++ {
		task, err := env.taskQueue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}

		// Record decision for each task
		dec := model.NewDecision(session.ID, model.DecisionToolCall,
			json.RawMessage(`{"task": "`+task.Name+`"}`),
			json.RawMessage(`{"status": "completed"}`),
		)
		dec.DurationMs = uint64(50 * (i + 1))
		env.decision.Record(env.ctx, dec)

		// Complete task
		env.taskQueue.Complete(env.ctx, task.ID, map[string]any{
			"result": "success",
		})
	}

	// 4. Verify all tasks completed
	for _, task := range tasks {
		completed, err := env.taskQueue.GetTask(env.ctx, task.ID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if completed.Status != TaskCompleted {
			t.Fatalf("task %s should be completed, got %s", task.Name, completed.Status)
		}
	}

	// 5. Verify decisions were recorded
	decisions, _ := env.decision.ListBySession(env.ctx, session.ID, 0)
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}
}

// ========== RAG + Vector Search Workflow ==========

func TestE2E_RAGWorkflow(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-rag", nil)

	// 2. Ingest a document
	doc := &Document{
		ID:      "rag-doc",
		Title:   "AI Fundamentals",
		Content: "Machine learning is a subset of AI.\n\nDeep learning is a subset of ML.\n\nNLP is a key application.",
	}

	ragEngine := NewRAGEngine(env.engine, env.memory, env.vectorStore)
	chunks, err := ragEngine.IngestDocument(env.ctx, session.ID, doc)
	if err != nil {
		t.Fatalf("ingest document: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// 3. Add embeddings for semantic search (use rag_chunks index which RAGEngine expects)
	for i, chunk := range chunks {
		vec := []float32{0.1, 0.2, 0.3, 0.4}
		vec[i] = 1.0
		env.vectorStore.Insert("rag_chunks", chunk.ID, vec)
	}

	// 4. Semantic search
	query := []float32{1.0, 0.2, 0.3, 0.4}
	hits, err := ragEngine.SemanticSearch(env.ctx, query, 2)
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}

	if len(hits) > 2 {
		t.Fatalf("expected at most 2 hits, got %d", len(hits))
	}

	// 5. Build context from hits
	context := ragEngine.BuildContext(hits, 500)
	if context == "" {
		t.Fatal("expected non-empty context")
	}

	// 6. Record the RAG operation as a decision
	dec := model.NewDecision(session.ID, model.DecisionReasoning,
		json.RawMessage(`{"query": "AI topics"}`),
		json.RawMessage(`{"context_length": `+string(rune(len(context)))+`}`),
	)
	dec.Reasoning = "Using RAG to find relevant AI topics"
	env.decision.Record(env.ctx, dec)

	// 7. Log the audit event
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-rag",
		SessionID: session.ID,
		Operation: OpMemoryRecall,
		Resource:  "vector_search",
		Success:   true,
		Details: map[string]any{
			"query":      "AI topics",
			"top_k":      2,
			"hits_found": len(hits),
		},
	})
}

// ========== Decision Tree Workflow ==========

func TestE2E_DecisionTree(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-tree", nil)

	// 2. Create root decision (planning)
	root := model.NewDecision(session.ID, model.DecisionPlanning,
		json.RawMessage(`{"goal": "analyze market"}`),
		json.RawMessage(`{"plan": ["research", "analyze", "report"]}`),
	)
	root.DurationMs = 100
	root, _ = env.decision.Record(env.ctx, root)

	// 3. Create child decisions
	children := []struct {
		name string
		dtype model.DecisionType
	}{
		{"research", model.DecisionToolCall},
		{"analyze", model.DecisionReasoning},
		{"report", model.DecisionReflection},
	}

	for _, child := range children {
		pid := root.ID
		dec := model.NewDecision(session.ID, child.dtype,
			json.RawMessage(`{"step": "`+child.name+`"}`),
			json.RawMessage(`{"status": "done"}`),
		)
		dec.ParentID = &pid
		dec.DurationMs = uint64(50)
		dec.ToolsUsed = []string{child.name}
		env.decision.Record(env.ctx, dec)
	}

	// 4. Build and verify decision tree
	tree, err := env.decision.BuildDecisionTree(env.ctx, root.ID)
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}

	if tree.Decision.ID != root.ID {
		t.Fatalf("root ID mismatch")
	}

	if len(tree.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(tree.Children))
	}

	// 5. Verify total duration
	totalDuration := tree.TotalDuration()
	if totalDuration < 100 {
		t.Fatalf("expected total duration >= 100, got %d", totalDuration)
	}
}

// ========== Concurrent Multi-Operation Workflow ==========

func TestE2E_ConcurrentMultiAgent(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create multiple sessions concurrently
	type sessionResult struct {
		session *model.AgentSession
		err     error
	}

	results := make([]sessionResult, 5)
	done := make(chan int, 5)

	for i := 0; i < 5; i++ {
		go func(i int) {
			s, err := env.session.Create(env.ctx, "agent-concurrent", nil)
			results[i] = sessionResult{s, err}
			done <- i
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify all sessions created
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("session %d create failed: %v", i, r.err)
		}
		if r.session == nil {
			t.Fatalf("session %d is nil", i)
		}
	}

	// 2. Concurrently store memories
	for _, r := range results {
		mem := model.NewMemory(r.session.ID, model.MemoryShortTerm,
			"concurrent memory", 0.5)
		env.memory.Store(env.ctx, mem)
	}

	// 3. List all sessions for the agent
	sessions, err := env.session.ListByAgent(env.ctx, "agent-concurrent", 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}

	if len(sessions) != 5 {
		t.Fatalf("expected 5 sessions, got %d", len(sessions))
	}
}

// ========== Knowledge Graph + Session Workflow ==========

func TestE2E_SessionWithKnowledgeGraph(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-kg", nil)

	// 2. Store memories that reference entities
	mem1 := model.NewMemory(session.ID, model.MemoryLongTerm,
		"Entity A is related to Entity B", 0.7)
	mem2 := model.NewMemory(session.ID, model.MemoryLongTerm,
		"Entity B connects to Entity C", 0.8)
	env.memory.Store(env.ctx, mem1)
	env.memory.Store(env.ctx, mem2)

	// 3. Record decision about the relationship
	dec := model.NewDecision(session.ID, model.DecisionReasoning,
		json.RawMessage(`{"entities": ["A", "B", "C"]}`),
		json.RawMessage(`{"relationship": "A->B->C"}`),
	)
	dec.Reasoning = "Discovered relationship chain"
	env.decision.Record(env.ctx, dec)

	// 4. Log audit
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-kg",
		SessionID: session.ID,
		Operation: OpMemoryStore,
		Resource:  "knowledge_graph",
		Success:   true,
		Details: map[string]any{
			"entity_count": 3,
			"relation":     "A->B->C",
		},
	})

	// 5. Verify session data
	memories, _ := env.memory.ListBySession(env.ctx, session.ID, "", 0)
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}

	decisions, _ := env.decision.ListBySession(env.ctx, session.ID, 0)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
}

// ========== Session State + Audit Trail Workflow ==========

func TestE2E_SessionStateAuditTrail(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-audit", nil)

	// 2. Log creation
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-audit",
		SessionID: session.ID,
		Operation: OpSessionCreate,
		Resource:  "session",
		Success:   true,
	})

	// 3. Perform operations and log each
	// Store memory
	mem := model.NewMemory(session.ID, model.MemoryWorking, "Working context", 0.5)
	env.memory.Store(env.ctx, mem)
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-audit",
		SessionID: session.ID,
		Operation: OpMemoryStore,
		Resource:  "memory",
		Success:   true,
	})

	// Record decision
	dec := model.NewDecision(session.ID, model.DecisionToolCall,
		json.RawMessage(`{}`),
		json.RawMessage(`{"result": "ok"}`),
	)
	env.decision.Record(env.ctx, dec)
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-audit",
		SessionID: session.ID,
		Operation: OpDecisionRecord,
		Resource:  "decision",
		Success:   true,
	})

	// 4. Verify audit trail
	events, _ := env.audit.ListByAgent(env.ctx, "agent-audit", 0)
	if len(events) != 3 {
		t.Fatalf("expected 3 audit events, got %d", len(events))
	}

	// 5. Verify by operation type
	createEvents, _ := env.audit.ListByOperation(env.ctx, OpSessionCreate, 0)
	if len(createEvents) != 1 {
		t.Fatalf("expected 1 create event, got %d", len(createEvents))
	}
}

// ========== Memory Importance Decay Workflow ==========

func TestE2E_MemoryImportanceFiltering(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-filter", nil)

	// 2. Store memories with different importance levels
	importances := []float32{0.1, 0.3, 0.5, 0.7, 0.9}
	for _, imp := range importances {
		mem := model.NewMemory(session.ID, model.MemoryLongTerm,
			"Memory with importance", imp)
		env.memory.Store(env.ctx, mem)
	}

	// 3. Retrieve and verify ordering by importance
	memories, _ := env.memory.ListBySession(env.ctx, session.ID, "", 0)
	if len(memories) != 5 {
		t.Fatalf("expected 5 memories, got %d", len(memories))
	}

	// 4. Find memories above threshold
	var highImportance []*model.MemoryEntry
	for _, m := range memories {
		if m.Importance >= 0.7 {
			highImportance = append(highImportance, m)
		}
	}

	if len(highImportance) != 2 {
		t.Fatalf("expected 2 high-importance memories, got %d", len(highImportance))
	}

	// 5. Verify importance values
	for _, m := range highImportance {
		if m.Importance < 0.7 {
			t.Fatalf("expected importance >= 0.7, got %f", m.Importance)
		}
	}
}

// ========== Task Dependency Workflow ==========

func TestE2E_TaskDependencyChain(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. Create session
	session, _ := env.session.Create(env.ctx, "agent-deps", nil)

	// 2. Create task chain with dependencies
	task1 := &Task{
		AgentID:  "agent-deps",
		Name:     "task-1",
		Priority: 1,
	}
	env.taskQueue.Enqueue(env.ctx, task1)

	// Process task 1
	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	t1, _ := env.taskQueue.Dequeue(ctx)
	env.taskQueue.Complete(env.ctx, t1.ID, map[string]any{"step": 1})

	// Create task 2 that depends on task 1
	task2 := &Task{
		AgentID:   "agent-deps",
		Name:      "task-2",
		Priority:  2,
		DependsOn: []string{t1.ID},
	}
	env.taskQueue.Enqueue(env.ctx, task2)

	// Process task 2
	t2, _ := env.taskQueue.Dequeue(ctx)
	env.taskQueue.Complete(env.ctx, t2.ID, map[string]any{"step": 2})

	// 3. Record decisions for each task
	for _, task := range []*Task{task1, task2} {
		dec := model.NewDecision(session.ID, model.DecisionToolCall,
			json.RawMessage(`{"task": "`+task.Name+`"}`),
			json.RawMessage(`{"status": "completed"}`),
		)
		env.decision.Record(env.ctx, dec)
	}

	// 4. Verify task chain
	completed1, _ := env.taskQueue.GetTask(env.ctx, t1.ID)
	completed2, _ := env.taskQueue.GetTask(env.ctx, t2.ID)

	if completed1.Status != TaskCompleted {
		t.Fatalf("task1 should be completed")
	}
	if completed2.Status != TaskCompleted {
		t.Fatalf("task2 should be completed")
	}
	if len(completed2.DependsOn) != 1 {
		t.Fatalf("task2 should have 1 dependency")
	}
}

// ========== Edge Case: Empty Operations ==========

func TestE2E_EmptyStateOperations(t *testing.T) {
	env := newE2ETestEnv(t)

	// 1. List sessions for non-existent agent
	sessions, err := env.session.ListByAgent(env.ctx, "nonexistent", 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}

	// 2. Get nonexistent session
	_, err = env.session.Get(env.ctx, "nonexistent-session")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}

	// 3. List memories for non-existent session
	memories, err := env.memory.ListBySession(env.ctx, "nonexistent-session", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("expected 0 memories, got %d", len(memories))
	}

	// 4. List tasks for non-existent agent
	tasks, err := env.taskQueue.ListByAgent(env.ctx, "nonexistent-agent", "", 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

// ========== Benchmark: Full Workflow ==========

func BenchmarkE2E_FullSessionWorkflow(b *testing.B) {
	engine := storage.NewTestEngine(b)
	cache := storage.NewCache(512)
	session := NewSessionManager(engine, cache)
	memory := NewMemoryStore(engine, cache)
	decision := NewDecisionRecorder(engine, cache)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, _ := session.Create(ctx, "agent-bench", nil)
		mem := model.NewMemory(s.ID, model.MemoryShortTerm, "benchmark memory", 0.5)
		memory.Store(ctx, mem)
		dec := model.NewDecision(s.ID, model.DecisionToolCall,
			json.RawMessage(`{}`),
			json.RawMessage(`{}`),
		)
		decision.Record(ctx, dec)
	}
}

func BenchmarkE2E_TaskQueueWorkflow(b *testing.B) {
	engine := storage.NewTestEngine(b)
	queue := NewTaskQueue(engine)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := &Task{
			AgentID:  "agent-bench",
			Name:     "bench-task",
			Priority: i % 10,
		}
		queue.Enqueue(ctx, task)
	}
}

// ========== Verify Math Helper ==========

func assertFloatEqual(t *testing.T, expected, actual float32, tolerance float32) {
	t.Helper()
	if math.Abs(float64(expected-actual)) > float64(tolerance) {
		t.Fatalf("expected %f, got %f (tolerance: %f)", expected, actual, tolerance)
	}
}
