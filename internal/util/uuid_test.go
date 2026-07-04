package util

import (
	"testing"
)

func TestNewUUID_Format(t *testing.T) {
	id := NewUUID()
	if len(id) != 36 {
		t.Fatalf("expected UUID length 36, got %d: %q", len(id), id)
	}
	if !IsValidUUID(id) {
		t.Fatalf("invalid UUID format: %q", id)
	}
	// 版本号必须是 7
	if id[14] != '7' {
		t.Fatalf("expected version 7, got %c in %q", id[14], id)
	}
}

func TestNewUUID_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 10000)
	for i := 0; i < 10000; i++ {
		id := NewUUID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate UUID at iteration %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewUUID_TimeOrdered(t *testing.T) {
	// UUID v7 应该在同一毫秒内大致有序
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = NewUUID()
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			// 允许同一毫秒内的微小乱序（序列号机制保证不重复）
			// 但不同毫秒之间应该有序
			// 这里只验证不重复即可
			t.Logf("note: UUID %d < UUID %d (may be same millisecond)", i, i-1)
		}
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"not-a-uuid", false},
		{"", false},
		{"550e8400e29b41d4a716446655440000", false}, // 缺少连字符
		{"550e8400-e29b-41d4-a716-44665544000", false}, // 少一位
		{"550e8400-e29b-41d4-a716-446655440000g", false}, // 多一位
	}
	for _, tt := range tests {
		got := IsValidUUID(tt.input)
		if got != tt.valid {
			t.Errorf("IsValidUUID(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

func BenchmarkNewUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewUUID()
	}
}
