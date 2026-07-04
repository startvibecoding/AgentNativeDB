package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// DecisionRecorder 管理 Agent 决策记录
type DecisionRecorder struct {
	engine storage.Engine
	cache  *storage.Cache
}

// NewDecisionRecorder 创建决策记录器
func NewDecisionRecorder(engine storage.Engine, cache *storage.Cache) *DecisionRecorder {
	return &DecisionRecorder{
		engine: engine,
		cache:  cache,
	}
}

// Record 记录决策
func (r *DecisionRecorder) Record(ctx context.Context, d *model.Decision) (*model.Decision, error) {
	if d.ID == "" {
		d.ID = util.NewUUID()
	}

	data, err := model.DecisionToJSON(d)
	if err != nil {
		return nil, fmt.Errorf("marshal decision: %w", err)
	}

	key := storage.EncodeKey(storage.PrefixDecision, d.ID)
	if err := r.engine.Set(ctx, key, data); err != nil {
		return nil, fmt.Errorf("store decision: %w", err)
	}

	// session_id 索引
	idxKey := storage.EncodeIndexKey(storage.PrefixDecision, d.SessionID, d.ID)
	if err := r.engine.Set(ctx, idxKey, []byte{1}); err != nil {
		return nil, fmt.Errorf("index decision: %w", err)
	}

	// parent_id 索引（如果有父决策）
	if d.ParentID != nil {
		parentIdxKey := storage.EncodeIndexKey(storage.PrefixDecision, *d.ParentID, d.ID)
		if err := r.engine.Set(ctx, parentIdxKey, []byte{1}); err != nil {
			return nil, fmt.Errorf("index decision parent: %w", err)
		}
	}

	r.cache.Set(key, data)
	return d, nil
}

// Get 获取决策
func (r *DecisionRecorder) Get(ctx context.Context, id string) (*model.Decision, error) {
	key := storage.EncodeKey(storage.PrefixDecision, id)

	if data, ok := r.cache.Get(key); ok {
		return model.DecisionFromJSON(data)
	}

	data, err := r.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("decision not found: %s", id)
	}

	r.cache.Set(key, data)
	return model.DecisionFromJSON(data)
}

// Delete 删除决策
func (r *DecisionRecorder) Delete(ctx context.Context, id string) error {
	d, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixDecision, id)
	sessionIdx := storage.EncodeIndexKey(storage.PrefixDecision, d.SessionID, d.ID)

	r.engine.Delete(ctx, key)
	r.engine.Delete(ctx, sessionIdx)

	if d.ParentID != nil {
		parentIdx := storage.EncodeIndexKey(storage.PrefixDecision, *d.ParentID, d.ID)
		r.engine.Delete(ctx, parentIdx)
	}

	r.cache.Delete(key)
	return nil
}

// ListBySession 按 session_id 列出决策
func (r *DecisionRecorder) ListBySession(ctx context.Context, sessionID string, limit int) ([]*model.Decision, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixDecision, sessionID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := r.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("scan decisions: %w", err)
	}
	defer iter.Close()

	var decisions []*model.Decision
	for iter.Next() {
		key, _ := iter.Item()
		decID := storage.DecodeIndexID(key)
		d, err := r.Get(ctx, decID)
		if err != nil {
			continue
		}
		decisions = append(decisions, d)
	}

	return decisions, nil
}

// ListByParent 按 parent_id 列出子决策
func (r *DecisionRecorder) ListByParent(ctx context.Context, parentID string, limit int) ([]*model.Decision, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixDecision, parentID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := r.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("scan child decisions: %w", err)
	}
	defer iter.Close()

	var decisions []*model.Decision
	for iter.Next() {
		key, _ := iter.Item()
		decID := storage.DecodeIndexID(key)
		d, err := r.Get(ctx, decID)
		if err != nil {
			continue
		}
		decisions = append(decisions, d)
	}

	return decisions, nil
}

// BuildDecisionTree 构建决策树（从指定决策开始递归获取所有子决策）
func (r *DecisionRecorder) BuildDecisionTree(ctx context.Context, rootID string) (*DecisionTreeNode, error) {
	root, err := r.Get(ctx, rootID)
	if err != nil {
		return nil, err
	}

	node := &DecisionTreeNode{Decision: root}

	children, err := r.ListByParent(ctx, rootID, 0)
	if err != nil {
		return node, nil // 返回部分树
	}

	for _, child := range children {
		childNode, err := r.BuildDecisionTree(ctx, child.ID)
		if err != nil {
			continue
		}
		node.Children = append(node.Children, childNode)
	}

	return node, nil
}

// DecisionTreeNode 决策树节点
type DecisionTreeNode struct {
	Decision *model.Decision      `json:"decision"`
	Children []*DecisionTreeNode  `json:"children,omitempty"`
}

// TotalDuration 计算决策树总耗时
func (n *DecisionTreeNode) TotalDuration() uint64 {
	total := n.Decision.DurationMs
	for _, child := range n.Children {
		total += child.TotalDuration()
	}
	return total
}

// ToJSON 序列化决策树
func (n *DecisionTreeNode) ToJSON() ([]byte, error) {
	return json.MarshalIndent(n, "", "  ")
}
