package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// MCPServer MCP 服务端
type MCPServer struct {
	engine   storage.Engine
	session  *agent.SessionManager
	memory   *agent.MemoryStore
	decision *agent.DecisionRecorder
	reader   io.Reader
	writer   io.Writer
}

// NewMCPServer 创建 MCP 服务端
func NewMCPServer(engine storage.Engine, session *agent.SessionManager, memory *agent.MemoryStore, decision *agent.DecisionRecorder) *MCPServer {
	return &MCPServer{
		engine:   engine,
		session:  session,
		memory:   memory,
		decision: decision,
		reader:   os.Stdin,
		writer:   os.Stdout,
	}
}

// Run 运行 MCP 服务端（stdio 传输）
func (s *MCPServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "parse error", err.Error())
			continue
		}

		s.handleRequest(ctx, &req)
	}

	return scanner.Err()
}

// handleRequest 处理 MCP 请求
func (s *MCPServer) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	case "ping":
		s.sendResult(req.ID, map[string]any{"pong": true})
	default:
		s.sendError(req.ID, -32601, "method not found", req.Method)
	}
}

// handleInitialize 初始化握手
func (s *MCPServer) handleInitialize(req *JSONRPCRequest) {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "AgentNativeDB",
			"version": "0.1.0",
		},
	}
	s.sendResult(req.ID, result)
}

// handleToolsList 返回可用工具列表
func (s *MCPServer) handleToolsList(req *JSONRPCRequest) {
	tools := []Tool{
		{
			Name:        "query_sql",
			Description: "Execute SQL query on AgentNativeDB. Supports SELECT, INSERT, UPDATE, DELETE with WHERE, ORDER BY, LIMIT, GROUP BY, aggregates.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{
						"type":        "string",
						"description": "The SQL query to execute",
					},
				},
				"required": []string{"sql"},
			},
		},
		{
			Name:        "create_session",
			Description: "Create a new agent session",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "The agent identifier",
					},
				},
				"required": []string{"agent_id"},
			},
		},
		{
			Name:        "store_memory",
			Description: "Store a memory entry for an agent session",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session ID",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Memory content",
					},
					"type": map[string]any{
						"type":        "string",
						"description": "Memory type: short_term, long_term, working",
						"default":     "short_term",
					},
					"importance": map[string]any{
						"type":        "number",
						"description": "Importance score 0.0-1.0",
						"default":     0.5,
					},
				},
				"required": []string{"session_id", "content"},
			},
		},
		{
			Name:        "recall_memories",
			Description: "Recall memories from a session, optionally filtered by type",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session ID",
					},
					"type": map[string]any{
						"type":        "string",
						"description": "Memory type filter",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max results",
						"default":     10,
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			Name:        "record_decision",
			Description: "Record an agent decision with reasoning chain",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session ID",
					},
					"type": map[string]any{
						"type":        "string",
						"description": "Decision type: reasoning, tool_call, planning, reflection",
					},
					"input": map[string]any{
						"type":        "object",
						"description": "Decision input",
					},
					"output": map[string]any{
						"type":        "object",
						"description": "Decision output",
					},
					"reasoning": map[string]any{
						"type":        "string",
						"description": "Reasoning explanation",
					},
				},
				"required": []string{"session_id", "type", "input", "output"},
			},
		},
	}

	s.sendResult(req.ID, map[string]any{"tools": tools})
}

// handleToolsCall 处理工具调用
func (s *MCPServer) handleToolsCall(ctx context.Context, req *JSONRPCRequest) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "invalid params", err.Error())
		return
	}

	switch params.Name {
	case "query_sql":
		s.handleQuerySQL(ctx, req, params.Arguments)
	case "create_session":
		s.handleCreateSession(ctx, req, params.Arguments)
	case "store_memory":
		s.handleStoreMemory(ctx, req, params.Arguments)
	case "recall_memories":
		s.handleRecallMemories(ctx, req, params.Arguments)
	case "record_decision":
		s.handleRecordDecision(ctx, req, params.Arguments)
	default:
		s.sendError(req.ID, -32602, "unknown tool", params.Name)
	}
}

func (s *MCPServer) handleQuerySQL(ctx context.Context, req *JSONRPCRequest, args map[string]any) {
	sqlStr, _ := args["sql"].(string)
	if sqlStr == "" {
		s.sendToolResult(req.ID, "error: sql is required", true)
		return
	}

	stmt, err := sql.Parse(sqlStr)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("parse error: %v", err), true)
		return
	}

	planner := sql.NewPlanner()
	plan, err := planner.Plan(stmt)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("plan error: %v", err), true)
		return
	}

	executor := sql.NewExecutor(s.engine)
	result, err := executor.Execute(ctx, plan)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("execute error: %v", err), true)
		return
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	s.sendToolResult(req.ID, string(data), false)
}

func (s *MCPServer) handleCreateSession(ctx context.Context, req *JSONRPCRequest, args map[string]any) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		s.sendToolResult(req.ID, "error: agent_id is required", true)
		return
	}

	session, err := s.session.Create(ctx, agentID, nil)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("error: %v", err), true)
		return
	}

	data, _ := json.Marshal(session)
	s.sendToolResult(req.ID, string(data), false)
}

func (s *MCPServer) handleStoreMemory(ctx context.Context, req *JSONRPCRequest, args map[string]any) {
	sessionID, _ := args["session_id"].(string)
	content, _ := args["content"].(string)
	memType, _ := args["type"].(string)
	importance, _ := args["importance"].(float64)

	if sessionID == "" || content == "" {
		s.sendToolResult(req.ID, "error: session_id and content are required", true)
		return
	}

	if memType == "" {
		memType = "short_term"
	}
	if importance == 0 {
		importance = 0.5
	}

	mem := model.NewMemory(sessionID, model.MemoryType(memType), content, float32(importance))
	stored, err := s.memory.Store(ctx, mem)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("error: %v", err), true)
		return
	}

	data, _ := json.Marshal(stored)
	s.sendToolResult(req.ID, string(data), false)
}

func (s *MCPServer) handleRecallMemories(ctx context.Context, req *JSONRPCRequest, args map[string]any) {
	sessionID, _ := args["session_id"].(string)
	memType, _ := args["type"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	if sessionID == "" {
		s.sendToolResult(req.ID, "error: session_id is required", true)
		return
	}

	memories, err := s.memory.ListBySession(ctx, sessionID, model.MemoryType(memType), limit)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("error: %v", err), true)
		return
	}

	data, _ := json.Marshal(memories)
	s.sendToolResult(req.ID, string(data), false)
}

func (s *MCPServer) handleRecordDecision(ctx context.Context, req *JSONRPCRequest, args map[string]any) {
	sessionID, _ := args["session_id"].(string)
	decType, _ := args["type"].(string)
	reasoning, _ := args["reasoning"].(string)

	if sessionID == "" {
		s.sendToolResult(req.ID, "error: session_id is required", true)
		return
	}

	inputJSON, _ := json.Marshal(args["input"])
	outputJSON, _ := json.Marshal(args["output"])

	d := model.NewDecision(sessionID, model.DecisionType(decType), inputJSON, outputJSON)
	d.Reasoning = reasoning

	recorded, err := s.decision.Record(ctx, d)
	if err != nil {
		s.sendToolResult(req.ID, fmt.Sprintf("error: %v", err), true)
		return
	}

	data, _ := json.Marshal(recorded)
	s.sendToolResult(req.ID, string(data), false)
}

// sendResult 发送成功结果
func (s *MCPServer) sendResult(id any, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(&resp)
}

// sendToolResult 发送工具调用结果
func (s *MCPServer) sendToolResult(id any, content string, isError bool) {
	result := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
		"isError": isError,
	}
	s.sendResult(id, result)
}

// sendError 发送错误
func (s *MCPServer) sendError(id any, code int, message string, data any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.writeResponse(&resp)
}

func (s *MCPServer) writeResponse(resp *JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("mcp: marshal response", "error", err)
		return
	}
	data = append(data, '\n')
	s.writer.Write(data) //nolint:errcheck
}

// JSON-RPC 类型

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Result  any            `json:"result,omitempty"`
	Error   *JSONRPCError  `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// ToolCallParams 工具调用参数
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}
