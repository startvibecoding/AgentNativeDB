package agent

import (
	"context"
	"fmt"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// MemoryStore 管理 Agent 记忆
type MemoryStore struct {
	engine storage.Engine
	cache  *storage.Cache
}

// NewMemoryStore 创建记忆存储
func NewMemoryStore(engine storage.Engine, cache *storage.Cache) *MemoryStore {
	return &MemoryStore{
		engine: engine,
		cache:  cache,
	}
}

// Store 存储记忆
func (s *MemoryStore) Store(ctx context.Context, m *model.MemoryEntry) (*model.MemoryEntry, error) {
	if m.ID == "" {
		m.ID = util.NewUUID()
	}

	// 存储记忆数据
	data, err := model.MemoryToJSON(m)
	if err != nil {
		return nil, fmt.Errorf("marshal memory: %w", err)
	}

	key := storage.EncodeKey(storage.PrefixMemory, m.ID)
	if err := s.engine.Set(ctx, key, data); err != nil {
		return nil, fmt.Errorf("store memory: %w", err)
	}

	// 存储 embedding（如果有）
	if len(m.Embedding) > 0 {
		vecKey := storage.EncodeVectorKey("memory_embedding", m.ID)
		vecData := storage.Float32sToBytes(m.Embedding)
		if err := s.engine.Set(ctx, vecKey, vecData); err != nil {
			return nil, fmt.Errorf("store embedding: %w", err)
		}
	}

	// 写入 session_id 索引
	idxKey := storage.EncodeIndexKey(storage.PrefixMemory, m.SessionID, m.ID)
	if err := s.engine.Set(ctx, idxKey, []byte{1}); err != nil {
		return nil, fmt.Errorf("index memory: %w", err)
	}

	// 写入类型索引
	typeIdxKey := storage.EncodeIndexKey(storage.PrefixMemory, string(m.Type), m.ID)
	if err := s.engine.Set(ctx, typeIdxKey, []byte{1}); err != nil {
		return nil, fmt.Errorf("index memory type: %w", err)
	}

	// 缓存
	s.cache.Set(key, data)

	return m, nil
}

// Get 获取记忆（同时更新访问时间）
func (s *MemoryStore) Get(ctx context.Context, id string) (*model.MemoryEntry, error) {
	key := storage.EncodeKey(storage.PrefixMemory, id)

	// 先查缓存
	if data, ok := s.cache.Get(key); ok {
		m, err := model.MemoryFromJSON(data)
		if err == nil {
			s.touchMemory(ctx, m)
		}
		return m, err
	}

	data, err := s.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("memory not found: %s", id)
	}

	s.cache.Set(key, data)
	m, err := model.MemoryFromJSON(data)
	if err == nil {
		s.touchMemory(ctx, m)
	}
	return m, err
}

// touchMemory 更新记忆的访问时间和计数
func (s *MemoryStore) touchMemory(ctx context.Context, m *model.MemoryEntry) {
	m.AccessCount++
	m.AccessedAt = Timestamp_now()
	data, err := model.MemoryToJSON(m)
	if err != nil {
		return
	}
	key := storage.EncodeKey(storage.PrefixMemory, m.ID)
	s.engine.Set(ctx, key, data)
	s.cache.Set(key, data)
}

// GetWithEmbedding 获取带 embedding 的记忆
func (s *MemoryStore) GetWithEmbedding(ctx context.Context, id string) (*model.MemoryEntry, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// 获取 embedding
	vecKey := storage.EncodeVectorKey("memory_embedding", id)
	vecData, err := s.engine.Get(ctx, vecKey)
	if err == nil && len(vecData) > 0 {
		m.Embedding = storage.BytesToFloat32s(vecData)
	}

	return m, nil
}

// Delete 删除记忆
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixMemory, id)
	sessionIdx := storage.EncodeIndexKey(storage.PrefixMemory, m.SessionID, m.ID)
	typeIdx := storage.EncodeIndexKey(storage.PrefixMemory, string(m.Type), m.ID)
	vecKey := storage.EncodeVectorKey("memory_embedding", id)

	s.engine.Delete(ctx, key)
	s.engine.Delete(ctx, sessionIdx)
	s.engine.Delete(ctx, typeIdx)
	s.engine.Delete(ctx, vecKey)
	s.cache.Delete(key)

	return nil
}

// ListBySession 按 session_id 列出记忆
func (s *MemoryStore) ListBySession(ctx context.Context, sessionID string, memType model.MemoryType, limit int) ([]*model.MemoryEntry, error) {
	if memType != "" {
		return s.listBySessionAndType(ctx, sessionID, memType, limit)
	}

	prefix := storage.EncodeIndexKey(storage.PrefixMemory, sessionID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := s.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("scan memories: %w", err)
	}
	defer iter.Close()

	var memories []*model.MemoryEntry
	for iter.Next() {
		key, _ := iter.Item()
		memID := storage.DecodeIndexID(key)
		m, err := s.Get(ctx, memID)
		if err != nil {
			continue
		}
		memories = append(memories, m)
	}

	return memories, nil
}

// listBySessionAndType 按 session_id 和 type 列出记忆
func (s *MemoryStore) listBySessionAndType(ctx context.Context, sessionID string, memType model.MemoryType, limit int) ([]*model.MemoryEntry, error) {
	// 先获取该 session 的所有记忆，再按 type 过滤
	// TODO: 优化为组合索引
	all, err := s.ListBySession(ctx, sessionID, "", 0)
	if err != nil {
		return nil, err
	}

	var filtered []*model.MemoryEntry
	for _, m := range all {
		if m.Type == memType {
			filtered = append(filtered, m)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

// UpdateAccess 更新记忆访问信息
func (s *MemoryStore) UpdateAccess(ctx context.Context, id string) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	m.AccessCount++
	m.AccessedAt = Timestamp_now()

	data, err := model.MemoryToJSON(m)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixMemory, id)
	if err := s.engine.Set(ctx, key, data); err != nil {
		return err
	}

	s.cache.Set(key, data)
	return nil
}

// BatchStore 批量存储记忆
func (s *MemoryStore) BatchStore(ctx context.Context, memories []*model.MemoryEntry) error {
	ops := make([]storage.WriteOp, 0, len(memories)*2)

	for _, m := range memories {
		if m.ID == "" {
			m.ID = util.NewUUID()
		}

		data, err := model.MemoryToJSON(m)
		if err != nil {
			return fmt.Errorf("marshal memory %s: %w", m.ID, err)
		}

		key := storage.EncodeKey(storage.PrefixMemory, m.ID)
		ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: key, Value: data})

		// 索引
		idxKey := storage.EncodeIndexKey(storage.PrefixMemory, m.SessionID, m.ID)
		ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: idxKey, Value: []byte{1}})
	}

	return s.engine.BatchWrite(ctx, ops)
}
