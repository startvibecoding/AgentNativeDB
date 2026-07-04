package model

import (
	"encoding/json"
	"time"
)

// MemoryEntry Agent 记忆
type MemoryEntry struct {
	ID           UUID       `json:"id"`
	SessionID    UUID       `json:"session_id"`
	Type         MemoryType `json:"type"`
	Content      string     `json:"content"`
	Embedding    []float32  `json:"-"` // 不序列化到 JSON，单独存储
	Importance   float32    `json:"importance"`
	AccessCount  uint32     `json:"access_count"`
	Associations []UUID     `json:"associations,omitempty"`
	CreatedAt    Timestamp  `json:"created_at"`
	AccessedAt   Timestamp  `json:"accessed_at"`
}

// NewMemory 创建新记忆
func NewMemory(sessionID UUID, memType MemoryType, content string, importance float32) *MemoryEntry {
	now := time.Now()
	return &MemoryEntry{
		SessionID:   sessionID,
		Type:        memType,
		Content:     content,
		Importance:  importance,
		AccessCount: 0,
		CreatedAt:   now,
		AccessedAt:  now,
	}
}

// MemoryToJSON 序列化记忆（不含 embedding）
func MemoryToJSON(m *MemoryEntry) ([]byte, error) {
	return json.Marshal(m)
}

// MemoryFromJSON 反序列化记忆
func MemoryFromJSON(data []byte) (*MemoryEntry, error) {
	var m MemoryEntry
	err := json.Unmarshal(data, &m)
	return &m, err
}

// MemoryWithEmbedding 包含 embedding 的记忆（用于向量存储）
type MemoryWithEmbedding struct {
	*MemoryEntry
	Embedding []float32
}
