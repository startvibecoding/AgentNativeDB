package agent

import (
	"context"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

type ragTestEnv struct {
	engine      storage.Engine
	cache       *storage.Cache
	memory      *MemoryStore
	vectorStore *vector.VectorStore
	rag         *RAGEngine
	ctx         context.Context
}

func newRAGTestEnv(t *testing.T) *ragTestEnv {
	t.Helper()
	engine := storage.NewTestEngine(t)

	cache := storage.NewCache(512)
	memory := NewMemoryStore(engine, cache)
	vectorStore := vector.NewVectorStore(engine)

	// 创建向量索引
	if err := vectorStore.CreateIndex("rag_chunks", 4, "cosine"); err != nil {
		t.Fatalf("create index: %v", err)
	}

	rag := NewRAGEngine(engine, memory, vectorStore)

	return &ragTestEnv{
		engine:      engine,
		cache:       cache,
		memory:      memory,
		vectorStore: vectorStore,
		rag:         rag,
		ctx:         context.Background(),
	}
}

// ========== Document Chunking Tests ==========

func TestRAG_ChunkByParagraph(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:      "doc-001",
		Title:   "Test Document",
		Content: "First paragraph.\n\nSecond paragraph.\n\n\n\nThird paragraph with more content.",
	}

	chunks := env.rag.chunkDocument(doc)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	if chunks[0].Content != "First paragraph." {
		t.Fatalf("expected 'First paragraph.', got %q", chunks[0].Content)
	}
	if chunks[1].Content != "Second paragraph." {
		t.Fatalf("expected 'Second paragraph.', got %q", chunks[1].Content)
	}
	if chunks[2].Content != "Third paragraph with more content." {
		t.Fatalf("expected third paragraph, got %q", chunks[2].Content)
	}
}

func TestRAG_ChunkBySentence(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:            "doc-002",
		Title:         "Sentence Test",
		Content:       "First sentence. Second sentence? Third sentence! Fourth sentence.",
		ChunkStrategy: ChunkBySentence,
		ChunkSize:     30,
	}

	chunks := env.rag.chunkDocument(doc)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestRAG_ChunkByFixedSize(t *testing.T) {
	env := newRAGTestEnv(t)

	content := ""
	for i := 0; i < 100; i++ {
		content += "x"
	}

	doc := &Document{
		ID:            "doc-003",
		Title:         "Fixed Size Test",
		Content:       content,
		ChunkStrategy: ChunkByFixedSize,
		ChunkSize:     25,
	}

	chunks := env.rag.chunkDocument(doc)

	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks (100/25), got %d", len(chunks))
	}

	for _, chunk := range chunks {
		if chunk.DocumentID != "doc-003" {
			t.Fatalf("expected document_id 'doc-003', got %q", chunk.DocumentID)
		}
	}
}

func TestRAG_ChunkBySentenceWithChinese(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:            "doc-004",
		Title:         "Chinese Test",
		Content:       "这是第一句话。这是第二句话？这是第三句话！",
		ChunkStrategy: ChunkBySentence,
		ChunkSize:     20,
	}

	chunks := env.rag.chunkDocument(doc)

	if len(chunks) < 1 {
		t.Fatal("expected at least 1 chunk")
	}
}

func TestRAG_ChunkBySentenceDefaultSize(t *testing.T) {
	env := newRAGTestEnv(t)

	content := ""
	for i := 0; i < 600; i++ {
		content += "a"
	}
	content += "."

	doc := &Document{
		ID:            "doc-005",
		Title:         "Default Size Test",
		Content:       content,
		ChunkStrategy: ChunkBySentence,
		ChunkSize:     0, // 使用默认值
	}

	chunks := env.rag.chunkDocument(doc)

	if len(chunks) < 1 {
		t.Fatal("expected at least 1 chunk")
	}
}

func TestRAG_EmptyDocument(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:      "doc-empty",
		Title:   "Empty",
		Content: "",
	}

	chunks := env.rag.chunkDocument(doc)

	// Empty document should produce no chunks (or handle gracefully)
	if len(chunks) != 0 {
		t.Logf("empty document produced %d chunks", len(chunks))
	}
}

func TestRAG_ChunkMetadata(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:      "doc-meta",
		Title:   "Metadata Test",
		Content: "Para 1.\n\nPara 2.",
	}

	chunks := env.rag.chunkDocument(doc)

	for _, chunk := range chunks {
		if chunk.ID == "" {
			t.Fatal("expected chunk ID to be set")
		}
		if chunk.Importance != 0.5 {
			t.Fatalf("expected importance 0.5, got %f", chunk.Importance)
		}
		if chunk.Metadata["source"] != "Metadata Test" {
			t.Fatalf("expected source 'Metadata Test', got %q", chunk.Metadata["source"])
		}
	}
}

// ========== Ingest & Search Tests ==========

func TestRAG_IngestDocument(t *testing.T) {
	env := newRAGTestEnv(t)

	doc := &Document{
		ID:      "doc-ingest",
		Title:   "Ingest Test",
		Content: "First paragraph about AI.\n\nSecond paragraph about ML.\n\nThird paragraph about DL.",
	}

	chunks, err := env.rag.IngestDocument(env.ctx, "session-001", doc)
	if err != nil {
		t.Fatalf("ingest document: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// Verify memories were created
	for _, chunk := range chunks {
		mem, err := env.memory.Get(env.ctx, chunk.ID)
		if err != nil {
			t.Fatalf("get memory for chunk %s: %v", chunk.ID, err)
		}
		if mem.Content != chunk.Content {
			t.Fatalf("content mismatch: expected %q, got %q", chunk.Content, mem.Content)
		}
	}
}

func TestRAG_BuildContext(t *testing.T) {
	env := newRAGTestEnv(t)

	hits := []*SearchHit{
		{ChunkID: "1", Content: "First relevant chunk.", Distance: 0.1, Score: 0.9},
		{ChunkID: "2", Content: "Second relevant chunk.", Distance: 0.2, Score: 0.8},
		{ChunkID: "3", Content: "Third relevant chunk.", Distance: 0.3, Score: 0.7},
	}

	ctx := env.rag.BuildContext(hits, 100)
	if ctx == "" {
		t.Fatal("expected context to be built")
	}

	// Test with small token limit (enough for first hit only)
	smallCtx := env.rag.BuildContext(hits, 25)
	if smallCtx != "First relevant chunk." {
		t.Fatalf("expected truncated context, got %q", smallCtx)
	}

	// Test with very large limit
	largeCtx := env.rag.BuildContext(hits, 10000)
	if largeCtx == "" {
		t.Fatal("expected large context")
	}
}

// ========== Sentence Splitting Tests ==========

func TestSplitSentences_English(t *testing.T) {
	sentences := splitSentences("Hello world. How are you? I am fine!")
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d", len(sentences))
	}
	if sentences[0] != "Hello world." {
		t.Fatalf("expected 'Hello world.', got %q", sentences[0])
	}
}

func TestSplitSentences_Chinese(t *testing.T) {
	sentences := splitSentences("你好。世界！测试？")
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d", len(sentences))
	}
}

func TestSplitSentences_Mixed(t *testing.T) {
	sentences := splitSentences("Hello world. 你好。How are you? 很好！")
	if len(sentences) != 4 {
		t.Fatalf("expected 4 sentences, got %d", len(sentences))
	}
}

func TestSplitSentences_Trailing(t *testing.T) {
	sentences := splitSentences("No punctuation here")
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
}

func TestSplitSentences_Empty(t *testing.T) {
	sentences := splitSentences("")
	if len(sentences) != 0 {
		t.Fatalf("expected 0 sentences, got %d", len(sentences))
	}
}

// ========== Semantic Search Tests ==========

func TestRAG_SemanticSearch(t *testing.T) {
	env := newRAGTestEnv(t)

	// Ingest documents with embeddings
	doc := &Document{
		ID:      "doc-search",
		Title:   "Search Test",
		Content: "Machine learning is great.\n\nDeep learning is powerful.\n\nNatural language processing is fascinating.",
	}

	chunks, _ := env.rag.IngestDocument(env.ctx, "session-search", doc)

	// Add embeddings to the vector index
	for i, chunk := range chunks {
		vec := []float32{0.1, 0.2, 0.3, 0.4}
		vec[i] = 1.0 // Make each chunk have a distinct vector
		env.vectorStore.Insert("rag_chunks", chunk.ID, vec)
	}

	// Search
	query := []float32{0.1, 0.2, 0.3, 0.4}
	hits, err := env.rag.SemanticSearch(env.ctx, query, 2)
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}

	if len(hits) > 2 {
		t.Fatalf("expected at most 2 hits, got %d", len(hits))
	}
}

func TestRAG_SemanticSearchEmptyIndex(t *testing.T) {
	env := newRAGTestEnv(t)

	// Search without any vectors inserted
	query := []float32{0.1, 0.2, 0.3, 0.4}
	hits, err := env.rag.SemanticSearch(env.ctx, query, 5)
	if err != nil {
		t.Fatalf("semantic search on empty index: %v", err)
	}

	if len(hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(hits))
	}
}
