package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TableMetadata 表元数据
type TableMetadata struct {
	Name    string       `json:"name"`
	Columns []ColumnMeta `json:"columns"`
	Prefix  byte         `json:"prefix"`
}

// ColumnMeta 列元数据
type ColumnMeta struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Length     int    `json:"length,omitempty"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Default    any    `json:"default,omitempty"`
}

// TableManager 表管理器
type TableManager struct {
	engine  Engine
	mu      sync.RWMutex
	tables  map[string]*TableMetadata
	nextPrefix byte
}

// 用户表元数据键前缀（在 PrefixSystem 命名空间下）
const (
	tableMetaKeyPrefix   = "table:"
	userTablePrefixStart = 0x30 // 用户表存储前缀起始（避免与内置表冲突）
)

// NewTableManager 创建表管理器
func NewTableManager(engine Engine) *TableManager {
	return &TableManager{
		engine:     engine,
		tables:     make(map[string]*TableMetadata),
		nextPrefix: userTablePrefixStart,
	}
}

// Init 初始化表管理器，从存储中加载表元数据
func (tm *TableManager) Init(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 只加载 [PrefixSystem]"table:" 前缀的元数据，避免与 room/task/audit 等其他系统数据混淆
	prefix := EncodeKey(PrefixSystem, tableMetaKeyPrefix)
	iter, err := tm.engine.PrefixScan(ctx, prefix, ScanOptions{})
	if err != nil {
		return fmt.Errorf("load table metadata: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		_, val := iter.Item()
		var meta TableMetadata
		if err := json.Unmarshal(val, &meta); err != nil {
			continue
		}
		// 二次校验：仅接受具备名称与合法用户前缀的记录
		if meta.Name == "" || meta.Prefix < userTablePrefixStart {
			continue
		}
		tm.tables[meta.Name] = &meta
		if meta.Prefix >= tm.nextPrefix {
			tm.nextPrefix = meta.Prefix + 1
		}
	}

	// 注册内置表
	tm.registerBuiltinTables()

	return nil
}

// registerBuiltinTables 注册内置表
func (tm *TableManager) registerBuiltinTables() {
	builtins := []TableMetadata{
		{
			Name:   "agent_sessions",
			Prefix: PrefixSession,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "agent_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "updated_at", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "status", Type: "VARCHAR", Length: 16, Nullable: false},
				{Name: "context", Type: "TEXT", Nullable: true},
			},
		},
		{
			Name:   "agent_memories",
			Prefix: PrefixMemory,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "agent_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "session_id", Type: "VARCHAR", Length: 64, Nullable: true},
				{Name: "type", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "content", Type: "TEXT", Nullable: false},
				{Name: "importance", Type: "FLOAT", Nullable: false},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
			},
		},
		{
			Name:   "agent_decisions",
			Prefix: PrefixDecision,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "agent_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "session_id", Type: "VARCHAR", Length: 64, Nullable: true},
				{Name: "decision_type", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "reasoning", Type: "TEXT", Nullable: true},
				{Name: "action", Type: "TEXT", Nullable: false},
				{Name: "result", Type: "TEXT", Nullable: true},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
			},
		},
		{
			Name:   "knowledge_entities",
			Prefix: PrefixEntity,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "name", Type: "VARCHAR", Length: 256, Nullable: false},
				{Name: "type", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "properties", Type: "TEXT", Nullable: true},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "updated_at", Type: "VARCHAR", Length: 32, Nullable: false},
			},
		},
		{
			Name:   "knowledge_relations",
			Prefix: PrefixRelation,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "source_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "target_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "relation_type", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "properties", Type: "TEXT", Nullable: true},
				{Name: "weight", Type: "FLOAT", Nullable: false},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
			},
		},
		{
			Name:   "data_lineage",
			Prefix: PrefixLineage,
			Columns: []ColumnMeta{
				{Name: "id", Type: "VARCHAR", Length: 64, Nullable: false, PrimaryKey: true},
				{Name: "source_type", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "source_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "target_type", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "target_id", Type: "VARCHAR", Length: 64, Nullable: false},
				{Name: "operation", Type: "VARCHAR", Length: 32, Nullable: false},
				{Name: "metadata", Type: "TEXT", Nullable: true},
				{Name: "created_at", Type: "VARCHAR", Length: 32, Nullable: false},
			},
		},
	}

	for i := range builtins {
		meta := &builtins[i]
		if _, exists := tm.tables[meta.Name]; !exists {
			tm.tables[meta.Name] = meta
		}
	}
}

// CreateTable 创建表
func (tm *TableManager) CreateTable(ctx context.Context, name string, columns []ColumnMeta, ifNotExists bool) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查表是否已存在
	if _, exists := tm.tables[name]; exists {
		if ifNotExists {
			return nil
		}
		return fmt.Errorf("table %s already exists", name)
	}

	// 分配前缀
	prefix := tm.nextPrefix
	tm.nextPrefix++

	// 创建表元数据
	meta := &TableMetadata{
		Name:    name,
		Columns: columns,
		Prefix:  prefix,
	}

	// 保存到存储
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal table metadata: %w", err)
	}

	key := EncodeKey(PrefixSystem, tableMetaKeyPrefix+name)
	if err := tm.engine.Set(ctx, key, data); err != nil {
		return fmt.Errorf("save table metadata: %w", err)
	}

	tm.tables[name] = meta
	return nil
}

// DropTable 删除表
func (tm *TableManager) DropTable(ctx context.Context, name string, ifExists bool) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查表是否存在
	meta, exists := tm.tables[name]
	if !exists {
		if ifExists {
			return nil
		}
		return fmt.Errorf("table %s does not exist", name)
	}

	// 检查是否是内置表
	if isBuiltinTable(name) {
		return fmt.Errorf("cannot drop builtin table %s", name)
	}

	// 删除表中的所有数据
	start, end := PrefixRange([]byte{meta.Prefix})
	iter, err := tm.engine.Scan(ctx, start, end, ScanOptions{})
	if err != nil {
		return fmt.Errorf("scan table data: %w", err)
	}
	defer iter.Close()

	var keys [][]byte
	for iter.Next() {
		key, _ := iter.Item()
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		keys = append(keys, keyCopy)
	}

	// 批量删除
	ops := make([]WriteOp, len(keys))
	for i, key := range keys {
		ops[i] = WriteOp{Type: OpDelete, Key: key}
	}
	if err := tm.engine.BatchWrite(ctx, ops); err != nil {
		return fmt.Errorf("delete table data: %w", err)
	}

	// 删除表元数据
	metaKey := EncodeKey(PrefixSystem, tableMetaKeyPrefix+name)
	if err := tm.engine.Delete(ctx, metaKey); err != nil {
		return fmt.Errorf("delete table metadata: %w", err)
	}

	delete(tm.tables, name)
	return nil
}

// AlterTable 修改表结构
func (tm *TableManager) AlterTable(ctx context.Context, name string, action AlterAction) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查表是否存在
	meta, exists := tm.tables[name]
	if !exists {
		return fmt.Errorf("table %s does not exist", name)
	}

	// 检查是否是内置表
	if isBuiltinTable(name) {
		return fmt.Errorf("cannot alter builtin table %s", name)
	}

	switch a := action.(type) {
	case *AddColumnAction:
		// 检查列是否已存在
		for _, col := range meta.Columns {
			if col.Name == a.Column.Name {
				return fmt.Errorf("column %s already exists in table %s", a.Column.Name, name)
			}
		}
		meta.Columns = append(meta.Columns, a.Column)

	case *DropColumnAction:
		// 检查列是否存在
		found := false
		for i, col := range meta.Columns {
			if col.Name == a.Column {
				// 不能删除主键列
				if col.PrimaryKey {
					return fmt.Errorf("cannot drop primary key column %s", a.Column)
				}
				meta.Columns = append(meta.Columns[:i], meta.Columns[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("column %s does not exist in table %s", a.Column, name)
		}

	case *ModifyColumnAction:
		// 查找并修改列
		found := false
		for i, col := range meta.Columns {
			if col.Name == a.Column.Name {
				// 不能修改主键列的类型
				if col.PrimaryKey && col.Type != a.Column.Type {
					return fmt.Errorf("cannot change type of primary key column %s", a.Column.Name)
				}
				meta.Columns[i] = a.Column
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("column %s does not exist in table %s", a.Column.Name, name)
		}

	default:
		return fmt.Errorf("unsupported alter action: %T", action)
	}

	// 保存更新后的元数据
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal table metadata: %w", err)
	}

	key := EncodeKey(PrefixSystem, tableMetaKeyPrefix+name)
	if err := tm.engine.Set(ctx, key, data); err != nil {
		return fmt.Errorf("save table metadata: %w", err)
	}

	return nil
}

// GetTable 获取表元数据
func (tm *TableManager) GetTable(name string) (*TableMetadata, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	meta, exists := tm.tables[name]
	return meta, exists
}

// ListTables 列出所有表
func (tm *TableManager) ListTables() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tables := make([]string, 0, len(tm.tables))
	for name := range tm.tables {
		tables = append(tables, name)
	}
	return tables
}

// GetTablePrefix 获取表前缀
func (tm *TableManager) GetTablePrefix(name string) (byte, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	meta, exists := tm.tables[name]
	if !exists {
		return 0, false
	}
	return meta.Prefix, true
}

// AlterAction 变更操作类型
type AlterAction interface {
	alterActionNode()
}

// AddColumnAction ADD COLUMN 操作
type AddColumnAction struct {
	Column ColumnMeta
}

func (a *AddColumnAction) alterActionNode() {}

// DropColumnAction DROP COLUMN 操作
type DropColumnAction struct {
	Column string
}

func (a *DropColumnAction) alterActionNode() {}

// ModifyColumnAction MODIFY COLUMN 操作
type ModifyColumnAction struct {
	Column ColumnMeta
}

func (a *ModifyColumnAction) alterActionNode() {}

// isBuiltinTable 检查是否是内置表
func isBuiltinTable(name string) bool {
	builtinTables := []string{
		"agent_sessions", "sessions",
		"agent_memories", "memories",
		"agent_decisions", "decisions",
		"knowledge_entities", "entities",
		"knowledge_relations", "relations",
		"data_lineage", "lineage",
	}

	lowerName := strings.ToLower(name)
	for _, t := range builtinTables {
		if t == lowerName {
			return true
		}
	}
	return false
}
