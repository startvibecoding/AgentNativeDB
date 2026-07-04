package storage

import (
	"encoding/binary"
	"math"
)

// Key 编码规则：
//
//	[表前缀 1字节][字段值 变长]
//
// 表前缀定义：
//
//	0x01 = agent_sessions
//	0x02 = agent_memories
//	0x03 = agent_decisions
//	0x04 = knowledge_entities
//	0x05 = knowledge_relations
//	0x06 = data_lineage
//	0x10 = vector_index
//	0x20 = graph_adjacency
//	0xFF = system_metadata
const (
	PrefixSession  byte = 0x01
	PrefixMemory   byte = 0x02
	PrefixDecision byte = 0x03
	PrefixEntity   byte = 0x04
	PrefixRelation byte = 0x05
	PrefixLineage  byte = 0x06
	PrefixVector   byte = 0x10
	PrefixGraph    byte = 0x20
	PrefixSystem   byte = 0xFF
)

// EncodeKey 编码主键: [prefix][id]
func EncodeKey(prefix byte, id string) []byte {
	key := make([]byte, 1+len(id))
	key[0] = prefix
	copy(key[1:], id)
	return key
}

// EncodeIndexKey 编码索引 key: [prefix][field][0x00][id]
// 用于二级索引，如按 agent_id 索引 session
func EncodeIndexKey(prefix byte, field string, id string) []byte {
	key := make([]byte, 1+len(field)+1+len(id))
	key[0] = prefix
	copy(key[1:], field)
	key[1+len(field)] = 0x00 // 分隔符
	copy(key[2+len(field):], id)
	return key
}

// DecodeIndexField 从索引 key 中提取 field 部分
func DecodeIndexField(key []byte) string {
	if len(key) < 2 {
		return ""
	}
	for i := 1; i < len(key); i++ {
		if key[i] == 0x00 {
			return string(key[1:i])
		}
	}
	return string(key[1:])
}

// DecodeIndexID 从索引 key 中提取 id 部分
func DecodeIndexID(key []byte) string {
	if len(key) < 3 {
		return ""
	}
	for i := 1; i < len(key); i++ {
		if key[i] == 0x00 {
			return string(key[i+1:])
		}
	}
	return ""
}

// EncodeVectorKey 编码向量存储 key: [0x10][indexName][0x00][id]
func EncodeVectorKey(indexName string, id string) []byte {
	return EncodeIndexKey(PrefixVector, indexName, id)
}

// PrefixRange 返回 prefix 扫描的 [start, end) 范围
// end 通过将最后一个字节加 1 实现
func PrefixRange(prefix []byte) (start, end []byte) {
	start = make([]byte, len(prefix))
	copy(start, prefix)

	end = make([]byte, len(prefix))
	copy(end, prefix)

	// 从最后一个字节开始进位
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xFF {
			end[i]++
			return
		}
		end[i] = 0
	}

	// 全部是 0xFF，返回 nil 表示无上界
	return start, nil
}

// Float32sToBytes 将 float32 切片转为字节（用于向量存储）
func Float32sToBytes(floats []float32) []byte {
	bytes := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
	}
	return bytes
}

// BytesToFloat32s 将字节转回 float32 切片
func BytesToFloat32s(data []byte) []float32 {
	n := len(data) / 4
	floats := make([]float32, n)
	for i := 0; i < n; i++ {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return floats
}
