package sql

import (
	"testing"
	"time"
)

func TestQueryStats_Record(t *testing.T) {
	qs := NewQueryStats(100, 50)

	qs.Record(QueryRecord{SQL: "SELECT * FROM t", DurationMs: 10, RowsReturned: 5, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "SELECT * FROM t WHERE id = 1", DurationMs: 200, RowsReturned: 1, Timestamp: time.Now()})

	summary := qs.GetSummary()
	if summary.TotalQueries != 2 {
		t.Fatalf("expected 2 queries, got %d", summary.TotalQueries)
	}
	if summary.SlowQueries != 1 {
		t.Fatalf("expected 1 slow query, got %d", summary.SlowQueries)
	}
}

func TestQueryStats_SlowQueries(t *testing.T) {
	qs := NewQueryStats(100, 100)

	qs.Record(QueryRecord{SQL: "fast", DurationMs: 10, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "slow1", DurationMs: 150, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "slow2", DurationMs: 300, Timestamp: time.Now()})

	slow := qs.GetSlowQueries(10)
	if len(slow) != 2 {
		t.Fatalf("expected 2 slow queries, got %d", len(slow))
	}
	// 按耗时降序
	if slow[0].DurationMs < slow[1].DurationMs {
		t.Fatal("expected descending order")
	}
}

func TestQueryStats_TopQueries(t *testing.T) {
	qs := NewQueryStats(100, 1000)

	// 同一模式重复多次
	for i := 0; i < 5; i++ {
		qs.Record(QueryRecord{SQL: "SELECT * FROM users WHERE id = 1", DurationMs: 10, Timestamp: time.Now()})
	}
	for i := 0; i < 3; i++ {
		qs.Record(QueryRecord{SQL: "SELECT * FROM orders WHERE id = 2", DurationMs: 20, Timestamp: time.Now()})
	}

	top := qs.GetTopQueries(10)
	if len(top) < 2 {
		t.Fatalf("expected at least 2 patterns, got %d", len(top))
	}

	// 最频繁的应该是归一化后的模式
	if top[0].Count < top[1].Count {
		t.Fatal("expected first pattern to have more occurrences")
	}
}

func TestNormalizeSQL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT * FROM t WHERE id = 1", "SELECT * FROM t WHERE id = ?"},
		{"SELECT * FROM t WHERE name = 'hello'", "SELECT * FROM t WHERE name = ?"},
		{"INSERT INTO t (id) VALUES (42)", "INSERT INTO t (id) VALUES (?)"},
	}

	for _, tt := range tests {
		got := normalizeSQL(tt.input)
		if got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestQueryStats_Summary(t *testing.T) {
	qs := NewQueryStats(100, 100)

	qs.Record(QueryRecord{SQL: "a", DurationMs: 10, RowsReturned: 5, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "b", DurationMs: 30, RowsReturned: 10, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "c", DurationMs: 50, RowsReturned: 15, Timestamp: time.Now()})

	summary := qs.GetSummary()
	if summary.TotalQueries != 3 {
		t.Fatalf("expected 3, got %d", summary.TotalQueries)
	}
	if summary.AvgDurationMs != 30 {
		t.Fatalf("expected avg 30, got %d", summary.AvgDurationMs)
	}
	if summary.MaxDurationMs != 50 {
		t.Fatalf("expected max 50, got %d", summary.MaxDurationMs)
	}
	if summary.MinDurationMs != 10 {
		t.Fatalf("expected min 10, got %d", summary.MinDurationMs)
	}
	if summary.TotalRows != 30 {
		t.Fatalf("expected 30 rows, got %d", summary.TotalRows)
	}
}

func TestQueryStats_Eviction(t *testing.T) {
	qs := NewQueryStats(3, 1000) // 最多 3 条

	qs.Record(QueryRecord{SQL: "q1", DurationMs: 10, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "q2", DurationMs: 20, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "q3", DurationMs: 30, Timestamp: time.Now()})
	qs.Record(QueryRecord{SQL: "q4", DurationMs: 40, Timestamp: time.Now()}) // 应该淘汰 q1

	summary := qs.GetSummary()
	if summary.TotalQueries != 3 {
		t.Fatalf("expected 3 after eviction, got %d", summary.TotalQueries)
	}
}

func BenchmarkQueryStats_Record(b *testing.B) {
	qs := NewQueryStats(10000, 100)
	qr := QueryRecord{SQL: "SELECT * FROM t WHERE id = ?", DurationMs: 10, RowsReturned: 1, Timestamp: time.Now()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qs.Record(qr)
	}
}
