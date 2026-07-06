package apihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

func setupTestServer(t *testing.T) (*Router, *httptest.Server) {
	t.Helper()
	engine := storage.NewTestEngine(t)

	cache := storage.NewCache(512)
	session := agent.NewSessionManager(engine, cache)
	memory := agent.NewMemoryStore(engine, cache)
	decision := agent.NewDecisionRecorder(engine, cache)
	executor := sql.NewExecutor(engine)
	if err := executor.Init(context.Background()); err != nil {
		t.Fatalf("init executor: %v", err)
	}
	router := NewRouter(engine, session, memory, decision, executor, vector.NewVectorStore(engine))

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return router, server
}

func TestAPI_Health(t *testing.T) {
	_, server := setupTestServer(t)

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body)
	}
}

func TestAPI_CreateAndGetSession(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建会话
	body := `{"agent_id": "test-agent", "metadata": {"model": "gpt-4"}}`
	resp, err := http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result apiResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		t.Fatalf("expected ok, got error: %s", result.Error)
	}

	// 获取会话
	sessionData := result.Data.(map[string]any)
	sessionID := sessionData["id"].(string)

	resp2, err := http.Get(server.URL + "/api/v1/sessions/" + sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	defer resp2.Body.Close()

	var result2 apiResponse
	json.NewDecoder(resp2.Body).Decode(&result2)
	if !result2.OK {
		t.Fatalf("expected ok, got error: %s", result2.Error)
	}
}

func TestAPI_ListSessions(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建两个会话
	http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a1"}`))
	http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a2"}`))

	// 列出所有
	resp, err := http.Get(server.URL + "/api/v1/sessions")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	defer resp.Body.Close()

	var result apiResponse
	json.NewDecoder(resp.Body).Decode(&result)

	sessions := result.Data.([]any)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestAPI_StoreAndListMemories(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建会话
	resp1, _ := http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a1"}`))
	var r1 apiResponse
	json.NewDecoder(resp1.Body).Decode(&r1)
	sessionID := r1.Data.(map[string]any)["id"].(string)

	// 存储记忆
	memBody := `{"session_id": "` + sessionID + `", "content": "test memory", "type": "long_term", "importance": 0.8}`
	resp2, _ := http.Post(server.URL+"/api/v1/memories", "application/json", bytes.NewBufferString(memBody))
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// 列出记忆
	resp3, err := http.Get(server.URL + "/api/v1/memories?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	defer resp3.Body.Close()

	var r3 apiResponse
	json.NewDecoder(resp3.Body).Decode(&r3)
	memories := r3.Data.([]any)
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
}

func TestAPI_RecordAndListDecisions(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建会话
	resp1, _ := http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a1"}`))
	var r1 apiResponse
	json.NewDecoder(resp1.Body).Decode(&r1)
	sessionID := r1.Data.(map[string]any)["id"].(string)

	// 记录决策
	decBody := `{"session_id": "` + sessionID + `", "type": "tool_call", "input": {"tool": "search"}, "output": {"results": []}, "reasoning": "test"}`
	resp2, _ := http.Post(server.URL+"/api/v1/decisions", "application/json", bytes.NewBufferString(decBody))
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// 列出决策
	resp3, err := http.Get(server.URL + "/api/v1/decisions?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	defer resp3.Body.Close()

	var r3 apiResponse
	json.NewDecoder(resp3.Body).Decode(&r3)
	decisions := r3.Data.([]any)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
}

func TestAPI_SQLQuery(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建会话
	http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a1"}`))

	// SQL 查询
	queryBody := `{"sql": "SELECT * FROM agent_sessions"}`
	resp, err := http.Post(server.URL+"/api/v1/query", "application/json", bytes.NewBufferString(queryBody))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var r apiResponse
	json.NewDecoder(resp.Body).Decode(&r)
	if !r.OK {
		t.Fatalf("expected ok, got error: %s", r.Error)
	}
}

func TestAPI_VectorSearchWithPayloadFlag(t *testing.T) {
	_, server := setupTestServer(t)

	resp, err := http.Post(server.URL+"/api/v1/vector/indexes", "application/json", bytes.NewBufferString(`{"name":"test","dim":3}`))
	if err != nil {
		t.Fatalf("create index: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Post(server.URL+"/api/v1/vector/indexes/test/vectors", "application/json", bytes.NewBufferString(`{"id":"vec-001","vector":[1,0,0],"payload":{"secret":"hidden"}}`))
	if err != nil {
		t.Fatalf("insert vector: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Post(server.URL+"/api/v1/vector/indexes/test/search", "application/json", bytes.NewBufferString(`{"vector":[1,0,0],"top_k":1,"with_payload":false}`))
	if err != nil {
		t.Fatalf("search vector: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	rows := result.Data.([]any)
	first := rows[0].(map[string]any)
	if _, ok := first["payload"]; ok {
		t.Fatalf("expected payload omitted, got %v", first["payload"])
	}
}

func TestAPI_DeleteSession(t *testing.T) {
	_, server := setupTestServer(t)

	// 创建
	resp1, _ := http.Post(server.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(`{"agent_id": "a1"}`))
	var r1 apiResponse
	json.NewDecoder(resp1.Body).Decode(&r1)
	sessionID := r1.Data.(map[string]any)["id"].(string)

	// 删除
	req, _ := http.NewRequest("DELETE", server.URL+"/api/v1/sessions/"+sessionID, nil)
	resp2, _ := http.DefaultClient.Do(req)
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// 获取应该 404
	resp3, _ := http.Get(server.URL + "/api/v1/sessions/" + sessionID)
	if resp3.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp3.StatusCode)
	}
}

func TestAPI_NotFound(t *testing.T) {
	_, server := setupTestServer(t)

	resp, _ := http.Get(server.URL + "/api/v1/sessions/nonexistent")
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
