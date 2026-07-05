package vector

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

type vectorStoreTestEnv struct {
	engine *badgerstore.BadgerEngine
	store  *VectorStore
	ctx    context.Context
}

func newVectorStoreTestEnv(t *testing.T) *vectorStoreTestEnv {
	t.Helper()
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	if err := engine.Open(opts); err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() {
		engine.Close()
		os.RemoveAll(dir)
	})

	return &vectorStoreTestEnv{
		engine: engine,
		store:  NewVectorStore(engine),
		ctx:    context.Background(),
	}
}

// ========== Index Creation Tests ==========

func TestVectorStore_CreateIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	err := env.store.CreateIndex("test_index", 4, "cosine")
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	if env.store.Len("test_index") != 0 {
		t.Fatal("expected empty index after creation")
	}
}

func TestVectorStore_CreateDuplicateIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test_index", 4, "cosine")
	err := env.store.CreateIndex("test_index", 4, "cosine")
	if err == nil {
		t.Fatal("expected error for duplicate index")
	}
}

func TestVectorStore_CreateMultipleIndexes(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("cosine_idx", 4, "cosine")
	env.store.CreateIndex("l2_idx", 4, "l2")
	env.store.CreateIndex("dot_idx", 4, "dot")

	// Verify all indexes exist
	if env.store.Len("cosine_idx") != 0 {
		t.Fatal("cosine_idx should be empty")
	}
	if env.store.Len("l2_idx") != 0 {
		t.Fatal("l2_idx should be empty")
	}
	if env.store.Len("dot_idx") != 0 {
		t.Fatal("dot_idx should be empty")
	}
}

// ========== Insert and Search Tests ==========

func TestVectorStore_Insert(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test", 3, "cosine")

	vec := []float32{0.1, 0.2, 0.3}
	err := env.store.Insert("test", "vec-001", vec)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if env.store.Len("test") != 1 {
		t.Fatalf("expected 1 vector, got %d", env.store.Len("test"))
	}
}

func TestVectorStore_InsertNotFoundIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	err := env.store.Insert("nonexistent", "vec-001", []float32{0.1, 0.2})
	if err == nil {
		t.Fatal("expected error for nonexistent index")
	}
}

func TestVectorStore_Search(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test", 3, "cosine")

	// Insert vectors
	vectors := map[string][]float32{
		"vec-a": {1.0, 0.0, 0.0},
		"vec-b": {0.0, 1.0, 0.0},
		"vec-c": {0.0, 0.0, 1.0},
	}
	for id, vec := range vectors {
		env.store.Insert("test", id, vec)
	}

	// Search for vec-a
	results, err := env.store.Search("test", []float32{1.0, 0.0, 0.0}, 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].ID != "vec-a" {
		t.Fatalf("expected vec-a, got %s", results[0].ID)
	}
}

func TestVectorStore_SearchNotFoundIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	_, err := env.store.Search("nonexistent", []float32{0.1}, 5)
	if err == nil {
		t.Fatal("expected error for nonexistent index")
	}
}

func TestVectorStore_SearchTopK(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test", 3, "cosine")

	// Insert 10 vectors with unique IDs
	for i := 0; i < 10; i++ {
		vec := []float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3}
		env.store.Insert("test", fmt.Sprintf("vec-%03d", i), vec)
	}

	// Search with topK=3
	results, err := env.store.Search("test", []float32{0.5, 1.0, 1.5}, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

// ========== Delete Tests ==========

func TestVectorStore_Delete(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test", 3, "cosine")
	env.store.Insert("test", "vec-001", []float32{0.1, 0.2, 0.3})

	if err := env.store.Delete("test", "vec-001"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if env.store.Len("test") != 0 {
		t.Fatalf("expected 0 vectors after delete, got %d", env.store.Len("test"))
	}
}

func TestVectorStore_DeleteNotFoundIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	err := env.store.Delete("nonexistent", "vec-001")
	if err == nil {
		t.Fatal("expected error for nonexistent index")
	}
}

// ========== Search With Vectors ==========

func TestVectorStore_SearchWithVectors(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("test", 3, "cosine")

	vectors := map[string][]float32{
		"vec-a": {1.0, 0.0, 0.0},
		"vec-b": {0.5, 0.5, 0.0},
		"vec-c": {0.0, 0.0, 1.0},
	}
	for id, vec := range vectors {
		env.store.Insert("test", id, vec)
	}

	results, err := env.store.SearchWithVectors("test", []float32{1.0, 0.0, 0.0}, 2, false)
	if err != nil {
		t.Fatalf("search with vectors: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Verify vector data is included
	if len(results[0].Vector) != 3 {
		t.Fatalf("expected vector of length 3, got %d", len(results[0].Vector))
	}
}

func TestVectorStore_SearchWithVectorsNotFoundIndex(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	_, err := env.store.SearchWithVectors("nonexistent", []float32{0.1}, 5, false)
	if err == nil {
		t.Fatal("expected error for nonexistent index")
	}
}

// ========== List Indexes ==========

func TestVectorStore_ListIndexes(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	// Create indexes in non-alphabetical order
	env.store.CreateIndex("zebra", 4, "cosine")
	env.store.CreateIndex("alpha", 4, "cosine")
	env.store.CreateIndex("middle", 4, "cosine")

	indexes := env.store.ListIndexes()

	if len(indexes) != 3 {
		t.Fatalf("expected 3 indexes, got %d", len(indexes))
	}

	// Verify sorted order
	if indexes[0] != "alpha" || indexes[1] != "middle" || indexes[2] != "zebra" {
		t.Fatalf("expected sorted order, got %v", indexes)
	}
}

func TestVectorStore_ListIndexesEmpty(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	indexes := env.store.ListIndexes()
	if len(indexes) != 0 {
		t.Fatalf("expected 0 indexes, got %d", len(indexes))
	}
}

// ========== Persistence Tests ==========

func TestVectorStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)

	store := NewVectorStore(engine)
	store.CreateIndex("test", 3, "cosine")
	store.Insert("test", "vec-001", []float32{0.1, 0.2, 0.3})

	// Close and reopen
	engine.Close()

	engine2 := badgerstore.New()
	opts2 := storage.DefaultOptions()
	opts2.DataDir = dir
	opts2.SyncWrites = false
	engine2.Open(opts2)
	defer engine2.Close()

	store2 := NewVectorStore(engine2)
	store2.CreateIndex("test", 3, "cosine")

	// Verify vector was loaded from storage
	if store2.Len("test") != 1 {
		t.Fatalf("expected 1 vector after reload, got %d", store2.Len("test"))
	}
}

// ========== Multi-Index Scenario Tests ==========

func TestVectorStore_MultiIndexDifferentDims(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	env.store.CreateIndex("idx-4d", 4, "cosine")
	env.store.CreateIndex("idx-8d", 8, "cosine")
	env.store.CreateIndex("idx-128d", 128, "l2")

	// Insert vectors of appropriate dimensions
	env.store.Insert("idx-4d", "v1", []float32{0.1, 0.2, 0.3, 0.4})
	env.store.Insert("idx-8d", "v2", []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8})

	vec128 := make([]float32, 128)
	for i := range vec128 {
		vec128[i] = float32(i) / 128.0
	}
	env.store.Insert("idx-128d", "v3", vec128)

	// Verify each index has correct count
	if env.store.Len("idx-4d") != 1 {
		t.Fatalf("idx-4d: expected 1, got %d", env.store.Len("idx-4d"))
	}
	if env.store.Len("idx-8d") != 1 {
		t.Fatalf("idx-8d: expected 1, got %d", env.store.Len("idx-8d"))
	}
	if env.store.Len("idx-128d") != 1 {
		t.Fatalf("idx-128d: expected 1, got %d", env.store.Len("idx-128d"))
	}
}

func TestVectorStore_MultiIndexDifferentMetrics(t *testing.T) {
	env := newVectorStoreTestEnv(t)

	vec := []float32{1.0, 0.0, 0.0, 0.0}

	env.store.CreateIndex("cosine-idx", 4, "cosine")
	env.store.CreateIndex("l2-idx", 4, "l2")
	env.store.CreateIndex("dot-idx", 4, "dot")

	env.store.Insert("cosine-idx", "vec-001", vec)
	env.store.Insert("l2-idx", "vec-001", vec)
	env.store.Insert("dot-idx", "vec-001", vec)

	// Search in each index
	q := []float32{0.9, 0.1, 0.0, 0.0}

	cosineRes, _ := env.store.Search("cosine-idx", q, 1)
	l2Res, _ := env.store.Search("l2-idx", q, 1)
	dotRes, _ := env.store.Search("dot-idx", q, 1)

	// All should return the vector
	if len(cosineRes) != 1 || cosineRes[0].ID != "vec-001" {
		t.Fatalf("cosine search failed")
	}
	if len(l2Res) != 1 || l2Res[0].ID != "vec-001" {
		t.Fatalf("l2 search failed")
	}
	if len(dotRes) != 1 || dotRes[0].ID != "vec-001" {
		t.Fatalf("dot search failed")
	}
}

// ========== Key Encoding Tests ==========

func TestEncodeVectorKey(t *testing.T) {
	key := EncodeVectorKey("my_index", "vec-001")
	if len(key) == 0 {
		t.Fatal("expected non-empty key")
	}
}

func TestExtractVectorID(t *testing.T) {
	key := EncodeVectorKey("my_index", "vec-001")
	id := extractVectorID(key, "my_index")
	if id != "vec-001" {
		t.Fatalf("expected 'vec-001', got %q", id)
	}
}

func TestExtractVectorIDInvalid(t *testing.T) {
	id := extractVectorID([]byte{0x11}, "my_index")
	if id != "" {
		t.Fatalf("expected empty id for invalid key, got %q", id)
	}
}

// ========== Benchmark Tests ==========

func BenchmarkVectorStore_Insert(b *testing.B) {
	dir := b.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	store := NewVectorStore(engine)
	store.CreateIndex("bench", 128, "cosine")

	vec := make([]float32, 128)
	for i := range vec {
		vec[i] = float32(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := "vec"
		store.Insert("bench", id, vec)
	}
}

func BenchmarkVectorStore_Search(b *testing.B) {
	dir := b.TempDir()
	engine := badgerstore.New()
	opts := storage.DefaultOptions()
	opts.DataDir = dir
	opts.SyncWrites = false
	engine.Open(opts)
	defer engine.Close()

	store := NewVectorStore(engine)
	store.CreateIndex("bench", 128, "cosine")

	// Insert 1000 vectors
	for i := 0; i < 1000; i++ {
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = float32(i*j) / 1000.0
		}
		store.Insert("bench", fmt.Sprintf("vec-%04d", i), vec)
	}

	query := make([]float32, 128)
	for i := range query {
		query[i] = float32(i) / 128.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search("bench", query, 10)
	}
}
