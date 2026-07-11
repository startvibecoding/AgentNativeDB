package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// ---- RPC 消息定义 ----

// RequestVoteRequest 请求投票
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
	PreVote      bool   `json:"pre_vote,omitempty"` // PreVote: 不增加term，仅探测是否能获选
}

// RequestVoteResponse 投票响应
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// AppendEntriesRequest 日志复制/心跳请求
type AppendEntriesRequest struct {
	Term         uint64      `json:"term"`
	LeaderID     string      `json:"leader_id"`
	PrevLogIndex uint64      `json:"prev_log_index"`
	PrevLogTerm  uint64      `json:"prev_log_term"`
	Entries      []*LogEntry `json:"entries,omitempty"`
	LeaderCommit uint64      `json:"leader_commit"`
}

// AppendEntriesResponse 日志复制响应
type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
	// 用于快速回退 nextIndex
	ConflictIndex uint64 `json:"conflict_index,omitempty"`
}

// InstallSnapshotRequest 安装快照请求
type InstallSnapshotRequest struct {
	Term              uint64 `json:"term"`
	LeaderID          string `json:"leader_id"`
	LastIncludedIndex uint64 `json:"last_included_index"`
	LastIncludedTerm  uint64 `json:"last_included_term"`
	// 快照数据通过请求体直接传输
}

// InstallSnapshotResponse 安装快照响应
type InstallSnapshotResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

const (
	pathRequestVote    = "/raft/vote"
	pathAppendEntries  = "/raft/append"
	pathInstallSnapshot = "/raft/snapshot"
)

// HTTPTransport 基于HTTP的Raft传输层
type HTTPTransport struct {
	addr   string
	node   *Node
	server *http.Server
	mux    *http.ServeMux
	logger *slog.Logger
	client *http.Client
}

// NewHTTPTransport 创建HTTP传输层
func NewHTTPTransport(addr string, node *Node) *HTTPTransport {
	t := &HTTPTransport{
		addr:   addr,
		node:   node,
		logger: slog.With("module", "raft-transport"),
		mux:    http.NewServeMux(),
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 10,
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
	t.mux.HandleFunc(pathRequestVote, t.handleRequestVote)
	t.mux.HandleFunc(pathAppendEntries, t.handleAppendEntries)
	t.mux.HandleFunc(pathInstallSnapshot, t.handleInstallSnapshot)
	return t
}

// Handler 返回HTTP Handler，用于挂载到现有HTTP服务
func (t *HTTPTransport) Handler() http.Handler {
	return t.mux
}

// Start 启动独立的HTTP服务（如果不挂载到现有服务）
func (t *HTTPTransport) Start() error {
	// 不默认启动，通过Handler()挂载到主服务
	// 这里保留接口，实际不监听端口
	<-make(chan struct{})
	return nil
}

// ---- 服务端处理 ----

func (t *HTTPTransport) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	var req RequestVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp := t.node.HandleRequestVote(&req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) handleAppendEntries(w http.ResponseWriter, r *http.Request) {
	var req AppendEntriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp := t.node.HandleAppendEntries(&req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) handleInstallSnapshot(w http.ResponseWriter, r *http.Request) {
	// TODO: 快照安装实现
	resp := &InstallSnapshotResponse{Term: t.node.Term(), Success: false}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ---- 客户端调用 ----

// RequestVote 发送投票请求
func (t *HTTPTransport) RequestVote(peerAddr string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	resp := &RequestVoteResponse{}
	if err := t.rpcCall(peerAddr, pathRequestVote, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// AppendEntries 发送日志复制请求
func (t *HTTPTransport) AppendEntries(peerAddr string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	resp := &AppendEntriesResponse{}
	if err := t.rpcCall(peerAddr, pathAppendEntries, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// InstallSnapshot 发送快照安装请求
func (t *HTTPTransport) InstallSnapshot(peerAddr string, req *InstallSnapshotRequest, snapshot io.Reader) (*InstallSnapshotResponse, error) {
	// TODO: 实现快照传输
	return &InstallSnapshotResponse{Term: t.node.Term()}, nil
}

func (t *HTTPTransport) rpcCall(peerAddr, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s%s", peerAddr, path)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc error: status %d", httpResp.StatusCode)
	}
	return json.NewDecoder(httpResp.Body).Decode(resp)
}

// HandleRequestVote 处理投票请求（节点逻辑）
func (n *Node) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 重置选举定时器（收到有效RPC，除了PreVote拒绝外）
	shouldReset := true
	defer func() {
		if shouldReset {
			n.resetElectionTimer()
		}
	}()

	resp := &RequestVoteResponse{Term: n.currentTerm.Load()}

	// 如果请求任期小于当前任期，拒绝
	if req.Term < n.currentTerm.Load() {
		resp.VoteGranted = false
		return resp
	}

	// PreVote处理：不修改当前term，仅做可达性和日志新鲜度检查
	if req.PreVote {
		// 如果我们已经有Leader，并且Leader在正常工作，拒绝PreVote
		// 简单判断：如果当前term等于req.Term-1且我们最近收到过Leader心跳？这里简化处理
		// 只要日志足够新就投票，不修改自身状态
		lastLogIdx := n.getLastLogIndex()
		lastLogTerm := n.getLastLogTerm()
		logOk := false
		if req.LastLogTerm > lastLogTerm {
			logOk = true
		} else if req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIdx {
			logOk = true
		}
		resp.VoteGranted = logOk
		resp.Term = n.currentTerm.Load()
		// PreVote不重置选举计时器
		shouldReset = false
		return resp
	}

	// 如果请求任期更大，更新任期并转为Follower
	if req.Term > n.currentTerm.Load() {
		n.currentTerm.Store(req.Term)
		_ = n.storage.SetUint64(KeyCurrentTerm, req.Term)
		n.votedFor = ""
		_ = n.storage.SetString(KeyVotedFor, "")
		n.state.Store(Follower)
		n.leaderID.Store("")
		resp.Term = req.Term
	}

	// 检查是否已投票
	canVote := n.votedFor == "" || n.votedFor == req.CandidateID
	if !canVote {
		resp.VoteGranted = false
		return resp
	}

	// 检查候选人日志是否至少和自己一样新
	lastLogIdx := n.getLastLogIndex()
	lastLogTerm := n.getLastLogTerm()
	logOk := false
	if req.LastLogTerm > lastLogTerm {
		logOk = true
	} else if req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIdx {
		logOk = true
	}

	if !logOk {
		resp.VoteGranted = false
		return resp
	}

	// 投票给候选人
	n.votedFor = req.CandidateID
	_ = n.storage.SetString(KeyVotedFor, req.CandidateID)
	n.state.Store(Follower)
	resp.VoteGranted = true

	n.logger.Info("voted for", "candidate", req.CandidateID, "term", req.Term)
	return resp
}

// HandleAppendEntries 处理日志复制/心跳请求
func (n *Node) HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	// 第一阶段：持锁处理日志追加和commit更新，收集待应用日志
	n.mu.Lock()
	resp := &AppendEntriesResponse{Term: n.currentTerm.Load()}
	var ops []applyOp

	// 任期太小，拒绝
	if req.Term < n.currentTerm.Load() {
		resp.Success = false
		n.mu.Unlock()
		n.resetElectionTimer()
		return resp
	}

	// 如果任期更大或相等，更新Leader信息
	if req.Term >= n.currentTerm.Load() {
		if req.Term > n.currentTerm.Load() {
			n.currentTerm.Store(req.Term)
			_ = n.storage.SetUint64(KeyCurrentTerm, req.Term)
			n.votedFor = ""
			_ = n.storage.SetString(KeyVotedFor, "")
		}
		n.state.Store(Follower)
		n.leaderID.Store(req.LeaderID)
		resp.Term = req.Term

		// 停止心跳（如果正在作为候选人/Leader）
		if n.heartbeatTicker != nil {
			n.heartbeatTicker.Stop()
			n.heartbeatTicker = nil
		}
	}

	// 检查日志是否包含 PrevLogIndex, PrevLogTerm
	if req.PrevLogIndex > 0 {
		lastLogIdx := n.getLastLogIndex()
		if lastLogIdx < req.PrevLogIndex {
			// 缺少日志，返回冲突索引
			resp.Success = false
			resp.ConflictIndex = lastLogIdx + 1
			n.mu.Unlock()
			n.resetElectionTimer()
			return resp
		}
		prevEntry, err := n.storage.GetLog(req.PrevLogIndex)
		if err != nil || prevEntry.Term != req.PrevLogTerm {
			// 日志任期不匹配，回退
			resp.Success = false
			if prevEntry != nil {
				// 找到该任期的第一条日志
				conflictTerm := prevEntry.Term
				conflictIdx := req.PrevLogIndex
				for conflictIdx > 0 {
					e, err := n.storage.GetLog(conflictIdx - 1)
					if err != nil || e.Term != conflictTerm {
						break
					}
					conflictIdx--
				}
				resp.ConflictIndex = conflictIdx
			} else {
				resp.ConflictIndex = 1
			}
			n.mu.Unlock()
			n.resetElectionTimer()
			return resp
		}
	}

	// 追加新日志条目
	for _, entry := range req.Entries {
		// 检查是否已存在冲突的条目
		existing, err := n.storage.GetLog(entry.Index)
		if err == nil && existing.Term != entry.Term {
			// 删除冲突的条目及之后的所有条目
			lastIdx := n.getLastLogIndex()
			if entry.Index <= lastIdx {
				_ = n.storage.DeleteRange(entry.Index, lastIdx)
			}
		}
		// 如果不存在则存储
		if err != nil {
			if err := n.storage.StoreLog(entry); err != nil {
				n.logger.Error("store log error", "err", err)
				resp.Success = false
				n.mu.Unlock()
				n.resetElectionTimer()
				return resp
			}
		}
	}

	// 更新 Leader 的 commit 信息
	if req.LeaderCommit > n.commitIndex.Load() {
		lastLogIdx := n.getLastLogIndex()
		newCommit := req.LeaderCommit
		if newCommit > lastLogIdx {
			newCommit = lastLogIdx
		}
		n.commitIndex.Store(newCommit)
		_ = n.storage.SetUint64(KeyCommitIndex, newCommit)
		// 收集待应用日志
		ops = n.collectApplyOps()
	}

	resp.Success = true
	// 释放锁
	n.mu.Unlock()

	// 锁外应用日志
	if len(ops) > 0 {
		n.doApply(ops)
	}

	n.resetElectionTimer()
	return resp
}
