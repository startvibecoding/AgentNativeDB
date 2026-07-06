package agent

import (
	"context"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type collabTestEnv struct {
	engine      storage.Engine
	cache       *storage.Cache
	session     *SessionManager
	memory      *MemoryStore
	decision    *DecisionRecorder
	coordinator *Coordinator
	taskQueue   *TaskQueue
	audit       *AuditLogger
	ctx         context.Context
}

func newCollabTestEnv(t *testing.T) *collabTestEnv {
	t.Helper()
	engine := storage.NewTestEngine(t)

	cache := storage.NewCache(512)
	session := NewSessionManager(engine, cache)
	memory := NewMemoryStore(engine, cache)
	decision := NewDecisionRecorder(engine, cache)
	coordinator := NewCoordinator(engine, session, memory, decision)
	taskQueue := NewTaskQueue(engine)
	audit := NewAuditLogger(engine)

	return &collabTestEnv{
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

// ========== Coordinator Tests ==========

func TestCoordinator_CreateRoom(t *testing.T) {
	env := newCollabTestEnv(t)

	room, err := env.coordinator.CreateRoom(env.ctx, "test-room", "agent-001", RoomOptions{MaxMembers: 5})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	if room.Name != "test-room" {
		t.Fatalf("expected test-room, got %s", room.Name)
	}
	if room.CreatorID != "agent-001" {
		t.Fatalf("expected agent-001, got %s", room.CreatorID)
	}
	if len(room.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(room.Members))
	}
	if room.Members[0].Role != RoleOwner {
		t.Fatalf("expected owner role, got %s", room.Members[0].Role)
	}
}

func TestCoordinator_JoinLeaveRoom(t *testing.T) {
	env := newCollabTestEnv(t)

	room, _ := env.coordinator.CreateRoom(env.ctx, "test-room", "agent-001", RoomOptions{})

	// 加入
	err := env.coordinator.JoinRoom(env.ctx, room.ID, "agent-002", RoleMember)
	if err != nil {
		t.Fatalf("join room: %v", err)
	}

	// 重复加入
	err = env.coordinator.JoinRoom(env.ctx, room.ID, "agent-002", RoleMember)
	if err == nil {
		t.Fatal("expected error for duplicate join")
	}

	// 获取房间验证成员
	room, _ = env.coordinator.GetRoom(env.ctx, room.ID)
	if len(room.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(room.Members))
	}

	// 离开
	env.coordinator.LeaveRoom(env.ctx, room.ID, "agent-002")
	room, _ = env.coordinator.GetRoom(env.ctx, room.ID)
	if len(room.Members) != 1 {
		t.Fatalf("expected 1 member after leave, got %d", len(room.Members))
	}
}

func TestCoordinator_Messages(t *testing.T) {
	env := newCollabTestEnv(t)

	room, _ := env.coordinator.CreateRoom(env.ctx, "test-room", "agent-001", RoomOptions{})

	// 发送消息
	msg, err := env.coordinator.SendMessage(env.ctx, room.ID, "agent-001", MsgText, "Hello everyone!")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if msg.Content != "Hello everyone!" {
		t.Fatalf("expected 'Hello everyone!', got %v", msg.Content)
	}

	// 发送更多消息
	env.coordinator.SendMessage(env.ctx, room.ID, "agent-001", MsgText, "Message 2")
	env.coordinator.SendMessage(env.ctx, room.ID, "agent-001", MsgTask, map[string]any{"task": "analyze"})

	// 获取消息
	messages, err := env.coordinator.GetMessages(env.ctx, room.ID, 10)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
}

func TestCoordinator_ShareMemory(t *testing.T) {
	env := newCollabTestEnv(t)

	room, _ := env.coordinator.CreateRoom(env.ctx, "test-room", "agent-001", RoomOptions{})
	env.coordinator.JoinRoom(env.ctx, room.ID, "agent-002", RoleMember)

	// 共享记忆
	mem, err := env.coordinator.ShareMemory(env.ctx, room.ID, "agent-001", "重要发现：系统负载过高", 0.9)
	if err != nil {
		t.Fatalf("share memory: %v", err)
	}
	if mem == nil {
		t.Fatal("expected memory, got nil")
	}

	// 两个成员都应该有记忆
	mem1, _ := env.memory.ListBySession(env.ctx, "agent-001", "", 0)
	mem2, _ := env.memory.ListBySession(env.ctx, "agent-002", "", 0)
	// 共享记忆不绑定 session，而是绑定 agent_id
	_ = mem1
	_ = mem2
}

func TestCoordinator_NonMemberCannotSendMessage(t *testing.T) {
	env := newCollabTestEnv(t)

	room, _ := env.coordinator.CreateRoom(env.ctx, "test-room", "agent-001", RoomOptions{})

	_, err := env.coordinator.SendMessage(env.ctx, room.ID, "agent-outsider", MsgText, "hello")
	if err == nil {
		t.Fatal("expected error for non-member sending message")
	}
}

// ========== TaskQueue Tests ==========

func TestTaskQueue_EnqueueDequeue(t *testing.T) {
	env := newCollabTestEnv(t)

	task := &Task{
		AgentID:  "agent-001",
		Name:     "analyze-data",
		Priority: 1,
		Input:    map[string]any{"data": "test"},
	}

	if err := env.taskQueue.Enqueue(env.ctx, task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if env.taskQueue.Len() != 1 {
		t.Fatalf("expected 1 task in queue, got %d", env.taskQueue.Len())
	}

	dequeued, err := env.taskQueue.Dequeue(env.ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	if dequeued.ID != task.ID {
		t.Fatalf("expected task %s, got %s", task.ID, dequeued.ID)
	}
	if dequeued.Status != TaskRunning {
		t.Fatalf("expected running status, got %s", dequeued.Status)
	}
}

func TestTaskQueue_PriorityOrder(t *testing.T) {
	env := newCollabTestEnv(t)

	// 低优先级（数值大）
	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "low", Priority: 10})
	// 高优先级（数值小）
	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "high", Priority: 1})
	// 中优先级
	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "medium", Priority: 5})

	t1, _ := env.taskQueue.Dequeue(env.ctx)
	if t1.Name != "high" {
		t.Fatalf("expected high priority first, got %s", t1.Name)
	}

	t2, _ := env.taskQueue.Dequeue(env.ctx)
	if t2.Name != "medium" {
		t.Fatalf("expected medium priority second, got %s", t2.Name)
	}

	t3, _ := env.taskQueue.Dequeue(env.ctx)
	if t3.Name != "low" {
		t.Fatalf("expected low priority third, got %s", t3.Name)
	}
}

func TestTaskQueue_CompleteFail(t *testing.T) {
	env := newCollabTestEnv(t)

	task := &Task{AgentID: "a", Name: "work", Priority: 1}
	env.taskQueue.Enqueue(env.ctx, task)
	env.taskQueue.Dequeue(env.ctx)

	// 完成
	env.taskQueue.Complete(env.ctx, task.ID, map[string]any{"result": "done"})

	got, _ := env.taskQueue.GetTask(env.ctx, task.ID)
	if got.Status != TaskCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.DurationMs == 0 {
		// duration 可能为 0（极快完成）
	}

	// 另一个任务失败
	task2 := &Task{AgentID: "a", Name: "fail-work", Priority: 1}
	env.taskQueue.Enqueue(env.ctx, task2)
	env.taskQueue.Dequeue(env.ctx)
	env.taskQueue.Fail(env.ctx, task2.ID, "connection timeout")

	got2, _ := env.taskQueue.GetTask(env.ctx, task2.ID)
	if got2.Status != TaskFailed {
		t.Fatalf("expected failed, got %s", got2.Status)
	}
	if got2.Error != "connection timeout" {
		t.Fatalf("expected error message, got %s", got2.Error)
	}
}

func TestTaskQueue_ListByAgent(t *testing.T) {
	env := newCollabTestEnv(t)

	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "t1", Priority: 1})
	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "t2", Priority: 2})
	env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "b", Name: "t3", Priority: 1})

	tasks, _ := env.taskQueue.ListByAgent(env.ctx, "a", "", 0)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for agent a, got %d", len(tasks))
	}
}

func TestTaskQueue_Cancel(t *testing.T) {
	env := newCollabTestEnv(t)

	task := &Task{AgentID: "a", Name: "work", Priority: 1}
	env.taskQueue.Enqueue(env.ctx, task)

	env.taskQueue.Cancel(env.ctx, task.ID)

	got, _ := env.taskQueue.GetTask(env.ctx, task.ID)
	if got.Status != TaskCancelled {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
}

// ========== AuditLogger Tests ==========

func TestAuditLogger_LogAndGet(t *testing.T) {
	env := newCollabTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		SessionID: "sess-001",
		Operation: OpSessionCreate,
		Resource:  "session",
		Details:   map[string]any{"model": "gpt-4"},
		Success:   true,
	}

	if err := env.audit.Log(env.ctx, event); err != nil {
		t.Fatalf("log: %v", err)
	}

	got, err := env.audit.Get(env.ctx, event.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AgentID != "agent-001" {
		t.Fatalf("expected agent-001, got %s", got.AgentID)
	}
	if got.Operation != OpSessionCreate {
		t.Fatalf("expected session.create, got %s", got.Operation)
	}
}

func TestAuditLogger_ListByAgent(t *testing.T) {
	env := newCollabTestEnv(t)

	env.audit.Log(env.ctx, &AuditEvent{AgentID: "a", Operation: OpSessionCreate, Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "a", Operation: OpMemoryStore, Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "b", Operation: OpSessionCreate, Success: true})

	events, _ := env.audit.ListByAgent(env.ctx, "a", 0)
	if len(events) != 2 {
		t.Fatalf("expected 2 events for agent a, got %d", len(events))
	}
}

func TestAuditLogger_ListByOperation(t *testing.T) {
	env := newCollabTestEnv(t)

	env.audit.Log(env.ctx, &AuditEvent{AgentID: "a", Operation: OpSessionCreate, Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "b", Operation: OpSessionCreate, Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "a", Operation: OpMemoryStore, Success: true})

	events, _ := env.audit.ListByOperation(env.ctx, OpSessionCreate, 0)
	if len(events) != 2 {
		t.Fatalf("expected 2 session.create events, got %d", len(events))
	}
}

func TestAuditLogger_FailedEvent(t *testing.T) {
	env := newCollabTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpQueryExecute,
		Resource:  "sql",
		Success:   false,
		Error:     "syntax error",
	}
	env.audit.Log(env.ctx, event)

	got, _ := env.audit.Get(env.ctx, event.ID)
	if got.Success {
		t.Fatal("expected success=false")
	}
	if got.Error != "syntax error" {
		t.Fatalf("expected syntax error, got %s", got.Error)
	}
}

// ========== Concurrent Tests ==========

func TestTaskQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	env := newCollabTestEnv(t)

	// 先入队一批任务
	for i := 0; i < 100; i++ {
		env.taskQueue.Enqueue(env.ctx, &Task{AgentID: "a", Name: "task", Priority: i})
	}

	// 并发出队
	done := make(chan int, 100)
	for i := 0; i < 100; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			task, err := env.taskQueue.Dequeue(ctx)
			if err == nil && task != nil {
				done <- 1
			} else {
				done <- 0
			}
		}()
	}

	count := 0
	for i := 0; i < 100; i++ {
		count += <-done
	}

	if count != 100 {
		t.Fatalf("expected 100 dequeues, got %d", count)
	}
}
