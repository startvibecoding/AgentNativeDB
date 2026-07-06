package graph

import (
	"context"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func setupGraphStore(t *testing.T) *GraphStore {
	t.Helper()
	engine := storage.NewTestEngine(t)
	return NewGraphStore(engine)
}

func TestGraphStore_NodeCRUD(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	// 添加节点
	node := &Node{ID: "n1", Type: "Person", Name: "Alice", Properties: map[string]any{"age": 30}}
	if err := g.AddNode(ctx, node); err != nil {
		t.Fatalf("add node: %v", err)
	}

	// 获取节点
	got, err := g.GetNode(ctx, "n1")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.Name != "Alice" {
		t.Fatalf("expected Alice, got %s", got.Name)
	}
	if got.Type != "Person" {
		t.Fatalf("expected Person, got %s", got.Type)
	}

	// 删除节点
	g.DeleteNode(ctx, "n1")
	_, err = g.GetNode(ctx, "n1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestGraphStore_EdgeCRUD(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	g.AddNode(ctx, &Node{ID: "a", Type: "Person", Name: "Alice"})
	g.AddNode(ctx, &Node{ID: "b", Type: "Person", Name: "Bob"})

	// 添加边
	edge := &Edge{ID: "e1", Type: "KNOWS", FromID: "a", ToID: "b", Weight: 0.8}
	if err := g.AddEdge(ctx, edge); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	// 获取边
	got, err := g.GetEdge(ctx, "e1")
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if got.Type != "KNOWS" {
		t.Fatalf("expected KNOWS, got %s", got.Type)
	}
	if got.FromID != "a" || got.ToID != "b" {
		t.Fatalf("expected a->b, got %s->%s", got.FromID, got.ToID)
	}

	// 删除边
	g.DeleteEdge(ctx, "e1")
	_, err = g.GetEdge(ctx, "e1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestGraphStore_Neighbors(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	// 构建: Alice -> Bob -> Charlie
	g.AddNode(ctx, &Node{ID: "a", Type: "Person", Name: "Alice"})
	g.AddNode(ctx, &Node{ID: "b", Type: "Person", Name: "Bob"})
	g.AddNode(ctx, &Node{ID: "c", Type: "Person", Name: "Charlie"})
	g.AddEdge(ctx, &Edge{ID: "e1", Type: "KNOWS", FromID: "a", ToID: "b"})
	g.AddEdge(ctx, &Edge{ID: "e2", Type: "KNOWS", FromID: "b", ToID: "c"})

	// Alice 的出边邻居 = Bob
	outNeighbors, _ := g.GetNeighbors(ctx, "a", DirOut)
	if len(outNeighbors) != 1 || outNeighbors[0].Name != "Bob" {
		t.Fatalf("expected [Bob], got %v", outNeighbors)
	}

	// Bob 的入边邻居 = Alice
	inNeighbors, _ := g.GetNeighbors(ctx, "b", DirIn)
	if len(inNeighbors) != 1 || inNeighbors[0].Name != "Alice" {
		t.Fatalf("expected [Alice], got %v", inNeighbors)
	}

	// Bob 的双向邻居 = Alice, Charlie
	bothNeighbors, _ := g.GetNeighbors(ctx, "b", DirBoth)
	if len(bothNeighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(bothNeighbors))
	}
}

func TestGraphStore_KHopNeighbors(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	// 构建: a -> b -> c -> d
	g.AddNode(ctx, &Node{ID: "a", Type: "X", Name: "A"})
	g.AddNode(ctx, &Node{ID: "b", Type: "X", Name: "B"})
	g.AddNode(ctx, &Node{ID: "c", Type: "X", Name: "C"})
	g.AddNode(ctx, &Node{ID: "d", Type: "X", Name: "D"})
	g.AddEdge(ctx, &Edge{ID: "e1", Type: "R", FromID: "a", ToID: "b"})
	g.AddEdge(ctx, &Edge{ID: "e2", Type: "R", FromID: "b", ToID: "c"})
	g.AddEdge(ctx, &Edge{ID: "e3", Type: "R", FromID: "c", ToID: "d"})

	// 1 跳 = [b]
	oneHop, _ := g.KHopNeighbors(ctx, "a", 1, DirOut)
	if len(oneHop) != 1 || oneHop[0].ID != "b" {
		t.Fatalf("1-hop: expected [b], got %v", oneHop)
	}

	// 2 跳 = [c]
	twoHop, _ := g.KHopNeighbors(ctx, "a", 2, DirOut)
	if len(twoHop) != 1 || twoHop[0].ID != "c" {
		t.Fatalf("2-hop: expected [c], got %v", twoHop)
	}

	// 3 跳 = [d]
	threeHop, _ := g.KHopNeighbors(ctx, "a", 3, DirOut)
	if len(threeHop) != 1 || threeHop[0].ID != "d" {
		t.Fatalf("3-hop: expected [d], got %v", threeHop)
	}
}

func TestGraphStore_ShortestPath(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	// 构建: a -> b -> c -> d, a -> d (直达)
	g.AddNode(ctx, &Node{ID: "a", Type: "X", Name: "A"})
	g.AddNode(ctx, &Node{ID: "b", Type: "X", Name: "B"})
	g.AddNode(ctx, &Node{ID: "c", Type: "X", Name: "C"})
	g.AddNode(ctx, &Node{ID: "d", Type: "X", Name: "D"})
	g.AddEdge(ctx, &Edge{ID: "e1", Type: "R", FromID: "a", ToID: "b"})
	g.AddEdge(ctx, &Edge{ID: "e2", Type: "R", FromID: "b", ToID: "c"})
	g.AddEdge(ctx, &Edge{ID: "e3", Type: "R", FromID: "c", ToID: "d"})
	g.AddEdge(ctx, &Edge{ID: "e4", Type: "R", FromID: "a", ToID: "d"})

	// 最短路径 a -> d 应该是直达 [a, d]
	path, err := g.ShortestPath(ctx, "a", "d", DirOut)
	if err != nil {
		t.Fatalf("shortest path: %v", err)
	}
	if len(path) != 2 || path[0] != "a" || path[1] != "d" {
		t.Fatalf("expected [a, d], got %v", path)
	}

	// 最短路径 a -> c 应该是 [a, b, c]（因为 a->d 不经过 c）
	path, err = g.ShortestPath(ctx, "a", "c", DirOut)
	if err != nil {
		t.Fatalf("shortest path a->c: %v", err)
	}
	if len(path) != 3 || path[0] != "a" || path[1] != "b" || path[2] != "c" {
		t.Fatalf("expected [a, b, c], got %v", path)
	}
}

func TestGraphStore_FindByType(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	g.AddNode(ctx, &Node{ID: "a", Type: "Person", Name: "Alice"})
	g.AddNode(ctx, &Node{ID: "b", Type: "Person", Name: "Bob"})
	g.AddNode(ctx, &Node{ID: "c", Type: "Device", Name: "Sensor-1"})

	persons, _ := g.FindNodesByType(ctx, "Person", 0)
	if len(persons) != 2 {
		t.Fatalf("expected 2 persons, got %d", len(persons))
	}

	devices, _ := g.FindNodesByType(ctx, "Device", 0)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
}

func TestGraphStore_FindByEdgeType(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	g.AddNode(ctx, &Node{ID: "a", Type: "X", Name: "A"})
	g.AddNode(ctx, &Node{ID: "b", Type: "X", Name: "B"})
	g.AddNode(ctx, &Node{ID: "c", Type: "X", Name: "C"})
	g.AddEdge(ctx, &Edge{ID: "e1", Type: "KNOWS", FromID: "a", ToID: "b"})
	g.AddEdge(ctx, &Edge{ID: "e2", Type: "OWNS", FromID: "a", ToID: "c"})

	knows, _ := g.FindEdgesByType(ctx, "KNOWS", 0)
	if len(knows) != 1 {
		t.Fatalf("expected 1 KNOWS edge, got %d", len(knows))
	}
}

func TestGraphStore_DeleteNodeCascadesEdges(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	g.AddNode(ctx, &Node{ID: "a", Type: "X", Name: "A"})
	g.AddNode(ctx, &Node{ID: "b", Type: "X", Name: "B"})
	g.AddEdge(ctx, &Edge{ID: "e1", Type: "R", FromID: "a", ToID: "b"})

	// 删除 a 应该级联删除 e1
	g.DeleteNode(ctx, "a")

	_, err := g.GetEdge(ctx, "e1")
	if err == nil {
		t.Fatal("expected edge to be deleted with node")
	}
}

func TestGraphStore_ComplexGraph(t *testing.T) {
	g := setupGraphStore(t)
	ctx := context.Background()

	// 构建知识图谱: 组织结构
	g.AddNode(ctx, &Node{ID: "company", Type: "Organization", Name: "Acme Corp"})
	g.AddNode(ctx, &Node{ID: "dept1", Type: "Department", Name: "Engineering"})
	g.AddNode(ctx, &Node{ID: "dept2", Type: "Department", Name: "Sales"})
	g.AddNode(ctx, &Node{ID: "emp1", Type: "Employee", Name: "Alice"})
	g.AddNode(ctx, &Node{ID: "emp2", Type: "Employee", Name: "Bob"})
	g.AddNode(ctx, &Node{ID: "emp3", Type: "Employee", Name: "Charlie"})

	g.AddEdge(ctx, &Edge{ID: "e1", Type: "HAS_DEPT", FromID: "company", ToID: "dept1"})
	g.AddEdge(ctx, &Edge{ID: "e2", Type: "HAS_DEPT", FromID: "company", ToID: "dept2"})
	g.AddEdge(ctx, &Edge{ID: "e3", Type: "EMPLOYS", FromID: "dept1", ToID: "emp1"})
	g.AddEdge(ctx, &Edge{ID: "e4", Type: "EMPLOYS", FromID: "dept1", ToID: "emp2"})
	g.AddEdge(ctx, &Edge{ID: "e5", Type: "EMPLOYS", FromID: "dept2", ToID: "emp3"})
	g.AddEdge(ctx, &Edge{ID: "e6", Type: "REPORTS_TO", FromID: "emp1", ToID: "emp2"})

	// 公司的 2 跳邻居 = 所有员工
	twoHop, _ := g.KHopNeighbors(ctx, "company", 2, DirOut)
	if len(twoHop) != 3 {
		t.Fatalf("expected 3 employees at 2-hop, got %d", len(twoHop))
	}

	// 最短路径: emp3 -> company
	path, _ := g.ShortestPath(ctx, "emp3", "company", DirIn)
	if len(path) != 3 { // emp3 -> dept2 -> company
		t.Fatalf("expected path length 3, got %d: %v", len(path), path)
	}

	// 部门员工
	dept1Employees, _ := g.GetNeighbors(ctx, "dept1", DirOut)
	if len(dept1Employees) != 2 {
		t.Fatalf("expected 2 dept1 employees, got %d", len(dept1Employees))
	}
}

func BenchmarkGraphStore_AddNode(b *testing.B) {
	engine := storage.NewTestEngine(b)
	g := NewGraphStore(engine)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.AddNode(ctx, &Node{ID: "n" + string(rune(i)), Type: "X", Name: "Node"})
	}
}

func BenchmarkGraphStore_BFS(b *testing.B) {
	engine := storage.NewTestEngine(b)
	g := NewGraphStore(engine)
	ctx := context.Background()

	// 构建链式图: n0 -> n1 -> n2 -> ... -> n999
	g.AddNode(ctx, &Node{ID: "n0", Type: "X", Name: "Start"})
	for i := 1; i < 1000; i++ {
		g.AddNode(ctx, &Node{ID: "n" + itoa(i), Type: "X", Name: "N"})
		g.AddEdge(ctx, &Edge{ID: "e" + itoa(i), Type: "R", FromID: "n" + itoa(i-1), ToID: "n" + itoa(i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.ShortestPath(ctx, "n0", "n999", DirOut)
	}
}

func itoa(i int) string {
	s := ""
	if i == 0 {
		return "0"
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
