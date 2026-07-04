package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// Action 定义资源操作类型（细粒度权限）。
type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionAdmin  Action = "admin" // 管理类操作（创建角色/授予权限）
	ActionAll    Action = "*"     // 通配所有操作
)

// Resource 表示受控资源类型。使用字符串以便扩展，例如：
//
//	session, memory, decision, room, task, query, audit, permission
//
// 通配符 "*" 表示任意资源。
type Resource string

const (
	ResourceSession    Resource = "session"
	ResourceMemory     Resource = "memory"
	ResourceDecision   Resource = "decision"
	ResourceRoom       Resource = "room"
	ResourceTask       Resource = "task"
	ResourceQuery      Resource = "query"
	ResourceAudit      Resource = "audit"
	ResourcePermission Resource = "permission"
	ResourceAll        Resource = "*"
)

// Permission 表示单条权限规则。可选 Scope 用于限定实例，例如某个 session id；
// 空 Scope 或 "*" 表示对该资源的所有实例生效。
type Permission struct {
	Resource Resource `json:"resource"`
	Action   Action   `json:"action"`
	Scope    string   `json:"scope,omitempty"`
}

// PermRole 表示一组权限的集合（RBAC 中的“角色”）。
// 注意与 coordinator.go 中的房间成员角色 Role 区分，两者语义不同。
type PermRole struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// 预定义权限角色名称。
const (
	PermRoleAdmin    = "admin"    // 全部权限
	PermRoleWriter   = "writer"   // 读写业务资源
	PermRoleReader   = "reader"   // 只读业务资源
	PermRoleObserver = "observer" // 只读审计
)

// PermissionManager 管理 Agent 角色分配与权限校验。
//
// 存储布局（全部落在 storage.PrefixSystem 下，使用独立命名空间避免冲突）：
//
//	role:<name>              => JSON(Role)
//	agentrole:<agentID>      => JSON([]string) 分配给该 Agent 的角色列表
//
// 内存中维护一份读缓存，写入时同步刷新，避免每次校验都走 KV。
type PermissionManager struct {
	engine storage.Engine
	audit  *AuditLogger

	mu     sync.RWMutex
	roles  map[string]*PermRole
	agents map[string][]string // agentID -> role names
	loaded bool
}

// NewPermissionManager 创建权限管理器，会尝试从存储加载已有角色/分配。
// audit 可为 nil，此时不记录审计事件。
func NewPermissionManager(engine storage.Engine, audit *AuditLogger) (*PermissionManager, error) {
	pm := &PermissionManager{
		engine: engine,
		audit:  audit,
		roles:  make(map[string]*PermRole),
		agents: make(map[string][]string),
	}
	if err := pm.load(context.Background()); err != nil {
		return nil, err
	}
	if err := pm.ensureBuiltins(context.Background()); err != nil {
		return nil, err
	}
	return pm, nil
}

// ------------- 角色管理 -------------

// CreateRole 创建（或覆盖）角色。
func (p *PermissionManager) CreateRole(ctx context.Context, role *PermRole) error {
	if role == nil || role.Name == "" {
		return fmt.Errorf("role name required")
	}
	now := time.Now()
	if role.CreatedAt.IsZero() {
		role.CreatedAt = now
	}
	role.UpdatedAt = now

	if err := p.putRole(ctx, role); err != nil {
		return err
	}

	p.mu.Lock()
	p.roles[role.Name] = role
	p.mu.Unlock()
	return nil
}

// GetRole 获取角色。
func (p *PermissionManager) GetRole(name string) (*PermRole, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.roles[name]
	return r, ok
}

// ListRoles 列出所有角色。
func (p *PermissionManager) ListRoles() []*PermRole {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*PermRole, 0, len(p.roles))
	for _, r := range p.roles {
		out = append(out, r)
	}
	return out
}

// DeleteRole 删除角色（不会自动解绑已分配的 agent，调用方需自行处理）。
func (p *PermissionManager) DeleteRole(ctx context.Context, name string) error {
	key := roleKey(name)
	if err := p.engine.Delete(ctx, key); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.roles, name)
	p.mu.Unlock()
	return nil
}

// ------------- 角色分配 -------------

// AssignRole 给 Agent 分配一个角色（幂等）。
func (p *PermissionManager) AssignRole(ctx context.Context, agentID, roleName string) error {
	if agentID == "" || roleName == "" {
		return fmt.Errorf("agentID and roleName required")
	}
	p.mu.RLock()
	_, ok := p.roles[roleName]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("role not found: %s", roleName)
	}

	p.mu.Lock()
	roles := p.agents[agentID]
	for _, r := range roles {
		if r == roleName {
			p.mu.Unlock()
			return nil
		}
	}
	roles = append(roles, roleName)
	p.agents[agentID] = roles
	p.mu.Unlock()

	return p.putAgentRoles(ctx, agentID, roles)
}

// RevokeRole 撤销 Agent 的某个角色。
func (p *PermissionManager) RevokeRole(ctx context.Context, agentID, roleName string) error {
	p.mu.Lock()
	roles := p.agents[agentID]
	out := roles[:0:0]
	for _, r := range roles {
		if r != roleName {
			out = append(out, r)
		}
	}
	p.agents[agentID] = out
	p.mu.Unlock()

	return p.putAgentRoles(ctx, agentID, out)
}

// RolesOf 返回 Agent 拥有的角色名列表。
func (p *PermissionManager) RolesOf(agentID string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	roles := p.agents[agentID]
	out := make([]string, len(roles))
	copy(out, roles)
	return out
}

// ------------- 权限校验 -------------

// Check 判断 agent 是否可对 (resource, scope) 执行 action。
// scope 可为空，表示不关心实例；策略中 Scope 为空或 "*" 视为通配。
// 若配置了审计日志，会记录一次 permission.check 事件。
func (p *PermissionManager) Check(ctx context.Context, agentID string, resource Resource, action Action, scope string) (bool, error) {
	allowed := p.evaluate(agentID, resource, action, scope)

	if p.audit != nil {
		_ = p.audit.Log(ctx, &AuditEvent{
			AgentID:   agentID,
			Operation: OpPermission,
			Resource:  string(resource),
			Details: map[string]any{
				"action": string(action),
				"scope":  scope,
			},
			Success:   allowed,
			Timestamp: time.Now(),
		})
	}
	return allowed, nil
}

// Require 是 Check 的强制版本：不允许则返回 ErrPermissionDenied。
func (p *PermissionManager) Require(ctx context.Context, agentID string, resource Resource, action Action, scope string) error {
	ok, err := p.Check(ctx, agentID, resource, action, scope)
	if err != nil {
		return err
	}
	if !ok {
		return &PermissionDeniedError{
			AgentID:  agentID,
			Resource: resource,
			Action:   action,
			Scope:    scope,
		}
	}
	return nil
}

func (p *PermissionManager) evaluate(agentID string, resource Resource, action Action, scope string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	roles := p.agents[agentID]
	for _, name := range roles {
		role, ok := p.roles[name]
		if !ok {
			continue
		}
		for _, perm := range role.Permissions {
			if matchResource(perm.Resource, resource) &&
				matchAction(perm.Action, action) &&
				matchScope(perm.Scope, scope) {
				return true
			}
		}
	}
	return false
}

func matchResource(pattern, want Resource) bool {
	return pattern == ResourceAll || pattern == want
}

func matchAction(pattern, want Action) bool {
	if pattern == ActionAll || pattern == want {
		return true
	}
	// admin 隐含所有能力
	if pattern == ActionAdmin {
		return true
	}
	return false
}

func matchScope(pattern, want string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	// 支持前缀匹配：pattern 以 "*" 结尾表示前缀
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(want, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == want
}

// ------------- 持久化 -------------

func roleKey(name string) []byte {
	return storage.EncodeKey(storage.PrefixSystem, "role:"+name)
}

func agentRolesKey(agentID string) []byte {
	return storage.EncodeKey(storage.PrefixSystem, "agentrole:"+agentID)
}

func (p *PermissionManager) putRole(ctx context.Context, role *PermRole) error {
	data, err := json.Marshal(role)
	if err != nil {
		return err
	}
	return p.engine.Set(ctx, roleKey(role.Name), data)
}

func (p *PermissionManager) putAgentRoles(ctx context.Context, agentID string, roles []string) error {
	if len(roles) == 0 {
		return p.engine.Delete(ctx, agentRolesKey(agentID))
	}
	data, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	return p.engine.Set(ctx, agentRolesKey(agentID), data)
}

func (p *PermissionManager) load(ctx context.Context) error {
	// 加载角色
	rolePrefix := storage.EncodeKey(storage.PrefixSystem, "role:")
	if err := p.scanJSON(ctx, rolePrefix, func(_ []byte, val []byte) error {
		var role PermRole
		if err := json.Unmarshal(val, &role); err != nil {
			return nil // 跳过坏记录
		}
		p.roles[role.Name] = &role
		return nil
	}); err != nil {
		return err
	}

	// 加载 agent -> roles 映射
	agentPrefix := storage.EncodeKey(storage.PrefixSystem, "agentrole:")
	if err := p.scanJSON(ctx, agentPrefix, func(key []byte, val []byte) error {
		// key = [PrefixSystem]"agentrole:<agentID>"
		full := string(key[1:])
		const p1 = "agentrole:"
		if !strings.HasPrefix(full, p1) {
			return nil
		}
		agentID := full[len(p1):]
		var roles []string
		if err := json.Unmarshal(val, &roles); err != nil {
			return nil
		}
		p.agents[agentID] = roles
		return nil
	}); err != nil {
		return err
	}
	p.loaded = true
	return nil
}

func (p *PermissionManager) scanJSON(ctx context.Context, prefix []byte, fn func(key, val []byte) error) error {
	start, end := storage.PrefixRange(prefix)
	iter, err := p.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()
	for iter.Next() {
		k, v := iter.Item()
		// 拷贝，避免 badger 迭代器复用底层切片
		kc := append([]byte(nil), k...)
		vc := append([]byte(nil), v...)
		if err := fn(kc, vc); err != nil {
			return err
		}
	}
	return iter.Error()
}

// ensureBuiltins 确保内置角色存在（若已存在则保留用户自定义修改）。
func (p *PermissionManager) ensureBuiltins(ctx context.Context) error {
	builtins := []*PermRole{
		{
			Name:        PermRoleAdmin,
			Description: "Full access to all resources",
			Permissions: []Permission{{Resource: ResourceAll, Action: ActionAll}},
		},
		{
			Name:        PermRoleWriter,
			Description: "Read and write business resources",
			Permissions: []Permission{
				{Resource: ResourceSession, Action: ActionRead},
				{Resource: ResourceSession, Action: ActionWrite},
				{Resource: ResourceMemory, Action: ActionRead},
				{Resource: ResourceMemory, Action: ActionWrite},
				{Resource: ResourceDecision, Action: ActionRead},
				{Resource: ResourceDecision, Action: ActionWrite},
				{Resource: ResourceRoom, Action: ActionRead},
				{Resource: ResourceRoom, Action: ActionWrite},
				{Resource: ResourceTask, Action: ActionRead},
				{Resource: ResourceTask, Action: ActionWrite},
				{Resource: ResourceQuery, Action: ActionRead},
			},
		},
		{
			Name:        PermRoleReader,
			Description: "Read-only access to business resources",
			Permissions: []Permission{
				{Resource: ResourceSession, Action: ActionRead},
				{Resource: ResourceMemory, Action: ActionRead},
				{Resource: ResourceDecision, Action: ActionRead},
				{Resource: ResourceRoom, Action: ActionRead},
				{Resource: ResourceTask, Action: ActionRead},
				{Resource: ResourceQuery, Action: ActionRead},
			},
		},
		{
			Name:        PermRoleObserver,
			Description: "Read-only audit access",
			Permissions: []Permission{
				{Resource: ResourceAudit, Action: ActionRead},
			},
		},
	}

	for _, r := range builtins {
		if _, ok := p.roles[r.Name]; ok {
			continue
		}
		if err := p.CreateRole(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// PermissionDeniedError 用于 Require 拒绝时返回。
type PermissionDeniedError struct {
	AgentID  string
	Resource Resource
	Action   Action
	Scope    string
}

func (e *PermissionDeniedError) Error() string {
	if e.Scope != "" {
		return fmt.Sprintf("permission denied: agent=%s cannot %s %s (scope=%s)",
			e.AgentID, e.Action, e.Resource, e.Scope)
	}
	return fmt.Sprintf("permission denied: agent=%s cannot %s %s", e.AgentID, e.Action, e.Resource)
}

// IsPermissionDenied 便捷判断。
func IsPermissionDenied(err error) bool {
	_, ok := err.(*PermissionDeniedError)
	return ok
}
