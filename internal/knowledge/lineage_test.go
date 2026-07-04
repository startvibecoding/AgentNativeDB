package knowledge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func setupLineageTracker(t *testing.T) *LineageTracker {
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
	return NewLineageTracker(engine)
}

func TestLineageTracker_RecordAndGet(t *testing.T) {
	lt := setupLineageTracker(t)
	ctx := context.Background()

	lineage := &Lineage{
		DataID:     "data-001",
		SourceType: "raw",
		SourceIDs:  []string{"src-1", "src-2"},
		Steps: []Step{
			{Type: "transform", Operation: "filter", Timestamp: now()},
			{Type: "aggregate", Operation: "sum", Timestamp: now()},
		},
		DecisionIDs: []string{"dec-001"},
	}

	if err := lt.Record(ctx, lineage); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := lt.Get(ctx, "data-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.DataID != "data-001" {
		t.Fatalf("expected data-001, got %s", got.DataID)
	}
	if got.SourceType != "raw" {
		t.Fatalf("expected raw, got %s", got.SourceType)
	}
	if len(got.SourceIDs) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(got.SourceIDs))
	}
	if len(got.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(got.Steps))
	}
}

func TestLineageTracker_UpstreamTrace(t *testing.T) {
	lt := setupLineageTracker(t)
	ctx := context.Background()

	// 原始数据
	lt.Record(ctx, &Lineage{DataID: "raw-1", SourceType: "raw"})
	lt.Record(ctx, &Lineage{DataID: "raw-2", SourceType: "raw"})

	// 中间数据（来自 raw-1 和 raw-2）
	lt.Record(ctx, &Lineage{
		DataID:     "derived-1",
		SourceType: "derived",
		SourceIDs:  []string{"raw-1", "raw-2"},
	})

	// 最终数据（来自 derived-1）
	lt.Record(ctx, &Lineage{
		DataID:     "final-1",
		SourceType: "agent_generated",
		SourceIDs:  []string{"derived-1"},
	})

	// 追溯 2 层
	tree, err := lt.TraceUpstream(ctx, "final-1", 2)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}

	if tree.Lineage.DataID != "final-1" {
		t.Fatalf("expected final-1, got %s", tree.Lineage.DataID)
	}

	if len(tree.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(tree.Sources))
	}

	if tree.Sources[0].Lineage.DataID != "derived-1" {
		t.Fatalf("expected derived-1, got %s", tree.Sources[0].Lineage.DataID)
	}

	// 深度
	if tree.Depth() != 3 { // final-1 -> derived-1 -> raw-1/raw-2
		t.Fatalf("expected depth 3, got %d", tree.Depth())
	}

	// 所有 ID
	ids := tree.AllIDs()
	if len(ids) != 4 { // final-1, derived-1, raw-1, raw-2
		t.Fatalf("expected 4 IDs, got %d: %v", len(ids), ids)
	}
}

func TestLineageTracker_ListByDecision(t *testing.T) {
	lt := setupLineageTracker(t)
	ctx := context.Background()

	lt.Record(ctx, &Lineage{DataID: "d1", DecisionIDs: []string{"dec-001", "dec-002"}})
	lt.Record(ctx, &Lineage{DataID: "d2", DecisionIDs: []string{"dec-003"}})
	lt.Record(ctx, &Lineage{DataID: "d3", DecisionIDs: []string{"dec-001"}})

	results, _ := lt.ListByDecision(ctx, "dec-001")
	if len(results) != 2 {
		t.Fatalf("expected 2 results for dec-001, got %d", len(results))
	}
}

func now() time.Time {
	return time.Now()
}
