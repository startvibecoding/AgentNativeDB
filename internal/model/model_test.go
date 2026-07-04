package model

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// ========== Session Serialization Tests ==========

func TestSessionSerialization(t *testing.T) {
	now := time.Now()
	session := &AgentSession{
		ID:        "session-001",
		AgentID:   "agent-alpha",
		State:     SessionActive,
		Context:   map[string]any{"task": "analyze", "model": "gpt-4"},
		Metadata:  map[string]any{"version": "1.0"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := SessionToJSON(session)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded AgentSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.ID != session.ID {
		t.Fatalf("ID mismatch: expected %s, got %s", session.ID, decoded.ID)
	}
	if decoded.AgentID != session.AgentID {
		t.Fatalf("AgentID mismatch: expected %s, got %s", session.AgentID, decoded.AgentID)
	}
	if decoded.State != session.State {
		t.Fatalf("State mismatch: expected %s, got %s", session.State, decoded.State)
	}
}

func TestSessionFromJSON(t *testing.T) {
	jsonData := `{
		"id": "session-002",
		"agent_id": "agent-beta",
		"state": "completed",
		"context": {"key": "value"},
		"metadata": null
	}`

	session, err := SessionFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if session.ID != "session-002" {
		t.Fatalf("expected ID 'session-002', got %q", session.ID)
	}
	if session.AgentID != "agent-beta" {
		t.Fatalf("expected agent 'agent-beta', got %q", session.AgentID)
	}
	if session.State != SessionCompleted {
		t.Fatalf("expected state 'completed', got %q", session.State)
	}
}

func TestSessionFromJSONInvalid(t *testing.T) {
	_, err := SessionFromJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSessionNewSession(t *testing.T) {
	metadata := map[string]any{"source": "api"}
	session := NewSession("agent-001", metadata)

	if session.AgentID != "agent-001" {
		t.Fatalf("expected agent 'agent-001', got %q", session.AgentID)
	}
	if session.State != SessionActive {
		t.Fatalf("expected active state, got %q", session.State)
	}
	if session.Context == nil {
		t.Fatal("expected non-nil context")
	}
	if session.Metadata["source"] != "api" {
		t.Fatalf("expected metadata source 'api', got %q", session.Metadata["source"])
	}
	if session.CreatedAt.IsZero() {
		t.Fatal("expected non-zero created_at")
	}
}

// ========== Decision Serialization Tests ==========

func TestDecisionSerialization(t *testing.T) {
	parentID := "parent-001"
	decision := &Decision{
		ID:         "decision-001",
		SessionID:  "session-001",
		ParentID:   &parentID,
		Type:       DecisionToolCall,
		Input:      json.RawMessage(`{"tool": "search", "query": "test"}`),
		Output:     json.RawMessage(`{"results": 5}`),
		Reasoning:  "Search test query",
		ToolsUsed:  []string{"search"},
		DurationMs: 150,
		TokenUsage: &TokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	data, err := DecisionToJSON(decision)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded Decision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.ID != decision.ID {
		t.Fatalf("ID mismatch")
	}
	if decoded.Type != decision.Type {
		t.Fatalf("Type mismatch: expected %s, got %s", decision.Type, decoded.Type)
	}
	if decoded.Reasoning != decision.Reasoning {
		t.Fatalf("Reasoning mismatch")
	}
	if decoded.TokenUsage.TotalTokens != 150 {
		t.Fatalf("TokenUsage mismatch")
	}
}

func TestDecisionFromJSON(t *testing.T) {
	jsonData := `{
		"id": "dec-001",
		"session_id": "sess-001",
		"type": "reasoning",
		"input": {"context": "user asked about weather"},
		"output": {"response": "It's sunny"},
		"reasoning": "User wants weather info",
		"tools_used": ["weather_api"],
		"duration_ms": 200,
		"token_usage": {
			"prompt_tokens": 50,
			"completion_tokens": 30,
			"total_tokens": 80
		}
	}`

	decision, err := DecisionFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decision.Type != DecisionReasoning {
		t.Fatalf("expected type reasoning, got %q", decision.Type)
	}
	if decision.TokenUsage.TotalTokens != 80 {
		t.Fatalf("expected 80 tokens, got %d", decision.TokenUsage.TotalTokens)
	}
	if len(decision.ToolsUsed) != 1 || decision.ToolsUsed[0] != "weather_api" {
		t.Fatalf("expected tools_used ['weather_api'], got %v", decision.ToolsUsed)
	}
}

func TestDecisionFromJSONInvalid(t *testing.T) {
	_, err := DecisionFromJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewDecision(t *testing.T) {
	input := json.RawMessage(`{"query": "test"}`)
	output := json.RawMessage(`{"result": "ok"}`)

	decision := NewDecision("session-001", DecisionPlanning, input, output)

	if decision.SessionID != "session-001" {
		t.Fatalf("expected session 'session-001', got %q", decision.SessionID)
	}
	if decision.Type != DecisionPlanning {
		t.Fatalf("expected type planning, got %q", decision.Type)
	}
	if string(decision.Input) != string(input) {
		t.Fatalf("input mismatch")
	}
	if decision.CreatedAt.IsZero() {
		t.Fatal("expected non-zero created_at")
	}
}

// ========== Memory Tests ==========

func TestMemoryEntryFromJSON(t *testing.T) {
	jsonData := `{
		"id": "mem-001",
		"session_id": "session-001",
		"type": "long_term",
		"content": "User prefers dark mode",
		"importance": 0.85
	}`

	memory, err := MemoryFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if memory.Type != MemoryLongTerm {
		t.Fatalf("expected type long_term, got %q", memory.Type)
	}
	if memory.Content != "User prefers dark mode" {
		t.Fatalf("expected content 'User prefers dark mode', got %q", memory.Content)
	}
	if math.Abs(float64(memory.Importance-0.85)) > 1e-6 {
		t.Fatalf("expected importance 0.85, got %f", memory.Importance)
	}
}

func TestMemorySerialization(t *testing.T) {
	memory := &MemoryEntry{
		ID:           "mem-002",
		SessionID:    "session-002",
		Type:         MemoryWorking,
		Content:      "Current task: analyze data",
		Importance:   0.5,
		Associations: []UUID{"assoc-1"},
	}

	data, err := MemoryToJSON(memory)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded MemoryEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.Content != memory.Content {
		t.Fatalf("content mismatch")
	}
	if decoded.Type != memory.Type {
		t.Fatalf("type mismatch")
	}
}

func TestNewMemory(t *testing.T) {
	memory := NewMemory("session-001", MemoryShortTerm, "Short term content", 0.3)

	if memory.SessionID != "session-001" {
		t.Fatalf("expected session 'session-001', got %q", memory.SessionID)
	}
	if memory.Type != MemoryShortTerm {
		t.Fatalf("expected type short_term, got %q", memory.Type)
	}
	if memory.Content != "Short term content" {
		t.Fatalf("expected content 'Short term content', got %q", memory.Content)
	}
	if math.Abs(float64(memory.Importance-0.3)) > 1e-6 {
		t.Fatalf("expected importance 0.3, got %f", memory.Importance)
	}
	if memory.CreatedAt.IsZero() {
		t.Fatal("expected non-zero created_at")
	}
	if memory.AccessCount != 0 {
		t.Fatalf("expected access count 0, got %d", memory.AccessCount)
	}
}

// ========== Entity and Relation Tests ==========

func TestEntitySerialization(t *testing.T) {
	entity := &Entity{
		ID:         "entity-001",
		Type:       "Person",
		Name:       "Alice",
		Properties: json.RawMessage(`{"age": 30, "city": "Beijing"}`),
		Source:     "import",
		Confidence: 0.95,
	}

	data, err := EntityToJSON(entity)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded Entity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.Name != "Alice" {
		t.Fatalf("expected name Alice, got %q", decoded.Name)
	}
	if decoded.Type != "Person" {
		t.Fatalf("expected type Person, got %q", decoded.Type)
	}
	if math.Abs(float64(decoded.Confidence-0.95)) > 1e-6 {
		t.Fatalf("expected confidence 0.95, got %f", decoded.Confidence)
	}
}

func TestEntityFromJSON(t *testing.T) {
	jsonData := `{
		"id": "entity-002",
		"type": "Location",
		"name": "Shanghai",
		"properties": {"population": 24000000},
		"confidence": 0.9
	}`

	entity, err := EntityFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if entity.Name != "Shanghai" {
		t.Fatalf("expected name Shanghai, got %q", entity.Name)
	}
}

func TestNewEntity(t *testing.T) {
	props := json.RawMessage(`{"color": "blue"}`)
	entity := NewEntity("Car", "Tesla", props, 0.99)

	if entity.Type != "Car" {
		t.Fatalf("expected type Car, got %q", entity.Type)
	}
	if entity.Name != "Tesla" {
		t.Fatalf("expected name Tesla, got %q", entity.Name)
	}
	if math.Abs(float64(entity.Confidence-0.99)) > 1e-6 {
		t.Fatalf("expected confidence 0.99, got %f", entity.Confidence)
	}
}

func TestRelationSerialization(t *testing.T) {
	relation := &Relation{
		ID:         "rel-001",
		Type:       "works_at",
		SourceID:   "person-001",
		TargetID:   "company-001",
		Properties: json.RawMessage(`{"since": "2020"}`),
		Weight:     0.8,
	}

	data, err := RelationToJSON(relation)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded Relation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.Type != "works_at" {
		t.Fatalf("expected type works_at, got %q", decoded.Type)
	}
	if decoded.SourceID != "person-001" {
		t.Fatalf("expected source_id person-001, got %q", decoded.SourceID)
	}
	if math.Abs(float64(decoded.Weight-0.8)) > 1e-6 {
		t.Fatalf("expected weight 0.8, got %f", decoded.Weight)
	}
}

func TestRelationFromJSON(t *testing.T) {
	jsonData := `{
		"id": "rel-002",
		"type": "lives_in",
		"source_id": "person-002",
		"target_id": "city-001",
		"weight": 1.0
	}`

	relation, err := RelationFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if relation.Type != "lives_in" {
		t.Fatalf("expected type lives_in, got %q", relation.Type)
	}
}

func TestNewRelation(t *testing.T) {
	props := json.RawMessage(`{"confidence": 0.9}`)
	relation := NewRelation("friend_of", "person-001", "person-002", props, 0.7)

	if relation.Type != "friend_of" {
		t.Fatalf("expected type friend_of, got %q", relation.Type)
	}
	if relation.SourceID != "person-001" {
		t.Fatalf("expected source_id person-001, got %q", relation.SourceID)
	}
	if relation.TargetID != "person-002" {
		t.Fatalf("expected target_id person-002, got %q", relation.TargetID)
	}
	if math.Abs(float64(relation.Weight-0.7)) > 1e-6 {
		t.Fatalf("expected weight 0.7, got %f", relation.Weight)
	}
}

// ========== DataLineage Tests ==========

func TestDataLineageSerialization(t *testing.T) {
	lineage := &DataLineage{
		DataID:     "data-001",
		SourceType: SourceDerived,
		SourceIDs:  []string{"data-source-1", "data-source-2"},
		Transformations: []Transformation{
			{Type: "filter", Params: map[string]any{"condition": "age > 18"}},
			{Type: "map", Params: map[string]any{"field": "name"}},
		},
		AgentDecisions: []string{"decision-001", "decision-002"},
	}

	data, err := LineageToJSON(lineage)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded DataLineage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.SourceType != SourceDerived {
		t.Fatalf("expected source type derived, got %q", decoded.SourceType)
	}
	if len(decoded.SourceIDs) != 2 {
		t.Fatalf("expected 2 source IDs, got %d", len(decoded.SourceIDs))
	}
	if len(decoded.Transformations) != 2 {
		t.Fatalf("expected 2 transformations, got %d", len(decoded.Transformations))
	}
}

func TestDataLineageFromJSON(t *testing.T) {
	jsonData := `{
		"data_id": "data-002",
		"source_type": "agent_generated",
		"source_ids": ["source-1"],
		"transformations": [{"type": "normalize"}],
		"agent_decisions": ["dec-1"]
	}`

	lineage, err := LineageFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if lineage.SourceType != SourceAgentGenerated {
		t.Fatalf("expected source type agent_generated, got %q", lineage.SourceType)
	}
}

// ========== Map/JSON Conversion Tests ==========

func TestMapToJSON(t *testing.T) {
	m := map[string]any{
		"key":   "value",
		"count": 42,
		"flag":  true,
	}

	raw := MapToJSON(m)
	if raw == nil {
		t.Fatal("expected non-nil result")
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["key"] != "value" {
		t.Fatalf("expected key 'value', got %q", decoded["key"])
	}
}

func TestMapToJSONNil(t *testing.T) {
	raw := MapToJSON(nil)
	if raw != nil {
		t.Fatalf("expected nil for nil input, got %v", raw)
	}
}

func TestJSONToMap(t *testing.T) {
	raw := json.RawMessage(`{"name": "test", "value": 123}`)
	m := JSONToMap(raw)

	if m["name"] != "test" {
		t.Fatalf("expected name 'test', got %q", m["name"])
	}
}

func TestJSONToMapNil(t *testing.T) {
	m := JSONToMap(nil)
	if m != nil {
		t.Fatalf("expected nil for nil input, got %v", m)
	}
}

// ========== Type Constants Tests ==========

func TestSessionStateValues(t *testing.T) {
	states := []SessionState{SessionActive, SessionPaused, SessionCompleted, SessionFailed}
	expected := []string{"active", "paused", "completed", "failed"}

	for i, state := range states {
		if string(state) != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], state)
		}
	}
}

func TestMemoryTypeValues(t *testing.T) {
	types := []MemoryType{MemoryShortTerm, MemoryLongTerm, MemoryWorking}
	expected := []string{"short_term", "long_term", "working"}

	for i, mt := range types {
		if string(mt) != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], mt)
		}
	}
}

func TestDecisionTypeValues(t *testing.T) {
	types := []DecisionType{DecisionReasoning, DecisionToolCall, DecisionPlanning, DecisionReflection}
	expected := []string{"reasoning", "tool_call", "planning", "reflection"}

	for i, dt := range types {
		if string(dt) != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], dt)
		}
	}
}

func TestSourceTypeValues(t *testing.T) {
	types := []SourceType{SourceRaw, SourceDerived, SourceAgentGenerated}
	expected := []string{"raw", "derived", "agent_generated"}

	for i, st := range types {
		if string(st) != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], st)
		}
	}
}

// ========== TokenUsage Tests ==========

func TestTokenUsageSerialization(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TokenUsage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.TotalTokens != 150 {
		t.Fatalf("expected total 150, got %d", decoded.TotalTokens)
	}
}

func TestTokenUsageZero(t *testing.T) {
	usage := TokenUsage{}
	if usage.TotalTokens != 0 {
		t.Fatalf("expected total 0, got %d", usage.TotalTokens)
	}
}

// ========== Transformation Tests ==========

func TestTransformationSerialization(t *testing.T) {
	trans := Transformation{
		Type:   "aggregate",
		Params: map[string]any{"function": "sum", "field": "amount"},
	}

	data, err := json.Marshal(trans)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Transformation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != "aggregate" {
		t.Fatalf("expected type aggregate, got %q", decoded.Type)
	}
	if decoded.Params["function"] != "sum" {
		t.Fatalf("expected function 'sum', got %q", decoded.Params["function"])
	}
}

func TestTransformationEmptyParams(t *testing.T) {
	trans := Transformation{Type: "filter"}

	data, err := json.Marshal(trans)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Transformation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != "filter" {
		t.Fatalf("expected type filter, got %q", decoded.Type)
	}
}

// ========== Edge Cases ==========

func TestSessionWithNilContext(t *testing.T) {
	session := &AgentSession{
		ID:      "session-nil",
		AgentID: "agent-001",
		State:   SessionActive,
	}

	data, err := SessionToJSON(session)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded AgentSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.ID != "session-nil" {
		t.Fatalf("ID mismatch")
	}
}

func TestDecisionWithNilParentID(t *testing.T) {
	decision := &Decision{
		ID:         "decision-nil",
		SessionID:  "session-001",
		ParentID:   nil,
		Type:       DecisionReasoning,
		Input:      json.RawMessage(`{}`),
		Output:     json.RawMessage(`{}`),
	}

	data, err := DecisionToJSON(decision)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded Decision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.ParentID != nil {
		t.Fatalf("expected nil parent_id, got %v", decoded.ParentID)
	}
}

func TestEntityWithNilProperties(t *testing.T) {
	entity := &Entity{
		ID:         "entity-nil",
		Type:       "Test",
		Name:       "TestEntity",
		Properties: nil,
		Confidence: 1.0,
	}

	data, err := EntityToJSON(entity)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var decoded Entity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if decoded.Name != "TestEntity" {
		t.Fatalf("name mismatch")
	}
}
