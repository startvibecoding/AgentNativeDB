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

// init 注册 BadgerDB 引擎到全局注册表
func init() {
	storage.Register(storage.BackendBadger, func(opts storage.Options) (storage.Engine, error) {
		e := &BadgerEngine{}
		if err := e.Open(opts); err != nil {
			return nil, err
		}
		return e, nil
	})
}

// New 创建 BadgerEngine 实例（不自动 Open）。
//
// 推荐使用 storage.CreateEngine(opts) 来创建并打开引擎，
// New() 保留用于需要手动控制 Open 时序的场景。
func New() *BadgerEngine {
	return &BadgerEngine{}
}

// Open 打开数据库
func (e *BadgerEngine) Open(opts storage.Options) error {
	e.opts = opts

	// 从 BackendOpts 提取 Badger 专属配置
	valueLogFileSize, _ := storage.GetOpt[int64](opts, "value_log_file_size")
	memTableSize, _ := storage.GetOpt[int64](opts, "mem_table_size")
	numMemTables, _ := storage.GetOpt[int](opts, "num_mem_tables")

	// 未指定时使用 BadgerDB 自身默认值
	badgerOpts := badger.DefaultOptions(opts.DataDir).
		WithSyncWrites(opts.SyncWrites)
	if valueLogFileSize > 0 {
		badgerOpts = badgerOpts.WithValueLogFileSize(valueLogFileSize)
	}
	if memTableSize > 0 {
		badgerOpts = badgerOpts.WithMemTableSize(memTableSize)
	}
	if numMemTables > 0 {
		badgerOpts = badgerOpts.WithNumMemtables(numMemTables)
	}

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
