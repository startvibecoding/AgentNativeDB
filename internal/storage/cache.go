package storage

import (
	"container/list"
	"sync"
)

// Cache 是一个并发安全的 LRU 缓存
type Cache struct {
	mu       sync.RWMutex
	maxSize  int
	items    map[string]*list.Element
	order    *list.List
	hits     int64
	misses   int64
}

type cacheEntry struct {
	key   string
	value []byte
}

// NewCache 创建指定容量的 LRU 缓存
func NewCache(maxSize int) *Cache {
	return &Cache{
		maxSize: maxSize,
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
	}
}

// Get 从缓存中获取值
func (c *Cache) Get(key []byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := string(key)
	if elem, ok := c.items[k]; ok {
		c.order.MoveToFront(elem)
		c.hits++
		return elem.Value.(*cacheEntry).value, true
	}
	c.misses++
	return nil, false
}

// Set 设置缓存值
func (c *Cache) Set(key, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := string(key)

	// 已存在则更新
	if elem, ok := c.items[k]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = value
		return
	}

	// 缓存满时淘汰
	if c.order.Len() >= c.maxSize {
		c.evict()
	}

	// 插入新元素
	entry := &cacheEntry{key: k, value: value}
	elem := c.order.PushFront(entry)
	c.items[k] = elem
}

// Delete 从缓存中删除值
func (c *Cache) Delete(key []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := string(key)
	if elem, ok := c.items[k]; ok {
		c.removeElement(elem)
	}
}

// evict 淘汰最久未使用的元素（需要在持有写锁时调用）
func (c *Cache) evict() {
	elem := c.order.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement 移除元素（需要在持有锁时调用）
func (c *Cache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}

// Len 返回当前缓存大小
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// Stats 返回缓存统计信息
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:   c.hits,
		Misses: c.misses,
		Size:   c.order.Len(),
		MaxSize: c.maxSize,
	}
}

// CacheStats 缓存统计
type CacheStats struct {
	Hits    int64
	Misses  int64
	Size    int
	MaxSize int
}

// HitRate 命中率
func (s CacheStats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}

// Clear 清空缓存
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.maxSize)
	c.order.Init()
	c.hits = 0
	c.misses = 0
}
