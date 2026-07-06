package agent

import (
	"context"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type taskQueueTestEnv struct {
	engine storage.Engine
	queue  *TaskQueue
	ctx    context.Context
}

func newTaskQueueTestEnv(t *testing.T) *taskQueueTestEnv {
	t.Helper()
	engine := storage.NewTestEngine(t)

	queue := NewTaskQueue(engine)
	t.Cleanup(func() { queue.Close() })

	return &taskQueueTestEnv{
		engine: engine,
		queue:  queue,
		ctx:    context.Background(),
	}
}

// ========== Basic Task Lifecycle ==========

func TestTaskQueue_EnqueueDequeueStandalone(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	task := &Task{
		AgentID:  "agent-001",
		Name:     "test-task",
		Priority: 1,
		Input:    map[string]any{"query": "hello"},
	}

	if err := env.queue.Enqueue(env.ctx, task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if env.queue.Len() != 1 {
		t.Fatalf("expected queue length 1, got %d", env.queue.Len())
	}

	// Dequeue with timeout context
	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	dequeued, err := env.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	if dequeued.Name != "test-task" {
		t.Fatalf("expected name 'test-task', got %q", dequeued.Name)
	}
	if dequeued.Status != TaskRunning {
		t.Fatalf("expected status running, got %s", dequeued.Status)
	}
	if env.queue.Len() != 0 {
		t.Fatalf("expected queue length 0 after dequeue, got %d", env.queue.Len())
	}
}

func TestTaskQueue_Complete(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	task := &Task{AgentID: "agent-001", Name: "complete-me", Priority: 1}
	env.queue.Enqueue(env.ctx, task)

	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	dequeued, _ := env.queue.Dequeue(ctx)

	result := map[string]any{"output": "success", "count": 42}
	if err := env.queue.Complete(env.ctx, dequeued.ID, result); err != nil {
		t.Fatalf("complete: %v", err)
	}

	completed, err := env.queue.GetTask(env.ctx, dequeued.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}

	if completed.Status != TaskCompleted {
		t.Fatalf("expected status completed, got %s", completed.Status)
	}
	if completed.Result == nil {
		t.Fatal("expected result to be set")
	}
	if completed.EndedAt == nil {
		t.Fatal("expected ended_at to be set")
	}
	// DurationMs may be 0 if complete happens within same millisecond
	_ = completed.DurationMs
}

func TestTaskQueue_Fail(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	task := &Task{AgentID: "agent-001", Name: "fail-me", Priority: 1}
	env.queue.Enqueue(env.ctx, task)

	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	dequeued, _ := env.queue.Dequeue(ctx)

	if err := env.queue.Fail(env.ctx, dequeued.ID, "timeout exceeded"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	failed, _ := env.queue.GetTask(env.ctx, dequeued.ID)
	if failed.Status != TaskFailed {
		t.Fatalf("expected status failed, got %s", failed.Status)
	}
	if failed.Error != "timeout exceeded" {
		t.Fatalf("expected error 'timeout exceeded', got %q", failed.Error)
	}
}

func TestTaskQueue_CancelStandalone(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	task := &Task{AgentID: "agent-001", Name: "cancel-me", Priority: 1}
	env.queue.Enqueue(env.ctx, task)

	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	dequeued, _ := env.queue.Dequeue(ctx)

	if err := env.queue.Cancel(env.ctx, dequeued.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	cancelled, _ := env.queue.GetTask(env.ctx, dequeued.ID)
	if cancelled.Status != TaskCancelled {
		t.Fatalf("expected status cancelled, got %s", cancelled.Status)
	}
}

// ========== Priority Ordering ==========

func TestTaskQueue_PriorityOrderStandalone(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	// Enqueue tasks with different priorities (lower = higher priority)
	tasks := []*Task{
		{AgentID: "agent-001", Name: "low-priority", Priority: 10},
		{AgentID: "agent-001", Name: "high-priority", Priority: 1},
		{AgentID: "agent-001", Name: "medium-priority", Priority: 5},
	}

	for _, task := range tasks {
		env.queue.Enqueue(env.ctx, task)
	}

	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	// Should dequeue in priority order
	first, _ := env.queue.Dequeue(ctx)
	if first.Name != "high-priority" {
		t.Fatalf("expected 'high-priority' first, got %q", first.Name)
	}

	second, _ := env.queue.Dequeue(ctx)
	if second.Name != "medium-priority" {
		t.Fatalf("expected 'medium-priority' second, got %q", second.Name)
	}

	third, _ := env.queue.Dequeue(ctx)
	if third.Name != "low-priority" {
		t.Fatalf("expected 'low-priority' third, got %q", third.Name)
	}
}

// ========== Task Lookup and Filtering ==========

func TestTaskQueue_GetTaskNotFound(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	_, err := env.queue.GetTask(env.ctx, "nonexistent-task-id")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestTaskQueue_ListByAgentStandalone(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	// Create tasks for different agents
	agents := []string{"agent-001", "agent-002", "agent-001"}
	for i, agent := range agents {
		task := &Task{AgentID: agent, Name: "task", Priority: i + 1}
		env.queue.Enqueue(env.ctx, task)
	}

	// List all tasks for agent-001
	tasks, err := env.queue.ListByAgent(env.ctx, "agent-001", "", 0)
	if err != nil {
		t.Fatalf("list by agent: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for agent-001, got %d", len(tasks))
	}

	// List with status filter (all pending)
	pending, _ := env.queue.ListByAgent(env.ctx, "agent-001", TaskPending, 0)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending tasks, got %d", len(pending))
	}
}

// ========== Dequeue Timeout ==========

func TestTaskQueue_DequeueTimeout(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	ctx, cancel := context.WithTimeout(env.ctx, 50*time.Millisecond)
	defer cancel()

	_, err := env.queue.Dequeue(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestTaskQueue_DequeueCancelled(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	ctx, cancel := context.WithCancel(env.ctx)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := env.queue.Dequeue(ctx)
	if err != context.Canceled {
		t.Fatalf("expected Canceled, got %v", err)
	}
}

// ========== Concurrent Operations ==========

func TestTaskQueue_ConcurrentEnqueue(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	done := make(chan struct{})
	n := 100

	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			task := &Task{
				AgentID:  "agent-001",
				Name:     "concurrent-task",
				Priority: i % 10,
				Input:    i,
			}
			env.queue.Enqueue(env.ctx, task)
		}(i)
	}

	for i := 0; i < n; i++ {
		<-done
	}

	if env.queue.Len() != n {
		t.Fatalf("expected %d tasks, got %d", n, env.queue.Len())
	}
}

// ========== Task with Dependencies ==========

func TestTaskQueue_TaskWithDependencies(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	// Create a parent task
	parent := &Task{AgentID: "agent-001", Name: "parent-task", Priority: 1}
	env.queue.Enqueue(env.ctx, parent)

	// Dequeue and complete parent
	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()

	p, _ := env.queue.Dequeue(ctx)
	env.queue.Complete(env.ctx, p.ID, map[string]any{"step": "done"})

	// Create child task that depends on parent
	child := &Task{
		AgentID:   "agent-001",
		Name:      "child-task",
		Priority:  2,
		DependsOn: []string{p.ID},
	}
	env.queue.Enqueue(env.ctx, child)

	// Verify child has dependency info
	childTask, err := env.queue.GetTask(env.ctx, child.ID)
	if err != nil {
		t.Fatalf("get child task: %v", err)
	}

	if len(childTask.DependsOn) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(childTask.DependsOn))
	}
	if childTask.DependsOn[0] != p.ID {
		t.Fatalf("expected dependency on %s, got %s", p.ID, childTask.DependsOn[0])
	}
}

// ========== Auto-generated Fields ==========

func TestTaskQueue_AutoFields(t *testing.T) {
	env := newTaskQueueTestEnv(t)

	task := &Task{
		AgentID:  "agent-001",
		Name:     "auto-fields",
		Priority: 1,
	}

	env.queue.Enqueue(env.ctx, task)

	// Verify auto-generated ID
	if task.ID == "" {
		t.Fatal("expected auto-generated ID")
	}

	// Verify auto-set status
	if task.Status != TaskPending {
		t.Fatalf("expected status pending, got %s", task.Status)
	}

	// Verify auto-set created time
	if task.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}

	// Verify task can be retrieved
	retrieved, err := env.queue.GetTask(env.ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if retrieved.ID != task.ID {
		t.Fatalf("ID mismatch: %s vs %s", retrieved.ID, task.ID)
	}
}
