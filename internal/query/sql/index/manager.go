package index

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// Type \u7d22\u5f15\u7c7b\u578b
type Type string

const (
	TypeHash     Type = "HASH"
	TypeBTree    Type = "BTREE"
	TypeInverted Type = "INVERTED"
)

// Meta \u7d22\u5f15\u5143\u6570\u636e\uff08\u6301\u4e45\u5316\uff09
type Meta struct {
	Name    string `json:"name"`
	Table   string `json:"table"`
	Column  string `json:"column"`
	Type    Type   `json:"type"`
	Unique  bool   `json:"unique,omitempty"`
}

// Manager \u7d22\u5f15\u7ba1\u7406\u5668\uff1a\u7ef4\u62a4\u540d\u79f0\u2192\u5143\u6570\u636e\u6620\u5c04\u53ca\u8868\u2192\u7d22\u5f15\u5217\u8868\u3002
type Manager struct {
	engine storage.Engine
	mu     sync.RWMutex
	byName map[string]*Meta
}

const (
	sysKeyPrefix   = "index:"       // PrefixSystem \u4e0b\u7684\u5143\u6570\u636e\u524d\u7f00
	dataHashPrefix = "h:"           // PrefixIndex \u4e0b Hash \u6570\u636e\u524d\u7f00
	dataBTPrefix   = "b:"           // PrefixIndex \u4e0b BTree \u6570\u636e\u524d\u7f00
	dataInvPrefix  = "i:"           // PrefixIndex \u4e0b Inverted \u6570\u636e\u524d\u7f00
	sep            = byte(0x00)
)

// NewManager \u521b\u5efa\u7d22\u5f15\u7ba1\u7406\u5668
func NewManager(engine storage.Engine) *Manager {
	return &Manager{
		engine: engine,
		byName: make(map[string]*Meta),
	}
}

// Init \u4ece\u5b58\u50a8\u52a0\u8f7d\u5df2\u6709\u7d22\u5f15\u5143\u6570\u636e
func (m *Manager) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := storage.EncodeKey(storage.PrefixSystem, sysKeyPrefix)
	iter, err := m.engine.PrefixScan(ctx, prefix, storage.ScanOptions{})
	if err != nil {
		return fmt.Errorf("load index metadata: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		_, val := iter.Item()
		var meta Meta
		if err := json.Unmarshal(val, &meta); err != nil {
			continue
		}
		if meta.Name == "" || meta.Table == "" || meta.Column == "" {
			continue
		}
		m.byName[meta.Name] = &meta
	}
	return nil
}

// Create \u521b\u5efa\u7d22\u5f15\u3002\u5982\u9700\u6784\u5efa\u5df2\u6709\u6570\u636e\uff0c\u7531\u8c03\u7528\u65b9\u5728\u5916\u90e8\u5b8c\u6210\u3002
func (m *Manager) Create(ctx context.Context, meta Meta, ifNotExists bool) error {
	if meta.Name == "" || meta.Table == "" || meta.Column == "" {
		return fmt.Errorf("index name/table/column are required")
	}
	if meta.Type == "" {
		meta.Type = TypeBTree
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.byName[meta.Name]; ok {
		if ifNotExists {
			return nil
		}
		return fmt.Errorf("index %s already exists", meta.Name)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	key := storage.EncodeKey(storage.PrefixSystem, sysKeyPrefix+meta.Name)
	if err := m.engine.Set(ctx, key, data); err != nil {
		return err
	}
	cp := meta
	m.byName[meta.Name] = &cp
	return nil
}

// Drop \u5220\u9664\u7d22\u5f15\u5143\u6570\u636e\u4e0e\u6570\u636e\u9879
func (m *Manager) Drop(ctx context.Context, name string, ifExists bool) error {
	m.mu.Lock()
	meta, ok := m.byName[name]
	if !ok {
		m.mu.Unlock()
		if ifExists {
			return nil
		}
		return fmt.Errorf("index %s not found", name)
	}
	delete(m.byName, name)
	m.mu.Unlock()

	// \u5220\u9664\u5143\u6570\u636e
	metaKey := storage.EncodeKey(storage.PrefixSystem, sysKeyPrefix+name)
	if err := m.engine.Delete(ctx, metaKey); err != nil {
		return err
	}

	// \u5220\u9664\u6570\u636e\u9879
	dataPrefix := indexDataPrefix(meta)
	iter, err := m.engine.PrefixScan(ctx, dataPrefix, storage.ScanOptions{})
	if err != nil {
		return err
	}
	var ops []storage.WriteOp
	for iter.Next() {
		k, _ := iter.Item()
		kc := make([]byte, len(k))
		copy(kc, k)
		ops = append(ops, storage.WriteOp{Type: storage.OpDelete, Key: kc})
	}
	iter.Close()
	if len(ops) == 0 {
		return nil
	}
	return m.engine.BatchWrite(ctx, ops)
}

// Get \u83b7\u53d6\u7d22\u5f15\u5143\u6570\u636e
func (m *Manager) Get(name string) (*Meta, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	meta, ok := m.byName[name]
	return meta, ok
}

// ListAll \u5217\u51fa\u6240\u6709\u7d22\u5f15
func (m *Manager) ListAll() []*Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Meta, 0, len(m.byName))
	for _, v := range m.byName {
		out = append(out, v)
	}
	return out
}

// ListByTable \u5217\u51fa\u67d0\u5f20\u8868\u4e0a\u7684\u6240\u6709\u7d22\u5f15
func (m *Manager) ListByTable(table string) []*Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Meta
	for _, v := range m.byName {
		if strings.EqualFold(v.Table, table) {
			out = append(out, v)
		}
	}
	return out
}

// FindByColumn \u67e5\u627e\u53ef\u7528\u4e8e\u67d0\u5217\u7684\u7d22\u5f15
func (m *Manager) FindByColumn(table, column string) []*Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Meta
	for _, v := range m.byName {
		if strings.EqualFold(v.Table, table) && strings.EqualFold(v.Column, column) {
			out = append(out, v)
		}
	}
	return out
}

// indexDataPrefix \u8fd4\u56de\u67d0\u4e2a\u7d22\u5f15\u7684\u6570\u636e key \u524d\u7f00\uff08\u4e0d\u5305\u542b\u503c\u4e0e rowID\uff09
func indexDataPrefix(meta *Meta) []byte {
	var head string
	switch meta.Type {
	case TypeHash:
		head = dataHashPrefix
	case TypeInverted:
		head = dataInvPrefix
	default:
		head = dataBTPrefix
	}
	// [PrefixIndex]"h:"<name>0x00
	buf := make([]byte, 0, 2+len(head)+len(meta.Name)+1)
	buf = append(buf, storage.PrefixIndex)
	buf = append(buf, head...)
	buf = append(buf, meta.Name...)
	buf = append(buf, sep)
	return buf
}

// entryKey \u62fc\u63a5\u5b8c\u6570\u636e key\uff1a<dataPrefix><encodedValue|term>0x00<rowID>
func entryKey(meta *Meta, valueOrTerm []byte, rowID string) []byte {
	prefix := indexDataPrefix(meta)
	buf := make([]byte, 0, len(prefix)+len(valueOrTerm)+1+len(rowID))
	buf = append(buf, prefix...)
	buf = append(buf, valueOrTerm...)
	buf = append(buf, sep)
	buf = append(buf, rowID...)
	return buf
}

// entryKeyPrefix \u62fc\u63a5\u65e0 rowID \u7684\u524d\u7f00\uff1a<dataPrefix><encodedValue|term>0x00
func entryKeyPrefix(meta *Meta, valueOrTerm []byte) []byte {
	prefix := indexDataPrefix(meta)
	buf := make([]byte, 0, len(prefix)+len(valueOrTerm)+1)
	buf = append(buf, prefix...)
	buf = append(buf, valueOrTerm...)
	buf = append(buf, sep)
	return buf
}
