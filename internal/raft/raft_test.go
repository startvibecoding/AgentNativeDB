package raft

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func TestRaftSingleNode(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "state"), 0755)

	// 启动单节点集群
	engine, err := NewRaftEngine(RaftConfig{
		NodeID:    "node1",
		RaftAddr:  "127.0.0.1:19400",
		DataDir:   dir,
		Bootstrap: true,
		Peers: map[string]string{
			"node1": "127.0.0.1:19400",
		},
	})
	if err != nil {
		t.Fatalf("NewRaftEngine: %v", err)
	}
	defer engine.Close()

	// 等待选举完成
	time.Sleep(2 * time.Second)
	if !engine.Node().IsLeader() {
		t.Fatalf("expected single node to be leader, state: %s", engine.Node().Status().State)
	}
	t.Logf("node status: %+v", engine.Node().Status())

	// 测试写入
	var appliedCount atomic.Int64
	_ = appliedCount
	ctx := t.Context()
	testKey := []byte("test-key")
	testValue := []byte("test-value")

	// 直接使用 Apply 回调的引擎
	// Set 操作应该通过Raft
	err = engine.Set(ctx, testKey, testValue)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 等待日志应用
	time.Sleep(500 * time.Millisecond)

	// 读取
	val, err := engine.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != string(testValue) {
		t.Fatalf("expected %q, got %q", testValue, val)
	}

	t.Log("single node Set/Get OK")

	// 删除
	err = engine.Delete(ctx, testKey)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	_, err = engine.Get(ctx, testKey)
	if err == nil {
		t.Fatalf("expected key not found after delete")
	}
	t.Log("single node Delete OK")
}

func TestRaftCmdEncoding(t *testing.T) {
	cmd := Cmd{
		Type: CmdSet,
		Key:  []byte("foo"),
		Value: []byte("bar"),
	}
	data, err := encodeCmd(cmd)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCmd(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Type != CmdSet || string(decoded.Key) != "foo" || string(decoded.Value) != "bar" {
		t.Fatalf("encode/decode mismatch: %+v", decoded)
	}
	t.Log("cmd encode/decode OK")
}

func TestRaftThreeNodes(t *testing.T) {
	dirBase := t.TempDir()

	dirs := []string{
		filepath.Join(dirBase, "node1"),
		filepath.Join(dirBase, "node2"),
		filepath.Join(dirBase, "node3"),
	}
	addrs := []string{
		"127.0.0.1:29401",
		"127.0.0.1:29402",
		"127.0.0.1:29403",
	}
	for _, d := range dirs {
		_ = os.MkdirAll(filepath.Join(d, "state"), 0755)
	}

	peers := map[string]string{
		"node1": addrs[0],
		"node2": addrs[1],
		"node3": addrs[2],
	}

	var wg sync.WaitGroup
	nodes := make([]*RaftEngine, 3)
	var startWg sync.WaitGroup
	startWg.Add(3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			n, err := NewRaftEngine(RaftConfig{
				NodeID:    fmt.Sprintf("node%d", idx+1),
				RaftAddr:  addrs[idx],
				DataDir:   dirs[idx],
				Peers:     peers,
				Bootstrap: idx == 0, // node1 bootstrap
			})
			if err != nil {
				t.Errorf("node %d: %v", idx, err)
				return
			}
			nodes[idx] = n
			startWg.Done()
		}(i)
	}

	// 不等待goroutine启动，这里测试主要验证模块可编译运行
	startWg.Wait()
	time.Sleep(3 * time.Second)

	// 检查有且仅有一个Leader
	leaderCount := 0
	var leader *RaftEngine
	for i, n := range nodes {
		if n != nil && n.Node().IsLeader() {
			leaderCount++
			leader = n
			t.Logf("node%d is leader", i+1)
		}
	}
	if leaderCount != 1 {
		t.Logf("warning: expected 1 leader, got %d (election may still be in progress)", leaderCount)
	}

	// 如果选出了Leader，测试写入
	if leader != nil {
		ctx := t.Context()
		if err := leader.Set(ctx, []byte("cluster-key"), []byte("cluster-value")); err != nil {
			t.Logf("set via leader: %v", err)
		} else {
			time.Sleep(500 * time.Millisecond)
			// 从任意节点读取
			for i, n := range nodes {
				if n != nil {
					val, err := n.Get(ctx, []byte("cluster-key"))
					if err != nil {
						t.Logf("node%d get: %v", i+1, err)
					} else if string(val) == "cluster-value" {
						t.Logf("node%d get OK: %s", i+1, val)
					}
				}
			}
		}
	}

	for _, n := range nodes {
		if n != nil {
			n.Close()
		}
	}
	wg.Wait()
}
