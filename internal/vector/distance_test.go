package vector

import (
	"math"
	"testing"
)

// ========== Cosine Distance Tests ==========

func TestCosineDistance_Identical(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	d := CosineDistance(a, b)
	if d != 0.0 {
		t.Fatalf("expected distance 0 for identical vectors, got %f", d)
	}
}

func TestCosineDistance_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}

	d := CosineDistance(a, b)
	if math.Abs(float64(d-1.0)) > 1e-6 {
		t.Fatalf("expected distance 1 for orthogonal vectors, got %f", d)
	}
}

func TestCosineDistance_Opposite(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{-1.0, 0.0, 0.0}

	d := CosineDistance(a, b)
	if math.Abs(float64(d-2.0)) > 1e-6 {
		t.Fatalf("expected distance 2 for opposite vectors, got %f", d)
	}
}

func TestCosineDistance_Similar(t *testing.T) {
	a := []float32{1.0, 0.5, 0.0}
	b := []float32{0.9, 0.5, 0.0}

	d := CosineDistance(a, b)
	if d >= 0.01 {
		t.Fatalf("expected small distance for similar vectors, got %f", d)
	}
}

func TestCosineDistance_DifferentDims(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	d := CosineDistance(a, b)
	if d != 1.0 {
		t.Fatalf("expected distance 1 for different dimensions, got %f", d)
	}
}

func TestCosineDistance_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	d := CosineDistance(a, b)
	if d != 1.0 {
		t.Fatalf("expected distance 1 for zero vector, got %f", d)
	}
}

func TestCosineDistance_LargeVector(t *testing.T) {
	a := make([]float32, 1000)
	b := make([]float32, 1000)
	for i := range a {
		a[i] = float32(i)
		b[i] = float32(i)
	}

	d := CosineDistance(a, b)
	if math.Abs(float64(d)) > 1e-5 {
		t.Fatalf("expected distance ~0 for identical large vectors, got %f", d)
	}
}

func TestCosineDistance_UnitVectors(t *testing.T) {
	// Two unit vectors at 60 degrees
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.5, float32(math.Sqrt(3) / 2), 0.0}

	d := CosineDistance(a, b)
	// cos(60°) = 0.5, so distance = 1 - 0.5 = 0.5
	if math.Abs(float64(d-0.5)) > 1e-6 {
		t.Fatalf("expected distance 0.5, got %f", d)
	}
}

// ========== L2 Distance Tests ==========

func TestL2Distance_Identical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}

	d := L2Distance(a, b)
	if d != 0.0 {
		t.Fatalf("expected distance 0 for identical vectors, got %f", d)
	}
}

func TestL2Distance_Simple(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{3.0, 4.0}

	d := L2Distance(a, b)
	// sqrt(9 + 16) = 5
	if math.Abs(float64(d-5.0)) > 1e-6 {
		t.Fatalf("expected distance 5, got %f", d)
	}
}

func TestL2Distance_1D(t *testing.T) {
	a := []float32{1.0}
	b := []float32{5.0}

	d := L2Distance(a, b)
	if math.Abs(float64(d-4.0)) > 1e-6 {
		t.Fatalf("expected distance 4, got %f", d)
	}
}

func TestL2Distance_DifferentDims(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	d := L2Distance(a, b)
	if d != math.MaxFloat32 {
		t.Fatalf("expected MaxFloat32 for different dimensions, got %f", d)
	}
}

func TestL2Distance_NegativeValues(t *testing.T) {
	a := []float32{-1.0, -2.0}
	b := []float32{1.0, 2.0}

	d := L2Distance(a, b)
	// sqrt(4 + 16) = sqrt(20) ≈ 4.472
	expected := float32(math.Sqrt(20))
	if math.Abs(float64(d-expected)) > 1e-6 {
		t.Fatalf("expected distance %f, got %f", expected, d)
	}
}

// ========== Dot Distance Tests ==========

func TestDotDistance_Identical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}

	d := DotDistance(a, b)
	// dot = 1 + 4 + 9 = 14, distance = -14
	if d != -14.0 {
		t.Fatalf("expected distance -14, got %f", d)
	}
}

func TestDotDistance_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}

	d := DotDistance(a, b)
	if d != 0.0 {
		t.Fatalf("expected distance 0 for orthogonal vectors, got %f", d)
	}
}

func TestDotDistance_DifferentDims(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	d := DotDistance(a, b)
	if d != 1.0 {
		t.Fatalf("expected distance 1 for different dimensions, got %f", d)
	}
}

func TestDotDistance_Negative(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}

	d := DotDistance(a, b)
	// dot = -1, distance = 1
	if d != 1.0 {
		t.Fatalf("expected distance 1, got %f", d)
	}
}

// ========== GetDistanceFunc Tests ==========

func TestGetDistanceFunc_Cosine(t *testing.T) {
	fn := GetDistanceFunc("cosine")
	if fn == nil {
		t.Fatal("expected cosine function")
	}

	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0}
	d := fn(a, b)
	if d != 0.0 {
		t.Fatalf("cosine distance: expected 0, got %f", d)
	}
}

func TestGetDistanceFunc_L2(t *testing.T) {
	fn := GetDistanceFunc("l2")
	if fn == nil {
		t.Fatal("expected l2 function")
	}

	a := []float32{0.0}
	b := []float32{3.0}
	d := fn(a, b)
	if d != 3.0 {
		t.Fatalf("l2 distance: expected 3, got %f", d)
	}
}

func TestGetDistanceFunc_Euclidean(t *testing.T) {
	fn := GetDistanceFunc("euclidean")
	if fn == nil {
		t.Fatal("expected euclidean function")
	}

	a := []float32{0.0}
	b := []float32{5.0}
	d := fn(a, b)
	if d != 5.0 {
		t.Fatalf("euclidean distance: expected 5, got %f", d)
	}
}

func TestGetDistanceFunc_Dot(t *testing.T) {
	fn := GetDistanceFunc("dot")
	if fn == nil {
		t.Fatal("expected dot function")
	}

	a := []float32{2.0, 3.0}
	b := []float32{4.0, 1.0}
	d := fn(a, b)
	// dot = 8 + 3 = 11, distance = -11
	if d != -11.0 {
		t.Fatalf("dot distance: expected -11, got %f", d)
	}
}

func TestGetDistanceFunc_DotProduct(t *testing.T) {
	fn := GetDistanceFunc("dotproduct")
	if fn == nil {
		t.Fatal("expected dotproduct function")
	}

	a := []float32{1.0, 2.0}
	b := []float32{3.0, 4.0}
	d := fn(a, b)
	// dot = 3 + 8 = 11, distance = -11
	if d != -11.0 {
		t.Fatalf("dotproduct distance: expected -11, got %f", d)
	}
}

func TestGetDistanceFunc_Default(t *testing.T) {
	fn := GetDistanceFunc("unknown")
	if fn == nil {
		t.Fatal("expected default function")
	}

	// Should default to cosine
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0}
	d := fn(a, b)
	if d != 0.0 {
		t.Fatalf("default distance: expected 0, got %f", d)
	}
}

func TestGetDistanceFunc_Empty(t *testing.T) {
	fn := GetDistanceFunc("")
	if fn == nil {
		t.Fatal("expected default function for empty string")
	}

	// Empty string should default to cosine
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	d := fn(a, b)
	if math.Abs(float64(d-1.0)) > 1e-6 {
		t.Fatalf("empty string defaults to cosine: expected 1, got %f", d)
	}
}

// ========== Vector Serialization Tests ==========

func TestFloat32sToBytes(t *testing.T) {
	input := []float32{1.5, -2.5, 0.0, 3.14159}
	bytes := Float32sToBytes(input)

	if len(bytes) != 16 {
		t.Fatalf("expected 16 bytes (4 * 4), got %d", len(bytes))
	}
}

func TestBytesToFloat32s(t *testing.T) {
	input := []float32{1.5, -2.5, 0.0, 3.14159}
	bytes := Float32sToBytes(input)
	result := BytesToFloat32s(bytes)

	if len(result) != len(input) {
		t.Fatalf("expected %d floats, got %d", len(input), len(result))
	}

	for i := range input {
		if math.Abs(float64(result[i]-input[i])) > 1e-5 {
			t.Fatalf("mismatch at index %d: expected %f, got %f", i, input[i], result[i])
		}
	}
}

func TestFloat32sToBytesRoundTrip(t *testing.T) {
	testCases := [][]float32{
		{0.0},
		{1.0, 2.0, 3.0},
		{-1.0, -2.0, -3.0},
		{0.001, 0.002, 0.003},
		{1000.0, -1000.0, 0.0},
		make([]float32, 1000), // large vector
	}

	for _, tc := range testCases {
		// Fill large vector with values
		if len(tc) == 1000 {
			for i := range tc {
				tc[i] = float32(i)
			}
		}

		bytes := Float32sToBytes(tc)
		result := BytesToFloat32s(bytes)

		for i := range tc {
			if math.Abs(float64(result[i]-tc[i])) > 1e-5 {
				t.Fatalf("round trip failed at index %d", i)
			}
		}
	}
}

func TestParseVectorLiteral(t *testing.T) {
	tests := []struct {
		input   string
		want    []float32
		wantErr bool
	}{
		{"[0.1, 0.2, 0.3]", []float32{0.1, 0.2, 0.3}, false},
		{"[1.0]", []float32{1.0}, false},
		{"[]", nil, false},
		{"[ -1.5 , 2.5 ]", []float32{-1.5, 2.5}, false},
		{"not a vector", nil, true},
		{"[", nil, true},
		{"]", nil, true},
		{"[abc]", nil, true},
		{"[1.0, xyz]", nil, true},
	}

	for _, tt := range tests {
		got, err := ParseVectorLiteral(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseVectorLiteral(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVectorLiteral(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if tt.want == nil {
			if got != nil {
				t.Errorf("ParseVectorLiteral(%q) expected nil, got %v", tt.input, got)
			}
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParseVectorLiteral(%q) expected length %d, got %d", tt.input, len(tt.want), len(got))
			continue
		}
		for i := range got {
			if math.Abs(float64(got[i]-tt.want[i])) > 1e-5 {
				t.Errorf("ParseVectorLiteral(%q) at index %d: expected %f, got %f", tt.input, i, tt.want[i], got[i])
			}
		}
	}
}
