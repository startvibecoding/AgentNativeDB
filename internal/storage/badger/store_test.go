package badger

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

func openTestEngine(t *testing.T) *BadgerEngine {
	t.Helper()
	dir := t.TempDir()
	engine := New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false // 测试时关闭 sync 加速
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})
	return engine
}

func TestEngine_SetGet(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	if err := engine.Set(ctx, []byte("key1"), []byte("value1")); err != nil {
		t.Fatalf("set: %v", err)
	}

	val, err := engine.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(val) != "value1" {
		t.Fatalf("expected 'value1', got %q", val)
	}
}

func TestEngine_GetNotFound(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	_, err := engine.Get(ctx, []byte("nonexistent"))
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestEngine_Delete(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	engine.Set(ctx, []byte("key1"), []byte("value1"))
	engine.Delete(ctx, []byte("key1"))

	_, err := engine.Get(ctx, []byte("key1"))
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestEngine_PrefixScan(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	// 写入测试数据
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("test-%02d", i))
		engine.Set(ctx, key, []byte(fmt.Sprintf("val-%d", i)))
	}

	// 前缀扫描
	prefix := []byte("test-")
	iter, err := engine.PrefixScan(ctx, prefix, storage.ScanOptions{})
	if err != nil {
		t.Fatalf("prefix scan: %v", err)
	}
	defer iter.Close()

	count := 0
	for iter.Next() {
		count++
	}
	if count != 10 {
		t.Fatalf("expected 10 items, got %d", count)
	}
}

func TestEngine_PrefixScanWithLimit(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("test-%02d", i))
		engine.Set(ctx, key, []byte(fmt.Sprintf("val-%d", i)))
	}

	iter, err := engine.PrefixScan(ctx, []byte("test-"), storage.ScanOptions{Limit: 3})
	if err != nil {
		t.Fatalf("prefix scan: %v", err)
	}
	defer iter.Close()

	count := 0
	for iter.Next() {
		count++
	}
	if count != 3 {
		t.Fatalf("expected 3 items, got %d", count)
	}
}

func TestEngine_BatchWrite(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	ops := []storage.WriteOp{
		{Type: storage.OpPut, Key: []byte("k1"), Value: []byte("v1")},
		{Type: storage.OpPut, Key: []byte("k2"), Value: []byte("v2")},
		{Type: storage.OpPut, Key: []byte("k3"), Value: []byte("v3")},
	}

	if err := engine.BatchWrite(ctx, ops); err != nil {
		t.Fatalf("batch write: %v", err)
	}

	for _, op := range ops {
		val, err := engine.Get(ctx, op.Key)
		if err != nil {
			t.Fatalf("get %s: %v", op.Key, err)
		}
		if string(val) != string(op.Value) {
			t.Fatalf("expected %q, got %q", op.Value, val)
		}
	}
}

func TestEngine_ConcurrentReadWrite(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 1000

	// 并发写入
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%04d", i))
			val := []byte(fmt.Sprintf("val-%d", i))
			if err := engine.Set(ctx, key, val); err != nil {
				t.Errorf("set key-%d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// 并发读写
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%04d", i))
			engine.Get(ctx, key)
		}(i)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%04d", i))
			val := []byte(fmt.Sprintf("val-%d-updated", i))
			engine.Set(ctx, key, val)
		}(i)
	}
	wg.Wait()
}

func TestEngine_Transaction(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	// 写入初始值
	engine.Set(ctx, []byte("key1"), []byte("old"))

	// 开启事务
	tx, err := engine.NewTransaction(true)
	if err != nil {
		t.Fatalf("new tx: %v", err)
	}

	// 事务内写入
	tx.Set([]byte("key1"), []byte("new"))

	// 事务外读取（应该还是 old）
	val, _ := engine.Get(ctx, []byte("key1"))
	if string(val) != "old" {
		t.Fatalf("expected 'old' before commit, got %q", val)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// 提交后读取（应该变成 new）
	val, _ = engine.Get(ctx, []byte("key1"))
	if string(val) != "new" {
		t.Fatalf("expected 'new' after commit, got %q", val)
	}
}

func TestEngine_TransactionDiscard(t *testing.T) {
	engine := openTestEngine(t)
	ctx := context.Background()

	engine.Set(ctx, []byte("key1"), []byte("old"))

	tx, _ := engine.NewTransaction(true)
	tx.Set([]byte("key1"), []byte("new"))
	tx.Discard() // 丢弃

	val, _ := engine.Get(ctx, []byte("key1"))
	if string(val) != "old" {
		t.Fatalf("expected 'old' after discard, got %q", val)
	}
}

func TestEngine_CloseAndReopen(t *testing.T) {
	dir := t.TempDir()
	engine := New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false

	engine.Open(opts)
	engine.Set(context.Background(), []byte("key1"), []byte("value1"))
	engine.Close()

	// 重新打开
	engine2 := New()
	engine2.Open(opts)
	defer engine2.Close()

	val, err := engine2.Get(context.Background(), []byte("key1"))
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if string(val) != "value1" {
		t.Fatalf("expected 'value1', got %q", val)
	}
}

func BenchmarkEngine_Set(b *testing.B) {
	dir := b.TempDir()
	engine := New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		engine.Set(ctx, key, []byte("value"))
	}
}

func BenchmarkEngine_Get(b *testing.B) {
	dir := b.TempDir()
	engine := New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	ctx := context.Background()
	// 预填充数据
	for i := 0; i < 10000; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		engine.Set(ctx, key, []byte("value"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key-%d", i%10000))
		engine.Get(ctx, key)
	}
}
