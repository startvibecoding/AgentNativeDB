package apihttp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

// ========== 向量 API ==========
//
// 路由:
//   GET    /api/v1/vector/indexes                              列出所有索引
//   POST   /api/v1/vector/indexes                              创建索引 {name, dim, metric}
//   GET    /api/v1/vector/indexes/{name}                       索引信息
//   POST   /api/v1/vector/indexes/{name}/vectors               插入向量 {id, vector}
//   DELETE /api/v1/vector/indexes/{name}/vectors/{id}          删除向量
//   POST   /api/v1/vector/indexes/{name}/search                检索 {vector, top_k}

func (r *Router) handleListVectorIndexes(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	names := r.vectors.ListIndexes()
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{
			"name": n,
			"dim":  r.vectors.Dim(n),
			"size": r.vectors.Len(n),
		})
	}
	jsonOK(w, out)
}

func (r *Router) handleCreateVectorIndex(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	var body struct {
		Name   string `json:"name"`
		Dim    int    `json:"dim"`
		Metric string `json:"metric"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.Name == "" || body.Dim <= 0 {
		jsonError(w, 400, "name and dim are required (dim > 0)")
		return
	}
	if body.Metric == "" {
		body.Metric = "cosine"
	}
	if err := r.vectors.CreateIndex(body.Name, body.Dim, body.Metric); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	jsonOK(w, map[string]any{
		"name":   body.Name,
		"dim":    body.Dim,
		"metric": body.Metric,
	})
}

func (r *Router) handleGetVectorIndex(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	name := req.PathValue("name")
	if !r.vectors.HasIndex(name) {
		jsonError(w, 404, fmt.Sprintf("index %s not found", name))
		return
	}
	jsonOK(w, map[string]any{
		"name": name,
		"dim":  r.vectors.Dim(name),
		"size": r.vectors.Len(name),
	})
}

func (r *Router) handleInsertVector(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	name := req.PathValue("name")
	var body struct {
		ID      string          `json:"id"`
		Vector  []float32       `json:"vector"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if body.ID == "" || len(body.Vector) == 0 {
		jsonError(w, 400, "id and vector are required")
		return
	}

	var payload []byte
	if len(body.Payload) > 0 {
		payload = []byte(body.Payload)
	}

	if err := r.vectors.InsertWithPayload(name, body.ID, body.Vector, payload); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	jsonOK(w, map[string]any{"id": body.ID})
}

func (r *Router) handleDeleteVector(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	name := req.PathValue("name")
	id := req.PathValue("id")
	if err := r.vectors.Delete(name, id); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	jsonOK(w, nil)
}

func (r *Router) handleSearchVector(w http.ResponseWriter, req *http.Request) {
	if r.vectors == nil {
		jsonError(w, 500, "vector store not initialized")
		return
	}
	name := req.PathValue("name")
	var body struct {
		Vector      []float32 `json:"vector"`
		TopK        int       `json:"top_k"`
		WithPayload bool      `json:"with_payload"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		jsonError(w, 400, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if len(body.Vector) == 0 {
		jsonError(w, 400, "vector is required")
		return
	}
	if body.TopK <= 0 {
		body.TopK = 10
	}

	results, err := r.vectors.SearchWithPayloads(name, body.Vector, body.TopK)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}

	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		item := map[string]any{
			"id":       r.ID,
			"distance": r.Distance,
			"score":    1.0 - r.Distance,
		}
		if len(r.Payload) > 0 {
			var payloadObj any
			if err := json.Unmarshal(r.Payload, &payloadObj); err == nil {
				item["payload"] = payloadObj
			} else {
				item["payload"] = string(r.Payload)
			}
		}
		out = append(out, item)
	}
	jsonOK(w, out)
}

// 引用防止 unused (vector 包已在签名中使用)
var _ = vector.SearchResult{}
