package agent

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// TaskQueue 任务队列
type TaskQueue struct {
	engine storage.Engine
	mu     sync.Mutex
	pq     priorityQueue
	notify chan struct{} // 有新任务的通知
	closed bool
}

// NewTaskQueue 创建任务队列
func NewTaskQueue(engine storage.Engine) *TaskQueue {
	q := &TaskQueue{
		engine: engine,
		notify: make(chan struct{}, 1),
	}
	heap.Init(&q.pq)
	return q
}

// Enqueue 入队任务
func (q *TaskQueue) Enqueue(ctx context.Context, task *Task) error {
	if task.ID == "" {
		task.ID = util.NewUUID()
	}
	if task.Status == "" {
		task.Status = TaskPending
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	// 持久化
	if err := q.saveTask(ctx, task); err != nil {
		return err
	}

	q.mu.Lock()
	heap.Push(&q.pq, task)
	q.mu.Unlock()

	// 通知消费者
	select {
	case q.notify <- struct{}{}:
	default:
	}

	return nil
}

// Dequeue 出队任务（阻塞直到有任务或超时）
func (q *TaskQueue) Dequeue(ctx context.Context) (*Task, error) {
	for {
		q.mu.Lock()
		if q.pq.Len() > 0 {
			task := heap.Pop(&q.pq).(*Task)
			task.Status = TaskRunning
			task.StartedAt = time.Now()
			q.saveTask(ctx, task)
			q.mu.Unlock()
			return task, nil
		}
		q.mu.Unlock()

		select {
		case <-q.notify:
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Complete 标记任务完成
func (q *TaskQueue) Complete(ctx context.Context, taskID string, result any) error {
	return q.updateStatus(ctx, taskID, TaskCompleted, result, "")
}

// Fail 标记任务失败
func (q *TaskQueue) Fail(ctx context.Context, taskID string, reason string) error {
	return q.updateStatus(ctx, taskID, TaskFailed, nil, reason)
}

// Cancel 取消任务
func (q *TaskQueue) Cancel(ctx context.Context, taskID string) error {
	return q.updateStatus(ctx, taskID, TaskCancelled, nil, "")
}

// GetTask 获取任务
func (q *TaskQueue) GetTask(ctx context.Context, taskID string) (*Task, error) {
	key := storage.EncodeKey(storage.PrefixSystem, "task:"+taskID)
	data, err := q.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// ListByAgent 列出 Agent 的任务
func (q *TaskQueue) ListByAgent(ctx context.Context, agentID string, status TaskStatus, limit int) ([]*Task, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixSystem, "agenttask:"+agentID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := q.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tasks []*Task
	for iter.Next() {
		key, _ := iter.Item()
		taskID := storage.DecodeIndexID(key)
		task, err := q.GetTask(ctx, taskID)
		if err != nil {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// Len 队列长度
func (q *TaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pq.Len()
}

// Close 关闭队列
func (q *TaskQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
}

func (q *TaskQueue) updateStatus(ctx context.Context, taskID string, status TaskStatus, result any, reason string) error {
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Status = status
	now := time.Now()
	task.EndedAt = &now

	if result != nil {
		task.Result = result
	}
	if reason != "" {
		task.Error = reason
	}

	if status == TaskCompleted && !task.StartedAt.IsZero() {
		task.DurationMs = uint64(now.Sub(task.StartedAt).Milliseconds())
	}

	return q.saveTask(ctx, task)
}

func (q *TaskQueue) saveTask(ctx context.Context, task *Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixSystem, "task:"+task.ID)
	if err := q.engine.Set(ctx, key, data); err != nil {
		return err
	}

	// Agent 索引
	if task.AgentID != "" {
		idxKey := storage.EncodeIndexKey(storage.PrefixSystem, "agenttask:"+task.AgentID, task.ID)
		q.engine.Set(ctx, idxKey, []byte{1})
	}

	return nil
}

// Task 任务定义
type Task struct {
	ID          string      `json:"id"`
	AgentID     string      `json:"agent_id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Priority    int         `json:"priority"` // 数值越小优先级越高
	Status      TaskStatus  `json:"status"`
	Input       any         `json:"input,omitempty"`
	Result      any         `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`
	DependsOn   []string    `json:"depends_on,omitempty"` // 依赖的任务 ID
	DurationMs  uint64      `json:"duration_ms,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	StartedAt   time.Time   `json:"started_at,omitempty"`
	EndedAt     *time.Time  `json:"ended_at,omitempty"`
}

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// ========== 优先级队列实现 ==========

type priorityQueue []*Task

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x any) {
	*pq = append(*pq, x.(*Task))
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}
