package index

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// InsertRow 将某行在所有相关索引中的条目写入
func (m *Manager) InsertRow(ctx context.Context, table string, row map[string]any, rowID string) error {
	ops, err := m.InsertRowOps(ctx, table, row, rowID)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	return m.engine.BatchWrite(ctx, ops)
}

// DeleteRow 将某行在所有相关索引中的条目删除
func (m *Manager) DeleteRow(ctx context.Context, table string, row map[string]any, rowID string) error {
	ops := m.DeleteRowOps(table, row, rowID)
	if len(ops) == 0 {
		return nil
	}
	return m.engine.BatchWrite(ctx, ops)
}

// UpdateRow 更新时：先删旧值再写新值
func (m *Manager) UpdateRow(ctx context.Context, table string, oldRow, newRow map[string]any, rowID string) error {
	ops, err := m.UpdateRowOps(ctx, table, oldRow, newRow, rowID)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	return m.engine.BatchWrite(ctx, ops)
}

// InsertRowOps 返回插入某行所需的索引写操作，并执行唯一索引校验。
func (m *Manager) InsertRowOps(ctx context.Context, table string, row map[string]any, rowID string) ([]storage.WriteOp, error) {
	metas := m.ListByTable(table)
	if len(metas) == 0 {
		return nil, nil
	}
	var ops []storage.WriteOp
	for _, meta := range metas {
		val, ok := row[meta.Column]
		if !ok {
			continue
		}
		if err := m.validateUnique(ctx, meta, val, rowID); err != nil {
			return nil, err
		}
		for _, k := range keysForValue(meta, val, rowID) {
			ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: k, Value: []byte{1}})
		}
	}
	return ops, nil
}

// DeleteRowOps 返回删除某行所需的索引写操作。
func (m *Manager) DeleteRowOps(table string, row map[string]any, rowID string) []storage.WriteOp {
	metas := m.ListByTable(table)
	if len(metas) == 0 {
		return nil
	}
	var ops []storage.WriteOp
	for _, meta := range metas {
		val, ok := row[meta.Column]
		if !ok {
			continue
		}
		for _, k := range keysForValue(meta, val, rowID) {
			ops = append(ops, storage.WriteOp{Type: storage.OpDelete, Key: k})
		}
	}
	return ops
}

// UpdateRowOps 返回更新某行所需的索引写操作，并执行唯一索引校验。
func (m *Manager) UpdateRowOps(ctx context.Context, table string, oldRow, newRow map[string]any, rowID string) ([]storage.WriteOp, error) {
	insertOps, err := m.InsertRowOps(ctx, table, newRow, rowID)
	if err != nil {
		return nil, err
	}
	deleteOps := m.DeleteRowOps(table, oldRow, rowID)
	ops := make([]storage.WriteOp, 0, len(deleteOps)+len(insertOps))
	ops = append(ops, deleteOps...)
	ops = append(ops, insertOps...)
	return ops, nil
}

// RebuildFromRows 用已有行数据重建某索引（用于 CREATE INDEX 之后回填）
func (m *Manager) RebuildFromRows(ctx context.Context, meta *Meta, rows []map[string]any, rowIDs []string) error {
	if len(rows) != len(rowIDs) {
		return nil
	}
	var ops []storage.WriteOp
	seen := make(map[string]string, len(rows))
	for i, row := range rows {
		val, ok := row[meta.Column]
		if !ok {
			continue
		}
		if err := validateUniqueInBatch(meta, val, rowIDs[i], seen); err != nil {
			return err
		}
		for _, k := range keysForValue(meta, val, rowIDs[i]) {
			ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: k, Value: []byte{1}})
		}
	}
	if len(ops) == 0 {
		return nil
	}
	return m.engine.BatchWrite(ctx, ops)
}

func (m *Manager) validateUnique(ctx context.Context, meta *Meta, val any, rowID string) error {
	if !meta.Unique {
		return nil
	}
	if meta.Type == TypeInverted {
		return fmt.Errorf("unique inverted index %s is not supported", meta.Name)
	}
	ids, err := m.LookupEqual(ctx, meta, val)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id != rowID {
			return fmt.Errorf("unique index %s violation on %s", meta.Name, meta.Column)
		}
	}
	return nil
}

func validateUniqueInBatch(meta *Meta, val any, rowID string, seen map[string]string) error {
	if !meta.Unique {
		return nil
	}
	if meta.Type == TypeInverted {
		return fmt.Errorf("unique inverted index %s is not supported", meta.Name)
	}
	key := string(EncodeValue(val))
	if prev, ok := seen[key]; ok && prev != rowID {
		return fmt.Errorf("unique index %s violation on %s", meta.Name, meta.Column)
	}
	seen[key] = rowID
	return nil
}

// keysForValue 根据索引类型和列值生成需要写入的 key 集合
func keysForValue(meta *Meta, val any, rowID string) [][]byte {
	switch meta.Type {
	case TypeInverted:
		s, ok := val.(string)
		if !ok {
			return nil
		}
		terms := Tokenize(s)
		out := make([][]byte, 0, len(terms))
		for _, t := range terms {
			out = append(out, entryKey(meta, []byte(t), rowID))
		}
		return out
	case TypeHash, TypeBTree:
		enc := EncodeValue(val)
		return [][]byte{entryKey(meta, enc, rowID)}
	}
	return nil
}

// LookupEqual 返回等值命中的所有 rowID（Hash / BTree 均支持）
func (m *Manager) LookupEqual(ctx context.Context, meta *Meta, val any) ([]string, error) {
	enc := EncodeValue(val)
	prefix := entryKeyPrefix(meta, enc)
	return m.scanRowIDs(ctx, prefix, 0)
}

// LookupRange 返回 [low, high] 或 (low, high) 等范围命中的所有 rowID（仅 BTree/Hash 语义）
// includeLow/includeHigh 控制端点是否包含；nil 表示无边界
func (m *Manager) LookupRange(ctx context.Context, meta *Meta, low, high any, includeLow, includeHigh bool) ([]string, error) {
	dataPrefix := indexDataPrefix(meta)

	var start, end []byte
	if low == nil {
		start = append([]byte{}, dataPrefix...)
	} else {
		lenc := EncodeValue(low)
		if includeLow {
			start = entryKeyPrefix(meta, lenc)
		} else {
			// 排除 low：跳到 <encodedLow>0x00 之后的下一个字节
			start = append(entryKeyPrefix(meta, lenc), 0xFF)
		}
	}
	if high == nil {
		// 使用 dataPrefix + 0xFF... 作为上界（PrefixRange 语义）
		_, end = storage.PrefixRange(dataPrefix)
	} else {
		henc := EncodeValue(high)
		if includeHigh {
			end = append(entryKeyPrefix(meta, henc), 0xFF)
		} else {
			end = entryKeyPrefix(meta, henc)
		}
	}

	iter, err := m.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []string
	for iter.Next() {
		k, _ := iter.Item()
		if rid := decodeRowID(k, dataPrefix); rid != "" {
			out = append(out, rid)
		}
	}
	return out, nil
}

// LookupTerm 返回倒排索引中某一 term 对应的所有 rowID
func (m *Manager) LookupTerm(ctx context.Context, meta *Meta, term string) ([]string, error) {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return nil, nil
	}
	prefix := entryKeyPrefix(meta, []byte(term))
	return m.scanRowIDs(ctx, prefix, 0)
}

// Match 对查询进行分词，返回同时命中所有 term 的 rowID（AND 语义）
func (m *Manager) Match(ctx context.Context, meta *Meta, query string) ([]string, error) {
	terms := Tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	// 逐 term 求交集
	var acc map[string]struct{}
	for i, t := range terms {
		ids, err := m.LookupTerm(ctx, meta, t)
		if err != nil {
			return nil, err
		}
		set := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			set[id] = struct{}{}
		}
		if i == 0 {
			acc = set
			continue
		}
		for k := range acc {
			if _, ok := set[k]; !ok {
				delete(acc, k)
			}
		}
		if len(acc) == 0 {
			return nil, nil
		}
	}
	out := make([]string, 0, len(acc))
	for k := range acc {
		out = append(out, k)
	}
	return out, nil
}

// scanRowIDs 扫描给定前缀下的所有 rowID
func (m *Manager) scanRowIDs(ctx context.Context, prefix []byte, limit int) ([]string, error) {
	iter, err := m.engine.PrefixScan(ctx, prefix, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	for iter.Next() {
		k, _ := iter.Item()
		rid := trimPrefixRowID(k, prefix)
		if rid != "" {
			out = append(out, rid)
		}
	}
	return out, nil
}

// trimPrefixRowID 从 key 中去掉前缀后返回剩余（=rowID）
func trimPrefixRowID(key, prefix []byte) string {
	if !bytes.HasPrefix(key, prefix) {
		return ""
	}
	return string(key[len(prefix):])
}

// decodeRowID 从完整的 entry key 中提取 rowID（rowID 在最后一个 0x00 之后）
func decodeRowID(key, dataPrefix []byte) string {
	if !bytes.HasPrefix(key, dataPrefix) {
		return ""
	}
	rest := key[len(dataPrefix):]
	// rest = <encodedValueOrTerm> 0x00 <rowID>
	// 由于 encodedValue 内不会出现裸 0x00（string 已被转义，number 定长 9 无 0x00 保证不成立——
	// number 的 8 字节可能出现 0x00。因此用"最后一个 0x00"作为分隔符。
	idx := bytes.LastIndexByte(rest, 0x00)
	if idx < 0 {
		return ""
	}
	return string(rest[idx+1:])
}
