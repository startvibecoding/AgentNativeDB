package agent

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// MemoryConfig 记忆管理配置
type MemoryConfig struct {
	// ShortTermWindow 短期记忆滑窗大小
	ShortTermWindow int

	// ImportanceDecayRate 重要度衰减率（每小时）
	ImportanceDecayRate float32

	// MaxMemoriesPerSession 每会话最大记忆数（0=不限制）
	MaxMemoriesPerSession int

	// WorkingMemoryTTL 工作记忆 TTL
	WorkingMemoryTTL time.Duration

	// DecayInterval 衰减计算间隔
	DecayInterval time.Duration
}

// DefaultMemoryConfig 默认配置
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		ShortTermWindow:       50,
		ImportanceDecayRate:   0.01, // 每小时衰减 1%
		MaxMemoriesPerSession: 10000,
		WorkingMemoryTTL:      1 * time.Hour,
		DecayInterval:         30 * time.Minute,
	}
}

// MemoryStoreEnhanced 增强的记忆管理器
type MemoryStoreEnhanced struct {
	*MemoryStore
	config MemoryConfig
	stopCh chan struct{}
	mu     sync.RWMutex
}

// NewMemoryStoreEnhanced 创建增强记忆管理器
func NewMemoryStoreEnhanced(engine storage.Engine, cache *storage.Cache, config MemoryConfig) *MemoryStoreEnhanced {
	m := &MemoryStoreEnhanced{
		MemoryStore: NewMemoryStore(engine, cache),
		config:      config,
		stopCh:      make(chan struct{}),
	}

	go m.decayLoop()

	return m
}

// StoreShortTerm 存储短期记忆（自动滑窗管理）
func (m *MemoryStoreEnhanced) StoreShortTerm(ctx context.Context, sessionID string, content string, importance float32) (*model.MemoryEntry, error) {
	mem := model.NewMemory(sessionID, model.MemoryShortTerm, content, importance)
	stored, err := m.Store(ctx, mem)
	if err != nil {
		return nil, err
	}

	// 滑窗淘汰
	if m.config.ShortTermWindow > 0 {
		go m.evictShortTerm(ctx, sessionID)
	}

	return stored, nil
}

// StoreLongTerm 存储长期记忆
func (m *MemoryStoreEnhanced) StoreLongTerm(ctx context.Context, sessionID string, content string, importance float32) (*model.MemoryEntry, error) {
	mem := model.NewMemory(sessionID, model.MemoryLongTerm, content, importance)
	return m.Store(ctx, mem)
}

// StoreWorking 存储工作记忆（可跨会话共享）
func (m *MemoryStoreEnhanced) StoreWorking(ctx context.Context, sessionID string, content string) (*model.MemoryEntry, error) {
	mem := model.NewMemory(sessionID, model.MemoryWorking, content, 1.0)
	return m.Store(ctx, mem)
}

// GetRecentShortTerm 获取最近的短期记忆（滑窗）
func (m *MemoryStoreEnhanced) GetRecentShortTerm(ctx context.Context, sessionID string, limit int) ([]*model.MemoryEntry, error) {
	if limit <= 0 {
		limit = m.config.ShortTermWindow
	}

	memories, err := m.ListBySession(ctx, sessionID, model.MemoryShortTerm, 0)
	if err != nil {
		return nil, err
	}

	// 按创建时间降序排序
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})

	if len(memories) > limit {
		memories = memories[:limit]
	}

	return memories, nil
}

// GetWorkingMemory 获取工作记忆
func (m *MemoryStoreEnhanced) GetWorkingMemory(ctx context.Context, sessionID string) ([]*model.MemoryEntry, error) {
	return m.ListBySession(ctx, sessionID, model.MemoryWorking, 0)
}

// RecallByImportance 按重要度检索记忆
func (m *MemoryStoreEnhanced) RecallByImportance(ctx context.Context, sessionID string, memType model.MemoryType, limit int) ([]*model.MemoryEntry, error) {
	memories, err := m.ListBySession(ctx, sessionID, memType, 0)
	if err != nil {
		return nil, err
	}

	// 按重要度降序排序
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Importance > memories[j].Importance
	})

	if limit > 0 && len(memories) > limit {
		memories = memories[:limit]
	}

	return memories, nil
}

// RecallRecent 按时间检索最近记忆
func (m *MemoryStoreEnhanced) RecallRecent(ctx context.Context, sessionID string, since time.Time, limit int) ([]*model.MemoryEntry, error) {
	memories, err := m.ListBySession(ctx, sessionID, "", 0)
	if err != nil {
		return nil, err
	}

	var recent []*model.MemoryEntry
	for _, mem := range memories {
		if mem.CreatedAt.After(since) || mem.CreatedAt.Equal(since) {
			recent = append(recent, mem)
		}
	}

	sort.Slice(recent, func(i, j int) bool {
		return recent[i].CreatedAt.After(recent[j].CreatedAt)
	})

	if limit > 0 && len(recent) > limit {
		recent = recent[:limit]
	}

	return recent, nil
}

// GetSessionStats 获取会话记忆统计
func (m *MemoryStoreEnhanced) GetSessionStats(ctx context.Context, sessionID string) (*MemoryStats, error) {
	memories, err := m.ListBySession(ctx, sessionID, "", 0)
	if err != nil {
		return nil, err
	}

	stats := &MemoryStats{
		SessionID: sessionID,
	}

	for _, mem := range memories {
		stats.TotalCount++
		switch mem.Type {
		case model.MemoryShortTerm:
			stats.ShortTermCount++
		case model.MemoryLongTerm:
			stats.LongTermCount++
		case model.MemoryWorking:
			stats.WorkingCount++
		}
		stats.TotalImportance += mem.Importance
		stats.TotalAccessCount += int64(mem.AccessCount)
	}

	if stats.TotalCount > 0 {
		stats.AvgImportance = stats.TotalImportance / float32(stats.TotalCount)
	}

	return stats, nil
}

// MemoryStats 记忆统计
type MemoryStats struct {
	SessionID        string  `json:"session_id"`
	TotalCount       int     `json:"total_count"`
	ShortTermCount   int     `json:"short_term_count"`
	LongTermCount    int     `json:"long_term_count"`
	WorkingCount     int     `json:"working_count"`
	AvgImportance    float32 `json:"avg_importance"`
	TotalImportance  float32 `json:"total_importance"`
	TotalAccessCount int64   `json:"total_access_count"`
}

// evictShortTerm 淘汰多余的短期记忆
func (m *MemoryStoreEnhanced) evictShortTerm(ctx context.Context, sessionID string) {
	memories, err := m.ListBySession(ctx, sessionID, model.MemoryShortTerm, 0)
	if err != nil {
		return
	}

	if len(memories) <= m.config.ShortTermWindow {
		return
	}

	// 按创建时间排序，保留最新的
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})

	// 删除超出窗口的旧记忆
	toDelete := memories[m.config.ShortTermWindow:]
	for _, mem := range toDelete {
		m.Delete(ctx, mem.ID)
	}
}

// decayLoop 后台重要度衰减
func (m *MemoryStoreEnhanced) decayLoop() {
	ticker := time.NewTicker(m.config.DecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.decayImportance()
		case <-m.stopCh:
			return
		}
	}
}

// decayImportance 衰减记忆重要度
func (m *MemoryStoreEnhanced) decayImportance() {
	ctx := context.Background()
	sessions, err := m.ListAllSessions(ctx)
	if err != nil {
		return
	}

	now := time.Now()
	for _, sessionID := range sessions {
		memories, err := m.ListBySession(ctx, sessionID, "", 0)
		if err != nil {
			continue
		}

		for _, mem := range memories {
			if mem.Type == model.MemoryWorking {
				continue // 工作记忆不衰减
			}

			hours := now.Sub(mem.AccessedAt).Hours()
			if hours < 1 {
				continue
			}

			// 对数衰减：importance *= e^(-rate * hours * log(access_count + 1))
			decayFactor := float32(math.Exp(float64(-m.config.ImportanceDecayRate * float32(hours) * float32(math.Log(float64(mem.AccessCount+1))))))

			newImportance := mem.Importance * decayFactor
			if newImportance < 0.01 {
				newImportance = 0.01 // 最低重要度
			}

			if newImportance != mem.Importance {
				mem.Importance = newImportance
				mem.AccessedAt = now
				// 重新序列化并存储
				data, err := model.MemoryToJSON(mem)
				if err != nil {
					continue
				}
				key := storage.EncodeKey(storage.PrefixMemory, mem.ID)
				m.engine.Set(ctx, key, data)
				m.cache.Set(key, data)
			}
		}
	}
}

// ListAllSessions 列出所有会话 ID（用于衰减遍历）
func (m *MemoryStoreEnhanced) ListAllSessions(ctx context.Context) ([]string, error) {
	prefix := []byte{storage.PrefixMemory}
	start, end := storage.PrefixRange(prefix)

	iter, err := m.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan memories: %w", err)
	}
	defer iter.Close()

	seen := make(map[string]bool)
	var sessionIDs []string

	for iter.Next() {
		_, val := iter.Item()
		mem, err := model.MemoryFromJSON(val)
		if err != nil {
			continue
		}
		if !seen[mem.SessionID] {
			seen[mem.SessionID] = true
			sessionIDs = append(sessionIDs, mem.SessionID)
		}
	}

	return sessionIDs, nil
}

// Stop 停止后台衰减
func (m *MemoryStoreEnhanced) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}
