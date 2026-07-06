package storage_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type tableManagerTestEnv struct {
	engine *badgerstore.BadgerEngine
	tm     *storage.TableManager
	ctx    context.Context
}

func newTableManagerTestEnv(t *testing.T) *tableManagerTestEnv {
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

	tm := storage.NewTableManager(engine)
	if err := tm.Init(context.Background()); err != nil {
		t.Fatalf("init table manager: %v", err)
	}

	return &tableManagerTestEnv{
		engine: engine,
		tm:     tm,
		ctx:    context.Background(),
	}
}

// ========== Builtin Tables Tests ==========

func TestTableManager_BuiltinTables(t *testing.T) {
	env := newTableManagerTestEnv(t)

	expectedBuiltinTables := []string{
		"agent_sessions",
		"agent_memories",
		"agent_decisions",
		"knowledge_entities",
		"knowledge_relations",
		"data_lineage",
	}

	for _, name := range expectedBuiltinTables {
		meta, exists := env.tm.GetTable(name)
		if !exists {
			t.Fatalf("expected builtin table %s to exist", name)
		}
		if len(meta.Columns) == 0 {
			t.Fatalf("table %s should have columns", name)
		}
	}
}

func TestTableManager_BuiltinTablePrefixes(t *testing.T) {
	env := newTableManagerTestEnv(t)

	prefix, ok := env.tm.GetTablePrefix("agent_sessions")
	if !ok || prefix != storage.PrefixSession {
		t.Fatalf("agent_sessions: expected prefix %d, got %d", storage.PrefixSession, prefix)
	}

	prefix, ok = env.tm.GetTablePrefix("agent_memories")
	if !ok || prefix != storage.PrefixMemory {
		t.Fatalf("agent_memories: expected prefix %d, got %d", storage.PrefixMemory, prefix)
	}

	prefix, ok = env.tm.GetTablePrefix("agent_decisions")
	if !ok || prefix != storage.PrefixDecision {
		t.Fatalf("agent_decisions: expected prefix %d, got %d", storage.PrefixDecision, prefix)
	}
}

func TestTableManager_BuiltinTableColumnsMatchJSON(t *testing.T) {
	env := newTableManagerTestEnv(t)

	assertColumns := func(table string, want []string, absent []string) {
		t.Helper()
		meta, exists := env.tm.GetTable(table)
		if !exists {
			t.Fatalf("expected builtin table %s to exist", table)
		}
		got := make(map[string]bool, len(meta.Columns))
		for _, col := range meta.Columns {
			got[col.Name] = true
		}
		for _, name := range want {
			if !got[name] {
				t.Fatalf("%s: expected column %s", table, name)
			}
		}
		for _, name := range absent {
			if got[name] {
				t.Fatalf("%s: unexpected stale column %s", table, name)
			}
		}
	}

	assertColumns("agent_sessions", []string{"state", "metadata"}, []string{"status"})
	assertColumns("agent_memories", []string{"session_id", "access_count", "accessed_at"}, []string{"agent_id"})
	assertColumns("agent_decisions", []string{"type", "input", "output", "duration_ms"}, []string{"agent_id", "decision_type", "action", "result"})
}

func TestTableManager_ListTables(t *testing.T) {
	env := newTableManagerTestEnv(t)

	tables := env.tm.ListTables()
	if len(tables) < 6 {
		t.Fatalf("expected at least 6 tables, got %d", len(tables))
	}
}

// ========== Create Table Tests ==========

func TestTableManager_CreateTable(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128, Nullable: false},
		{Name: "description", Type: "TEXT", Nullable: true},
		{Name: "score", Type: "FLOAT", Nullable: true},
	}

	if err := env.tm.CreateTable(env.ctx, "my_table", columns, false); err != nil {
		t.Fatalf("create table: %v", err)
	}

	meta, exists := env.tm.GetTable("my_table")
	if !exists {
		t.Fatal("expected table to exist after creation")
	}
	if len(meta.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(meta.Columns))
	}
	if meta.Columns[0].Name != "id" {
		t.Fatalf("expected first column 'id', got %q", meta.Columns[0].Name)
	}
	if meta.Columns[0].PrimaryKey != true {
		t.Fatal("expected first column to be primary key")
	}
}

func TestTableManager_CreateTableDuplicate(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}

	env.tm.CreateTable(env.ctx, "dup_table", columns, false)
	err := env.tm.CreateTable(env.ctx, "dup_table", columns, false)
	if err == nil {
		t.Fatal("expected error for duplicate table")
	}
}

func TestTableManager_CreateTableIfNotExists(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}

	env.tm.CreateTable(env.ctx, "safe_table", columns, false)
	err := env.tm.CreateTable(env.ctx, "safe_table", columns, true)
	if err != nil {
		t.Fatalf("expected no error with IF NOT EXISTS, got %v", err)
	}
}

func TestTableManager_CreateTableAndGetPrefix(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}

	env.tm.CreateTable(env.ctx, "prefix_test", columns, false)

	prefix, ok := env.tm.GetTablePrefix("prefix_test")
	if !ok {
		t.Fatal("expected prefix to exist")
	}
	if prefix < 0x30 {
		t.Fatalf("expected prefix >= 0x30, got 0x%02x", prefix)
	}
}

func TestTableManager_CreateTablePrefixExhaustion(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}
	for i := 0; i < 0xFE-0x30+1; i++ {
		if err := env.tm.CreateTable(env.ctx, fmt.Sprintf("table_%03d", i), columns, false); err != nil {
			t.Fatalf("create table %d: %v", i, err)
		}
	}
	err := env.tm.CreateTable(env.ctx, "overflow", columns, false)
	if err == nil {
		t.Fatal("expected prefix exhaustion error")
	}
}

// ========== Drop Table Tests ==========

func TestTableManager_DropTable(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
	}

	env.tm.CreateTable(env.ctx, "drop_test", columns, false)

	if err := env.tm.DropTable(env.ctx, "drop_test", false); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	_, exists := env.tm.GetTable("drop_test")
	if exists {
		t.Fatal("expected table to be dropped")
	}
}

func TestTableManager_DropTableIfExists(t *testing.T) {
	env := newTableManagerTestEnv(t)

	err := env.tm.DropTable(env.ctx, "nonexistent", true)
	if err != nil {
		t.Fatalf("expected no error with IF EXISTS, got %v", err)
	}
}

func TestTableManager_DropTableNotFound(t *testing.T) {
	env := newTableManagerTestEnv(t)

	err := env.tm.DropTable(env.ctx, "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}

func TestTableManager_DropBuiltinTable(t *testing.T) {
	env := newTableManagerTestEnv(t)

	err := env.tm.DropTable(env.ctx, "agent_sessions", false)
	if err == nil {
		t.Fatal("expected error when dropping builtin table")
	}
}

// ========== Alter Table Tests ==========

func TestTableManager_AlterTableAddColumn(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
	}
	env.tm.CreateTable(env.ctx, "alter_test", columns, false)

	action := &storage.AddColumnAction{
		Column: storage.ColumnMeta{
			Name:     "email",
			Type:     "VARCHAR",
			Length:   256,
			Nullable: true,
		},
	}

	if err := env.tm.AlterTable(env.ctx, "alter_test", action); err != nil {
		t.Fatalf("add column: %v", err)
	}

	meta, _ := env.tm.GetTable("alter_test")
	if len(meta.Columns) != 3 {
		t.Fatalf("expected 3 columns after add, got %d", len(meta.Columns))
	}
	if meta.Columns[2].Name != "email" {
		t.Fatalf("expected third column 'email', got %q", meta.Columns[2].Name)
	}
}

func TestTableManager_AlterTableAddDuplicateColumn(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
	}
	env.tm.CreateTable(env.ctx, "alter_dup", columns, false)

	action := &storage.AddColumnAction{
		Column: storage.ColumnMeta{Name: "name", Type: "VARCHAR", Length: 256},
	}

	err := env.tm.AlterTable(env.ctx, "alter_dup", action)
	if err == nil {
		t.Fatal("expected error when adding duplicate column")
	}
}

func TestTableManager_AlterTableDropColumn(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
		{Name: "email", Type: "VARCHAR", Length: 256, Nullable: true},
	}
	env.tm.CreateTable(env.ctx, "alter_drop", columns, false)

	action := &storage.DropColumnAction{Column: "email"}

	if err := env.tm.AlterTable(env.ctx, "alter_drop", action); err != nil {
		t.Fatalf("drop column: %v", err)
	}

	meta, _ := env.tm.GetTable("alter_drop")
	if len(meta.Columns) != 2 {
		t.Fatalf("expected 2 columns after drop, got %d", len(meta.Columns))
	}
}

func TestTableManager_AlterTableDropPrimaryKey(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
	}
	env.tm.CreateTable(env.ctx, "alter_pk", columns, false)

	action := &storage.DropColumnAction{Column: "id"}

	err := env.tm.AlterTable(env.ctx, "alter_pk", action)
	if err == nil {
		t.Fatal("expected error when dropping primary key column")
	}
}

func TestTableManager_AlterTableDropNonExistentColumn(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}
	env.tm.CreateTable(env.ctx, "alter_ne", columns, false)

	action := &storage.DropColumnAction{Column: "nonexistent"}

	err := env.tm.AlterTable(env.ctx, "alter_ne", action)
	if err == nil {
		t.Fatal("expected error when dropping nonexistent column")
	}
}

func TestTableManager_AlterTableModifyColumn(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "name", Type: "VARCHAR", Length: 128},
	}
	env.tm.CreateTable(env.ctx, "alter_modify", columns, false)

	action := &storage.ModifyColumnAction{
		Column: storage.ColumnMeta{
			Name:     "name",
			Type:     "VARCHAR",
			Length:   256,
			Nullable: true,
		},
	}

	if err := env.tm.AlterTable(env.ctx, "alter_modify", action); err != nil {
		t.Fatalf("modify column: %v", err)
	}

	meta, _ := env.tm.GetTable("alter_modify")
	if meta.Columns[1].Length != 256 {
		t.Fatalf("expected length 256, got %d", meta.Columns[1].Length)
	}
	if meta.Columns[1].Nullable != true {
		t.Fatal("expected nullable to be true")
	}
}

func TestTableManager_AlterTableModifyPrimaryKeyType(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}
	env.tm.CreateTable(env.ctx, "alter_pk_type", columns, false)

	action := &storage.ModifyColumnAction{
		Column: storage.ColumnMeta{Name: "id", Type: "INT", PrimaryKey: true},
	}

	err := env.tm.AlterTable(env.ctx, "alter_pk_type", action)
	if err == nil {
		t.Fatal("expected error when modifying primary key type")
	}
}

func TestTableManager_AlterTableNonExistentTable(t *testing.T) {
	env := newTableManagerTestEnv(t)

	action := &storage.AddColumnAction{
		Column: storage.ColumnMeta{Name: "col", Type: "VARCHAR", Length: 64},
	}

	err := env.tm.AlterTable(env.ctx, "nonexistent", action)
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}

func TestTableManager_AlterBuiltinTable(t *testing.T) {
	env := newTableManagerTestEnv(t)

	action := &storage.AddColumnAction{
		Column: storage.ColumnMeta{Name: "extra", Type: "VARCHAR", Length: 64},
	}

	err := env.tm.AlterTable(env.ctx, "agent_sessions", action)
	if err == nil {
		t.Fatal("expected error when altering builtin table")
	}
}

func TestTableManager_AlterUnsupportedAction(t *testing.T) {
	env := newTableManagerTestEnv(t)

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
	}
	env.tm.CreateTable(env.ctx, "alter_unsupported", columns, false)

	err := env.tm.AlterTable(env.ctx, "alter_unsupported", nil)
	if err == nil {
		t.Fatal("expected error for unsupported alter action")
	}
}

// ========== Get Table Tests ==========

func TestTableManager_GetTableNotFound(t *testing.T) {
	env := newTableManagerTestEnv(t)

	_, exists := env.tm.GetTable("nonexistent")
	if exists {
		t.Fatal("expected table to not exist")
	}
}

func TestTableManager_GetTablePrefixNotFound(t *testing.T) {
	env := newTableManagerTestEnv(t)

	_, ok := env.tm.GetTablePrefix("nonexistent")
	if ok {
		t.Fatal("expected prefix to not exist")
	}
}

// ========== Persistence Tests ==========

func TestTableManager_Persistence(t *testing.T) {
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)

	tm := storage.NewTableManager(engine)
	tm.Init(context.Background())

	columns := []storage.ColumnMeta{
		{Name: "id", Type: "VARCHAR", Length: 64, PrimaryKey: true},
		{Name: "data", Type: "TEXT"},
	}
	tm.CreateTable(context.Background(), "persist_test", columns, false)

	engine.Close()

	engine2 := badgerstore.New()
	opts2 := storage.DefaultOptions()
	opts2.DataDir = dir
	opts2.SyncWrites = false
	engine2.Open(opts2)
	defer engine2.Close()

	tm2 := storage.NewTableManager(engine2)
	if err := tm2.Init(context.Background()); err != nil {
		t.Fatalf("init after reopen: %v", err)
	}

	meta, exists := tm2.GetTable("persist_test")
	if !exists {
		t.Fatal("expected table to persist after reopen")
	}
	if len(meta.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(meta.Columns))
	}
}
