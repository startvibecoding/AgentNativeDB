package sql

import (
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// QueryStats 查询统计收集器
type QueryStats struct {
	mu       sync.RWMutex
	queries  []QueryRecord
	maxSize  int
	slowMs   int64 // 慢查询阈值（毫秒）
}

// NewQueryStats 创建查询统计
func NewQueryStats(maxSize int, slowThresholdMs int64) *QueryStats {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if slowThresholdMs <= 0 {
		slowThresholdMs = 100 // 100ms
	}
	return &QueryStats{
		queries: make([]QueryRecord, 0, maxSize),
		maxSize: maxSize,
		slowMs:  slowThresholdMs,
	}
}

// Record 记录查询
func (qs *QueryStats) Record(qr QueryRecord) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.queries) >= qs.maxSize {
		// 淘汰最旧的
		qs.queries = qs.queries[1:]
	}
	qs.queries = append(qs.queries, qr)

	// 慢查询日志
	if qr.DurationMs > qs.slowMs {
		slog.Warn("slow query",
			"sql", qr.SQL,
			"duration_ms", qr.DurationMs,
			"rows", qr.RowsReturned,
		)
	}
}

// GetSlowQueries 获取慢查询
func (qs *QueryStats) GetSlowQueries(limit int) []QueryRecord {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	var slow []QueryRecord
	for _, q := range qs.queries {
		if q.DurationMs > qs.slowMs {
			slow = append(slow, q)
		}
	}

	// 按耗时降序
	sort.Slice(slow, func(i, j int) bool {
		return slow[i].DurationMs > slow[j].DurationMs
	})

	if limit > 0 && len(slow) > limit {
		slow = slow[:limit]
	}
	return slow
}

// GetTopQueries 获取最频繁的查询
func (qs *QueryStats) GetTopQueries(limit int) []QueryPattern {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	patterns := make(map[string]*QueryPattern)
	for _, q := range qs.queries {
		pattern := normalizeSQL(q.SQL)
		if p, ok := patterns[pattern]; ok {
			p.Count++
			p.TotalMs += q.DurationMs
			if q.DurationMs > p.MaxMs {
				p.MaxMs = q.DurationMs
			}
		} else {
			patterns[pattern] = &QueryPattern{
				Pattern:  pattern,
				Count:    1,
				TotalMs:  q.DurationMs,
				MaxMs:    q.DurationMs,
				LastSeen: q.Timestamp,
			}
		}
	}

	var result []QueryPattern
	for _, p := range patterns {
		p.AvgMs = p.TotalMs / int64(p.Count)
		result = append(result, *p)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

// GetSummary 获取统计摘要
func (qs *QueryStats) GetSummary() StatsSummary {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if len(qs.queries) == 0 {
		return StatsSummary{}
	}

	var totalMs int64
	var maxMs int64
	var minMs int64 = qs.queries[0].DurationMs
	var totalRows int64
	var slowCount int

	for _, q := range qs.queries {
		totalMs += q.DurationMs
		totalRows += int64(q.RowsReturned)
		if q.DurationMs > maxMs {
			maxMs = q.DurationMs
		}
		if q.DurationMs < minMs {
			minMs = q.DurationMs
		}
		if q.DurationMs > qs.slowMs {
			slowCount++
		}
	}

	n := int64(len(qs.queries))
	return StatsSummary{
		TotalQueries: n,
		AvgDurationMs: totalMs / n,
		MaxDurationMs: maxMs,
		MinDurationMs: minMs,
		TotalRows:     totalRows,
		SlowQueries:   slowCount,
		SlowThresholdMs: qs.slowMs,
	}
}

// QueryRecord 单次查询记录
type QueryRecord struct {
	SQL         string    `json:"sql"`
	DurationMs  int64     `json:"duration_ms"`
	RowsReturned int      `json:"rows_returned"`
	Table       string    `json:"table,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// QueryPattern 查询模式
type QueryPattern struct {
	Pattern  string    `json:"pattern"`
	Count    int       `json:"count"`
	TotalMs  int64     `json:"total_ms"`
	AvgMs    int64     `json:"avg_ms"`
	MaxMs    int64     `json:"max_ms"`
	LastSeen time.Time `json:"last_seen"`
}

// StatsSummary 统计摘要
type StatsSummary struct {
	TotalQueries   int64 `json:"total_queries"`
	AvgDurationMs  int64 `json:"avg_duration_ms"`
	MaxDurationMs  int64 `json:"max_duration_ms"`
	MinDurationMs  int64 `json:"min_duration_ms"`
	TotalRows      int64 `json:"total_rows"`
	SlowQueries    int   `json:"slow_queries"`
	SlowThresholdMs int64 `json:"slow_threshold_ms"`
}

// normalizeSQL 将 SQL 归一化为模式（替换具体值为 ?）
func normalizeSQL(sql string) string {
	var result []byte
	inString := false
	stringChar := byte(0)
	lastWasPlaceholder := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if inString {
			if ch == stringChar {
				inString = false
			}
			continue
		}

		if ch == '\'' || ch == '"' {
			inString = true
			stringChar = ch
			if !lastWasPlaceholder {
				result = append(result, '?')
			}
			lastWasPlaceholder = true
			continue
		}

		if ch >= '0' && ch <= '9' {
			// 数字：只有在非标识符上下文中才替换
			if i == 0 || !(isAlphaChar(sql[i-1]) || sql[i-1] == '_') {
				if !lastWasPlaceholder {
					result = append(result, '?')
				}
				lastWasPlaceholder = true
				continue
			}
		}

		lastWasPlaceholder = false
		result = append(result, ch)
	}

	return string(result)
}

func isAlphaChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// ToJSON 序列化
func (qs *QueryStats) ToJSON() []byte {
	data, _ := json.Marshal(qs.GetSummary())
	return data
}
