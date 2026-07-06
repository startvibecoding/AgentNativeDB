package agent

import (
	"context"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func newPermTestEnv(t *testing.T) (*PermissionManager, *AuditLogger, storage.Engine) {
	t.Helper()
	engine := storage.NewTestEngine(t)

	audit := NewAuditLogger(engine)
	pm, err := NewPermissionManager(engine, audit)
	if err != nil {
		t.Fatalf("new permission manager: %v", err)
	}
	return pm, audit, engine
}

func TestPermission_BuiltinRoles(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	for _, name := range []string{PermRoleAdmin, PermRoleWriter, PermRoleReader, PermRoleObserver} {
		if _, ok := pm.GetRole(name); !ok {
			t.Errorf("builtin role %q missing", name)
		}
	}
}

func TestPermission_AssignAndCheck(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()

	agent := "agent-1"
	if err := pm.AssignRole(ctx, agent, PermRoleReader); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	// reader 可读 session
	ok, err := pm.Check(ctx, agent, ResourceSession, ActionRead, "")
	if err != nil || !ok {
		t.Fatalf("reader should read session, ok=%v err=%v", ok, err)
	}
	// reader 不能写
	ok, _ = pm.Check(ctx, agent, ResourceSession, ActionWrite, "")
	if ok {
		t.Fatal("reader must not write")
	}
}

func TestPermission_AdminWildcard(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()
	agent := "root"
	if err := pm.AssignRole(ctx, agent, PermRoleAdmin); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		r Resource
		a Action
	}{
		{ResourceSession, ActionWrite},
		{ResourceMemory, ActionDelete},
		{ResourceAudit, ActionRead},
		{ResourcePermission, ActionAdmin},
	}
	for _, c := range cases {
		ok, err := pm.Check(ctx, agent, c.r, c.a, "any")
		if err != nil || !ok {
			t.Errorf("admin should be allowed for %s/%s, got ok=%v err=%v", c.r, c.a, ok, err)
		}
	}
}

func TestPermission_Require(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()

	err := pm.Require(ctx, "nobody", ResourceSession, ActionRead, "")
	if err == nil {
		t.Fatal("expected denial for unassigned agent")
	}
	if !IsPermissionDenied(err) {
		t.Fatalf("expected PermissionDeniedError, got %T", err)
	}

	if err := pm.AssignRole(ctx, "nobody", PermRoleReader); err != nil {
		t.Fatal(err)
	}
	if err := pm.Require(ctx, "nobody", ResourceSession, ActionRead, ""); err != nil {
		t.Fatalf("should be allowed now: %v", err)
	}
}

func TestPermission_RevokeRole(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()
	agent := "a1"

	if err := pm.AssignRole(ctx, agent, PermRoleWriter); err != nil {
		t.Fatal(err)
	}
	if err := pm.AssignRole(ctx, agent, PermRoleReader); err != nil {
		t.Fatal(err)
	}
	if err := pm.RevokeRole(ctx, agent, PermRoleWriter); err != nil {
		t.Fatal(err)
	}
	// 撤销 writer 后不能写
	if ok, _ := pm.Check(ctx, agent, ResourceSession, ActionWrite, ""); ok {
		t.Fatal("write should be denied after revoke")
	}
	// 仍可读
	if ok, _ := pm.Check(ctx, agent, ResourceSession, ActionRead, ""); !ok {
		t.Fatal("read should still be allowed")
	}
	if got := pm.RolesOf(agent); len(got) != 1 || got[0] != PermRoleReader {
		t.Fatalf("unexpected roles after revoke: %v", got)
	}
}

func TestPermission_CustomRoleAndScope(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()

	role := &PermRole{
		Name: "session-owner",
		Permissions: []Permission{
			{Resource: ResourceSession, Action: ActionWrite, Scope: "sess-123"},
			{Resource: ResourceMemory, Action: ActionRead, Scope: "mem-*"},
		},
	}
	if err := pm.CreateRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	if err := pm.AssignRole(ctx, "u1", "session-owner"); err != nil {
		t.Fatal(err)
	}

	// 精确匹配 scope
	if ok, _ := pm.Check(ctx, "u1", ResourceSession, ActionWrite, "sess-123"); !ok {
		t.Fatal("should allow sess-123")
	}
	// 不同 scope 拒绝
	if ok, _ := pm.Check(ctx, "u1", ResourceSession, ActionWrite, "sess-999"); ok {
		t.Fatal("should deny sess-999")
	}
	// 前缀匹配
	if ok, _ := pm.Check(ctx, "u1", ResourceMemory, ActionRead, "mem-abc"); !ok {
		t.Fatal("should allow prefix match mem-abc")
	}
	if ok, _ := pm.Check(ctx, "u1", ResourceMemory, ActionRead, "other-abc"); ok {
		t.Fatal("should deny non-prefix")
	}
}

func TestPermission_Persistence(t *testing.T) {
	dir := t.TempDir()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine, err := storage.CreateEngine(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	pm, err := NewPermissionManager(engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	custom := &PermRole{
		Name:        "auditor",
		Permissions: []Permission{{Resource: ResourceAudit, Action: ActionRead}},
	}
	if err := pm.CreateRole(ctx, custom); err != nil {
		t.Fatal(err)
	}
	if err := pm.AssignRole(ctx, "agent-x", "auditor"); err != nil {
		t.Fatal(err)
	}
	engine.Close()

	// 重开，验证持久化
	engine2, err := storage.CreateEngine(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer engine2.Close()
	pm2, err := NewPermissionManager(engine2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pm2.GetRole("auditor"); !ok {
		t.Fatal("custom role lost after reload")
	}
	roles := pm2.RolesOf("agent-x")
	if len(roles) != 1 || roles[0] != "auditor" {
		t.Fatalf("agent role assignment lost: %v", roles)
	}
	if ok, _ := pm2.Check(ctx, "agent-x", ResourceAudit, ActionRead, ""); !ok {
		t.Fatal("reloaded permissions should still allow")
	}
}

func TestPermission_AuditIntegration(t *testing.T) {
	pm, audit, _ := newPermTestEnv(t)
	ctx := context.Background()

	if err := pm.AssignRole(ctx, "a1", PermRoleReader); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Check(ctx, "a1", ResourceSession, ActionRead, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Check(ctx, "a1", ResourceSession, ActionDelete, ""); err != nil {
		t.Fatal(err)
	}

	events, err := audit.ListByOperation(ctx, OpPermission, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("expected >=2 permission audit events, got %d", len(events))
	}
	var sawSuccess, sawFail bool
	for _, e := range events {
		if e.Success {
			sawSuccess = true
		} else {
			sawFail = true
		}
	}
	if !sawSuccess || !sawFail {
		t.Fatalf("expected both allowed and denied audit events, success=%v fail=%v", sawSuccess, sawFail)
	}
}

func TestPermission_DeleteRole(t *testing.T) {
	pm, _, _ := newPermTestEnv(t)
	ctx := context.Background()

	role := &PermRole{Name: "tmp", Permissions: []Permission{{Resource: ResourceQuery, Action: ActionRead}}}
	if err := pm.CreateRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	if err := pm.DeleteRole(ctx, "tmp"); err != nil {
		t.Fatal(err)
	}
	if _, ok := pm.GetRole("tmp"); ok {
		t.Fatal("role should be deleted")
	}
}
