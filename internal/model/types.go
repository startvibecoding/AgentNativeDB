package model

import (
	"encoding/json"
	"time"
)

// UUID 是 UUID v7 字符串（时间有序）
type UUID = string

// Timestamp 统一时间戳
type Timestamp = time.Time

// SessionState 会话状态
type SessionState string

const (
	SessionActive    SessionState = "active"
	SessionPaused    SessionState = "paused"
	SessionCompleted SessionState = "completed"
	SessionFailed    SessionState = "failed"
)

// MemoryType 记忆类型
type MemoryType string

const (
	MemoryShortTerm MemoryType = "short_term"
	MemoryLongTerm  MemoryType = "long_term"
	MemoryWorking   MemoryType = "working"
)

// DecisionType 决策类型
type DecisionType string

const (
	DecisionReasoning  DecisionType = "reasoning"
	DecisionToolCall   DecisionType = "tool_call"
	DecisionPlanning   DecisionType = "planning"
	DecisionReflection DecisionType = "reflection"
)

// SourceType 数据来源类型
type SourceType string

const (
	SourceRaw            SourceType = "raw"
	SourceDerived        SourceType = "derived"
	SourceAgentGenerated SourceType = "agent_generated"
)

// Transformation 数据变换记录
type Transformation struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params,omitempty"`
}

// TokenUsage Token 用量统计
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// MapToJSON 将 map 转为 json.RawMessage
func MapToJSON(m map[string]any) json.RawMessage {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// JSONToMap 将 json.RawMessage 转为 map
func JSONToMap(raw json.RawMessage) map[string]any {
	if raw == nil {
		return nil
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	return m
}
