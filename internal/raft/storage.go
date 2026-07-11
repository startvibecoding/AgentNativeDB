package raft

import (
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// RaftStorage 封装 BadgerDB 用于 Raft 日志、稳定状态、集群配置存储
type RaftStorage struct {
	db *badger.DB
}

// NewRaftStorage 创建 Raft 存储
func NewRaftStorage(dataDir string) (*RaftStorage, error) {
	opts := badger.DefaultOptions(dataDir + "/raft").
		WithSyncWrites(true).
		WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open raft storage: %w", err)
	}
	return &RaftStorage{db: db}, nil
}

// Close 关闭存储
func (s *RaftStorage) Close() error {
	return s.db.Close()
}

// ---- 稳定状态存储 ----

// stableKey 返回稳定状态存储的key
func stableKey(key string) []byte {
	k := make([]byte, 0, 1+len(key))
	k = append(k, raftStablePrefix)
	k = append(k, []byte(key)...)
	return k
}

// GetUint64 获取 uint64 值
func (s *RaftStorage) GetUint64(key string) (uint64, error) {
	var v uint64
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(stableKey(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			if len(val) == 8 {
				v = btou64(val)
			}
			return nil
		})
	})
	if err == badger.ErrKeyNotFound {
		return 0, fmt.Errorf("key not found: %s", key)
	}
	return v, err
}

// SetUint64 设置 uint64 值
func (s *RaftStorage) SetUint64(key string, value uint64) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(stableKey(key), u64tob(value))
	})
}

// GetString 获取字符串值
func (s *RaftStorage) GetString(key string) (string, error) {
	var v string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(stableKey(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			v = string(val)
			return nil
		})
	})
	if err == badger.ErrKeyNotFound {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return v, err
}

// SetString 设置字符串值
func (s *RaftStorage) SetString(key string, value string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(stableKey(key), []byte(value))
	})
}

// GetCluster 获取集群配置
func (s *RaftStorage) GetCluster() (map[string]string, error) {
	var cfg map[string]string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(stableKey(KeyClusterConfig))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &cfg)
		})
	})
	if err == badger.ErrKeyNotFound {
		return nil, nil
	}
	return cfg, err
}

// SetCluster 设置集群配置
func (s *RaftStorage) SetCluster(peers map[string]string) error {
	data, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(stableKey(KeyClusterConfig), data)
	})
}

// ---- 日志存储 ----

// logKey 返回日志条目的key（按索引排序）
func logKey(index uint64) []byte {
	k := make([]byte, 1+8)
	k[0] = raftLogPrefix
	copy(k[1:], u64tob(index))
	return k
}

// StoreLog 存储单个日志条目
func (s *RaftStorage) StoreLog(entry *LogEntry) error {
	data, err := encodeLogEntry(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(logKey(entry.Index), data)
	})
}

// StoreLogs 批量存储日志
func (s *RaftStorage) StoreLogs(entries []*LogEntry) error {
	wb := s.db.NewWriteBatch()
	defer wb.Cancel()
	for _, entry := range entries {
		data, err := encodeLogEntry(entry)
		if err != nil {
			return err
		}
		if err := wb.Set(logKey(entry.Index), data); err != nil {
			return err
		}
	}
	return wb.Flush()
}

// GetLog 获取指定索引的日志
func (s *RaftStorage) GetLog(index uint64) (*LogEntry, error) {
	var entry LogEntry
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(logKey(index))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			var e error
			entry, e = decodeLogEntry(val)
			return e
		})
	})
	if err == badger.ErrKeyNotFound {
		return nil, fmt.Errorf("log %d not found", index)
	}
	return &entry, err
}

// DeleteRange 删除 [min,max] 范围内的日志
func (s *RaftStorage) DeleteRange(min, max uint64) error {
	wb := s.db.NewWriteBatch()
	defer wb.Cancel()
	for i := min; i <= max; i++ {
		if err := wb.Delete(logKey(i)); err != nil {
			return err
		}
	}
	return wb.Flush()
}

// FirstIndex 返回第一条日志索引
func (s *RaftStorage) FirstIndex() (uint64, error) {
	var idx uint64
	err := s.db.View(func(txn *badger.Txn) error {
		prefix := []byte{raftLogPrefix}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			item := it.Item()
			k := item.Key()
			idx = btou64(k[1:])
		}
		return nil
	})
	return idx, err
}

// LastIndex 返回最后一条日志索引
func (s *RaftStorage) LastIndex() (uint64, error) {
	var idx uint64
	err := s.db.View(func(txn *badger.Txn) error {
		prefix := []byte{raftLogPrefix}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()
		// 从最高位置seek
		it.Seek(append(prefix, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF))
		if it.ValidForPrefix(prefix) {
			item := it.Item()
			k := item.Key()
			idx = btou64(k[1:])
		}
		return nil
	})
	return idx, err
}

// LogEntries 返回 [start, end) 范围内的日志
func (s *RaftStorage) LogEntries(start, end uint64, maxSize uint64) ([]*LogEntry, error) {
	var entries []*LogEntry
	var size uint64
	err := s.db.View(func(txn *badger.Txn) error {
		for i := start; i < end; i++ {
			item, err := txn.Get(logKey(i))
			if err != nil {
				if err == badger.ErrKeyNotFound {
					break
				}
				return err
			}
			err = item.Value(func(val []byte) error {
				entry, e := decodeLogEntry(val)
				if e != nil {
					return e
				}
				entries = append(entries, &entry)
				size += uint64(len(val))
				return nil
			})
			if err != nil {
				return err
			}
			if maxSize > 0 && size > maxSize {
				break
			}
		}
		return nil
	})
	return entries, err
}

// 日志条目序列化: term(8) + index(8) + type(1) + data
func encodeLogEntry(e *LogEntry) ([]byte, error) {
	buf := make([]byte, 8+8+1+len(e.Data))
	copy(buf[0:8], u64tob(e.Term))
	copy(buf[8:16], u64tob(e.Index))
	buf[16] = byte(e.Type)
	copy(buf[17:], e.Data)
	return buf, nil
}

func decodeLogEntry(data []byte) (LogEntry, error) {
	if len(data) < 17 {
		return LogEntry{}, fmt.Errorf("log entry too short: %d", len(data))
	}
	return LogEntry{
		Term:  btou64(data[0:8]),
		Index: btou64(data[8:16]),
		Type:  LogType(data[16]),
		Data:  append([]byte(nil), data[17:]...),
	}, nil
}
