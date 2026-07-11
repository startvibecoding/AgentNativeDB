// Package raft 实现 AgentNativeDB 的 Raft 共识模块，用于集群高可用。
//
// 设计：
//   - 纯 Go 实现，无 CGO 依赖
//   - 复用 BadgerDB 作为 Raft 日志、稳定状态、快照存储
//   - HTTP 传输层复用现有 HTTP 服务端口
//   - 写操作通过 Raft 日志复制到多数节点后才应用到状态机
//   - 读操作默认从 Leader 读取（线性一致性），可配置允许 Follower 读
package raft

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Raft 状态常量
const (
	Follower     = iota // 跟随者
	PreCandidate        // 预候选人（PreVote阶段，不增加term）
	Candidate           // 候选人
	Leader              // 领导者
)

// 错误定义
var (
	ErrNotLeader     = errors.New("raft: not leader")
	ErrLeadershipLost = errors.New("raft: leadership lost during proposal")
	ErrTimeout       = errors.New("raft: request timeout")
	ErrShutdown      = errors.New("raft: node is shutting down")
	ErrClusterExists = errors.New("raft: cluster already bootstrapped")
)

// Config Raft 配置
type Config struct {
	// NodeID 当前节点ID
	NodeID string
	// RaftAddr Raft 通信地址（host:port），通常和HTTP API同端口通过路径区分
	RaftAddr string
	// DataDir Raft 数据目录（与 BadgerDB 数据目录隔离使用 key 前缀）
	DataDir string
	// Peers 初始集群节点列表 map[nodeID] = "host:port"
	Peers map[string]string
	// HeartbeatTimeout 心跳间隔
	HeartbeatTimeout time.Duration
	// ElectionTimeout 选举超时（随机化范围 [ElectionTimeout, 2*ElectionTimeout)）
	ElectionTimeout time.Duration
	// LeaseInterval Leader 租约有效期，用于读操作优化
	LeaseInterval time.Duration
	// SnapshotInterval 快照间隔（基于日志条数）
	SnapshotThreshold uint64
	// AppliedIndex 外部状态机回调，用于应用日志条目
	Apply func(term, index uint64, cmd []byte) error
	// ApplySnapshot 外部状态机回调，用于恢复快照
	ApplySnapshot func(snapshot io.Reader) error
	// GetSnapshot 外部状态机回调，用于生成快照
	GetSnapshot func() (io.ReadCloser, error)
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		HeartbeatTimeout:  100 * time.Millisecond,
		ElectionTimeout:   1000 * time.Millisecond,
		LeaseInterval:     200 * time.Millisecond,
		SnapshotThreshold: 10000,
	}
}

// Node 表示一个 Raft 节点
type Node struct {
	mu      sync.RWMutex
	cfg     Config
	logger  *slog.Logger
	storage *RaftStorage
	transport *HTTPTransport

	// 当前节点状态
	state       atomic.Int32
	currentTerm atomic.Uint64
	votedFor    string // 当前任期投票给的节点ID
	commitIndex atomic.Uint64
	lastApplied atomic.Uint64
	leaderID    atomic.Value // string

	// Leader 状态
	nextIndex  map[string]uint64
	matchIndex map[string]uint64

	// 选举计时器
	electionTimer *time.Timer
	// 心跳发送（仅 Leader）
	heartbeatTicker *time.Ticker

	// 提案等待队列：index -> proposal
	proposals map[uint64]*proposal
	proposalCh chan proposalWithCallback

	// 关闭
	shutdownCh chan struct{}
	running    atomic.Bool

	// 本地节点ID列表（用于确认）
	peers map[string]string // nodeID -> addr
}

type proposal struct {
	cmd  []byte
	done chan error
}

type proposalWithCallback struct {
	cmd []byte
	p   *proposal
}

// NewNode 创建新的 Raft 节点
func NewNode(cfg Config) (*Node, error) {
	if cfg.NodeID == "" {
		return nil, errors.New("raft: node ID is required")
	}
	if cfg.RaftAddr == "" {
		return nil, errors.New("raft: raft address is required")
	}
	if cfg.Apply == nil {
		return nil, errors.New("raft: Apply callback is required")
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = DefaultConfig().HeartbeatTimeout
	}
	if cfg.ElectionTimeout <= 0 {
		cfg.ElectionTimeout = DefaultConfig().ElectionTimeout
	}
	if cfg.LeaseInterval <= 0 {
		cfg.LeaseInterval = DefaultConfig().LeaseInterval
	}
	if cfg.SnapshotThreshold <= 0 {
		cfg.SnapshotThreshold = DefaultConfig().SnapshotThreshold
	}

	store, err := NewRaftStorage(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("raft: open storage: %w", err)
	}

	n := &Node{
		cfg:       cfg,
		logger:    slog.With("module", "raft", "node", cfg.NodeID),
		storage:   store,
		peers:     make(map[string]string),
		nextIndex: make(map[string]uint64),
		matchIndex: make(map[string]uint64),
		proposals: make(map[uint64]*proposal),
		proposalCh: make(chan proposalWithCallback, 1024),
		shutdownCh: make(chan struct{}),
	}

	// 初始化 peers
	for id, addr := range cfg.Peers {
		n.peers[id] = addr
	}

	// 恢复持久化状态
	if term, err := store.GetUint64(KeyCurrentTerm); err == nil {
		n.currentTerm.Store(term)
	}
	if votedFor, err := store.GetString(KeyVotedFor); err == nil {
		n.votedFor = votedFor
	}
	if cluster, err := store.GetCluster(); err == nil {
		for id, addr := range cluster {
			n.peers[id] = addr
		}
	}

	n.state.Store(Follower)
	return n, nil
}

// Start 启动 Raft 节点
func (n *Node) Start() error {
	if n.running.Load() {
		return errors.New("raft: already started")
	}
	n.running.Store(true)

	// 初始化传输层
	n.transport = NewHTTPTransport(n.cfg.RaftAddr, n)
	go n.transport.Start()

	// 应用已提交但未应用的日志
	if err := n.applyCommitted(); err != nil {
		return fmt.Errorf("apply committed logs: %w", err)
	}

	// 启动主循环
	go n.run()

	n.logger.Info("raft node started",
		"addr", n.cfg.RaftAddr,
		"term", n.currentTerm.Load(),
		"peers", len(n.peers),
	)
	return nil
}

// BootstrapCluster 初始化集群（仅在首次启动时在单个节点调用）
func (n *Node) BootstrapCluster(peers map[string]string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if existing, _ := n.storage.GetCluster(); len(existing) > 0 {
		return ErrClusterExists
	}

	// 保存集群配置
	if err := n.storage.SetCluster(peers); err != nil {
		return err
	}
	for id, addr := range peers {
		n.peers[id] = addr
	}

	n.logger.Info("cluster bootstrapped", "peers", peers)
	return nil
}

// AddPeer 向集群添加节点（仅 Leader 可调用，通过Raft日志复制）
func (n *Node) AddPeer(nodeID, addr string) error {
	if n.State() != Leader {
		return ErrNotLeader
	}

	// 通过提案提交配置变更
	type peerChange struct {
		Op     string `json:"op"` // "add" or "remove"
		NodeID string `json:"node_id"`
		Addr   string `json:"addr,omitempty"`
	}
	cmd := peerChange{Op: "add", NodeID: nodeID, Addr: addr}
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	// 包装成特殊命令类型
	logData := append([]byte{byte(LogAddPeer)}, data...)
	return n.Propose(logData, proposalTimeout)
}

// RemovePeer 从集群移除节点（仅 Leader 可调用，通过Raft日志复制）
func (n *Node) RemovePeer(nodeID string) error {
	if n.State() != Leader {
		return ErrNotLeader
	}
	if nodeID == n.cfg.NodeID {
		return errors.New("cannot remove self")
	}
	type peerChange struct {
		Op     string `json:"op"`
		NodeID string `json:"node_id"`
	}
	cmd := peerChange{Op: "remove", NodeID: nodeID}
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	logData := append([]byte{byte(LogRemovePeer)}, data...)
	return n.Propose(logData, proposalTimeout)
}

// applyConfigChange 应用配置变更（在Apply回调中调用）
func (n *Node) applyConfigChange(entry *LogEntry) error {
	if len(entry.Data) < 1 {
		return errors.New("empty config change entry")
	}
	opType := LogType(entry.Data[0])
	data := entry.Data[1:]

	type peerChange struct {
		Op     string `json:"op"`
		NodeID string `json:"node_id"`
		Addr   string `json:"addr,omitempty"`
	}
	var cmd peerChange
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	switch opType {
	case LogAddPeer:
		n.peers[cmd.NodeID] = cmd.Addr
		// 初始化nextIndex为当前最后一条日志+1
		lastIdx := n.getLastLogIndex()
		if _, ok := n.nextIndex[cmd.NodeID]; !ok {
			n.nextIndex[cmd.NodeID] = lastIdx + 1
			n.matchIndex[cmd.NodeID] = 0
		}
		n.logger.Info("peer added", "node_id", cmd.NodeID, "addr", cmd.Addr)
	case LogRemovePeer:
		delete(n.peers, cmd.NodeID)
		delete(n.nextIndex, cmd.NodeID)
		delete(n.matchIndex, cmd.NodeID)
		n.logger.Info("peer removed", "node_id", cmd.NodeID)
	}
	// 持久化集群配置
	return n.storage.SetCluster(n.peers)
}

// Propose 提交一个写操作（仅 Leader 可调用，返回后表示已复制到多数节点）
func (n *Node) Propose(cmd []byte, timeout time.Duration) error {
	if n.State() != Leader {
		return ErrNotLeader
	}

	p := &proposal{
		cmd:  cmd,
		done: make(chan error, 1),
	}

	// 发送到主循环，由主循环统一分配索引并注册提案
	select {
	case n.proposalCh <- proposalWithCallback{cmd: cmd, p: p}:
	case <-n.shutdownCh:
		return ErrShutdown
	}

	// 等待结果
	select {
	case err := <-p.done:
		return err
	case <-time.After(timeout):
		// 超时后提案可能仍在处理中，由主循环自动清理
		return ErrTimeout
	case <-n.shutdownCh:
		return ErrShutdown
	}
}

// State 返回当前节点状态
func (n *Node) State() int32 {
	return n.state.Load()
}

// IsLeader 返回是否为 Leader
func (n *Node) IsLeader() bool {
	return n.State() == Leader
}

// LeaderID 返回当前已知的 Leader ID
func (n *Node) LeaderID() string {
	if v := n.leaderID.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// LeaderAddr 返回当前 Leader 地址
func (n *Node) LeaderAddr() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	id := n.LeaderID()
	if id == "" {
		return ""
	}
	return n.peers[id]
}

// Term 返回当前任期
func (n *Node) Term() uint64 {
	return n.currentTerm.Load()
}

// CommitIndex 返回已提交索引
func (n *Node) CommitIndex() uint64 {
	return n.commitIndex.Load()
}

// LastApplied 返回已应用索引
func (n *Node) LastApplied() uint64 {
	return n.lastApplied.Load()
}

// Peers 返回集群节点列表
func (n *Node) Peers() map[string]string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	result := make(map[string]string, len(n.peers))
	for k, v := range n.peers {
		result[k] = v
	}
	return result
}

// getLastLogIndex 获取最后一条日志索引
func (n *Node) getLastLogIndex() uint64 {
	idx, err := n.storage.LastIndex()
	if err != nil {
		return 0
	}
	return idx
}

// getLastLogTerm 获取最后一条日志的任期
func (n *Node) getLastLogTerm() uint64 {
	idx := n.getLastLogIndex()
	if idx == 0 {
		return 0
	}
	entry, err := n.storage.GetLog(idx)
	if err != nil {
		return 0
	}
	return entry.Term
}

// applyCommitted 应用启动时已提交但未应用的日志
func (n *Node) applyCommitted() error {
	lastLog := n.getLastLogIndex()
	commitIdx := n.commitIndex.Load()
	applied := n.lastApplied.Load()

	// 从持久化中恢复 commitIndex
	if stored, err := n.storage.GetUint64(KeyCommitIndex); err == nil {
		commitIdx = stored
		n.commitIndex.Store(commitIdx)
	}
	if stored, err := n.storage.GetUint64(KeyLastApplied); err == nil {
		applied = stored
		n.lastApplied.Store(applied)
	}

	n.logger.Info("recovering state",
		"last_log", lastLog,
		"commit_index", commitIdx,
		"last_applied", applied,
	)

	// 应用未应用的日志（启动时无并发，直接调用Apply即可）
	for i := applied + 1; i <= commitIdx; i++ {
		entry, err := n.storage.GetLog(i)
		if err != nil {
			return fmt.Errorf("get log %d: %w", i, err)
		}
		if entry.Type == LogCommand {
			if err := n.cfg.Apply(entry.Term, entry.Index, entry.Data); err != nil {
				return fmt.Errorf("apply log %d: %w", i, err)
			}
		}
		n.lastApplied.Store(i)
	}
	_ = n.storage.SetUint64(KeyLastApplied, n.lastApplied.Load())
	return nil
}

// Status 返回集群状态
type Status struct {
	NodeID      string            `json:"node_id"`
	State       string            `json:"state"`
	Term        uint64            `json:"term"`
	CommitIndex uint64            `json:"commit_index"`
	LastApplied uint64            `json:"last_applied"`
	LeaderID    string            `json:"leader_id"`
	LeaderAddr  string            `json:"leader_addr,omitempty"`
	Peers       map[string]string `json:"peers"`
}

// Status 返回当前节点状态
func (n *Node) Status() Status {
	state := "follower"
	switch n.State() {
	case PreCandidate:
		state = "pre-candidate"
	case Candidate:
		state = "candidate"
	case Leader:
		state = "leader"
	}
	return Status{
		NodeID:      n.cfg.NodeID,
		State:       state,
		Term:        n.Term(),
		CommitIndex: n.CommitIndex(),
		LastApplied: n.LastApplied(),
		LeaderID:    n.LeaderID(),
		LeaderAddr:  n.LeaderAddr(),
		Peers:       n.Peers(),
	}
}

// HTTPHandler 返回 Raft HTTP 处理器，需要挂载到 HTTP 路由
func (n *Node) HTTPHandler() http.Handler {
	return n.transport.Handler()
}

// Shutdown 关闭 Raft 节点
func (n *Node) Shutdown() error {
	if !n.running.CompareAndSwap(true, false) {
		return nil
	}
	close(n.shutdownCh)

	if n.heartbeatTicker != nil {
		n.heartbeatTicker.Stop()
	}
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}

	_ = n.storage.Close()
	n.logger.Info("raft node stopped")
	return nil
}

// LogEntry Raft 日志条目
type LogEntry struct {
	Index uint64
	Term  uint64
	Type  LogType
	Data  []byte
}

// LogType 日志类型
type LogType byte

const (
	LogCommand    LogType = iota // 普通写命令
	LogNoOp                      // 空操作（Leader上任时追加）
	LogAddPeer                   // 添加节点（配置变更）
	LogRemovePeer                // 删除节点（配置变更）
	LogConfiguration             // 配置变更
)

// 存储key前缀（使用 Badger key 前缀与数据隔离）
const (
	raftLogPrefix     byte = 0xF0 // Raft日志前缀
	raftStablePrefix  byte = 0xF1 // Raft稳定状态前缀
	raftSnapshotPrefix byte = 0xF2 // Raft快照前缀
)

// Key 定义
const (
	KeyCurrentTerm  string = "current_term"
	KeyVotedFor     string = "voted_for"
	KeyCommitIndex  string = "commit_index"
	KeyLastApplied  string = "last_applied"
	KeyClusterConfig string = "cluster_config"
)

// u64tob 将uint64转为大端字节
func u64tob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// btou64 将大端字节转为uint64
func btou64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
