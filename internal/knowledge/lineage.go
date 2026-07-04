package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// LineageTracker 数据血缘追踪器
type LineageTracker struct {
	engine storage.Engine
}

// NewLineageTracker 创建血缘追踪器
func NewLineageTracker(engine storage.Engine) *LineageTracker {
	return &LineageTracker{engine: engine}
}

// Record 记录数据血缘
func (lt *LineageTracker) Record(ctx context.Context, lineage *Lineage) error {
	if lineage.DataID == "" {
		lineage.DataID = util.NewUUID()
	}
	if lineage.CreatedAt.IsZero() {
		lineage.CreatedAt = time.Now()
	}

	data, err := json.Marshal(lineage)
	if err != nil {
		return fmt.Errorf("marshal lineage: %w", err)
	}

	key := storage.EncodeKey(storage.PrefixLineage, lineage.DataID)
	return lt.engine.Set(ctx, key, data)
}

// Get 获取数据血缘
func (lt *LineageTracker) Get(ctx context.Context, dataID string) (*Lineage, error) {
	key := storage.EncodeKey(storage.PrefixLineage, dataID)
	data, err := lt.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("lineage not found: %s", dataID)
	}

	var lineage Lineage
	if err := json.Unmarshal(data, &lineage); err != nil {
		return nil, fmt.Errorf("unmarshal lineage: %w", err)
	}
	return &lineage, nil
}

// TraceUpstream 追溯上游数据来源
func (lt *LineageTracker) TraceUpstream(ctx context.Context, dataID string, depth int) (*LineageTree, error) {
	lineage, err := lt.Get(ctx, dataID)
	if err != nil {
		return nil, err
	}

	tree := &LineageTree{
		Lineage: lineage,
	}

	if depth <= 0 {
		return tree, nil
	}

	for _, srcID := range lineage.SourceIDs {
		child, err := lt.TraceUpstream(ctx, srcID, depth-1)
		if err != nil {
			continue
		}
		tree.Sources = append(tree.Sources, child)
	}

	return tree, nil
}

// ListByDecision 关联决策 ID 查询血缘
func (lt *LineageTracker) ListByDecision(ctx context.Context, decisionID string) ([]*Lineage, error) {
	prefix := []byte{storage.PrefixLineage}
	start, end := storage.PrefixRange(prefix)

	iter, err := lt.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []*Lineage
	for iter.Next() {
		_, val := iter.Item()
		var l Lineage
		if err := json.Unmarshal(val, &l); err != nil {
			continue
		}
		for _, did := range l.DecisionIDs {
			if did == decisionID {
				results = append(results, &l)
				break
			}
		}
	}
	return results, nil
}

// Lineage 数据血缘记录
type Lineage struct {
	DataID      string    `json:"data_id"`
	SourceType  string    `json:"source_type"` // raw, derived, agent_generated
	SourceIDs   []string  `json:"source_ids"`
	Steps       []Step    `json:"steps"`
	DecisionIDs []string  `json:"decision_ids,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Step 数据变换步骤
type Step struct {
	Type       string         `json:"type"`
	Operation  string         `json:"operation"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// LineageTree 血缘树
type LineageTree struct {
	Lineage *Lineage        `json:"lineage"`
	Sources []*LineageTree  `json:"sources,omitempty"`
}

// Depth 计算树深度
func (t *LineageTree) Depth() int {
	if len(t.Sources) == 0 {
		return 1
	}
	maxChild := 0
	for _, child := range t.Sources {
		d := child.Depth()
		if d > maxChild {
			maxChild = d
		}
	}
	return maxChild + 1
}

// AllIDs 返回树中所有数据 ID
func (t *LineageTree) AllIDs() []string {
	ids := []string{t.Lineage.DataID}
	for _, child := range t.Sources {
		ids = append(ids, child.AllIDs()...)
	}
	return ids
}
