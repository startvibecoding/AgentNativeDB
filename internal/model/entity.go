package model

import (
	"encoding/json"
	"time"
)

// Entity 知识实体
type Entity struct {
	ID         UUID            `json:"id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Properties json.RawMessage `json:"properties"`
	Embedding  []float32       `json:"-"` // 单独存储
	Source     string          `json:"source,omitempty"`
	Confidence float32         `json:"confidence"`
	CreatedAt  Timestamp       `json:"created_at"`
	UpdatedAt  Timestamp       `json:"updated_at"`
}

// NewEntity 创建新实体
func NewEntity(entityType, name string, properties json.RawMessage, confidence float32) *Entity {
	now := time.Now()
	return &Entity{
		Type:       entityType,
		Name:       name,
		Properties: properties,
		Confidence: confidence,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// EntityToJSON 序列化实体
func EntityToJSON(e *Entity) ([]byte, error) {
	return json.Marshal(e)
}

// EntityFromJSON 反序列化实体
func EntityFromJSON(data []byte) (*Entity, error) {
	var e Entity
	err := json.Unmarshal(data, &e)
	return &e, err
}

// Relation 知识关系
type Relation struct {
	ID         UUID            `json:"id"`
	Type       string          `json:"type"`
	SourceID   UUID            `json:"source_id"`
	TargetID   UUID            `json:"target_id"`
	Properties json.RawMessage `json:"properties"`
	Weight     float32         `json:"weight"`
	CreatedAt  Timestamp       `json:"created_at"`
}

// NewRelation 创建新关系
func NewRelation(relType string, sourceID, targetID UUID, properties json.RawMessage, weight float32) *Relation {
	return &Relation{
		Type:       relType,
		SourceID:   sourceID,
		TargetID:   targetID,
		Properties: properties,
		Weight:     weight,
		CreatedAt:  time.Now(),
	}
}

// RelationToJSON 序列化关系
func RelationToJSON(r *Relation) ([]byte, error) {
	return json.Marshal(r)
}

// RelationFromJSON 反序列化关系
func RelationFromJSON(data []byte) (*Relation, error) {
	var r Relation
	err := json.Unmarshal(data, &r)
	return &r, err
}

// DataLineage 数据血缘
type DataLineage struct {
	DataID          UUID              `json:"data_id"`
	SourceType      SourceType        `json:"source_type"`
	SourceIDs       []UUID            `json:"source_ids"`
	Transformations []Transformation  `json:"transformations"`
	AgentDecisions  []UUID            `json:"agent_decisions,omitempty"`
	CreatedAt       Timestamp         `json:"created_at"`
}

// LineageToJSON 序列化血缘
func LineageToJSON(l *DataLineage) ([]byte, error) {
	return json.Marshal(l)
}

// LineageFromJSON 反序列化血缘
func LineageFromJSON(data []byte) (*DataLineage, error) {
	var l DataLineage
	err := json.Unmarshal(data, &l)
	return &l, err
}
