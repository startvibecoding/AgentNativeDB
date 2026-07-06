package agent

import (
	"context"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type auditTestEnv struct {
	engine storage.Engine
	audit  *AuditLogger
	ctx    context.Context
}

func newAuditTestEnv(t *testing.T) *auditTestEnv {
	t.Helper()
	engine := storage.NewTestEngine(t)

	audit := NewAuditLogger(engine)

	return &auditTestEnv{
		engine: engine,
		audit:  audit,
		ctx:    context.Background(),
	}
}

// ========== Basic Audit Operations ==========

func TestAuditLog_Log(t *testing.T) {
	env := newAuditTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		SessionID: "session-001",
		Operation: OpSessionCreate,
		Resource:  "session",
		Details:   map[string]any{"model": "gpt-4"},
		Success:   true,
	}

	if err := env.audit.Log(env.ctx, event); err != nil {
		t.Fatalf("log: %v", err)
	}

	if event.ID == "" {
		t.Fatal("expected event ID to be set")
	}
	if event.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be set")
	}
}

func TestAuditLog_Get(t *testing.T) {
	env := newAuditTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpMemoryStore,
		Resource:  "memory",
		Success:   true,
	}
	env.audit.Log(env.ctx, event)

	fetched, err := env.audit.Get(env.ctx, event.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if fetched.Operation != OpMemoryStore {
		t.Fatalf("expected operation %s, got %s", OpMemoryStore, fetched.Operation)
	}
}

func TestAuditLog_GetNotFound(t *testing.T) {
	env := newAuditTestEnv(t)

	_, err := env.audit.Get(env.ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent event")
	}
}

func TestAuditLog_AutoFields(t *testing.T) {
	env := newAuditTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpQueryExecute,
		Resource:  "query",
		Success:   true,
	}

	before := time.Now()
	env.audit.Log(env.ctx, event)

	if event.ID == "" {
		t.Fatal("expected auto-generated ID")
	}
	if event.Timestamp.IsZero() {
		t.Fatal("expected auto-set timestamp")
	}
	if event.Timestamp.Before(before.Add(-time.Second)) {
		t.Fatal("timestamp should be recent")
	}
}

// ========== Query by Agent ==========

func TestAuditLog_ListByAgent(t *testing.T) {
	env := newAuditTestEnv(t)

	// Create events for different agents
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-001", Operation: OpSessionCreate, Resource: "session", Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-001", Operation: OpMemoryStore, Resource: "memory", Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-002", Operation: OpSessionCreate, Resource: "session", Success: true})

	events, err := env.audit.ListByAgent(env.ctx, "agent-001", 0)
	if err != nil {
		t.Fatalf("list by agent: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events for agent-001, got %d", len(events))
	}
}

func TestAuditLog_ListByAgentWithLimit(t *testing.T) {
	env := newAuditTestEnv(t)

	for i := 0; i < 5; i++ {
		env.audit.Log(env.ctx, &AuditEvent{
			AgentID:   "agent-001",
			Operation: OpSessionCreate,
			Resource:  "session",
			Success:   true,
		})
	}

	events, err := env.audit.ListByAgent(env.ctx, "agent-001", 3)
	if err != nil {
		t.Fatalf("list by agent: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

// ========== Query by Operation ==========

func TestAuditLog_ListByOperation(t *testing.T) {
	env := newAuditTestEnv(t)

	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-001", Operation: OpSessionCreate, Resource: "session", Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-001", Operation: OpMemoryStore, Resource: "memory", Success: true})
	env.audit.Log(env.ctx, &AuditEvent{AgentID: "agent-002", Operation: OpSessionCreate, Resource: "session", Success: true})

	events, err := env.audit.ListByOperation(env.ctx, OpSessionCreate, 0)
	if err != nil {
		t.Fatalf("list by operation: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events with OpSessionCreate, got %d", len(events))
	}
}

// ========== Query by Time ==========

func TestAuditLog_ListRecent(t *testing.T) {
	env := newAuditTestEnv(t)

	// Create some events
	for i := 0; i < 3; i++ {
		env.audit.Log(env.ctx, &AuditEvent{
			AgentID:   "agent-001",
			Operation: OpSessionCreate,
			Resource:  "session",
			Success:   true,
		})
	}

	// List all recent events
	recent, err := env.audit.ListRecent(env.ctx, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}

	if len(recent) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(recent))
	}
}

func TestAuditLog_ListRecentWithTimeFilter(t *testing.T) {
	env := newAuditTestEnv(t)

	// Create events before the filter time
	oldTime := time.Now().Add(-2 * time.Hour)
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpSessionCreate,
		Resource:  "session",
		Success:   true,
		Timestamp: oldTime,
	})

	// Create events after the filter time
	env.audit.Log(env.ctx, &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpMemoryStore,
		Resource:  "memory",
		Success:   true,
	})

	recent, err := env.audit.ListRecent(env.ctx, time.Now().Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}

	// Only the recent one should be returned
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent event, got %d", len(recent))
	}
}

func TestAuditLog_ListRecentWithLimit(t *testing.T) {
	env := newAuditTestEnv(t)

	for i := 0; i < 10; i++ {
		env.audit.Log(env.ctx, &AuditEvent{
			AgentID:   "agent-001",
			Operation: OpSessionCreate,
			Resource:  "session",
			Success:   true,
		})
	}

	recent, err := env.audit.ListRecent(env.ctx, time.Now().Add(-time.Hour), 3)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}

	if len(recent) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(recent))
	}
}

// ========== All Operation Types ==========

func TestAuditLog_AllOperations(t *testing.T) {
	env := newAuditTestEnv(t)

	operations := []AuditOperation{
		OpSessionCreate,
		OpSessionClose,
		OpMemoryStore,
		OpMemoryRecall,
		OpDecisionRecord,
		OpQueryExecute,
		OpTaskSubmit,
		OpTaskComplete,
		OpRoomCreate,
		OpRoomJoin,
		OpRoomMessage,
		OpPermission,
	}

	for _, op := range operations {
		event := &AuditEvent{
			AgentID:   "agent-001",
			Operation: op,
			Resource:  "test",
			Success:   true,
		}
		if err := env.audit.Log(env.ctx, event); err != nil {
			t.Fatalf("log operation %s: %v", op, err)
		}
	}

	for _, op := range operations {
		events, err := env.audit.ListByOperation(env.ctx, op, 0)
		if err != nil {
			t.Fatalf("list by operation %s: %v", op, err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event for %s, got %d", op, len(events))
		}
	}
}

// ========== Error and Success Tracking ==========

func TestAuditLog_ErrorEvent(t *testing.T) {
	env := newAuditTestEnv(t)

	event := &AuditEvent{
		AgentID:   "agent-001",
		Operation: OpQueryExecute,
		Resource:  "query",
		Success:   false,
		Error:     "syntax error in SQL",
		DurationMs: 150,
	}

	env.audit.Log(env.ctx, event)

	fetched, _ := env.audit.Get(env.ctx, event.ID)
	if fetched.Success {
		t.Fatal("expected success to be false")
	}
	if fetched.Error != "syntax error in SQL" {
		t.Fatalf("expected error message, got %q", fetched.Error)
	}
	if fetched.DurationMs != 150 {
		t.Fatalf("expected duration 150ms, got %d", fetched.DurationMs)
	}
}

// ========== Concurrent Operations ==========

func TestAuditLog_ConcurrentLogging(t *testing.T) {
	env := newAuditTestEnv(t)

	done := make(chan struct{})
	n := 100

	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			env.audit.Log(env.ctx, &AuditEvent{
				AgentID:   "agent-001",
				Operation: OpSessionCreate,
				Resource:  "session",
				Success:   true,
			})
		}(i)
	}

	for i := 0; i < n; i++ {
		<-done
	}

	events, _ := env.audit.ListByAgent(env.ctx, "agent-001", 0)
	if len(events) != n {
		t.Fatalf("expected %d events, got %d", n, len(events))
	}
}
