package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// SessionConfig 会话管理配置
type SessionConfig struct {
	// Timeout 会话超时时间
	Timeout time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// MaxSessionsPerAgent 每个 Agent 最大活跃会话数（0=不限制）
	MaxSessionsPerAgent int
}

// DefaultSessionConfig 默认配置
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Timeout:             24 * time.Hour,
		CleanupInterval:     10 * time.Minute,
		MaxSessionsPerAgent: 100,
	}
}

// StateTransition 无效状态转换错误
type StateTransition struct {
	From model.SessionState
	To   model.SessionState
}

func (e StateTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}

// 合法的状态转换
var validTransitions = map[model.SessionState][]model.SessionState{
	model.SessionActive:    {model.SessionPaused, model.SessionCompleted, model.SessionFailed},
	model.SessionPaused:    {model.SessionActive, model.SessionFailed},
	model.SessionCompleted: {}, // 终态
	model.SessionFailed:    {model.SessionActive}, // 可重试
}

// canTransition 检查状态转换是否合法
func canTransition(from, to model.SessionState) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// SessionManagerEnhanced 增强的会话管理器
type SessionManagerEnhanced struct {
	*SessionManager
	config SessionConfig
	stopCh chan struct{}
	mu     sync.RWMutex
}

// NewSessionManagerEnhanced 创建增强会话管理器
func NewSessionManagerEnhanced(engine storage.Engine, cache *storage.Cache, config SessionConfig) *SessionManagerEnhanced {
	m := &SessionManagerEnhanced{
		SessionManager: NewSessionManager(engine, cache),
		config:         config,
		stopCh:         make(chan struct{}),
	}

	// 启动后台清理
	go m.cleanupLoop()

	return m
}

// Create 创建会话（带配额检查）
func (m *SessionManagerEnhanced) Create(ctx context.Context, agentID string, metadata map[string]any) (*model.AgentSession, error) {
	// 检查配额
	if m.config.MaxSessionsPerAgent > 0 {
		sessions, err := m.ListActiveByAgent(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("check quota: %w", err)
		}
		if len(sessions) >= m.config.MaxSessionsPerAgent {
			return nil, fmt.Errorf("agent %s has reached maximum active sessions (%d)", agentID, m.config.MaxSessionsPerAgent)
		}
	}

	return m.SessionManager.Create(ctx, agentID, metadata)
}

// TransitionState 状态转换（带合法性检查）
func (m *SessionManagerEnhanced) TransitionState(ctx context.Context, id string, newState model.SessionState) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	if !canTransition(s.State, newState) {
		return StateTransition{From: s.State, To: newState}
	}

	s.State = newState
	s.UpdatedAt = time.Now()
	return m.Update(ctx, s)
}

// Pause 暂停会话
func (m *SessionManagerEnhanced) Pause(ctx context.Context, id string) error {
	return m.TransitionState(ctx, id, model.SessionPaused)
}

// Resume 恢复会话
func (m *SessionManagerEnhanced) Resume(ctx context.Context, id string) error {
	return m.TransitionState(ctx, id, model.SessionActive)
}

// Complete 完成会话
func (m *SessionManagerEnhanced) Complete(ctx context.Context, id string) error {
	return m.TransitionState(ctx, id, model.SessionCompleted)
}

// Fail 标记会话失败
func (m *SessionManagerEnhanced) Fail(ctx context.Context, id string) error {
	return m.TransitionState(ctx, id, model.SessionFailed)
}

// Heartbeat 心跳更新（刷新超时）
func (m *SessionManagerEnhanced) Heartbeat(ctx context.Context, id string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	s.UpdatedAt = time.Now()
	return m.Update(ctx, s)
}

// ListActiveByAgent 列出指定 Agent 的活跃会话
func (m *SessionManagerEnhanced) ListActiveByAgent(ctx context.Context, agentID string) ([]*model.AgentSession, error) {
	all, err := m.ListByAgent(ctx, agentID, 0)
	if err != nil {
		return nil, err
	}

	var active []*model.AgentSession
	for _, s := range all {
		if s.State == model.SessionActive || s.State == model.SessionPaused {
			active = append(active, s)
		}
	}
	return active, nil
}

// cleanupLoop 后台清理过期会话
func (m *SessionManagerEnhanced) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupExpired()
		case <-m.stopCh:
			return
		}
	}
}

// cleanupExpired 清理过期会话
func (m *SessionManagerEnhanced) cleanupExpired() {
	ctx := context.Background()
	sessions, err := m.ListAll(ctx, 0)
	if err != nil {
		slog.Error("cleanup: list sessions failed", "error", err)
		return
	}

	now := time.Now()
	for _, s := range sessions {
		if s.State != model.SessionActive && s.State != model.SessionPaused {
			continue
		}

		if now.Sub(s.UpdatedAt) > m.config.Timeout {
			slog.Info("cleanup: expiring session", "id", s.ID, "agent_id", s.AgentID, "last_update", s.UpdatedAt)
			if err := m.TransitionState(ctx, s.ID, model.SessionFailed); err != nil {
				slog.Error("cleanup: expire session failed", "id", s.ID, "error", err)
			}
		}
	}
}

// Stop 停止后台清理
func (m *SessionManagerEnhanced) Stop() {
	select {
	case <-m.stopCh:
		// 已关闭
	default:
		close(m.stopCh)
	}
}
