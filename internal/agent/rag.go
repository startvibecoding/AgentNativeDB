package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

// RAGEngine RAG (Retrieval-Augmented Generation) 引擎
type RAGEngine struct {
	engine      storage.Engine
	memory      *MemoryStore
	vectorStore *vector.VectorStore
	indexName   string
}

// NewRAGEngine 创建 RAG 引擎
func NewRAGEngine(engine storage.Engine, memory *MemoryStore, vectorStore *vector.VectorStore) *RAGEngine {
	return &RAGEngine{
		engine:      engine,
		memory:      memory,
		vectorStore: vectorStore,
		indexName:   "rag_chunks",
	}
}

// IngestDocument 导入文档（分块 + 存储）
func (r *RAGEngine) IngestDocument(ctx context.Context, sessionID string, doc *Document) ([]*Chunk, error) {
	chunks := r.chunkDocument(doc)

	var stored []*Chunk
	for _, chunk := range chunks {
		// 存储为记忆
		mem := model.NewMemory(sessionID, model.MemoryLongTerm, chunk.Content, chunk.Importance)
		mem.ID = chunk.ID

		if _, err := r.memory.Store(ctx, mem); err != nil {
			continue
		}

		// 如果有 embedding，存入向量索引
		if len(chunk.Embedding) > 0 {
			r.vectorStore.Insert(r.indexName, chunk.ID, chunk.Embedding)
		}

		stored = append(stored, chunk)
	}

	return stored, nil
}

// SemanticSearch 语义检索
func (r *RAGEngine) SemanticSearch(ctx context.Context, query []float32, topK int) ([]*SearchHit, error) {
	results, err := r.vectorStore.Search(r.indexName, query, topK)
	if err != nil {
		return nil, err
	}

	var hits []*SearchHit
	for _, result := range results {
		mem, err := r.memory.Get(ctx, result.ID)
		if err != nil {
			continue
		}

		hits = append(hits, &SearchHit{
			ChunkID:  result.ID,
			Content:  mem.Content,
			Distance: result.Distance,
			Score:    1.0 - result.Distance, // 相似度 = 1 - 距离
		})
	}

	return hits, nil
}

// BuildContext 从检索结果构建上下文
func (r *RAGEngine) BuildContext(hits []*SearchHit, maxTokens int) string {
	var parts []string
	totalLen := 0

	for _, hit := range hits {
		chunkLen := utf8.RuneCountInString(hit.Content)
		if totalLen+chunkLen > maxTokens {
			break
		}
		parts = append(parts, hit.Content)
		totalLen += chunkLen
	}

	return strings.Join(parts, "\n\n")
}

// chunkDocument 文档分块
func (r *RAGEngine) chunkDocument(doc *Document) []*Chunk {
	switch doc.ChunkStrategy {
	case ChunkBySentence:
		return r.chunkBySentence(doc)
	case ChunkByFixedSize:
		return r.chunkByFixedSize(doc, doc.ChunkSize)
	default:
		return r.chunkByParagraph(doc)
	}
}

// chunkByParagraph 按段落分块
func (r *RAGEngine) chunkByParagraph(doc *Document) []*Chunk {
	paragraphs := strings.Split(doc.Content, "\n\n")
	var chunks []*Chunk

	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		chunks = append(chunks, &Chunk{
			ID:         util.NewUUID(),
			DocumentID: doc.ID,
			Index:      i,
			Content:    para,
			Importance: 0.5,
			Metadata: map[string]any{
				"source":   doc.Title,
				"strategy": "paragraph",
			},
		})
	}

	return chunks
}

// chunkBySentence 按句子分块（合并短句到目标大小）
func (r *RAGEngine) chunkBySentence(doc *Document) []*Chunk {
	// 简单按句号/问号/感叹号分句
	sentences := splitSentences(doc.Content)
	var chunks []*Chunk
	var current []string
	currentLen := 0
	targetSize := doc.ChunkSize
	if targetSize <= 0 {
		targetSize = 500
	}

	for _, sent := range sentences {
		sentLen := utf8.RuneCountInString(sent)
		if currentLen+sentLen > targetSize && len(current) > 0 {
			chunks = append(chunks, &Chunk{
				ID:         util.NewUUID(),
				DocumentID: doc.ID,
				Index:      len(chunks),
				Content:    strings.Join(current, " "),
				Importance: 0.5,
			})
			current = nil
			currentLen = 0
		}
		current = append(current, sent)
		currentLen += sentLen
	}

	if len(current) > 0 {
		chunks = append(chunks, &Chunk{
			ID:         util.NewUUID(),
			DocumentID: doc.ID,
			Index:      len(chunks),
			Content:    strings.Join(current, " "),
			Importance: 0.5,
		})
	}

	return chunks
}

// chunkByFixedSize 按固定字符数分块
func (r *RAGEngine) chunkByFixedSize(doc *Document, size int) []*Chunk {
	if size <= 0 {
		size = 500
	}

	content := doc.Content
	var chunks []*Chunk

	for i := 0; i < len(content); i += size {
		end := i + size
		if end > len(content) {
			end = len(content)
		}

		chunks = append(chunks, &Chunk{
			ID:         util.NewUUID(),
			DocumentID: doc.ID,
			Index:      len(chunks),
			Content:    content[i:end],
			Importance: 0.5,
		})
	}

	return chunks
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for _, ch := range text {
		current.WriteRune(ch)
		if ch == '.' || ch == '!' || ch == '?' || ch == '。' || ch == '！' || ch == '？' {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}

	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	return sentences
}

// Document 文档
type Document struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	ChunkStrategy ChunkStrategy  `json:"chunk_strategy,omitempty"`
	ChunkSize     int            `json:"chunk_size,omitempty"`
	Embedding     []float32      `json:"-"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Chunk 文档分块
type Chunk struct {
	ID         string         `json:"id"`
	DocumentID string         `json:"document_id"`
	Index      int            `json:"index"`
	Content    string         `json:"content"`
	Embedding  []float32      `json:"-"`
	Importance float32        `json:"importance"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ChunkStrategy 分块策略
type ChunkStrategy string

const (
	ChunkByParagraph  ChunkStrategy = "paragraph"
	ChunkBySentence   ChunkStrategy = "sentence"
	ChunkByFixedSize  ChunkStrategy = "fixed_size"
)

// SearchHit 语义检索结果
type SearchHit struct {
	ChunkID  string  `json:"chunk_id"`
	Content  string  `json:"content"`
	Distance float32 `json:"distance"`
	Score    float32 `json:"score"`
}

// suppress unused
var _ = fmt.Sprintf
