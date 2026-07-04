package vector

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
)

func TestCosineDistance(t *testing.T) {
	tests := []struct {
		a, b []float32
		want float32
	}{
		{[]float32{1, 0}, []float32{1, 0}, 0},           // 相同向量
		{[]float32{1, 0}, []float32{0, 1}, 1},           // 正交向量
		{[]float32{1, 0}, []float32{-1, 0}, 2},          // 反向向量
		{[]float32{1, 1}, []float32{1, 1}, 0},           // 相同
		{[]float32{1, 0, 0}, []float32{0, 1, 0}, 1},    // 3D 正交
	}

	for _, tt := range tests {
		got := CosineDistance(tt.a, tt.b)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("CosineDistance(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestL2Distance(t *testing.T) {
	tests := []struct {
		a, b []float32
		want float32
	}{
		{[]float32{0, 0}, []float32{3, 4}, 5},
		{[]float32{1, 1}, []float32{1, 1}, 0},
		{[]float32{0, 0, 0}, []float32{1, 1, 1}, float32(math.Sqrt(3))},
	}

	for _, tt := range tests {
		got := L2Distance(tt.a, tt.b)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("L2Distance(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestHNSW_BasicInsertSearch(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		Dim:            3,
		M:              8,
		EfConstruction: 50,
		EfSearch:       32,
		DistFn:         L2Distance,
	})

	// 插入几个向量
	idx.Insert("a", []float32{1, 0, 0})
	idx.Insert("b", []float32{0, 1, 0})
	idx.Insert("c", []float32{0, 0, 1})
	idx.Insert("d", []float32{1, 1, 0})

	if idx.Len() != 4 {
		t.Fatalf("expected 4 nodes, got %d", idx.Len())
	}

	// 搜索与 [1, 0, 0] 最近的
	results := idx.Search([]float32{1, 0, 0}, 2)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].ID != "a" {
		t.Errorf("expected nearest to be 'a', got %q", results[0].ID)
	}
}

func TestHNSW_SearchAccuracy(t *testing.T) {
	dim := 16
	n := 1000
	k := 10

	idx := NewHNSWIndex(HNSWConfig{
		Dim:            dim,
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		DistFn:         CosineDistance,
	})

	// 生成随机向量
	vectors := make([][]float32, n)
	for i := 0; i < n; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()*2 - 1
		}
		vectors[i] = vec
		idx.Insert(fmt.Sprintf("v%d", i), vec)
	}

	// 暴力搜索作为 ground truth
	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rand.Float32()*2 - 1
	}

	bruteForce := bruteForceSearch(query, vectors, k, CosineDistance)

	// HNSW 搜索
	results := idx.Search(query, k)

	// 计算召回率
	bruteIDs := make(map[string]bool)
	for _, r := range bruteForce {
		bruteIDs[fmt.Sprintf("v%d", r.idx)] = true
	}

	recalled := 0
	for _, r := range results {
		if bruteIDs[r.ID] {
			recalled++
		}
	}

	recall := float64(recalled) / float64(k)
	t.Logf("Recall@%d: %.2f%% (%d/%d)", k, recall*100, recalled, k)

	if recall < 0.5 {
		t.Errorf("recall too low: %.2f%%", recall*100)
	}
}

func TestHNSW_Delete(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		Dim: 2,
		M:   8,
	})

	idx.Insert("a", []float32{1, 0})
	idx.Insert("b", []float32{0, 1})
	idx.Insert("c", []float32{0.5, 0.5})

	if idx.Len() != 3 {
		t.Fatalf("expected 3, got %d", idx.Len())
	}

	idx.Delete("b")

	if idx.Len() != 2 {
		t.Fatalf("expected 2 after delete, got %d", idx.Len())
	}

	if idx.Contains("b") {
		t.Fatal("expected 'b' to be deleted")
	}

	// 搜索应仍然工作
	results := idx.Search([]float32{1, 0}, 1)
	if len(results) == 0 {
		t.Fatal("expected results after delete")
	}
	if results[0].ID != "a" {
		t.Errorf("expected 'a', got %q", results[0].ID)
	}
}

func TestHNSW_Update(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dim: 2, M: 8})

	idx.Insert("a", []float32{1, 0})
	idx.Insert("a", []float32{0, 1}) // 更新

	if idx.Len() != 1 {
		t.Fatalf("expected 1 after update, got %d", idx.Len())
	}

	results := idx.Search([]float32{0, 1}, 1)
	if len(results) != 1 || results[0].ID != "a" {
		t.Fatal("expected 'a' to be nearest to [0,1]")
	}
}

func TestHNSW_EmptySearch(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dim: 2, M: 8})
	results := idx.Search([]float32{1, 0}, 10)
	if results != nil {
		t.Fatalf("expected nil results from empty index, got %d", len(results))
	}
}

func TestHNSW_DimensionMismatch(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dim: 3, M: 8})
	idx.Insert("a", []float32{1, 0, 0})
	idx.Insert("b", []float32{1, 0}) // 维度不匹配，应被忽略

	if idx.Len() != 1 {
		t.Fatalf("expected 1, got %d", idx.Len())
	}
}

func TestHNSW_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	dim := 128
	n := 5000
	k := 10

	idx := NewHNSWIndex(HNSWConfig{
		Dim:            dim,
		M:              16,
		EfConstruction: 100,
		EfSearch:       64,
	})

	// 生成随机向量
	for i := 0; i < n; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()*2 - 1
		}
		idx.Insert(fmt.Sprintf("v%d", i), vec)
	}

	// 搜索
	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rand.Float32()*2 - 1
	}

	results := idx.Search(query, k)
	if len(results) != k {
		t.Fatalf("expected %d results, got %d", k, len(results))
	}

	// 验证结果有序
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Errorf("results not sorted: dist[%d]=%f < dist[%d]=%f", i, results[i].Distance, i-1, results[i-1].Distance)
		}
	}
}

func BenchmarkHNSW_Insert(b *testing.B) {
	dim := 128
	idx := NewHNSWIndex(HNSWConfig{Dim: dim, M: 16, EfConstruction: 100})

	vec := make([]float32, dim)
	for j := range vec {
		vec[j] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Insert(fmt.Sprintf("v%d", i), vec)
	}
}

func BenchmarkHNSW_Search(b *testing.B) {
	dim := 128
	n := 10000
	idx := NewHNSWIndex(HNSWConfig{Dim: dim, M: 16, EfConstruction: 100, EfSearch: 64})

	for i := 0; i < n; i++ {
		vec := make([]float32, dim)
		for j := range vec {
			vec[j] = rand.Float32()*2 - 1
		}
		idx.Insert(fmt.Sprintf("v%d", i), vec)
	}

	query := make([]float32, dim)
	for j := range query {
		query[j] = rand.Float32()*2 - 1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(query, 10)
	}
}

// bruteForceSearch 暴力搜索（用于验证召回率）
func bruteForceSearch(query []float32, vectors [][]float32, k int, distFn DistanceFunc) []struct{ idx int; dist float32 } {
	type pair struct {
		idx  int
		dist float32
	}

	dists := make([]pair, len(vectors))
	for i, v := range vectors {
		dists[i] = pair{idx: i, dist: distFn(query, v)}
	}

	sort.Slice(dists, func(i, j int) bool {
		return dists[i].dist < dists[j].dist
	})

	if len(dists) > k {
		dists = dists[:k]
	}

	result := make([]struct{ idx int; dist float32 }, len(dists))
	for i, d := range dists {
		result[i] = struct{ idx int; dist float32 }{d.idx, d.dist}
	}
	return result
}
