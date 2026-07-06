package vector

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// VectorStore 向量存储（基于 BadgerDB + HNSW 索引）
type VectorStore struct {
	engine  storage.Engine
	indexes map[string]*HNSWIndex // indexName -> HNSW index
	dims    map[string]int        // indexName -> dimension
}

// NewVectorStore 创建向量存储
func NewVectorStore(engine storage.Engine) *VectorStore {
	return &VectorStore{
		engine:  engine,
		indexes: make(map[string]*HNSWIndex),
		dims:    make(map[string]int),
	}
}

// CreateIndex 创建向量索引
func (vs *VectorStore) CreateIndex(name string, dim int, metric string) error {
	if _, exists := vs.indexes[name]; exists {
		return fmt.Errorf("index %s already exists", name)
	}

	distFn := GetDistanceFunc(metric)
	idx := NewHNSWIndex(HNSWConfig{
		Dim:            dim,
		M:              16,
		EfConstruction: 200,
		EfSearch:       64,
		DistFn:         distFn,
	})

	vs.indexes[name] = idx
	vs.dims[name] = dim

	// 从存储中加载已有向量
	vs.loadFromStorage(name)

	return nil
}

// Insert 向量插入(兼容旧签名,不带 payload)
func (vs *VectorStore) Insert(indexName, id string, vector []float32) error {
	return vs.InsertWithPayload(indexName, id, vector, nil)
}

// InsertWithPayload 向量插入(可携带 payload / metadata)
func (vs *VectorStore) InsertWithPayload(indexName, id string, vector []float32, payload []byte) error {
	idx, ok := vs.indexes[indexName]
	if !ok {
		return fmt.Errorf("index %s not found", indexName)
	}
	if dim := vs.dims[indexName]; len(vector) != dim {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", dim, len(vector))
	}

	// 持久化向量到 BadgerDB
	vecKey := EncodeVectorKey(indexName, id)
	vecData := Float32sToBytes(vector)
	if err := vs.engine.Set(context.Background(), vecKey, vecData); err != nil {
		return fmt.Errorf("persist vector: %w", err)
	}

	// 持久化 payload(如有)
	if len(payload) > 0 {
		payloadKey := storage.EncodeVectorPayloadKey(indexName, id)
		if err := vs.engine.Set(context.Background(), payloadKey, payload); err != nil {
			return fmt.Errorf("persist payload: %w", err)
		}
	}

	// 插入 HNSW 索引
	idx.Insert(id, vector)
	return nil
}

// GetPayload 获取向量的 payload
func (vs *VectorStore) GetPayload(indexName, id string) ([]byte, error) {
	payloadKey := storage.EncodeVectorPayloadKey(indexName, id)
	return vs.engine.Get(context.Background(), payloadKey)
}

// Search 向量搜索
func (vs *VectorStore) Search(indexName string, query []float32, topK int) ([]SearchResult, error) {
	idx, ok := vs.indexes[indexName]
	if !ok {
		return nil, fmt.Errorf("index %s not found", indexName)
	}
	if dim := vs.dims[indexName]; len(query) != dim {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", dim, len(query))
	}

	results := idx.Search(query, topK)
	return results, nil
}

// Delete 向量删除
func (vs *VectorStore) Delete(indexName, id string) error {
	idx, ok := vs.indexes[indexName]
	if !ok {
		return fmt.Errorf("index %s not found", indexName)
	}

	// 从存储中删除
	vecKey := EncodeVectorKey(indexName, id)
	payloadKey := storage.EncodeVectorPayloadKey(indexName, id)
	ops := []storage.WriteOp{
		{Type: storage.OpDelete, Key: vecKey},
		{Type: storage.OpDelete, Key: payloadKey},
	}
	if err := vs.engine.BatchWrite(context.Background(), ops); err != nil {
		return fmt.Errorf("delete vector: %w", err)
	}

	// 从 HNSW 中删除
	idx.Delete(id)
	return nil
}

// Len 索引中的向量数量
func (vs *VectorStore) Len(indexName string) int {
	if idx, ok := vs.indexes[indexName]; ok {
		return idx.Len()
	}
	return 0
}

// loadFromStorage 从 BadgerDB 加载向量到 HNSW 索引
func (vs *VectorStore) loadFromStorage(indexName string) {
	idx := vs.indexes[indexName]
	if idx == nil {
		return
	}

	prefix := EncodeVectorKey(indexName, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := vs.engine.Scan(context.Background(), start, end, storage.ScanOptions{})
	if err != nil {
		return
	}
	defer iter.Close()

	for iter.Next() {
		key, val := iter.Item()
		// 从 key 中提取 ID
		id := extractVectorID(key, indexName)
		if id == "" {
			continue
		}

		vector := BytesToFloat32s(val)
		if len(vector) > 0 {
			idx.Insert(id, vector)
		}
	}
}

// extractVectorID 从向量 key 中提取 ID
func extractVectorID(key []byte, indexName string) string {
	// key 格式: [0x10][indexName][0x00][id]
	prefixLen := 1 + len(indexName) + 1
	if len(key) <= prefixLen {
		return ""
	}
	return string(key[prefixLen:])
}

// EncodeVectorKey 编码向量 key
func EncodeVectorKey(indexName, id string) []byte {
	return storage.EncodeIndexKey(storage.PrefixVector, indexName, id)
}

// Float32sToBytes float32 转字节
func Float32sToBytes(floats []float32) []byte {
	bytes := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
	}
	return bytes
}

// BytesToFloat32s 字节转 float32
func BytesToFloat32s(data []byte) []float32 {
	n := len(data) / 4
	floats := make([]float32, n)
	for i := 0; i < n; i++ {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return floats
}

// VectorSearchResult 向量搜索结果（可携带向量数据 / payload）
type VectorSearchResult struct {
	ID       string
	Distance float32
	Vector   []float32
	Payload  []byte
}

// SearchWithVectors 搜索并返回向量数据,withPayload 控制是否回查 payload
func (vs *VectorStore) SearchWithVectors(indexName string, query []float32, topK int, withPayload bool) ([]VectorSearchResult, error) {
	results, err := vs.Search(indexName, query, topK)
	if err != nil {
		return nil, err
	}

	var enriched []VectorSearchResult
	for _, r := range results {
		item := VectorSearchResult{
			ID:       r.ID,
			Distance: r.Distance,
		}

		vecKey := EncodeVectorKey(indexName, r.ID)
		if vecData, err := vs.engine.Get(context.Background(), vecKey); err == nil {
			item.Vector = BytesToFloat32s(vecData)
		}

		if withPayload {
			payloadKey := storage.EncodeVectorPayloadKey(indexName, r.ID)
			if payload, err := vs.engine.Get(context.Background(), payloadKey); err == nil {
				item.Payload = payload
			}
		}

		enriched = append(enriched, item)
	}

	return enriched, nil
}

// SearchWithPayloads 搜索并返回 payload
func (vs *VectorStore) SearchWithPayloads(indexName string, query []float32, topK int) ([]VectorSearchResult, error) {
	return vs.SearchWithVectors(indexName, query, topK, true)
}

// ListIndexes 列出所有索引
func (vs *VectorStore) ListIndexes() []string {
	var names []string
	for name := range vs.indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HasIndex 判断索引是否存在
func (vs *VectorStore) HasIndex(name string) bool {
	_, ok := vs.indexes[name]
	return ok
}

// Dim 获取索引维度, 不存在时返回 0
func (vs *VectorStore) Dim(name string) int {
	return vs.dims[name]
}

// ParseVectorLiteral 解析向量字面量 "[0.1, 0.2, 0.3]"
func ParseVectorLiteral(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("invalid vector literal: %s", s)
	}

	s = s[1 : len(s)-1]
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	vec := make([]float32, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		var f float64
		if _, err := fmt.Sscanf(p, "%f", &f); err != nil {
			return nil, fmt.Errorf("invalid float at position %d: %s", i, p)
		}
		vec[i] = float32(f)
	}
	return vec, nil
}
