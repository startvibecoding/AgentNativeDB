package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// SessionManager 管理 Agent 会话
type SessionManager struct {
	engine storage.Engine
	cache  *storage.Cache
}

// NewSessionManager 创建会话管理器
func NewSessionManager(engine storage.Engine, cache *storage.Cache) *SessionManager {
	return &SessionManager{
		engine: engine,
		cache:  cache,
	}
}

// Create 创建会话
func (m *SessionManager) Create(ctx context.Context, agentID string, metadata map[string]any) (*model.AgentSession, error) {
	s := model.NewSession(agentID, metadata)
	s.ID = util.NewUUID()

	data, err := model.SessionToJSON(s)
	if err != nil {
		return nil, fmt.Errorf("marshal session: %w", err)
	}

	key := storage.EncodeKey(storage.PrefixSession, s.ID)
	if err := m.engine.Set(ctx, key, data); err != nil {
		return nil, fmt.Errorf("store session: %w", err)
	}

	// 写入 agent_id 索引
	idxKey := storage.EncodeIndexKey(storage.PrefixSession, s.AgentID, s.ID)
	if err := m.engine.Set(ctx, idxKey, []byte{1}); err != nil {
		return nil, fmt.Errorf("index session: %w", err)
	}

	// 写入缓存
	m.cache.Set(key, data)

	return s, nil
}

// Get 获取会话
func (m *SessionManager) Get(ctx context.Context, id string) (*model.AgentSession, error) {
	key := storage.EncodeKey(storage.PrefixSession, id)

	// 先查缓存
	if data, ok := m.cache.Get(key); ok {
		return model.SessionFromJSON(data)
	}

	// 查存储
	data, err := m.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	// 写入缓存
	m.cache.Set(key, data)

	return model.SessionFromJSON(data)
}

// Update 更新会话
func (m *SessionManager) Update(ctx context.Context, s *model.AgentSession) error {
	data, err := model.SessionToJSON(s)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	key := storage.EncodeKey(storage.PrefixSession, s.ID)
	if err := m.engine.Set(ctx, key, data); err != nil {
		return fmt.Errorf("store session: %w", err)
	}

	m.cache.Set(key, data)
	return nil
}

// Delete 删除会话
func (m *SessionManager) Delete(ctx context.Context, id string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	key := storage.EncodeKey(storage.PrefixSession, id)
	idxKey := storage.EncodeIndexKey(storage.PrefixSession, s.AgentID, s.ID)

	m.engine.Delete(ctx, key)
	m.engine.Delete(ctx, idxKey)
	m.cache.Delete(key)

	return nil
}

// ListByAgent 按 agent_id 列出会话
func (m *SessionManager) ListByAgent(ctx context.Context, agentID string, limit int) ([]*model.AgentSession, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixSession, agentID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := m.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("scan sessions: %w", err)
	}
	defer iter.Close()

	var sessions []*model.AgentSession
	for iter.Next() {
		key, _ := iter.Item()
		// 从索引 key 中提取 session ID
		sessionID := storage.DecodeIndexID(key)
		s, err := m.Get(ctx, sessionID)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// ListAll 列出所有会话
func (m *SessionManager) ListAll(ctx context.Context, limit int) ([]*model.AgentSession, error) {
	prefix := []byte{storage.PrefixSession}
	start, end := storage.PrefixRange(prefix)

	iter, err := m.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("scan sessions: %w", err)
	}
	defer iter.Close()

	var sessions []*model.AgentSession
	for iter.Next() {
		_, val := iter.Item()
		var s model.AgentSession
		if err := json.Unmarshal(val, &s); err != nil {
			continue
		}
		sessions = append(sessions, &s)
	}

	return sessions, nil
}

// UpdateState 更新会话状态
func (m *SessionManager) UpdateState(ctx context.Context, id string, state model.SessionState) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	s.State = state
	return m.Update(ctx, s)
}
