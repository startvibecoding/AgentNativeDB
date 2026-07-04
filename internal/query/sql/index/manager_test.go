package index

import (
	"context"
	"os"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func setupTestIndex(t *testing.T) (*Manager, context.Context) {
	t.Helper()
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})
	mgr := NewManager(engine)
	ctx := context.Background()
	if err := mgr.Init(ctx); err != nil {
		t.Fatalf("init manager: %v", err)
	}
	return mgr, ctx
}

// ========== Manager 测试 ==========

func TestManager_CreateAndList(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_name", Table: "users", Column: "name", Type: TypeHash}
	if err := mgr.Create(ctx, meta, false); err != nil {
		t.Fatal(err)
	}

	if len(mgr.ListAll()) != 1 {
		t.Errorf("expected 1 index, got %d", len(mgr.ListAll()))
	}

	if len(mgr.ListByTable("users")) != 1 {
		t.Errorf("expected 1 index for users, got %d", len(mgr.ListByTable("users")))
	}

	if len(mgr.ListByTable("other")) != 0 {
		t.Errorf("expected 0 indexes for other, got %d", len(mgr.ListByTable("other")))
	}
}

func TestManager_CreateIfNotExists(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx1", Table: "t", Column: "c", Type: TypeBTree}
	mgr.Create(ctx, meta, false)

	// 再次创建应报错
	err := mgr.Create(ctx, meta, false)
	if err == nil {
		t.Error("expected error on duplicate index")
	}

	// IF NOT EXISTS 应静默
	err = mgr.Create(ctx, meta, true)
	if err != nil {
		t.Errorf("IF NOT EXISTS should not error, got %v", err)
	}
}

func TestManager_Drop(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx1", Table: "t", Column: "c", Type: TypeHash}
	mgr.Create(ctx, meta, false)

	if err := mgr.Drop(ctx, "idx1", false); err != nil {
		t.Fatal(err)
	}

	if len(mgr.ListAll()) != 0 {
		t.Errorf("expected 0 indexes after drop, got %d", len(mgr.ListAll()))
	}

	// DROP IF EXISTS 应静默
	err := mgr.Drop(ctx, "idx1", true)
	if err != nil {
		t.Errorf("DROP IF EXISTS should not error, got %v", err)
	}
}

func TestManager_FindByColumn(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	mgr.Create(ctx, Meta{Name: "idx1", Table: "users", Column: "name", Type: TypeHash}, false)
	mgr.Create(ctx, Meta{Name: "idx2", Table: "users", Column: "age", Type: TypeBTree}, false)
	mgr.Create(ctx, Meta{Name: "idx3", Table: "orders", Column: "name", Type: TypeBTree}, false)

	found := mgr.FindByColumn("users", "name")
	if len(found) != 1 {
		t.Errorf("expected 1, got %d", len(found))
	}
	if found[0].Name != "idx1" {
		t.Errorf("expected idx1, got %s", found[0].Name)
	}

	found2 := mgr.FindByColumn("users", "age")
	if len(found2) != 1 {
		t.Errorf("expected 1, got %d", len(found2))
	}
}

func TestManager_PersistAndReload(t *testing.T) {
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)

	// 创建索引
	mgr1 := NewManager(engine)
	mgr1.Init(context.Background())
	mgr1.Create(context.Background(), Meta{Name: "idx1", Table: "t", Column: "c", Type: TypeBTree}, false)
	engine.Close()

	// 重新加载
	engine2 := badgerstore.New()
	opts2 := storage.DefaultOptions()
	opts2.DataDir = dir
	engine2.Open(opts2)
	defer engine2.Close()

	mgr2 := NewManager(engine2)
	mgr2.Init(context.Background())

	metas := mgr2.ListAll()
	if len(metas) != 1 {
		t.Fatalf("expected 1 index after reload, got %d", len(metas))
	}
	if metas[0].Name != "idx1" {
		t.Errorf("expected idx1, got %s", metas[0].Name)
	}
}

// ========== Ops 测试 ==========

func TestOps_HashEqual(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_name", Table: "users", Column: "name", Type: TypeHash}
	mgr.Create(ctx, meta, false)

	rows := []map[string]any{
		{"id": "u1", "name": "alice"},
		{"id": "u2", "name": "bob"},
		{"id": "u3", "name": "alice"},
	}
	for _, r := range rows {
		mgr.InsertRow(ctx, "users", r, r["id"].(string))
	}

	metaPtr, _ := mgr.Get("idx_name")
	ids, err := mgr.LookupEqual(ctx, metaPtr, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 alice ids, got %d: %v", len(ids), ids)
	}

	ids2, _ := mgr.LookupEqual(ctx, metaPtr, "bob")
	if len(ids2) != 1 || ids2[0] != "u2" {
		t.Errorf("expected [u2], got %v", ids2)
	}

	ids3, _ := mgr.LookupEqual(ctx, metaPtr, "carol")
	if len(ids3) != 0 {
		t.Errorf("expected empty, got %v", ids3)
	}
}

func TestOps_BTreeRange(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_price", Table: "items", Column: "price", Type: TypeBTree}
	mgr.Create(ctx, meta, false)

	rows := []map[string]any{
		{"id": "a", "price": float64(10)},
		{"id": "b", "price": float64(20)},
		{"id": "c", "price": float64(30)},
		{"id": "d", "price": float64(40)},
	}
	for _, r := range rows {
		mgr.InsertRow(ctx, "items", r, r["id"].(string))
	}

	metaPtr, _ := mgr.Get("idx_price")

	// [15, 35] → b, c
	ids, _ := mgr.LookupRange(ctx, metaPtr, float64(15), float64(35), true, true)
	if len(ids) != 2 {
		t.Errorf("expected 2 in [15,35], got %d: %v", len(ids), ids)
	}

	// [20, 40] → b, c, d
	ids2, _ := mgr.LookupRange(ctx, metaPtr, float64(20), float64(40), true, true)
	if len(ids2) != 3 {
		t.Errorf("expected 3 in [20,40], got %d: %v", len(ids2), ids2)
	}

	// (20, 40) → c
	ids3, _ := mgr.LookupRange(ctx, metaPtr, float64(20), float64(40), false, false)
	if len(ids3) != 1 {
		t.Errorf("expected 1 in (20,40), got %d: %v", len(ids3), ids3)
	}

	// [5, 50] → all
	ids4, _ := mgr.LookupRange(ctx, metaPtr, float64(5), float64(50), true, true)
	if len(ids4) != 4 {
		t.Errorf("expected 4 in [5,50], got %d: %v", len(ids4), ids4)
	}

	// nil low → all ≤ high
	ids5, _ := mgr.LookupRange(ctx, metaPtr, nil, float64(25), false, true)
	if len(ids5) != 2 {
		t.Errorf("expected 2 ≤ 25, got %d: %v", len(ids5), ids5)
	}
}

func TestOps_InvertedMatch(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_body", Table: "docs", Column: "body", Type: TypeInverted}
	mgr.Create(ctx, meta, false)

	rows := []map[string]any{
		{"id": "d1", "body": "hello world"},
		{"id": "d2", "body": "world of databases"},
		{"id": "d3", "body": "hello databases"},
		{"id": "d4", "body": "completely different"},
	}
	for _, r := range rows {
		mgr.InsertRow(ctx, "docs", r, r["id"].(string))
	}

	metaPtr, _ := mgr.Get("idx_body")

	// 单 term: "hello" → d1, d3
	ids, _ := mgr.LookupTerm(ctx, metaPtr, "hello")
	if len(ids) != 2 {
		t.Errorf("expected 2 for 'hello', got %d: %v", len(ids), ids)
	}

	// 单 term: "world" → d1, d2
	ids2, _ := mgr.LookupTerm(ctx, metaPtr, "world")
	if len(ids2) != 2 {
		t.Errorf("expected 2 for 'world', got %d: %v", len(ids2), ids2)
	}

	// AND: "hello" AND "world" → d1
	ids3, _ := mgr.Match(ctx, metaPtr, "hello world")
	if len(ids3) != 1 {
		t.Errorf("expected 1 for 'hello world', got %d: %v", len(ids3), ids3)
	}

	// AND: "hello" AND "databases" → d3
	ids4, _ := mgr.Match(ctx, metaPtr, "hello databases")
	if len(ids4) != 1 {
		t.Errorf("expected 1 for 'hello databases', got %d: %v", len(ids4), ids4)
	}

	// 无匹配
	ids5, _ := mgr.Match(ctx, metaPtr, "nonexistent")
	if len(ids5) != 0 {
		t.Errorf("expected 0, got %v", ids5)
	}
}

func TestOps_DeleteRow(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_name", Table: "users", Column: "name", Type: TypeHash}
	mgr.Create(ctx, meta, false)

	mgr.InsertRow(ctx, "users", map[string]any{"id": "u1", "name": "alice"}, "u1")
	mgr.InsertRow(ctx, "users", map[string]any{"id": "u2", "name": "alice"}, "u2")

	metaPtr, _ := mgr.Get("idx_name")
	ids, _ := mgr.LookupEqual(ctx, metaPtr, "alice")
	if len(ids) != 2 {
		t.Fatalf("expected 2 before delete, got %d", len(ids))
	}

	mgr.DeleteRow(ctx, "users", map[string]any{"id": "u1", "name": "alice"}, "u1")

	ids2, _ := mgr.LookupEqual(ctx, metaPtr, "alice")
	if len(ids2) != 1 {
		t.Errorf("expected 1 after delete, got %d", len(ids2))
	}
}

func TestOps_UpdateRow(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_name", Table: "users", Column: "name", Type: TypeHash}
	mgr.Create(ctx, meta, false)

	old := map[string]any{"id": "u1", "name": "alice"}
	new := map[string]any{"id": "u1", "name": "bob"}
	mgr.InsertRow(ctx, "users", old, "u1")

	metaPtr, _ := mgr.Get("idx_name")
	ids, _ := mgr.LookupEqual(ctx, metaPtr, "alice")
	if len(ids) != 1 {
		t.Fatalf("expected 1 alice before update, got %d", len(ids))
	}

	mgr.UpdateRow(ctx, "users", old, new, "u1")

	ids2, _ := mgr.LookupEqual(ctx, metaPtr, "alice")
	if len(ids2) != 0 {
		t.Errorf("expected 0 alice after update, got %d", len(ids2))
	}

	ids3, _ := mgr.LookupEqual(ctx, metaPtr, "bob")
	if len(ids3) != 1 {
		t.Errorf("expected 1 bob after update, got %d", len(ids3))
	}
}

func TestOps_RebuildFromRows(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_name", Table: "users", Column: "name", Type: TypeBTree}
	mgr.Create(ctx, meta, false)

	rows := []map[string]any{
		{"id": "u1", "name": "alice"},
		{"id": "u2", "name": "bob"},
		{"id": "u3", "name": "charlie"},
	}
	ids := []string{"u1", "u2", "u3"}

	metaPtr, _ := mgr.Get("idx_name")
	if err := mgr.RebuildFromRows(ctx, metaPtr, rows, ids); err != nil {
		t.Fatal(err)
	}

	found, _ := mgr.LookupEqual(ctx, metaPtr, "bob")
	if len(found) != 1 || found[0] != "u2" {
		t.Errorf("expected [u2], got %v", found)
	}

	// 范围查询: alice < x ≤ charlie → bob, charlie
	found2, _ := mgr.LookupRange(ctx, metaPtr, "alice", "charlie", false, true)
	if len(found2) != 2 {
		t.Errorf("expected 2 in (alice, charlie], got %d: %v", len(found2), found2)
	}
}

func TestOps_InvertedChinese(t *testing.T) {
	mgr, ctx := setupTestIndex(t)

	meta := Meta{Name: "idx_body", Table: "docs", Column: "body", Type: TypeInverted}
	mgr.Create(ctx, meta, false)

	rows := []map[string]any{
		{"id": "d1", "body": "北京天气很好"},
		{"id": "d2", "body": "上海是个好地方"},
		{"id": "d3", "body": "北京欢迎你"},
	}
	for _, r := range rows {
		mgr.InsertRow(ctx, "docs", r, r["id"].(string))
	}

	metaPtr, _ := mgr.Get("idx_body")

	// "北京" → d1, d3
	ids, _ := mgr.Match(ctx, metaPtr, "北京")
	if len(ids) != 2 {
		t.Errorf("expected 2 for '北京', got %d: %v", len(ids), ids)
	}

	// "好" → d1, d2
	ids2, _ := mgr.Match(ctx, metaPtr, "好")
	if len(ids2) != 2 {
		t.Errorf("expected 2 for '好', got %d: %v", len(ids2), ids2)
	}

	// "北京" AND "好" → d1
	ids3, _ := mgr.Match(ctx, metaPtr, "北京 好")
	if len(ids3) != 1 {
		t.Errorf("expected 1 for '北京 好', got %d: %v", len(ids3), ids3)
	}
}
