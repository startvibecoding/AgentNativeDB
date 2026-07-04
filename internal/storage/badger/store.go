package badger

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v4"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// BadgerEngine 基于 BadgerDB 的存储引擎实现
type BadgerEngine struct {
	db   *badger.DB
	opts storage.Options
}

// 确保接口实现
var _ storage.Engine = (*BadgerEngine)(nil)

// New 创建 BadgerEngine 实例
func New() *BadgerEngine {
	return &BadgerEngine{}
}

// Open 打开数据库
func (e *BadgerEngine) Open(opts storage.Options) error {
	e.opts = opts

	badgerOpts := badger.DefaultOptions(opts.DataDir).
		WithSyncWrites(opts.SyncWrites).
		WithValueLogFileSize(opts.ValueLogFileSize).
		WithMemTableSize(opts.MemTableSize).
		WithNumMemtables(opts.NumMemTables)

	// 禁用 BadgerDB 的内置日志（我们自己管理日志）
	badgerOpts = badgerOpts.WithLogger(nil)

	db, err := badger.Open(badgerOpts)
	if err != nil {
		return fmt.Errorf("open badger: %w", err)
	}
	e.db = db
	return nil
}

// Close 关闭数据库
func (e *BadgerEngine) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// Get 获取键值
func (e *BadgerEngine) Get(ctx context.Context, key []byte) ([]byte, error) {
	var value []byte
	err := e.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})
	if err == badger.ErrKeyNotFound {
		return nil, storage.ErrKeyNotFound
	}
	return value, err
}

// Set 设置键值
func (e *BadgerEngine) Set(ctx context.Context, key, value []byte) error {
	return e.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// Delete 删除键
func (e *BadgerEngine) Delete(ctx context.Context, key []byte) error {
	return e.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// Scan 范围扫描 [start, end)
func (e *BadgerEngine) Scan(ctx context.Context, start, end []byte, opts storage.ScanOptions) (storage.Iterator, error) {
	txn := e.db.NewTransaction(false)
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.Reverse = opts.Reverse

	it := txn.NewIterator(iterOpts)
	return &badgerIterator{
		it:      it,
		txn:     txn,
		start:   start,
		end:     end,
		limit:   opts.Limit,
		reverse: opts.Reverse,
	}, nil
}

// PrefixScan 前缀扫描
func (e *BadgerEngine) PrefixScan(ctx context.Context, prefix []byte, opts storage.ScanOptions) (storage.Iterator, error) {
	txn := e.db.NewTransaction(false)
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.Reverse = opts.Reverse

	it := txn.NewIterator(iterOpts)
	start, end := storage.PrefixRange(prefix)
	return &badgerIterator{
		it:      it,
		txn:     txn,
		start:   start,
		end:     end,
		limit:   opts.Limit,
		reverse: opts.Reverse,
	}, nil
}

// NewTransaction 创建事务
func (e *BadgerEngine) NewTransaction(update bool) (storage.Transaction, error) {
	txn := e.db.NewTransaction(update)
	return &badgerTransaction{txn: txn}, nil
}

// BatchWrite 批量写入
func (e *BadgerEngine) BatchWrite(ctx context.Context, ops []storage.WriteOp) error {
	wb := e.db.NewWriteBatch()
	defer wb.Cancel()

	for _, op := range ops {
		switch op.Type {
		case storage.OpPut:
			if err := wb.Set(op.Key, op.Value); err != nil {
				return fmt.Errorf("batch set: %w", err)
			}
		case storage.OpDelete:
			if err := wb.Delete(op.Key); err != nil {
				return fmt.Errorf("batch delete: %w", err)
			}
		}
	}

	return wb.Flush()
}

// Sync 同步数据到磁盘
func (e *BadgerEngine) Sync() error {
	return e.db.Sync()
}
