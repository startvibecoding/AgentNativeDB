package model

import (
	"encoding/json"
	"time"
)

// Decision 决策记录
type Decision struct {
	ID         UUID            `json:"id"`
	SessionID  UUID            `json:"session_id"`
	ParentID   *UUID           `json:"parent_id,omitempty"`
	Type       DecisionType    `json:"type"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolsUsed  []string        `json:"tools_used,omitempty"`
	DurationMs uint64          `json:"duration_ms"`
	TokenUsage *TokenUsage     `json:"token_usage,omitempty"`
	CreatedAt  Timestamp       `json:"created_at"`
}

// NewDecision 创建新决策记录
func NewDecision(sessionID UUID, dtype DecisionType, input, output json.RawMessage) *Decision {
	return &Decision{
		SessionID: sessionID,
		Type:      dtype,
		Input:     input,
		Output:    output,
		CreatedAt: time.Now(),
	}
}

// DecisionToJSON 序列化决策
func DecisionToJSON(d *Decision) ([]byte, error) {
	return json.Marshal(d)
}

// DecisionFromJSON 反序列化决策
func DecisionFromJSON(data []byte) (*Decision, error) {
	var d Decision
	err := json.Unmarshal(data, &d)
	return &d, err
}
