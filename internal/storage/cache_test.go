package storage

import (
	"fmt"
	"sync"
	"testing"
)

func TestCache_SetGet(t *testing.T) {
	c := NewCache(3)

	c.Set([]byte("a"), []byte("1"))
	c.Set([]byte("b"), []byte("2"))
	c.Set([]byte("c"), []byte("3"))

	// 命中
	if v, ok := c.Get([]byte("a")); !ok || string(v) != "1" {
		t.Fatalf("expected '1', got %q, ok=%v", v, ok)
	}

	// 未命中
	if _, ok := c.Get([]byte("x")); ok {
		t.Fatal("expected miss for 'x'")
	}
}

func TestCache_Eviction(t *testing.T) {
	c := NewCache(2)

	c.Set([]byte("a"), []byte("1"))
	c.Set([]byte("b"), []byte("2"))
	c.Set([]byte("c"), []byte("3")) // 应该淘汰 "a"

	if _, ok := c.Get([]byte("a")); ok {
		t.Fatal("expected 'a' to be evicted")
	}

	if v, ok := c.Get([]byte("b")); !ok || string(v) != "2" {
		t.Fatalf("expected '2', got %q", v)
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := NewCache(3)

	c.Set([]byte("a"), []byte("1"))
	c.Set([]byte("b"), []byte("2"))
	c.Set([]byte("c"), []byte("3"))

	// 访问 "a"，使其变为最近使用
	c.Get([]byte("a"))

	// 插入 "d"，应该淘汰 "b"（最久未使用）
	c.Set([]byte("d"), []byte("4"))

	if _, ok := c.Get([]byte("b")); ok {
		t.Fatal("expected 'b' to be evicted")
	}

	if _, ok := c.Get([]byte("a")); !ok {
		t.Fatal("expected 'a' to still be present")
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache(3)
	c.Set([]byte("a"), []byte("1"))
	c.Delete([]byte("a"))

	if _, ok := c.Get([]byte("a")); ok {
		t.Fatal("expected 'a' to be deleted")
	}

	if c.Len() != 0 {
		t.Fatalf("expected size 0, got %d", c.Len())
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache(3)
	c.Set([]byte("a"), []byte("1"))
	c.Set([]byte("b"), []byte("2"))
	c.Clear()

	if c.Len() != 0 {
		t.Fatalf("expected size 0 after clear, got %d", c.Len())
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache(100)
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%d", i))
			val := []byte(fmt.Sprintf("val-%d", i))
			c.Set(key, val)
		}(i)
	}
	wg.Wait()

	// 并发读写
	for i := 0; i < 1000; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%d", i))
			c.Get(key)
		}(i)
		go func(i int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%d", i))
			val := []byte(fmt.Sprintf("val-%d-new", i))
			c.Set(key, val)
		}(i)
	}
	wg.Wait()
}

func TestCache_Stats(t *testing.T) {
	c := NewCache(2)
	c.Set([]byte("a"), []byte("1"))
	c.Get([]byte("a")) // hit
	c.Get([]byte("b")) // miss

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate() != 0.5 {
		t.Fatalf("expected hit rate 0.5, got %f", stats.HitRate())
	}
}

func TestCache_UpdateExistingKey(t *testing.T) {
	c := NewCache(2)
	c.Set([]byte("a"), []byte("1"))
	c.Set([]byte("a"), []byte("2"))

	if v, ok := c.Get([]byte("a")); !ok || string(v) != "2" {
		t.Fatalf("expected '2', got %q", v)
	}
	if c.Len() != 1 {
		t.Fatalf("expected size 1, got %d", c.Len())
	}
}

func BenchmarkCache_Get(b *testing.B) {
	c := NewCache(1000)
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		c.Set(key, key)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("key-%d", i%1000))
			c.Get(key)
			i++
		}
	})
}

func BenchmarkCache_Set(b *testing.B) {
	c := NewCache(1000)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("key-%d", i))
			c.Set(key, key)
			i++
		}
	})
}
