package storage

import "context"

// Engine 定义存储引擎的核心接口
type Engine interface {
	// 生命周期
	Open(opts Options) error
	Close() error

	// 读写操作
	Get(ctx context.Context, key []byte) ([]byte, error)
	Set(ctx context.Context, key, value []byte) error
	Delete(ctx context.Context, key []byte) error

	// 范围扫描
	Scan(ctx context.Context, start, end []byte, opts ScanOptions) (Iterator, error)

	// 前缀查询（用于按表/类型分区）
	PrefixScan(ctx context.Context, prefix []byte, opts ScanOptions) (Iterator, error)

	// 事务
	NewTransaction(update bool) (Transaction, error)

	// 批量写入
	BatchWrite(ctx context.Context, ops []WriteOp) error

	// 同步（确保数据持久化）
	Sync() error
}

// Transaction 定义事务接口
type Transaction interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	Discard()
}

// Iterator 定义迭代器接口
type Iterator interface {
	// Item 返回当前键值对
	Item() (key, value []byte)

	// Next 移动到下一个元素
	Next() bool

	// Valid 检查迭代器是否有效
	Valid() bool

	// Close 关闭迭代器
	Close() error

	// Error 返回迭代过程中的错误
	Error() error
}

// Options 存储引擎配置
type Options struct {
	// DataDir 数据目录
	DataDir string

	// SyncWrites 是否同步写入（fsync）
	SyncWrites bool

	// ValueLogFileSize value log 文件大小（字节）
	ValueLogFileSize int64

	// MemTableSize 内存表大小（字节）
	MemTableSize int64

	// NumMemTables 内存表数量
	NumMemTables int

	// CacheSizeMB 缓存大小（MB）
	CacheSizeMB int
}

// DefaultOptions 返回默认配置
func DefaultOptions() Options {
	return Options{
		DataDir:          "./data",
		SyncWrites:       true,
		ValueLogFileSize: 64 << 20, // 64MB
		MemTableSize:     16 << 20, // 16MB
		NumMemTables:     3,
		CacheSizeMB:      256,
	}
}

// ScanOptions 扫描选项
type ScanOptions struct {
	// Limit 最大返回数量，0 表示不限制
	Limit int

	// Reverse 是否反向扫描
	Reverse bool
}

// WriteOp 批量写入操作
type WriteOp struct {
	// Type 操作类型
	Type OpType

	// Key 键
	Key []byte

	// Value 值（Delete 操作时忽略）
	Value []byte
}

// OpType 操作类型
type OpType int

const (
	OpPut    OpType = iota // 写入
	OpDelete               // 删除
)
