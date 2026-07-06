package storage

import (
	"os"
	"testing"
)

// NewTestEngine 创建测试用的存储引擎。
//
// 使用配置中指定的 Backend（默认为 "badger"），数据目录为 tb.TempDir()（自动清理），
// SyncWrites=false 以加速测试。
//
// 接受 *testing.T 或 *testing.B（两者都实现了 testing.TB 接口）。
//
// 注意：调用方需确保对应引擎已通过 init() 注册到全局注册表。
// 对于 BadgerDB，在测试文件中添加空白导入：
//
//	import _ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
func NewTestEngine(tb testing.TB) Engine {
	tb.Helper()
	dir := tb.TempDir()
	opts := Options{
		Backend:     BackendBadger,
		DataDir:     dir,
		SyncWrites:  false,
		CacheSizeMB: 256,
		BackendOpts: map[string]any{
			"value_log_file_size": int64(64 << 20),
			"mem_table_size":      int64(16 << 20),
			"num_mem_tables":      3,
		},
	}
	engine, err := CreateEngine(opts)
	if err != nil {
		tb.Fatalf("create test engine: %v", err)
	}
	tb.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})
	return engine
}

// NewTestEngineWithOptions 创建测试用的存储引擎，可自定义 Options。
//
// DataDir 会自动设为 tb.TempDir()，SyncWrites 自动设为 false。
// 调用方需确保 Backend 对应的引擎已注册。
func NewTestEngineWithOptions(tb testing.TB, opts Options) Engine {
	tb.Helper()
	dir := tb.TempDir()
	opts.DataDir = dir
	opts.SyncWrites = false
	if opts.Backend == "" {
		opts.Backend = BackendBadger
	}
	engine, err := CreateEngine(opts)
	if err != nil {
		tb.Fatalf("create test engine: %v", err)
	}
	tb.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})
	return engine
}
