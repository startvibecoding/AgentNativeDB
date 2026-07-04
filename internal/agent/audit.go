package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// AuditLogger 操作审计日志
type AuditLogger struct {
	engine storage.Engine
}

// NewAuditLogger 创建审计日志
func NewAuditLogger(engine storage.Engine) *AuditLogger {
	return &AuditLogger{engine: engine}
}

// Log 记录审计事件
func (a *AuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	if event.ID == "" {
		event.ID = util.NewUUID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	// 存储事件
	key := storage.EncodeKey(storage.PrefixSystem, "audit:"+event.ID)
	if err := a.engine.Set(ctx, key, data); err != nil {
		return err
	}

	// Agent 索引
	if event.AgentID != "" {
		idxKey := storage.EncodeIndexKey(storage.PrefixSystem, "auditagent:"+event.AgentID, event.ID)
		a.engine.Set(ctx, idxKey, []byte{1})
	}

	// 操作类型索引
	typeIdx := storage.EncodeIndexKey(storage.PrefixSystem, "auditop:"+string(event.Operation), event.ID)
	a.engine.Set(ctx, typeIdx, []byte{1})

	return nil
}

// Get 获取审计事件
func (a *AuditLogger) Get(ctx context.Context, id string) (*AuditEvent, error) {
	key := storage.EncodeKey(storage.PrefixSystem, "audit:"+id)
	data, err := a.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("audit event not found: %s", id)
	}

	var event AuditEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// ListByAgent 列出 Agent 的审计事件
func (a *AuditLogger) ListByAgent(ctx context.Context, agentID string, limit int) ([]*AuditEvent, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixSystem, "auditagent:"+agentID, "")
	return a.listByPrefix(ctx, prefix, limit)
}

// ListByOperation 按操作类型列出
func (a *AuditLogger) ListByOperation(ctx context.Context, op AuditOperation, limit int) ([]*AuditEvent, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixSystem, "auditop:"+string(op), "")
	return a.listByPrefix(ctx, prefix, limit)
}

// ListRecent 最近的审计事件
func (a *AuditLogger) ListRecent(ctx context.Context, since time.Time, limit int) ([]*AuditEvent, error) {
	// 只扫审计事件前缀，避免与 room/task/msg 等 PrefixSystem 空间内其他类型混淆
	prefix := storage.EncodeKey(storage.PrefixSystem, "audit:")
	iter, err := a.engine.PrefixScan(ctx, prefix, storage.ScanOptions{Limit: limit * 2})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var events []*AuditEvent
	for iter.Next() {
		_, val := iter.Item()
		var event AuditEvent
		if err := json.Unmarshal(val, &event); err != nil {
			continue
		}
		if event.ID == "" {
			continue
		}
		if event.Timestamp.Before(since) {
			continue
		}
		events = append(events, &event)
		if limit > 0 && len(events) >= limit {
			break
		}
	}
	return events, nil
}

func (a *AuditLogger) listByPrefix(ctx context.Context, prefix []byte, limit int) ([]*AuditEvent, error) {
	start, end := storage.PrefixRange(prefix)

	iter, err := a.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var events []*AuditEvent
	for iter.Next() {
		key, _ := iter.Item()
		eventID := storage.DecodeIndexID(key)
		event, err := a.Get(ctx, eventID)
		if err == nil {
			events = append(events, event)
		}
	}
	return events, nil
}

// AuditEvent 审计事件
type AuditEvent struct {
	ID          string         `json:"id"`
	AgentID     string         `json:"agent_id"`
	SessionID   string         `json:"session_id,omitempty"`
	Operation   AuditOperation `json:"operation"`
	Resource    string         `json:"resource"`
	Details     map[string]any `json:"details,omitempty"`
	Success     bool           `json:"success"`
	Error       string         `json:"error,omitempty"`
	DurationMs  uint64         `json:"duration_ms,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

// AuditOperation 审计操作类型
type AuditOperation string

const (
	OpSessionCreate  AuditOperation = "session.create"
	OpSessionClose   AuditOperation = "session.close"
	OpMemoryStore    AuditOperation = "memory.store"
	OpMemoryRecall   AuditOperation = "memory.recall"
	OpDecisionRecord AuditOperation = "decision.record"
	OpQueryExecute   AuditOperation = "query.execute"
	OpTaskSubmit     AuditOperation = "task.submit"
	OpTaskComplete   AuditOperation = "task.complete"
	OpRoomCreate     AuditOperation = "room.create"
	OpRoomJoin       AuditOperation = "room.join"
	OpRoomMessage    AuditOperation = "room.message"
	OpPermission     AuditOperation = "permission.check"
)
