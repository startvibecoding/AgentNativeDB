// Package raft 提供 Raft 共识存储引擎包装
package raft

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// 确保 RaftEngine 实现 storage.Engine
var _ storage.Engine = (*RaftEngine)(nil)

const (
	// BackendRaft Raft 集群后端名称
	BackendRaft = "raft"
	proposalTimeout = 5 * time.Second
)

// CmdType Raft 日志中的命令类型
type CmdType byte

const (
	CmdSet    CmdType = iota // Set 操作
	CmdDelete                // Delete 操作
	CmdBatch                 // BatchWrite 操作
)

// Cmd Raft 日志命令
type Cmd struct {
	Type CmdType `json:"type"`
	Key  []byte  `json:"key,omitempty"`
	Value []byte `json:"value,omitempty"`
	Ops  []WriteOpCmd `json:"ops,omitempty"`
}

// WriteOpCmd 批量写操作
type WriteOpCmd struct {
	Type  storage.OpType `json:"type"`
	Key   []byte         `json:"key"`
	Value []byte         `json:"value,omitempty"`
}

// RaftEngine 包装本地存储引擎，通过Raft实现写操作复制
type RaftEngine struct {
	node   *Node
	local  storage.Engine // 本地状态机存储（BadgerDB）
	logger *slog.Logger
	ready  chan struct{} // 引擎就绪信号（FSM已恢复）
}

func init() {
	storage.Register(BackendRaft, func(opts storage.Options) (storage.Engine, error) {
		// Raft 引擎需要显式通过 NewRaftEngine 创建，这里防止直接调用
		return nil, errors.New("raft: use NewRaftEngine to create raft engine, see internal/raft package")
	})
}

// RaftConfig Raft 引擎配置
type RaftConfig struct {
	// NodeID 当前节点ID
	NodeID string
	// RaftAddr Raft 通信地址
	RaftAddr string
	// HTTPAddr HTTP API 地址（用于重定向）
	HTTPAddr string
	// DataDir 数据目录
	DataDir string
	// Peers 集群节点
	Peers map[string]string
	// Bootstrap 是否初始化集群（仅单节点首次启动设置）
	Bootstrap bool
	// LocalBackend 本地存储后端，默认 "badger"
	LocalBackend string
	// LocalSyncWrites 本地存储是否同步写
	LocalSyncWrites bool
}

// NewRaftEngine 创建 Raft 存储引擎
func NewRaftEngine(cfg RaftConfig) (*RaftEngine, error) {
	if cfg.LocalBackend == "" {
		cfg.LocalBackend = storage.BackendBadger
	}

	// 打开本地状态机存储
	localOpts := storage.Options{
		Backend:    cfg.LocalBackend,
		DataDir:    cfg.DataDir + "/state",
		SyncWrites: cfg.LocalSyncWrites,
		CacheSizeMB: 256,
	}
	local, err := storage.CreateEngine(localOpts)
	if err != nil {
		return nil, fmt.Errorf("raft: open local storage: %w", err)
	}

	engine := &RaftEngine{
		local:  local,
		logger: slog.With("module", "raft-engine", "node", cfg.NodeID),
		ready:  make(chan struct{}),
	}

	// 创建 Raft 节点
	raftCfg := Config{
		NodeID:            cfg.NodeID,
		RaftAddr:          cfg.RaftAddr,
		DataDir:           cfg.DataDir,
		Peers:             cfg.Peers,
		HeartbeatTimeout:  100 * time.Millisecond,
		ElectionTimeout:   1000 * time.Millisecond,
		SnapshotThreshold: 10000,
		Apply:             engine.applyCmd,
		ApplySnapshot:     engine.applySnapshot,
		GetSnapshot:       engine.getSnapshot,
	}
	node, err := NewNode(raftCfg)
	if err != nil {
		_ = local.Close()
		return nil, err
	}
	engine.node = node

	// Bootstrap 集群（如果需要）
	if cfg.Bootstrap {
		peers := cfg.Peers
		if peers == nil {
			peers = map[string]string{cfg.NodeID: cfg.RaftAddr}
		}
		if err := node.BootstrapCluster(peers); err != nil && !errors.Is(err, ErrClusterExists) {
			_ = local.Close()
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
	}

	// 启动 Raft 节点
	if err := node.Start(); err != nil {
		_ = local.Close()
		return nil, err
	}

	close(engine.ready)
	engine.logger.Info("raft engine started",
		"node_id", cfg.NodeID,
		"raft_addr", cfg.RaftAddr,
		"leader", node.IsLeader(),
	)
	return engine, nil
}

// ---- storage.Engine 接口实现 ----

// Open 打开引擎（Raft引擎不通过此方法打开）
func (e *RaftEngine) Open(opts storage.Options) error {
	return errors.New("raft: use NewRaftEngine instead of Open")
}

// Close 关闭引擎
func (e *RaftEngine) Close() error {
	var err error
	if e.node != nil {
		err = e.node.Shutdown()
	}
	if e.local != nil {
		if e2 := e.local.Close(); e2 != nil && err == nil {
			err = e2
		}
	}
	return err
}

// Get 读取操作，直接从本地状态机读取
func (e *RaftEngine) Get(ctx context.Context, key []byte) ([]byte, error) {
	<-e.ready
	// 可选：Leader读保证线性一致性；Follower读可能落后
	// 这里允许Follower读以提高性能
	return e.local.Get(ctx, key)
}

// Set 写入操作，需要通过 Raft 复制
func (e *RaftEngine) Set(ctx context.Context, key, value []byte) error {
	<-e.ready
	if !e.node.IsLeader() {
		return ErrNotLeader
	}
	cmd := Cmd{
		Type:  CmdSet,
		Key:   append([]byte(nil), key...),
		Value: append([]byte(nil), value...),
	}
	data, err := encodeCmd(cmd)
	if err != nil {
		return err
	}
	return e.node.Propose(data, proposalTimeout)
}

// Delete 删除操作，通过 Raft 复制
func (e *RaftEngine) Delete(ctx context.Context, key []byte) error {
	<-e.ready
	if !e.node.IsLeader() {
		return ErrNotLeader
	}
	cmd := Cmd{
		Type: CmdDelete,
		Key:  append([]byte(nil), key...),
	}
	data, err := encodeCmd(cmd)
	if err != nil {
		return err
	}
	return e.node.Propose(data, proposalTimeout)
}

// Scan 范围扫描，本地读取
func (e *RaftEngine) Scan(ctx context.Context, start, end []byte, opts storage.ScanOptions) (storage.Iterator, error) {
	<-e.ready
	return e.local.Scan(ctx, start, end, opts)
}

// PrefixScan 前缀扫描，本地读取
func (e *RaftEngine) PrefixScan(ctx context.Context, prefix []byte, opts storage.ScanOptions) (storage.Iterator, error) {
	<-e.ready
	return e.local.PrefixScan(ctx, prefix, opts)
}

// NewTransaction 事务：写事务在Leader上执行，读事务本地执行
func (e *RaftEngine) NewTransaction(update bool) (storage.Transaction, error) {
	<-e.ready
	if update {
		if !e.node.IsLeader() {
			return nil, ErrNotLeader
		}
		// 写事务：先在本地缓存，Commit时通过Raft提交
		return &raftTxn{
			engine: e,
			ops:    make([]storage.WriteOp, 0),
		}, nil
	}
	// 只读事务：本地执行
	return e.local.NewTransaction(false)
}

// BatchWrite 批量写入，通过Raft复制
func (e *RaftEngine) BatchWrite(ctx context.Context, ops []storage.WriteOp) error {
	<-e.ready
	if !e.node.IsLeader() {
		return ErrNotLeader
	}
	batchOps := make([]WriteOpCmd, len(ops))
	for i, op := range ops {
		batchOps[i] = WriteOpCmd{
			Type:  op.Type,
			Key:   append([]byte(nil), op.Key...),
			Value: append([]byte(nil), op.Value...),
		}
	}
	cmd := Cmd{
		Type: CmdBatch,
		Ops:  batchOps,
	}
	data, err := encodeCmd(cmd)
	if err != nil {
		return err
	}
	return e.node.Propose(data, proposalTimeout)
}

// Sync 同步
func (e *RaftEngine) Sync() error {
	<-e.ready
	return e.local.Sync()
}

// Node 返回底层 Raft 节点（用于集群管理API）
func (e *RaftEngine) Node() *Node {
	return e.node
}

// ---- 状态机应用 ----

// applyCmd 应用Raft日志命令到本地状态机
func (e *RaftEngine) applyCmd(term, index uint64, data []byte) error {
	if len(data) == 0 {
		// NoOp 空条目
		return nil
	}
	// 检查是否是配置变更日志
	firstByte := data[0]
	if firstByte == byte(LogAddPeer) || firstByte == byte(LogRemovePeer) {
		// 配置变更由Raft节点自身处理
		entry := &LogEntry{Term: term, Index: index, Type: LogType(firstByte), Data: data}
		return e.node.applyConfigChange(entry)
	}

	cmd, err := decodeCmd(data)
	if err != nil {
		return fmt.Errorf("decode command at index %d: %w", index, err)
	}

	ctx := context.Background()
	switch cmd.Type {
	case CmdSet:
		return e.local.Set(ctx, cmd.Key, cmd.Value)
	case CmdDelete:
		return e.local.Delete(ctx, cmd.Key)
	case CmdBatch:
		ops := make([]storage.WriteOp, len(cmd.Ops))
		for i, op := range cmd.Ops {
			ops[i] = storage.WriteOp{
				Type:  op.Type,
				Key:   op.Key,
				Value: op.Value,
			}
		}
		return e.local.BatchWrite(ctx, ops)
	default:
		return fmt.Errorf("unknown command type: %d", cmd.Type)
	}
}

// applySnapshot 应用快照
func (e *RaftEngine) applySnapshot(r io.Reader) error {
	// TODO: 实现快照恢复
	// 目前简化实现：快照通过BadgerDB自身的备份/恢复机制实现
	e.logger.Warn("snapshot restore not fully implemented")
	return nil
}

// getSnapshot 获取快照
func (e *RaftEngine) getSnapshot() (io.ReadCloser, error) {
	// TODO: 实现快照生成
	return nil, nil
}

// ---- 事务实现 ----

type raftTxn struct {
	engine *RaftEngine
	ops    []storage.WriteOp
}

func (t *raftTxn) Get(key []byte) ([]byte, error) {
	// 写事务中读取：先检查本地缓存，再读取本地状态机
	for i := len(t.ops) - 1; i >= 0; i-- {
		op := t.ops[i]
		if string(op.Key) == string(key) {
			if op.Type == storage.OpDelete {
				return nil, storage.ErrKeyNotFound
			}
			return append([]byte(nil), op.Value...), nil
		}
	}
	return t.engine.local.Get(context.Background(), key)
}

func (t *raftTxn) Set(key, value []byte) error {
	t.ops = append(t.ops, storage.WriteOp{
		Type:  storage.OpPut,
		Key:   append([]byte(nil), key...),
		Value: append([]byte(nil), value...),
	})
	return nil
}

func (t *raftTxn) Delete(key []byte) error {
	t.ops = append(t.ops, storage.WriteOp{
		Type: storage.OpDelete,
		Key:  append([]byte(nil), key...),
	})
	return nil
}

func (t *raftTxn) Commit() error {
	if len(t.ops) == 0 {
		return nil
	}
	return t.engine.BatchWrite(context.Background(), t.ops)
}

func (t *raftTxn) Discard() {
	t.ops = nil
}

// ---- 命令序列化 ----

func encodeCmd(cmd Cmd) ([]byte, error) {
	// 简单格式：type(1) + json body
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 1+4+len(body))
	buf[0] = byte(cmd.Type)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(body)))
	copy(buf[5:], body)
	return buf, nil
}

func decodeCmd(data []byte) (Cmd, error) {
	if len(data) < 5 {
		// 兼容旧格式（可能是NoOp空条目）
		return Cmd{}, nil
	}
	bodyLen := binary.BigEndian.Uint32(data[1:5])
	if int(5+bodyLen) > len(data) {
		return Cmd{}, fmt.Errorf("cmd data too short")
	}
	var cmd Cmd
	err := json.Unmarshal(data[5:5+bodyLen], &cmd)
	// 强制设置Type（与第一个字节一致）
	cmd.Type = CmdType(data[0])
	return cmd, err
}
