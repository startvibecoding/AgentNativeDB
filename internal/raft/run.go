package raft

import (
	"math/rand"
	"time"
)

// run 主循环，处理选举、心跳、日志复制
func (n *Node) run() {
	n.resetElectionTimer()

	for {
		select {
		case <-n.shutdownCh:
			return
		case <-n.electionTimer.C:
			n.startElection()
		case <-n.heartbeatChan():
			n.sendHeartbeats()
		case prop := <-n.proposalCh:
			n.handleProposal(prop)
		}
	}
}

// heartbeatChan 返回心跳 ticker channel（非leader时返回nil）
func (n *Node) heartbeatChan() <-chan time.Time {
	n.mu.RLock()
	ticker := n.heartbeatTicker
	n.mu.RUnlock()
	if ticker == nil {
		return nil
	}
	return ticker.C
}

// resetElectionTimer 重置选举计时器
func (n *Node) resetElectionTimer() {
	timeout := n.cfg.ElectionTimeout + time.Duration(rand.Int63n(int64(n.cfg.ElectionTimeout)))
	if n.electionTimer == nil {
		n.electionTimer = time.NewTimer(timeout)
	} else {
		if !n.electionTimer.Stop() {
			select {
			case <-n.electionTimer.C:
			default:
			}
		}
		n.electionTimer.Reset(timeout)
	}
}

// startElection 发起选举（先PreVote再真正选举）
func (n *Node) startElection() {
	// 首先进行PreVote，不增加term
	lastLogIndex := n.getLastLogIndex()
	lastLogTerm := n.getLastLogTerm()
	term := n.currentTerm.Load()

	peers := make(map[string]string)
	n.mu.RLock()
	for k, v := range n.peers {
		peers[k] = v
	}
	n.mu.RUnlock()

	otherPeers := 0
	for id := range peers {
		if id != n.cfg.NodeID {
			otherPeers++
		}
	}

	// 单节点直接当选
	if otherPeers == 0 {
		n.mu.Lock()
		n.state.Store(Candidate)
		n.currentTerm.Add(1)
		_ = n.storage.SetUint64(KeyCurrentTerm, n.currentTerm.Load())
		n.votedFor = n.cfg.NodeID
		_ = n.storage.SetString(KeyVotedFor, n.cfg.NodeID)
		n.mu.Unlock()
		n.becomeLeader()
		return
	}

	// PreVote 阶段：检查是否能获得多数投票
	preVotes := 1 // 自己
	granted := make(chan bool, otherPeers)
	for peerID, peerAddr := range peers {
		if peerID == n.cfg.NodeID {
			continue
		}
		go func(id, addr string) {
			resp, err := n.transport.RequestVote(addr, &RequestVoteRequest{
				Term:         term + 1, // 预投票用下一个term
				CandidateID:  n.cfg.NodeID,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
				PreVote:      true,
			})
			if err != nil {
				granted <- false
				return
			}
			// 如果收到更高term，重置选举超时，不发起选举
			if resp.Term > term {
				n.mu.Lock()
				n.currentTerm.Store(resp.Term)
				_ = n.storage.SetUint64(KeyCurrentTerm, resp.Term)
				n.votedFor = ""
				_ = n.storage.SetString(KeyVotedFor, "")
				n.state.Store(Follower)
				n.mu.Unlock()
				granted <- false
				return
			}
			granted <- resp.VoteGranted
		}(peerID, peerAddr)
	}

	// 收集PreVote结果
	preVoteSuccess := false
	for i := 0; i < otherPeers; i++ {
		if <-granted {
			preVotes++
		}
		if preVotes*2 > len(peers) {
			preVoteSuccess = true
			break
		}
	}

	if !preVoteSuccess {
		n.resetElectionTimer()
		return
	}

	// PreVote通过，开始真正选举
	n.mu.Lock()
	n.state.Store(Candidate)
	newTerm := n.currentTerm.Add(1)
	n.votedFor = n.cfg.NodeID
	_ = n.storage.SetUint64(KeyCurrentTerm, newTerm)
	_ = n.storage.SetString(KeyVotedFor, n.cfg.NodeID)
	n.leaderID.Store("")
	lastLogIndex = n.getLastLogIndex()
	lastLogTerm = n.getLastLogTerm()
	n.mu.Unlock()

	n.logger.Info("starting election", "term", newTerm)

	// 给自己投票
	votes := 1
	granted = make(chan bool, otherPeers)

	// 并行发送 RequestVote 给其他节点
	for peerID, peerAddr := range peers {
		if peerID == n.cfg.NodeID {
			continue
		}
		go func(id, addr string) {
			resp, err := n.transport.RequestVote(addr, &RequestVoteRequest{
				Term:         newTerm,
				CandidateID:  n.cfg.NodeID,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			})
			if err != nil {
				granted <- false
				return
			}
			// 如果发现更高任期，退回 Follower
			if resp.Term > newTerm {
				n.mu.Lock()
				n.currentTerm.Store(resp.Term)
				n.state.Store(Follower)
				n.votedFor = ""
				_ = n.storage.SetUint64(KeyCurrentTerm, resp.Term)
				_ = n.storage.SetString(KeyVotedFor, "")
				n.mu.Unlock()
				granted <- false
				return
			}
			granted <- resp.VoteGranted && resp.Term == newTerm
		}(peerID, peerAddr)
	}

	// 收集投票
	for i := 0; i < otherPeers; i++ {
		if <-granted {
			votes++
		}
		// 获得多数票即当选
		if votes*2 > len(peers) {
			n.becomeLeader()
			return
		}
	}

	// 未当选，重置选举超时
	n.mu.Lock()
	if n.state.Load() == Candidate {
		n.state.Store(Follower)
	}
	n.mu.Unlock()
	n.resetElectionTimer()
}

// becomeLeader 转换为 Leader 状态
func (n *Node) becomeLeader() {
	n.mu.Lock()

	if n.state.Load() == Leader {
		n.mu.Unlock()
		return
	}

	n.state.Store(Leader)
	n.leaderID.Store(n.cfg.NodeID)
	n.heartbeatTicker = time.NewTicker(n.cfg.HeartbeatTimeout)

	lastLogIdx := n.getLastLogIndex()
	for peerID := range n.peers {
		if peerID == n.cfg.NodeID {
			continue
		}
		n.nextIndex[peerID] = lastLogIdx + 1
		n.matchIndex[peerID] = 0
	}
	n.matchIndex[n.cfg.NodeID] = lastLogIdx

	n.logger.Info("became leader", "term", n.currentTerm.Load(), "index", lastLogIdx)
	n.mu.Unlock()

	// 追加一条 NoOp 条目提交前面任期的日志（不持锁调用）
	n.mu.Lock()
	entry := &LogEntry{
		Term:  n.currentTerm.Load(),
		Index: lastLogIdx + 1,
		Type:  LogNoOp,
		Data:  nil,
	}
	if err := n.storage.StoreLog(entry); err != nil {
		n.mu.Unlock()
		n.logger.Error("store NoOp log error", "err", err)
		return
	}
	n.matchIndex[n.cfg.NodeID] = entry.Index
	// 单节点立即提交
	var ops []applyOp
	if len(n.peers) == 1 {
		n.commitIndex.Store(entry.Index)
		_ = n.storage.SetUint64(KeyCommitIndex, entry.Index)
		ops = n.collectApplyOps()
	}
	n.mu.Unlock()
	if len(ops) > 0 {
		n.doApply(ops)
	}
}

// stepDown 降级为 Follower
func (n *Node) stepDown(term uint64) {
	if term > n.currentTerm.Load() {
		n.currentTerm.Store(term)
		_ = n.storage.SetUint64(KeyCurrentTerm, term)
		n.votedFor = ""
		_ = n.storage.SetString(KeyVotedFor, "")
	}
	if n.heartbeatTicker != nil {
		n.heartbeatTicker.Stop()
		n.heartbeatTicker = nil
	}
	n.state.Store(Follower)
	n.leaderID.Store("")
	n.resetElectionTimer()
}

// sendHeartbeats 向所有节点发送心跳/日志复制
func (n *Node) sendHeartbeats() {
	if n.State() != Leader {
		return
	}
	n.mu.RLock()
	term := n.currentTerm.Load()
	commitIdx := n.commitIndex.Load()
	peers := make(map[string]string)
	for k, v := range n.peers {
		if k != n.cfg.NodeID {
			peers[k] = v
		}
	}
	n.mu.RUnlock()

	for peerID, peerAddr := range peers {
		go n.replicateToPeer(peerID, peerAddr, term, commitIdx)
	}
}

// replicateToPeer 向单个节点复制日志
func (n *Node) replicateToPeer(peerID, peerAddr string, term, commitIdx uint64) {
	n.mu.Lock()
	nextIdx := n.nextIndex[peerID]
	prevLogIndex := nextIdx - 1
	prevLogTerm := uint64(0)
	if prevLogIndex > 0 {
		entry, err := n.storage.GetLog(prevLogIndex)
		if err == nil {
			prevLogTerm = entry.Term
		}
	}

	// 发送从 nextIdx 开始的日志
	lastLogIdx := n.getLastLogIndex()
	var entries []*LogEntry
	if nextIdx <= lastLogIdx {
		var err error
		entries, err = n.storage.LogEntries(nextIdx, lastLogIdx+1, 1024*1024) // 1MB batch
		if err != nil {
			n.mu.Unlock()
			return
		}
	}
	n.mu.Unlock()

	resp, err := n.transport.AppendEntries(peerAddr, &AppendEntriesRequest{
		Term:         term,
		LeaderID:     n.cfg.NodeID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: commitIdx,
	})
	if err != nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// 如果任期更大，降级
	if resp.Term > term {
		n.stepDown(resp.Term)
		return
	}
	if n.state.Load() != Leader || resp.Term != term {
		return
	}

	if resp.Success {
		// 更新 nextIndex 和 matchIndex
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			n.nextIndex[peerID] = lastEntry.Index + 1
			n.matchIndex[peerID] = lastEntry.Index
		} else {
			// 心跳成功
			n.nextIndex[peerID] = nextIdx
		}
		// 尝试推进 commitIndex
		n.advanceCommitIndex()
	} else {
		// 失败，回退 nextIndex 重试
		if n.nextIndex[peerID] > 1 {
			n.nextIndex[peerID]--
		}
	}
}

// advanceCommitIndex 尝试推进已提交索引
func (n *Node) advanceCommitIndex() {
	var ops []applyOp
	for n.commitIndex.Load() < n.getLastLogIndex() {
		next := n.commitIndex.Load() + 1
		// 检查 next 是否可以提交（复制到多数节点，且任期等于当前任期）
		entry, err := n.storage.GetLog(next)
		if err != nil {
			break
		}
		// 不能提交之前任期的日志（Raft 安全属性）
		if entry.Term != n.currentTerm.Load() {
			break
		}
		// 统计复制到该索引的节点数
		count := 1 // 自己
		for peerID := range n.peers {
			if peerID == n.cfg.NodeID {
				continue
			}
			if n.matchIndex[peerID] >= next {
				count++
			}
		}
		if count*2 > len(n.peers) {
			n.commitIndex.Store(next)
			_ = n.storage.SetUint64(KeyCommitIndex, next)
		} else {
			break
		}
	}
	// 收集需要应用的日志
	ops = n.collectApplyOps()
	// 释放锁后应用（调用replicateToPeer的地方已经持锁，调用后释放）
	if len(ops) > 0 {
		// 注意：调用者在调用advanceCommitIndex()后会释放锁
		// 我们只收集ops，由调用方在解锁后执行doApply
		// 这里存储到临时位置，通过返回值返回
	}
	// 简化：直接解锁apply然后重新加锁
	// 因为调用者replicateToPeer持有锁，我们在这里unlock-apply-lock是安全的
	n.mu.Unlock()
	if len(ops) > 0 {
		n.doApply(ops)
	}
	n.mu.Lock()
}

// collectApplyOps 收集需要应用的日志（持锁调用）
func (n *Node) collectApplyOps() []applyOp {
	var ops []applyOp
	for n.lastApplied.Load() < n.commitIndex.Load() {
		idx := n.lastApplied.Load() + 1
		entry, err := n.storage.GetLog(idx)
		if err != nil {
			break
		}
		n.lastApplied.Store(idx)
		_ = n.storage.SetUint64(KeyLastApplied, idx)
		p := n.proposals[idx]
		delete(n.proposals, idx)
		ops = append(ops, applyOp{entry: entry, p: p})
	}
	return ops
}

type applyOp struct {
	entry *LogEntry
	p     *proposal
}

// doApply 实际应用日志（不持锁调用，避免死锁）
func (n *Node) doApply(ops []applyOp) {
	for _, op := range ops {
		var err error
		if op.entry.Type == LogCommand && n.cfg.Apply != nil {
			err = n.cfg.Apply(op.entry.Term, op.entry.Index, op.entry.Data)
			if err != nil {
				n.logger.Error("apply log error", "index", op.entry.Index, "err", err)
			}
		}
		// 唤醒等待的提案
		if op.p != nil {
			op.p.done <- err
		}
	}
}

// handleProposal 处理 Leader 收到的新提案（必须在持锁时调用以原子分配索引）
func (n *Node) handleProposal(prop proposalWithCallback) {
	if n.State() != Leader {
		prop.p.done <- ErrNotLeader
		return
	}
	n.mu.Lock()
	idx := n.getLastLogIndex() + 1
	entry := &LogEntry{
		Term:  n.currentTerm.Load(),
		Index: idx,
		Type:  LogCommand,
		Data:  prop.cmd,
	}
	if err := n.storage.StoreLog(entry); err != nil {
		n.mu.Unlock()
		n.logger.Error("append log error", "err", err)
		prop.p.done <- err
		return
	}
	// 注册提案回调
	n.proposals[idx] = prop.p
	n.matchIndex[n.cfg.NodeID] = entry.Index
	n.nextIndex[n.cfg.NodeID] = entry.Index + 1

	// 单节点集群立即提交
	peerCount := len(n.peers)
	shouldApply := peerCount == 1
	var ops []applyOp
	if shouldApply {
		n.commitIndex.Store(entry.Index)
		_ = n.storage.SetUint64(KeyCommitIndex, entry.Index)
		ops = n.collectApplyOps()
	}
	n.mu.Unlock()
	if len(ops) > 0 {
		n.doApply(ops)
	}
	// 立即触发心跳/复制
	go n.sendHeartbeats()
}
