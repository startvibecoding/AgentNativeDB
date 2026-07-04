package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// GraphStore 图存储引擎
type GraphStore struct {
	engine storage.Engine
}

// NewGraphStore 创建图存储
func NewGraphStore(engine storage.Engine) *GraphStore {
	return &GraphStore{engine: engine}
}

// AddNode 添加节点
func (g *GraphStore) AddNode(ctx context.Context, node *Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID is required")
	}

	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node: %w", err)
	}

	// 主键存储
	key := storage.EncodeKey(storage.PrefixGraph, node.ID)
	if err := g.engine.Set(ctx, key, data); err != nil {
		return fmt.Errorf("store node: %w", err)
	}

	// 类型倒排索引
	typeIdx := storage.EncodeIndexKey(storage.PrefixGraph, "type:"+node.Type, node.ID)
	if err := g.engine.Set(ctx, typeIdx, []byte{1}); err != nil {
		return fmt.Errorf("index node type: %w", err)
	}

	// 名称倒排索引
	nameIdx := storage.EncodeIndexKey(storage.PrefixGraph, "name:"+node.Name, node.ID)
	if err := g.engine.Set(ctx, nameIdx, []byte{1}); err != nil {
		return fmt.Errorf("index node name: %w", err)
	}

	return nil
}

// GetNode 获取节点
func (g *GraphStore) GetNode(ctx context.Context, id string) (*Node, error) {
	key := storage.EncodeKey(storage.PrefixGraph, id)
	data, err := g.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("node not found: %s", id)
	}

	var node Node
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("unmarshal node: %w", err)
	}
	return &node, nil
}

// DeleteNode 删除节点及其所有关联边
func (g *GraphStore) DeleteNode(ctx context.Context, id string) error {
	node, err := g.GetNode(ctx, id)
	if err != nil {
		return err
	}

	// 删除所有关联边
	edges, err := g.GetEdges(ctx, id)
	if err == nil {
		for _, edge := range edges {
			g.DeleteEdge(ctx, edge.ID)
		}
	}

	// 删除节点
	key := storage.EncodeKey(storage.PrefixGraph, id)
	typeIdx := storage.EncodeIndexKey(storage.PrefixGraph, "type:"+node.Type, node.ID)
	nameIdx := storage.EncodeIndexKey(storage.PrefixGraph, "name:"+node.Name, node.ID)

	g.engine.Delete(ctx, key)
	g.engine.Delete(ctx, typeIdx)
	g.engine.Delete(ctx, nameIdx)

	return nil
}

// AddEdge 添加边
func (g *GraphStore) AddEdge(ctx context.Context, edge *Edge) error {
	if edge.ID == "" {
		return fmt.Errorf("edge ID is required")
	}
	if edge.FromID == "" || edge.ToID == "" {
		return fmt.Errorf("from and to node IDs are required")
	}

	data, err := json.Marshal(edge)
	if err != nil {
		return fmt.Errorf("marshal edge: %w", err)
	}

	// 主键存储
	key := storage.EncodeKey(storage.PrefixGraph, "edge:"+edge.ID)
	if err := g.engine.Set(ctx, key, data); err != nil {
		return fmt.Errorf("store edge: %w", err)
	}

	// 出边索引: fromID -> edgeID
	outIdx := storage.EncodeIndexKey(storage.PrefixGraph, "out:"+edge.FromID, edge.ID)
	if err := g.engine.Set(ctx, outIdx, []byte{1}); err != nil {
		return fmt.Errorf("index out-edge: %w", err)
	}

	// 入边索引: toID -> edgeID
	inIdx := storage.EncodeIndexKey(storage.PrefixGraph, "in:"+edge.ToID, edge.ID)
	if err := g.engine.Set(ctx, inIdx, []byte{1}); err != nil {
		return fmt.Errorf("index in-edge: %w", err)
	}

	// 边类型索引
	typeIdx := storage.EncodeIndexKey(storage.PrefixGraph, "etype:"+edge.Type, edge.ID)
	if err := g.engine.Set(ctx, typeIdx, []byte{1}); err != nil {
		return fmt.Errorf("index edge type: %w", err)
	}

	return nil
}

// GetEdge 获取边
func (g *GraphStore) GetEdge(ctx context.Context, edgeID string) (*Edge, error) {
	key := storage.EncodeKey(storage.PrefixGraph, "edge:"+edgeID)
	data, err := g.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("edge not found: %s", edgeID)
	}

	var edge Edge
	if err := json.Unmarshal(data, &edge); err != nil {
		return nil, fmt.Errorf("unmarshal edge: %w", err)
	}
	return &edge, nil
}

// DeleteEdge 删除边
func (g *GraphStore) DeleteEdge(ctx context.Context, edgeID string) error {
	edge, err := g.GetEdge(ctx, edgeID)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixGraph, "edge:"+edgeID)
	outIdx := storage.EncodeIndexKey(storage.PrefixGraph, "out:"+edge.FromID, edge.ID)
	inIdx := storage.EncodeIndexKey(storage.PrefixGraph, "in:"+edge.ToID, edge.ID)
	typeIdx := storage.EncodeIndexKey(storage.PrefixGraph, "etype:"+edge.Type, edge.ID)

	g.engine.Delete(ctx, key)
	g.engine.Delete(ctx, outIdx)
	g.engine.Delete(ctx, inIdx)
	g.engine.Delete(ctx, typeIdx)

	return nil
}

// GetEdges 获取节点的所有关联边
func (g *GraphStore) GetEdges(ctx context.Context, nodeID string) ([]*Edge, error) {
	var edges []*Edge

	// 出边
	outEdges, _ := g.getEdgesByIndex(ctx, "out:"+nodeID)
	edges = append(edges, outEdges...)

	// 入边
	inEdges, _ := g.getEdgesByIndex(ctx, "in:"+nodeID)
	edges = append(edges, inEdges...)

	return edges, nil
}

// GetOutEdges 获取节点的出边
func (g *GraphStore) GetOutEdges(ctx context.Context, nodeID string) ([]*Edge, error) {
	return g.getEdgesByIndex(ctx, "out:"+nodeID)
}

// GetInEdges 获取节点的入边
func (g *GraphStore) GetInEdges(ctx context.Context, nodeID string) ([]*Edge, error) {
	return g.getEdgesByIndex(ctx, "in:"+nodeID)
}

// GetNeighbors 获取节点的邻居节点
func (g *GraphStore) GetNeighbors(ctx context.Context, nodeID string, direction Direction) ([]*Node, error) {
	edges, err := g.GetEdges(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var neighbors []*Node

	for _, edge := range edges {
		var neighborID string
		switch direction {
		case DirOut:
			if edge.FromID != nodeID {
				continue
			}
			neighborID = edge.ToID
		case DirIn:
			if edge.ToID != nodeID {
				continue
			}
			neighborID = edge.FromID
		case DirBoth:
			if edge.FromID == nodeID {
				neighborID = edge.ToID
			} else {
				neighborID = edge.FromID
			}
		}

		if seen[neighborID] {
			continue
		}
		seen[neighborID] = true

		neighbor, err := g.GetNode(ctx, neighborID)
		if err != nil {
			continue
		}
		neighbors = append(neighbors, neighbor)
	}

	return neighbors, nil
}

// KHopNeighbors K 跳邻居查询
func (g *GraphStore) KHopNeighbors(ctx context.Context, nodeID string, k int, direction Direction) ([]*Node, error) {
	if k <= 0 {
		return nil, nil
	}

	visited := make(map[string]bool)
	visited[nodeID] = true
	currentLevel := []string{nodeID}

	for hop := 0; hop < k; hop++ {
		var nextLevel []string
		for _, id := range currentLevel {
			edges, _ := g.GetEdges(ctx, id)
			for _, edge := range edges {
				var neighborID string
				switch direction {
				case DirOut:
					if edge.FromID != id {
						continue
					}
					neighborID = edge.ToID
				case DirIn:
					if edge.ToID != id {
						continue
					}
					neighborID = edge.FromID
				case DirBoth:
					if edge.FromID == id {
						neighborID = edge.ToID
					} else {
						neighborID = edge.FromID
					}
				}
				if !visited[neighborID] {
					visited[neighborID] = true
					nextLevel = append(nextLevel, neighborID)
				}
			}
		}
		currentLevel = nextLevel
	}

	// 收集结果
	sort.Strings(currentLevel)
	var result []*Node
	for _, id := range currentLevel {
		node, err := g.GetNode(ctx, id)
		if err == nil {
			result = append(result, node)
		}
	}
	return result, nil
}

// ShortestPath 最短路径（BFS）
func (g *GraphStore) ShortestPath(ctx context.Context, fromID, toID string, direction Direction) ([]string, error) {
	if fromID == toID {
		return []string{fromID}, nil
	}

	visited := make(map[string]bool)
	parent := make(map[string]string)
	queue := []string{fromID}
	visited[fromID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		edges, _ := g.GetEdges(ctx, current)
		for _, edge := range edges {
			var neighborID string
			switch direction {
			case DirOut:
				if edge.FromID != current {
					continue
				}
				neighborID = edge.ToID
			case DirIn:
				if edge.ToID != current {
					continue
				}
				neighborID = edge.FromID
			case DirBoth:
				if edge.FromID == current {
					neighborID = edge.ToID
				} else {
					neighborID = edge.FromID
				}
			}

			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true
			parent[neighborID] = current

			if neighborID == toID {
				// 回溯路径
				path := []string{toID}
				for p := parent[toID]; p != ""; p = parent[p] {
					path = append([]string{p}, path...)
				}
				return path, nil
			}

			queue = append(queue, neighborID)
		}
	}

	return nil, fmt.Errorf("no path from %s to %s", fromID, toID)
}

// FindNodesByType 按类型查找节点
func (g *GraphStore) FindNodesByType(ctx context.Context, nodeType string, limit int) ([]*Node, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixGraph, "type:"+nodeType, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := g.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var nodes []*Node
	for iter.Next() {
		key, _ := iter.Item()
		nodeID := storage.DecodeIndexID(key)
		node, err := g.GetNode(ctx, nodeID)
		if err == nil {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

// FindNodesByName 按名称查找节点
func (g *GraphStore) FindNodesByName(ctx context.Context, name string, limit int) ([]*Node, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixGraph, "name:"+name, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := g.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var nodes []*Node
	for iter.Next() {
		key, _ := iter.Item()
		nodeID := storage.DecodeIndexID(key)
		node, err := g.GetNode(ctx, nodeID)
		if err == nil {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

// FindEdgesByType 按类型查找边
func (g *GraphStore) FindEdgesByType(ctx context.Context, edgeType string, limit int) ([]*Edge, error) {
	return g.getEdgesByIndex(ctx, "etype:"+edgeType)
}

// getEdgesByIndex 通过索引获取边列表
func (g *GraphStore) getEdgesByIndex(ctx context.Context, indexKey string) ([]*Edge, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixGraph, indexKey, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := g.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var edges []*Edge
	for iter.Next() {
		key, _ := iter.Item()
		edgeID := storage.DecodeIndexID(key)
		edge, err := g.GetEdge(ctx, edgeID)
		if err == nil {
			edges = append(edges, edge)
		}
	}
	return edges, nil
}

// Node 图节点
type Node struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Edge 图边
type Edge struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	FromID     string         `json:"from_id"`
	ToID       string         `json:"to_id"`
	Properties map[string]any `json:"properties,omitempty"`
	Weight     float64        `json:"weight,omitempty"`
}

// Direction 遍历方向
type Direction int

const (
	DirOut  Direction = iota // 出边
	DirIn                    // 入边
	DirBoth                  // 双向
)
