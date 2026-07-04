package vector

import "math"

// DistanceFunc 距离函数
type DistanceFunc func(a, b []float32) float32

// CosineDistance 余弦距离（1 - cosine_similarity）
func CosineDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return 1.0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 1.0
	}
	return 1.0 - dot/(float32(math.Sqrt(float64(normA)))*float32(math.Sqrt(float64(normB))))
}

// L2Distance 欧几里得距离
func L2Distance(a, b []float32) float32 {
	if len(a) != len(b) {
		return math.MaxFloat32
	}
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return float32(math.Sqrt(float64(sum)))
}

// DotDistance 点积距离（1 - dot_product）
func DotDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return 1.0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return -dot // 负点积，越小越好
}

// GetDistanceFunc 根据名称返回距离函数
func GetDistanceFunc(name string) DistanceFunc {
	switch name {
	case "l2", "euclidean":
		return L2Distance
	case "dot", "dotproduct":
		return DotDistance
	case "cosine", "":
		return CosineDistance
	default:
		return CosineDistance
	}
}
