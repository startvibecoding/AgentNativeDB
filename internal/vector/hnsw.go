package vector

import (
	"math"
	"math/rand"
	"sort"
	"sync"
)

// HNSWIndex HNSW 向量索引
type HNSWIndex struct {
	mu sync.RWMutex

	// 参数
	dim      int          // 向量维度
	m        int          // 每层最大连接数
	mMax     int          // 第 0 层最大连接数（= 2*M）
	efSearch int          // 搜索时的 beam width
	efConst  int          // 构建时的 beam width
	ml       float64      // 层级生成因子 (1/ln(M))

	distFn   DistanceFunc // 距离函数

	// 数据
	nodes    map[string]*hnswNode // id -> node
	entryID  string               // 入口节点 ID
	entryLevel int                // 入口节点层级
}

// hnswNode HNSW 节点
type hnswNode struct {
	id          string
	vector      []float32
	level       int
	connections []map[string]struct{} // connections[layer] = set of neighbor IDs
}

// HNSWConfig HNSW 配置
type HNSWConfig struct {
	Dim          int          // 向量维度（必须）
	M            int          // 最大连接数（默认 16）
	EfConstruction int        // 构建 beam width（默认 200）
	EfSearch     int          // 搜索 beam width（默认 64）
	DistFn       DistanceFunc // 距离函数（默认 cosine）
}

// DefaultHNSWConfig 默认配置
func DefaultHNSWConfig(dim int) HNSWConfig {
	return HNSWConfig{
		Dim:            dim,
		M:              16,
		EfConstruction: 200,
		EfSearch:       64,
		DistFn:         CosineDistance,
	}
}

// NewHNSWIndex 创建 HNSW 索引
func NewHNSWIndex(cfg HNSWConfig) *HNSWIndex {
	if cfg.M <= 0 {
		cfg.M = 16
	}
	if cfg.EfConstruction <= 0 {
		cfg.EfConstruction = 200
	}
	if cfg.EfSearch <= 0 {
		cfg.EfSearch = 64
	}
	if cfg.DistFn == nil {
		cfg.DistFn = CosineDistance
	}

	return &HNSWIndex{
		dim:      cfg.Dim,
		m:        cfg.M,
		mMax:     cfg.M * 2,
		efSearch: cfg.EfSearch,
		efConst:  cfg.EfConstruction,
		ml:       1.0 / math.Log(float64(cfg.M)),
		distFn:   cfg.DistFn,
		nodes:    make(map[string]*hnswNode),
	}
}

// Insert 插入向量
func (idx *HNSWIndex) Insert(id string, vector []float32) {
	if len(vector) != idx.dim {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 如果已存在则更新向量并重建局部连接
	if existing, ok := idx.nodes[id]; ok {
		existing.vector = vector
		// 重建该节点的所有连接
		for l := 0; l < len(existing.connections); l++ {
			// 从邻居的连接中移除自己
			for nbID := range existing.connections[l] {
				if nbNode, ok := idx.nodes[nbID]; ok && l < len(nbNode.connections) {
					delete(nbNode.connections[l], id)
				}
			}
			existing.connections[l] = make(map[string]struct{})
		}
		idx.rebuildConnections(id, existing.level)
		return
	}

	// 生成随机层级
	level := idx.randomLevel()

	node := &hnswNode{
		id:          id,
		vector:      vector,
		level:       level,
		connections: make([]map[string]struct{}, level+1),
	}
	for i := 0; i <= level; i++ {
		node.connections[i] = make(map[string]struct{})
	}

	idx.nodes[id] = node

	// 如果是第一个节点
	if idx.entryID == "" {
		idx.entryID = id
		idx.entryLevel = level
		return
	}

	// 搜索最近邻
	entryNode := idx.nodes[idx.entryID]
	curDist := idx.distFn(vector, entryNode.vector)
	entry := entryCandidate{id: idx.entryID, dist: curDist}

	// 从顶层向下搜索
	for l := idx.entryLevel; l > level; l-- {
		nearest := idx.searchLayer(entry, vector, 1, l)
		if len(nearest) > 0 {
			entry = nearest[0]
		}
	}

	// 从 level 层到第 0 层，插入并建立连接
	for l := min(level, idx.entryLevel); l >= 0; l-- {
		candidates := idx.searchLayer(entry, vector, idx.efConst, l)

		// 选择 M 个最近邻
		m := idx.m
		if l == 0 {
			m = idx.mMax
		}
		neighbors := idx.selectNeighbors(candidates, m)

		// 建立双向连接
		for _, nb := range neighbors {
			idx.addConnection(id, nb.id, l)
			idx.addConnection(nb.id, id, l)

			// 如果邻居的连接数超过限制，裁剪
			neighborNode := idx.nodes[nb.id]
			if neighborNode != nil && len(neighborNode.connections[l]) > idx.mMax {
				idx.pruneConnections(nb.id, l)
			}
		}

		// 更新入口为当前层最近的候选
		if len(candidates) > 0 {
			entry = candidates[0]
		}
	}

	// 更新入口
	if level > idx.entryLevel {
		idx.entryID = id
		idx.entryLevel = level
	}
}

// Search 搜索最近的 k 个向量
func (idx *HNSWIndex) Search(query []float32, k int) []SearchResult {
	if len(query) != idx.dim || k <= 0 || len(idx.nodes) == 0 {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entryNode := idx.nodes[idx.entryID]
	curDist := idx.distFn(query, entryNode.vector)
	entry := entryCandidate{id: idx.entryID, dist: curDist}

	// 从顶层向下搜索
	for l := idx.entryLevel; l > 0; l-- {
		nearest := idx.searchLayer(entry, query, 1, l)
		if len(nearest) > 0 {
			entry = nearest[0]
		}
	}

	// 在第 0 层搜索 efSearch 个候选
	candidates := idx.searchLayer(entry, query, max(idx.efSearch, k), 0)

	// 返回 top-k
	if len(candidates) > k {
		candidates = candidates[:k]
	}

	results := make([]SearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = SearchResult{ID: c.id, Distance: c.dist}
	}

	return results
}

// Delete 删除向量
func (idx *HNSWIndex) Delete(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	node, ok := idx.nodes[id]
	if !ok {
		return
	}

	// 从所有邻居的连接中移除
	for l := 0; l <= node.level; l++ {
		for nbID := range node.connections[l] {
			if nbNode, ok := idx.nodes[nbID]; ok && l < len(nbNode.connections) {
				delete(nbNode.connections[l], id)
			}
		}
	}

	delete(idx.nodes, id)

	// 如果删除的是入口，重新选择
	if idx.entryID == id {
		idx.entryID = ""
		idx.entryLevel = 0
		for nid, n := range idx.nodes {
			if idx.entryID == "" || n.level > idx.entryLevel {
				idx.entryID = nid
				idx.entryLevel = n.level
			}
		}
	}
}

// Len 返回索引中的向量数量
func (idx *HNSWIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// Contains 检查向量是否存在
func (idx *HNSWIndex) Contains(id string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	_, ok := idx.nodes[id]
	return ok
}

// GetVector 获取向量（用于持久化）
func (idx *HNSWIndex) GetVector(id string) []float32 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if node, ok := idx.nodes[id]; ok {
		return node.vector
	}
	return nil
}

// GetAllIDs 获取所有 ID
func (idx *HNSWIndex) GetAllIDs() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	ids := make([]string, 0, len(idx.nodes))
	for id := range idx.nodes {
		ids = append(ids, id)
	}
	return ids
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string
	Distance float32
}

// entryCandidate 候选节点
type entryCandidate struct {
	id   string
	dist float32
}

// searchLayer 在指定层搜索最近邻
func (idx *HNSWIndex) searchLayer(entry entryCandidate, query []float32, ef int, layer int) []entryCandidate {
	visited := make(map[string]bool)
	visited[entry.id] = true

	candidates := []entryCandidate{entry} // min-heap (by distance)
	dynamic := []entryCandidate{entry}    // dynamic list of results

	for len(candidates) > 0 {
		// 取距离最小的候选
		c := candidates[0]
		candidates = candidates[1:]

		// 获取当前最远距离的动态列表元素
		farthest := dynamic[len(dynamic)-1].dist

		if c.dist > farthest && len(dynamic) >= ef {
			break
		}

		// 遍历当前节点在该层的邻居
		node := idx.nodes[c.id]
		if node == nil || layer >= len(node.connections) {
			continue
		}

		for nbID := range node.connections[layer] {
			if visited[nbID] {
				continue
			}
			visited[nbID] = true

			nbNode := idx.nodes[nbID]
			if nbNode == nil {
				continue
			}

			dist := idx.distFn(query, nbNode.vector)

			if len(dynamic) < ef || dist < dynamic[len(dynamic)-1].dist {
				// 插入候选
				candidates = insertSorted(candidates, entryCandidate{id: nbID, dist: dist})

				// 插入动态列表
				dynamic = insertSorted(dynamic, entryCandidate{id: nbID, dist: dist})
				if len(dynamic) > ef {
					dynamic = dynamic[:ef]
				}
			}
		}
	}

	return dynamic
}

// selectNeighbors 选择最近的 m 个邻居（简单截断策略）
func (idx *HNSWIndex) selectNeighbors(candidates []entryCandidate, m int) []entryCandidate {
	if len(candidates) <= m {
		return candidates
	}
	return candidates[:m]
}

// addConnection 添加连接
func (idx *HNSWIndex) addConnection(from, to string, layer int) {
	node := idx.nodes[from]
	if node != nil && layer < len(node.connections) {
		node.connections[layer][to] = struct{}{}
	}
}

// pruneConnections 裁剪连接到 mMax
func (idx *HNSWIndex) pruneConnections(nodeID string, layer int) {
	node := idx.nodes[nodeID]
	if node == nil || layer >= len(node.connections) {
		return
	}

	if len(node.connections[layer]) <= idx.mMax {
		return
	}

	// 收集所有邻居及其距离
	query := node.vector
	var candidates []entryCandidate
	for nbID := range node.connections[layer] {
		nbNode := idx.nodes[nbID]
		if nbNode != nil {
			dist := idx.distFn(query, nbNode.vector)
			candidates = append(candidates, entryCandidate{id: nbID, dist: dist})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})

	// 保留最近的 mMax 个
	newConns := make(map[string]struct{}, idx.mMax)
	for i := 0; i < min(idx.mMax, len(candidates)); i++ {
		newConns[candidates[i].id] = struct{}{}
	}

	node.connections[layer] = newConns
}

// randomLevel 随机生成层级（使用 HNSW 标准公式）
func (idx *HNSWIndex) randomLevel() int {
	level := 0
	for rand.Float64() < 1.0/float64(idx.m) && level < 16 {
		level++
	}
	return level
}

// rebuildConnections 重建节点的连接
func (idx *HNSWIndex) rebuildConnections(nodeID string, level int) {
	node := idx.nodes[nodeID]
	if node == nil || idx.entryID == "" || idx.entryID == nodeID {
		return
	}

	entryNode := idx.nodes[idx.entryID]
	if entryNode == nil {
		return
	}

	curDist := idx.distFn(node.vector, entryNode.vector)
	entry := entryCandidate{id: idx.entryID, dist: curDist}

	for l := min(level, idx.entryLevel); l >= 0; l-- {
		candidates := idx.searchLayer(entry, node.vector, idx.efConst, l)

		m := idx.m
		if l == 0 {
			m = idx.mMax
		}
		neighbors := idx.selectNeighbors(candidates, m)

		for _, nb := range neighbors {
			idx.addConnection(nodeID, nb.id, l)
			idx.addConnection(nb.id, nodeID, l)
		}

		if len(candidates) > 0 {
			entry = candidates[0]
		}
	}
}

// insertSorted 有序插入（按距离升序）
func insertSorted(list []entryCandidate, item entryCandidate) []entryCandidate {
	pos := sort.Search(len(list), func(i int) bool {
		return list[i].dist > item.dist
	})
	list = append(list, entryCandidate{})
	copy(list[pos+1:], list[pos:])
	list[pos] = item
	return list
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
