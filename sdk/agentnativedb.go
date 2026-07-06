// Package agentnativedb 提供 AgentNativeDB 嵌入式数据库 SDK。
//
// 用法:
//
//	db, _ := agentnativedb.Open("./mydata")
//	defer db.Close()
//
//	// 创建会话
//	sess, _ := db.CreateSession("agent-001")
//
//	// 存储记忆
//	db.StoreMemory(sess.ID, "用户偏好中文", agentnativedb.LongTerm, 0.8)
//
//	// 检索记忆
//	memories, _ := db.RecallMemories(sess.ID, nil, 10)
//
//	// SQL 查询
//	result, _ := db.Query("SELECT * FROM agent_sessions")
package agentnativedb

import (
	"context"
	"fmt"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/graph"
	"github.com/startvibecoding/AgentNativeDB/internal/knowledge"
	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger" // 注册 badger 引擎
	"github.com/startvibecoding/AgentNativeDB/internal/util"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

// DB 嵌入式数据库实例
type DB struct {
	engine      storage.Engine
	cache       *storage.Cache
	session     *agent.SessionManager
	memory      *agent.MemoryStore
	decision    *agent.DecisionRecorder
	coordinator *agent.Coordinator
	taskQueue   *agent.TaskQueue
	audit       *agent.AuditLogger
	vectorStore *vector.VectorStore
	graphStore  *graph.GraphStore
	lineage     *knowledge.LineageTracker
	queryStats  *sql.QueryStats
	executor    *sql.Executor
	ctx         context.Context
}

// Open 打开或创建数据库
func Open(dataDir string) (*DB, error) {
	engine, err := storage.CreateEngine(storage.Options{
		Backend:     storage.BackendBadger,
		DataDir:     dataDir,
		SyncWrites:  true,
		CacheSizeMB: 256,
		BackendOpts: map[string]any{
			"value_log_file_size": int64(64 << 20),
			"mem_table_size":      int64(16 << 20),
			"num_mem_tables":      3,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	cache := storage.NewCache(1024)
	session := agent.NewSessionManager(engine, cache)
	memory := agent.NewMemoryStore(engine, cache)
	decision := agent.NewDecisionRecorder(engine, cache)
	coordinator := agent.NewCoordinator(engine, session, memory, decision)
	taskQueue := agent.NewTaskQueue(engine)
	audit := agent.NewAuditLogger(engine)
	vectorStore := vector.NewVectorStore(engine)
	graphStore := graph.NewGraphStore(engine)
	lineage := knowledge.NewLineageTracker(engine)
	queryStats := sql.NewQueryStats(1000, 100)

	executor := sql.NewExecutor(engine)
	if err := executor.Init(context.Background()); err != nil {
		engine.Close()
		return nil, fmt.Errorf("init executor: %w", err)
	}

	return &DB{
		engine:      engine,
		cache:       cache,
		session:     session,
		memory:      memory,
		decision:    decision,
		coordinator: coordinator,
		taskQueue:   taskQueue,
		audit:       audit,
		vectorStore: vectorStore,
		graphStore:  graphStore,
		lineage:     lineage,
		queryStats:  queryStats,
		executor:    executor,
		ctx:         context.Background(),
	}, nil
}

// Close 关闭数据库
func (db *DB) Close() error {
	return db.engine.Close()
}

// ========== 会话管理 ==========

// CreateSession 创建 Agent 会话
func (db *DB) CreateSession(agentID string, metadata ...map[string]any) (*Session, error) {
	var meta map[string]any
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	s, err := db.session.Create(db.ctx, agentID, meta)
	if err != nil {
		return nil, err
	}
	return &Session{inner: s}, nil
}

// GetSession 获取会话
func (db *DB) GetSession(id string) (*Session, error) {
	s, err := db.session.Get(db.ctx, id)
	if err != nil {
		return nil, err
	}
	return &Session{inner: s}, nil
}

// ListSessions 列出会话
func (db *DB) ListSessions(agentID string, limit ...int) ([]*Session, error) {
	l := 0
	if len(limit) > 0 {
		l = limit[0]
	}

	var sessions []*model.AgentSession
	var err error
	if agentID != "" {
		sessions, err = db.session.ListByAgent(db.ctx, agentID, l)
	} else {
		sessions, err = db.session.ListAll(db.ctx, l)
	}
	if err != nil {
		return nil, err
	}

	result := make([]*Session, len(sessions))
	for i, s := range sessions {
		result[i] = &Session{inner: s}
	}
	return result, nil
}

// CloseSession 完成会话
func (db *DB) CloseSession(id string) error {
	return db.session.UpdateState(db.ctx, id, model.SessionCompleted)
}

// ========== 记忆管理 ==========

// MemoryType 记忆类型
type MemoryType = model.MemoryType

const (
	ShortTerm MemoryType = model.MemoryShortTerm
	LongTerm  MemoryType = model.MemoryLongTerm
	Working   MemoryType = model.MemoryWorking
)

// StoreMemory 存储记忆
func (db *DB) StoreMemory(sessionID, content string, memType MemoryType, importance float32, embedding ...[]float32) (*Memory, error) {
	m := model.NewMemory(sessionID, memType, content, importance)
	if len(embedding) > 0 {
		m.Embedding = embedding[0]
	}
	stored, err := db.memory.Store(db.ctx, m)
	if err != nil {
		return nil, err
	}
	return &Memory{inner: stored}, nil
}

// RecallMemories 检索记忆
func (db *DB) RecallMemories(sessionID string, memType *MemoryType, limit ...int) ([]*Memory, error) {
	l := 0
	if len(limit) > 0 {
		l = limit[0]
	}

	var mt MemoryType
	if memType != nil {
		mt = *memType
	}

	memories, err := db.memory.ListBySession(db.ctx, sessionID, mt, l)
	if err != nil {
		return nil, err
	}

	result := make([]*Memory, len(memories))
	for i, m := range memories {
		result[i] = &Memory{inner: m}
	}
	return result, nil
}

// RecallByImportance 按重要度检索
func (db *DB) RecallByImportance(sessionID string, memType MemoryType, limit int) ([]*Memory, error) {
	enhanced := agent.NewMemoryStoreEnhanced(db.engine, db.cache, agent.DefaultMemoryConfig())
	defer enhanced.Stop()

	memories, err := enhanced.RecallByImportance(db.ctx, sessionID, memType, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*Memory, len(memories))
	for i, m := range memories {
		result[i] = &Memory{inner: m}
	}
	return result, nil
}

// ========== 决策记录 ==========

// DecisionType 决策类型
type DecisionType = model.DecisionType

const (
	Reasoning  DecisionType = model.DecisionReasoning
	ToolCall   DecisionType = model.DecisionToolCall
	Planning   DecisionType = model.DecisionPlanning
	Reflection DecisionType = model.DecisionReflection
)

// RecordDecision 记录决策
func (db *DB) RecordDecision(sessionID string, decType DecisionType, input, output any, reasoning ...string) (*Decision, error) {
	inputJSON := model.MapToJSON(map[string]any{"data": input})
	outputJSON := model.MapToJSON(map[string]any{"data": output})

	d := model.NewDecision(sessionID, decType, inputJSON, outputJSON)
	if len(reasoning) > 0 {
		d.Reasoning = reasoning[0]
	}

	recorded, err := db.decision.Record(db.ctx, d)
	if err != nil {
		return nil, err
	}
	return &Decision{inner: recorded}, nil
}

// GetDecisionTree 获取决策树
func (db *DB) GetDecisionTree(decisionID string) (*agent.DecisionTreeNode, error) {
	return db.decision.BuildDecisionTree(db.ctx, decisionID)
}

// ========== SQL 查询 ==========

// QueryResult SQL 查询结果
type QueryResult = sql.Result

// Query 执行 SQL 查询
func (db *DB) Query(query string) (*QueryResult, error) {
	start := time.Now()

	stmt, err := sql.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	planner := db.executor.Planner()
	plan, err := planner.Plan(stmt)
	if err != nil {
		return nil, fmt.Errorf("plan error: %w", err)
	}

	executor := db.executor
	result, err := executor.Execute(db.ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("execute error: %w", err)
	}

	// 记录统计
	duration := time.Since(start).Milliseconds()
	db.queryStats.Record(sql.QueryRecord{
		SQL:          query,
		DurationMs:   duration,
		RowsReturned: len(result.Rows),
		Timestamp:    time.Now(),
	})

	return result, nil
}

// ========== 向量索引 ==========

// CreateIndex 创建向量索引
func (db *DB) CreateIndex(name string, dimension int, metric ...string) error {
	m := "cosine"
	if len(metric) > 0 {
		m = metric[0]
	}
	return db.vectorStore.CreateIndex(name, dimension, m)
}

// InsertVector 插入向量
func (db *DB) InsertVector(indexName, id string, vector []float32) error {
	return db.vectorStore.Insert(indexName, id, vector)
}

// SearchVector 向量搜索(返回 payload)
func (db *DB) SearchVector(indexName string, query []float32, topK int) ([]VectorResult, error) {
	results, err := db.vectorStore.SearchWithPayloads(indexName, query, topK)
	if err != nil {
		return nil, err
	}

	out := make([]VectorResult, len(results))
	for i, r := range results {
		out[i] = VectorResult{ID: r.ID, Distance: r.Distance, Score: 1.0 - r.Distance, Payload: r.Payload}
	}
	return out, nil
}

// InsertVectorWithPayload 插入向量并携带 payload / metadata
func (db *DB) InsertVectorWithPayload(indexName, id string, vector []float32, payload []byte) error {
	return db.vectorStore.InsertWithPayload(indexName, id, vector, payload)
}

// DeleteVector 删除向量
func (db *DB) DeleteVector(indexName, id string) error {
	return db.vectorStore.Delete(indexName, id)
}

// ========== 图操作 ==========

// AddNode 添加图节点
func (db *DB) AddNode(id, nodeType, name string, properties ...map[string]any) error {
	var props map[string]any
	if len(properties) > 0 {
		props = properties[0]
	}
	return db.graphStore.AddNode(db.ctx, &graph.Node{
		ID:         id,
		Type:       nodeType,
		Name:       name,
		Properties: props,
	})
}

// AddEdge 添加图边
func (db *DB) AddEdge(id, edgeType, fromID, toID string, weight ...float64) error {
	w := 1.0
	if len(weight) > 0 {
		w = weight[0]
	}
	return db.graphStore.AddEdge(db.ctx, &graph.Edge{
		ID:     id,
		Type:   edgeType,
		FromID: fromID,
		ToID:   toID,
		Weight: w,
	})
}

// GetNeighbors 获取邻居节点
func (db *DB) GetNeighbors(nodeID string, direction ...string) ([]*graph.Node, error) {
	dir := graph.DirBoth
	if len(direction) > 0 {
		switch direction[0] {
		case "out":
			dir = graph.DirOut
		case "in":
			dir = graph.DirIn
		}
	}
	return db.graphStore.GetNeighbors(db.ctx, nodeID, dir)
}

// ShortestPath 最短路径
func (db *DB) ShortestPath(fromID, toID string) ([]string, error) {
	return db.graphStore.ShortestPath(db.ctx, fromID, toID, graph.DirBoth)
}

// KHopNeighbors K 跳邻居
func (db *DB) KHopNeighbors(nodeID string, k int) ([]*graph.Node, error) {
	return db.graphStore.KHopNeighbors(db.ctx, nodeID, k, graph.DirBoth)
}

// ========== 多 Agent 协作 ==========

// CreateRoom 创建协作房间
func (db *DB) CreateRoom(name, creatorID string) (*agent.Room, error) {
	return db.coordinator.CreateRoom(db.ctx, name, creatorID, agent.RoomOptions{})
}

// SendMessage 发送消息到房间
func (db *DB) SendMessage(roomID, fromID, content string) (*agent.Message, error) {
	return db.coordinator.SendMessage(db.ctx, roomID, fromID, agent.MsgText, content)
}

// GetMessages 获取房间消息
func (db *DB) GetMessages(roomID string, limit ...int) ([]*agent.Message, error) {
	l := 50
	if len(limit) > 0 {
		l = limit[0]
	}
	return db.coordinator.GetMessages(db.ctx, roomID, l)
}

// ShareMemory 共享记忆到房间
func (db *DB) ShareMemory(roomID, fromID, content string, importance float32) error {
	_, err := db.coordinator.ShareMemory(db.ctx, roomID, fromID, content, importance)
	return err
}

// ========== 任务队列 ==========

// EnqueueTask 入队任务
func (db *DB) EnqueueTask(agentID, name string, priority int, input ...any) (*agent.Task, error) {
	task := &agent.Task{
		AgentID:  agentID,
		Name:     name,
		Priority: priority,
	}
	if len(input) > 0 {
		task.Input = input[0]
	}
	if err := db.taskQueue.Enqueue(db.ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// DequeueTask 出队任务
func (db *DB) DequeueTask() (*agent.Task, error) {
	return db.taskQueue.Dequeue(db.ctx)
}

// CompleteTask 完成任务
func (db *DB) CompleteTask(taskID string, result any) error {
	return db.taskQueue.Complete(db.ctx, taskID, result)
}

// ========== 数据血缘 ==========

// RecordLineage 记录数据血缘
func (db *DB) RecordLineage(dataID, sourceType string, sourceIDs []string) error {
	return db.lineage.Record(db.ctx, &knowledge.Lineage{
		DataID:     dataID,
		SourceType: sourceType,
		SourceIDs:  sourceIDs,
	})
}

// TraceLineage 追溯数据血缘
func (db *DB) TraceLineage(dataID string, depth int) (*knowledge.LineageTree, error) {
	return db.lineage.TraceUpstream(db.ctx, dataID, depth)
}

// ========== 审计日志 ==========

// GetAuditLog 获取审计日志
func (db *DB) GetAuditLog(agentID string, limit ...int) ([]*agent.AuditEvent, error) {
	l := 20
	if len(limit) > 0 {
		l = limit[0]
	}
	return db.audit.ListByAgent(db.ctx, agentID, l)
}

// ========== 内部类型 ==========

// Session 会话
type Session struct {
	inner *model.AgentSession
}

func (s *Session) ID() string            { return s.inner.ID }
func (s *Session) AgentID() string       { return s.inner.AgentID }
func (s *Session) State() string         { return string(s.inner.State) }
func (s *Session) CreatedAt() time.Time  { return s.inner.CreatedAt }

// Memory 记忆
type Memory struct {
	inner *model.MemoryEntry
}

func (m *Memory) ID() string              { return m.inner.ID }
func (m *Memory) SessionID() string       { return m.inner.SessionID }
func (m *Memory) Content() string         { return m.inner.Content }
func (m *Memory) Type() MemoryType        { return m.inner.Type }
func (m *Memory) Importance() float32     { return m.inner.Importance }
func (m *Memory) AccessCount() uint32     { return m.inner.AccessCount }
func (m *Memory) CreatedAt() time.Time    { return m.inner.CreatedAt }

// Decision 决策
type Decision struct {
	inner *model.Decision
}

func (d *Decision) ID() string         { return d.inner.ID }
func (d *Decision) SessionID() string  { return d.inner.SessionID }
func (d *Decision) Type() DecisionType { return d.inner.Type }
func (d *Decision) Reasoning() string  { return d.inner.Reasoning }
func (d *Decision) DurationMs() uint64 { return d.inner.DurationMs }
func (d *Decision) CreatedAt() time.Time { return d.inner.CreatedAt }

// VectorResult 向量搜索结果
type VectorResult struct {
	ID       string
	Distance float32
	Score    float32
	Payload  []byte // 附加的 JSON / metadata
}

// Node 图节点
type Node = graph.Node

// Edge 图边
type Edge = graph.Edge

// UUID 生成 UUID v7
func UUID() string {
	return util.NewUUID()
}
