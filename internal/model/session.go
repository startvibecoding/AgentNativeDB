package model

import (
	"encoding/json"
	"time"
)

// AgentSession Agent 会话
type AgentSession struct {
	ID        UUID           `json:"id"`
	AgentID   string         `json:"agent_id"`
	State     SessionState   `json:"state"`
	Context   map[string]any `json:"context,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`
}

// NewSession 创建新会话
func NewSession(agentID string, metadata map[string]any) *AgentSession {
	now := time.Now()
	return &AgentSession{
		AgentID:   agentID,
		State:     SessionActive,
		Context:   make(map[string]any),
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// SessionToJSON 序列化会话
func SessionToJSON(s *AgentSession) ([]byte, error) {
	return json.Marshal(s)
}

// SessionFromJSON 反序列化会话
func SessionFromJSON(data []byte) (*AgentSession, error) {
	var s AgentSession
	err := json.Unmarshal(data, &s)
	return &s, err
}
