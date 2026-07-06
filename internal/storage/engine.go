package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrKeyNotFound 当 Get 未命中时返回（可用 errors.Is 判定）
var ErrKeyNotFound = errors.New("storage: key not found")

// Engine 定义存储引擎的核心接口。
//
// 所有上层模块（Agent Runtime、SQL Executor、Vector、Graph 等）
// 仅依赖此接口，不直接引用任何具体实现。
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

// Options 存储引擎通用配置。
//
// BackendOpts 用于传递具体引擎的专属参数（如 BadgerDB 的 ValueLogFileSize 等），
// 各实现自行从 BackendOpts 中提取所需字段。
type Options struct {
	// Backend 存储引擎名称（如 "badger"）
	Backend string

	// DataDir 数据目录
	DataDir string

	// SyncWrites 是否同步写入（fsync）
	SyncWrites bool

	// CacheSizeMB 缓存大小（MB）
	CacheSizeMB int

	// BackendOpts 具体引擎的专属配置（map 形式，灵活可扩展）
	BackendOpts map[string]any
}

// DefaultOptions 返回默认配置（BadgerDB 为默认引擎）
func DefaultOptions() Options {
	return Options{
		Backend:     BackendBadger,
		DataDir:     "./data",
		SyncWrites:  true,
		CacheSizeMB: 256,
		BackendOpts: map[string]any{
			"value_log_file_size": int64(64 << 20), // 64MB
			"mem_table_size":      int64(16 << 20), // 16MB
			"num_mem_tables":      3,
		},
	}
}

// GetOpt 从 BackendOpts 中提取指定类型的配置项。
// 如果不存在或类型不匹配，返回零值和 false。
func GetOpt[T any](opts Options, key string) (T, bool) {
	if opts.BackendOpts == nil {
		var zero T
		return zero, false
	}
	v, ok := opts.BackendOpts[key]
	if !ok {
		var zero T
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
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

// ========== 引擎注册表 ==========

// BackendBadger BadgerDB 引擎名称
const BackendBadger = "badger"

// registry 全局引擎注册表
var registry = struct {
	mu    sync.RWMutex
	factories map[string]Factory
}{
	factories: make(map[string]Factory),
}

// Factory 引擎工厂函数：根据 Options 创建 Engine 实例
type Factory func(opts Options) (Engine, error)

// Register 注册存储引擎工厂。
// 在 init() 中调用，例如 badger 包的 init() 会注册 "badger"。
// 重复注册同一名称会 panic。
func Register(name string, factory Factory) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.factories[name]; exists {
		panic(fmt.Sprintf("storage: engine already registered: %s", name))
	}
	registry.factories[name] = factory
}

// CreateEngine 根据 Options.Backend 创建对应的 Engine 实例。
// 默认使用 "badger"。
func CreateEngine(opts Options) (Engine, error) {
	backend := opts.Backend
	if backend == "" {
		backend = BackendBadger
		opts.Backend = backend
	}

	registry.mu.RLock()
	factory, ok := registry.factories[backend]
	registry.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("storage: unknown backend: %s (registered: %v)", backend, registeredBackends())
	}
	return factory(opts)
}

// registeredBackends 返回已注册的引擎名称列表
func registeredBackends() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	names := make([]string, 0, len(registry.factories))
	for name := range registry.factories {
		names = append(names, name)
	}
	return names
}
