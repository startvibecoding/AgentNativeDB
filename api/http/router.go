package apihttp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/startvibecoding/AgentNativeDB/api/mcp"
	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
	uistatic "github.com/startvibecoding/AgentNativeDB/ui/static"
)

// Router HTTP 路由器
type Router struct {
	mux      *http.ServeMux
	engine   storage.Engine
	session  *agent.SessionManager
	memory   *agent.MemoryStore
	decision *agent.DecisionRecorder
	executor *sql.Executor
	vectors  *vector.VectorStore
	mcp      *mcp.MCPServer
}

// NewRouter 创建路由器
func NewRouter(engine storage.Engine, session *agent.SessionManager, memory *agent.MemoryStore, decision *agent.DecisionRecorder, executor *sql.Executor, vectors *vector.VectorStore) *Router {
	r := &Router{
		mux:      http.NewServeMux(),
		engine:   engine,
		session:  session,
		memory:   memory,
		decision: decision,
		executor: executor,
		vectors:  vectors,
		mcp:      mcp.NewMCPServer(engine, session, memory, decision, executor),
	}
	r.registerRoutes()
	return r
}

// ServeHTTP 实现 http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 中间件：日志
	start := time.Now()
	r.mux.ServeHTTP(w, req)
	slog.Info("request",
		"method", req.Method,
		"path", req.URL.Path,
		"duration", time.Since(start),
	)
}

func (r *Router) registerRoutes() {
	// Session
	r.mux.HandleFunc("POST /api/v1/sessions", r.handleCreateSession)
	r.mux.HandleFunc("GET /api/v1/sessions/{id}", r.handleGetSession)
	r.mux.HandleFunc("PATCH /api/v1/sessions/{id}", r.handleUpdateSession)
	r.mux.HandleFunc("DELETE /api/v1/sessions/{id}", r.handleDeleteSession)
	r.mux.HandleFunc("GET /api/v1/sessions", r.handleListSessions)

	// Memory
	r.mux.HandleFunc("POST /api/v1/memories", r.handleStoreMemory)
	r.mux.HandleFunc("GET /api/v1/memories/{id}", r.handleGetMemory)
	r.mux.HandleFunc("DELETE /api/v1/memories/{id}", r.handleDeleteMemory)
	r.mux.HandleFunc("GET /api/v1/memories", r.handleListMemories)

	// Decision
	r.mux.HandleFunc("POST /api/v1/decisions", r.handleRecordDecision)
	r.mux.HandleFunc("GET /api/v1/decisions/{id}", r.handleGetDecision)
	r.mux.HandleFunc("DELETE /api/v1/decisions/{id}", r.handleDeleteDecision)
	r.mux.HandleFunc("GET /api/v1/decisions", r.handleListDecisions)
	r.mux.HandleFunc("GET /api/v1/decisions/{id}/tree", r.handleDecisionTree)

	// SQL 查询
	r.mux.HandleFunc("POST /api/v1/query", r.handleQuery)

	// Vector
	r.mux.HandleFunc("GET /api/v1/vector/indexes", r.handleListVectorIndexes)
	r.mux.HandleFunc("POST /api/v1/vector/indexes", r.handleCreateVectorIndex)
	r.mux.HandleFunc("GET /api/v1/vector/indexes/{name}", r.handleGetVectorIndex)
	r.mux.HandleFunc("POST /api/v1/vector/indexes/{name}/vectors", r.handleInsertVector)
	r.mux.HandleFunc("DELETE /api/v1/vector/indexes/{name}/vectors/{id}", r.handleDeleteVector)
	r.mux.HandleFunc("POST /api/v1/vector/indexes/{name}/search", r.handleSearchVector)

	// Health
	r.mux.HandleFunc("GET /health", r.handleHealth)

	// MCP over HTTP (JSON-RPC 单次请求/响应)
	r.mux.Handle("POST /mcp", r.mcp)

	// 静态文件服务 (Web UI)
	r.mux.HandleFunc("/", r.handleStatic)
}

// ========== 响应工具 ==========

type apiResponse struct {
	OK     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

func jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{OK: true, Data: data})
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(apiResponse{OK: false, Error: msg})
}

func parseLimit(r *http.Request) int {
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// ========== Health ==========

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	jsonOK(w, map[string]any{
		"status": "ok",
		"time":   time.Now(),
	})
}

// ========== Session Handlers ==========

func (r *Router) handleCreateSession(w http.ResponseWriter, req *http.Request) {
	var body struct {
		AgentID  string         `json:"agent_id"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.AgentID == "" {
		jsonError(w, 400, "agent_id is required")
		return
	}

	session, err := r.session.Create(req.Context(), body.AgentID, body.Metadata)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("create session: %v", err))
		return
	}

	jsonOK(w, session)
}

func (r *Router) handleGetSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	session, err := r.session.Get(req.Context(), id)
	if err != nil {
		jsonError(w, 404, fmt.Sprintf("session not found: %v", err))
		return
	}
	jsonOK(w, session)
}

func (r *Router) handleUpdateSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	session, err := r.session.Get(req.Context(), id)
	if err != nil {
		jsonError(w, 404, "session not found")
		return
	}

	var body struct {
		State   model.SessionState `json:"state"`
		Context map[string]any     `json:"context"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if body.State != "" {
		session.State = body.State
	}
	if body.Context != nil {
		for k, v := range body.Context {
			session.Context[k] = v
		}
	}

	if err := r.session.Update(req.Context(), session); err != nil {
		jsonError(w, 500, fmt.Sprintf("update session: %v", err))
		return
	}

	jsonOK(w, session)
}

func (r *Router) handleDeleteSession(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.session.Delete(req.Context(), id); err != nil {
		jsonError(w, 404, "session not found")
		return
	}
	jsonOK(w, nil)
}

func (r *Router) handleListSessions(w http.ResponseWriter, req *http.Request) {
	agentID := req.URL.Query().Get("agent_id")
	limit := parseLimit(req)

	var sessions []*model.AgentSession
	var err error

	if agentID != "" {
		sessions, err = r.session.ListByAgent(req.Context(), agentID, limit)
	} else {
		sessions, err = r.session.ListAll(req.Context(), limit)
	}

	if err != nil {
		jsonError(w, 500, fmt.Sprintf("list sessions: %v", err))
		return
	}

	jsonOK(w, sessions)
}

// ========== Memory Handlers ==========

func (r *Router) handleStoreMemory(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SessionID  string           `json:"session_id"`
		Type       model.MemoryType `json:"type"`
		Content    string           `json:"content"`
		Importance float32          `json:"importance"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.SessionID == "" || body.Content == "" {
		jsonError(w, 400, "session_id and content are required")
		return
	}
	if body.Type == "" {
		body.Type = model.MemoryShortTerm
	}

	m := model.NewMemory(body.SessionID, body.Type, body.Content, body.Importance)
	memory, err := r.memory.Store(req.Context(), m)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("store memory: %v", err))
		return
	}

	jsonOK(w, memory)
}

func (r *Router) handleGetMemory(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	memory, err := r.memory.Get(req.Context(), id)
	if err != nil {
		jsonError(w, 404, "memory not found")
		return
	}
	jsonOK(w, memory)
}

func (r *Router) handleDeleteMemory(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.memory.Delete(req.Context(), id); err != nil {
		jsonError(w, 404, "memory not found")
		return
	}
	jsonOK(w, nil)
}

func (r *Router) handleListMemories(w http.ResponseWriter, req *http.Request) {
	sessionID := req.URL.Query().Get("session_id")
	memType := model.MemoryType(req.URL.Query().Get("type"))
	limit := parseLimit(req)

	if sessionID == "" {
		jsonError(w, 400, "session_id is required")
		return
	}

	memories, err := r.memory.ListBySession(req.Context(), sessionID, memType, limit)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("list memories: %v", err))
		return
	}

	jsonOK(w, memories)
}

// ========== Decision Handlers ==========

func (r *Router) handleRecordDecision(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SessionID  string            `json:"session_id"`
		ParentID   *string           `json:"parent_id"`
		Type       model.DecisionType `json:"type"`
		Input      json.RawMessage   `json:"input"`
		Output     json.RawMessage   `json:"output"`
		Reasoning  string            `json:"reasoning"`
		ToolsUsed  []string          `json:"tools_used"`
		DurationMs uint64            `json:"duration_ms"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.SessionID == "" {
		jsonError(w, 400, "session_id is required")
		return
	}
	if body.Type == "" {
		body.Type = model.DecisionReasoning
	}

	d := model.NewDecision(body.SessionID, body.Type, body.Input, body.Output)
	if body.ParentID != nil {
		d.ParentID = body.ParentID
	}
	d.Reasoning = body.Reasoning
	d.ToolsUsed = body.ToolsUsed
	d.DurationMs = body.DurationMs

	decision, err := r.decision.Record(req.Context(), d)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("record decision: %v", err))
		return
	}

	jsonOK(w, decision)
}

func (r *Router) handleGetDecision(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	decision, err := r.decision.Get(req.Context(), id)
	if err != nil {
		jsonError(w, 404, "decision not found")
		return
	}
	jsonOK(w, decision)
}

func (r *Router) handleDeleteDecision(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if err := r.decision.Delete(req.Context(), id); err != nil {
		jsonError(w, 404, "decision not found")
		return
	}
	jsonOK(w, nil)
}

func (r *Router) handleListDecisions(w http.ResponseWriter, req *http.Request) {
	sessionID := req.URL.Query().Get("session_id")
	limit := parseLimit(req)

	if sessionID == "" {
		jsonError(w, 400, "session_id is required")
		return
	}

	decisions, err := r.decision.ListBySession(req.Context(), sessionID, limit)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("list decisions: %v", err))
		return
	}

	jsonOK(w, decisions)
}

func (r *Router) handleDecisionTree(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	tree, err := r.decision.BuildDecisionTree(req.Context(), id)
	if err != nil {
		jsonError(w, 404, "decision not found")
		return
	}
	jsonOK(w, tree)
}

// ========== SQL Query Handler ==========

func (r *Router) handleQuery(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.SQL == "" {
		jsonError(w, 400, "sql is required")
		return
	}

	// 解析
	stmt, err := sql.Parse(body.SQL)
	if err != nil {
		jsonError(w, 400, fmt.Sprintf("parse error: %v", err))
		return
	}

	// 计划
	planner := r.executor.Planner()
	plan, err := planner.Plan(stmt)
	if err != nil {
		jsonError(w, 400, fmt.Sprintf("plan error: %v", err))
		return
	}

	// 执行
	result, err := r.executor.Execute(req.Context(), plan)
	if err != nil {
		jsonError(w, 500, fmt.Sprintf("execute error: %v", err))
		return
	}

	jsonOK(w, result)
}

// ========== Static File Handler (Web UI) ==========

func (r *Router) handleStatic(w http.ResponseWriter, req *http.Request) {
	// 获取嵌入的文件系统
	sub, err := fs.Sub(uistatic.Files, "dist")
	if err != nil {
		http.Error(w, "UI not available", http.StatusInternalServerError)
		return
	}

	// SPA 回退: 如果请求的文件不存在，返回 index.html
	path := req.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// 检查文件是否存在
	_, err = sub.Open(strings.TrimPrefix(path, "/"))
	if err != nil {
		// 文件不存在，回退到 index.html (SPA 路由)
		req.URL.Path = "/"
	}

	http.FileServer(http.FS(sub)).ServeHTTP(w, req)
}
