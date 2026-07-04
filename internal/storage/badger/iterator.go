package badger

import (
	"bytes"

	"github.com/dgraph-io/badger/v4"
)

// badgerIterator 迭代器实现
type badgerIterator struct {
	it       *badger.Iterator
	txn      *badger.Txn
	start    []byte
	end      []byte
	limit    int
	count    int
	started  bool
}

// Item 返回当前键值对
func (iter *badgerIterator) Item() (key, value []byte) {
	if !iter.it.Valid() {
		return nil, nil
	}
	item := iter.it.Item()
	k := item.KeyCopy(nil)
	v, _ := item.ValueCopy(nil)
	return k, v
}

// Next 移动到下一个元素
func (iter *badgerIterator) Next() bool {
	if !iter.started {
		iter.started = true
		// 定位到起始位置
		iter.it.Seek(iter.start)
	} else {
		iter.it.Next()
	}

	if !iter.it.Valid() {
		return false
	}

	// 检查上界
	key := iter.it.Item().Key()
	if iter.end != nil && bytes.Compare(key, iter.end) >= 0 {
		return false
	}

	// 检查限制
	iter.count++
	if iter.limit > 0 && iter.count > iter.limit {
		return false
	}

	return true
}

// Valid 检查迭代器是否有效
func (iter *badgerIterator) Valid() bool {
	return iter.it.Valid()
}

// Close 关闭迭代器
func (iter *badgerIterator) Close() error {
	iter.it.Close()
	iter.txn.Discard()
	return nil
}

// Error 返回迭代过程中的错误
func (iter *badgerIterator) Error() error {
	return nil
}
