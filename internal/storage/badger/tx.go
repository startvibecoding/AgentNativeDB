package badger

import (
	"github.com/dgraph-io/badger/v4"
)

// badgerTransaction 事务实现
type badgerTransaction struct {
	txn *badger.Txn
}

// Get 读取
func (t *badgerTransaction) Get(key []byte) ([]byte, error) {
	item, err := t.txn.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

// Set 写入
func (t *badgerTransaction) Set(key, value []byte) error {
	return t.txn.Set(key, value)
}

// Delete 删除
func (t *badgerTransaction) Delete(key []byte) error {
	return t.txn.Delete(key)
}

// Commit 提交
func (t *badgerTransaction) Commit() error {
	return t.txn.Commit()
}

// Discard 丢弃
func (t *badgerTransaction) Discard() {
	t.txn.Discard()
}
