package sql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func setupIndexExecutor(t *testing.T) *Executor {
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
	e := NewExecutor(engine)
	if err := e.Init(context.Background()); err != nil {
		t.Fatalf("init executor: %v", err)
	}
	return e
}

func runSQL(t *testing.T, e *Executor, sql string) *Result {
	t.Helper()
	stmt, err := Parse(sql)
	if err != nil {
		t.Fatalf("parse %q: %v", sql, err)
	}
	plan, err := e.Planner().Plan(stmt)
	if err != nil {
		t.Fatalf("plan %q: %v", sql, err)
	}
	res, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute %q: %v", sql, err)
	}
	return res
}

func TestIndex_HashEqual(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64), age INT)")
	runSQL(t, e, "CREATE INDEX idx_users_name ON users(name) USING HASH")

	runSQL(t, e, "INSERT INTO users (id, name, age) VALUES ('u1', 'alice', 30)")
	runSQL(t, e, "INSERT INTO users (id, name, age) VALUES ('u2', 'bob', 25)")
	runSQL(t, e, "INSERT INTO users (id, name, age) VALUES ('u3', 'alice', 40)")

	// 使用 Planner 走 IndexScan
	stmt, _ := Parse("SELECT * FROM users WHERE name = 'alice'")
	plan, _ := e.Planner().Plan(stmt)
	if !containsIndexScan(plan) {
		t.Fatalf("expected IndexScan in plan, got %s", plan)
	}
	res := runSQL(t, e, "SELECT * FROM users WHERE name = 'alice'")
	if len(res.Rows) != 2 {
		t.Errorf("expected 2 alice rows, got %d", len(res.Rows))
	}

	// SHOW INDEXES
	shown := runSQL(t, e, "SHOW INDEXES FROM users")
	if len(shown.Rows) != 1 {
		t.Errorf("expected 1 index, got %d", len(shown.Rows))
	}

	// DROP INDEX
	runSQL(t, e, "DROP INDEX idx_users_name")
	shown2 := runSQL(t, e, "SHOW INDEXES FROM users")
	if len(shown2.Rows) != 0 {
		t.Errorf("expected 0 indexes after drop, got %d", len(shown2.Rows))
	}
}

func TestIndex_BTreeRange(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE items (id VARCHAR(64) PRIMARY KEY, price FLOAT)")
	runSQL(t, e, "CREATE INDEX idx_items_price ON items(price) USING BTREE")

	runSQL(t, e, "INSERT INTO items (id, price) VALUES ('a', 10.0)")
	runSQL(t, e, "INSERT INTO items (id, price) VALUES ('b', 20.0)")
	runSQL(t, e, "INSERT INTO items (id, price) VALUES ('c', 30.0)")
	runSQL(t, e, "INSERT INTO items (id, price) VALUES ('d', 40.0)")

	stmt, _ := Parse("SELECT * FROM items WHERE price >= 20.0 AND price < 40.0")
	plan, _ := e.Planner().Plan(stmt)
	if !containsIndexScan(plan) {
		t.Fatalf("expected IndexScan in plan, got %s", plan)
	}

	res := runSQL(t, e, "SELECT * FROM items WHERE price >= 20.0 AND price < 40.0")
	if len(res.Rows) != 2 {
		t.Errorf("expected 2 rows in range, got %d", len(res.Rows))
	}

	res2 := runSQL(t, e, "SELECT * FROM items WHERE price BETWEEN 15.0 AND 35.0")
	if len(res2.Rows) != 2 {
		t.Errorf("expected 2 rows in between, got %d", len(res2.Rows))
	}
}

func TestIndex_Inverted_Match(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT)")
	runSQL(t, e, "CREATE INDEX idx_docs_body ON docs(body) USING INVERTED")

	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d1', 'Hello world of databases')")
	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d2', 'Vector search is fun')")
	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d3', 'World of agents')")

	stmt, _ := Parse("SELECT * FROM docs WHERE MATCH(body) AGAINST ('world')")
	plan, _ := e.Planner().Plan(stmt)
	if !containsIndexScan(plan) {
		t.Fatalf("expected IndexScan in plan, got %s", plan)
	}
	res := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('world')")
	if len(res.Rows) != 2 {
		t.Errorf("expected 2 rows matching 'world', got %d", len(res.Rows))
	}

	// AND 语义：world + agents 只命中 d3
	res2 := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('world agents')")
	if len(res2.Rows) != 1 {
		t.Errorf("expected 1 row matching 'world agents', got %d", len(res2.Rows))
	}
}

func TestIndex_FullTextAlias_Match(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT)")
	runSQL(t, e, "CREATE FULLTEXT INDEX idx_docs_body ON docs(body)")

	shown := runSQL(t, e, "SHOW INDEXES FROM docs")
	if len(shown.Rows) != 1 {
		t.Fatalf("expected 1 index, got %d", len(shown.Rows))
	}
	if got := shown.Rows[0].Values["type"]; got != "INVERTED" {
		t.Fatalf("expected FULLTEXT alias to be stored as INVERTED, got %v", got)
	}

	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d1', 'Agent native full text search')")
	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d2', 'Vector search')")

	stmt, _ := Parse("SELECT * FROM docs WHERE MATCH(body) AGAINST ('full text')")
	plan, _ := e.Planner().Plan(stmt)
	if !containsIndexScan(plan) {
		t.Fatalf("expected IndexScan in plan, got %s", plan)
	}
	res := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('full text')")
	if len(res.Rows) != 1 {
		t.Errorf("expected 1 row matching full text, got %d", len(res.Rows))
	}
}

func TestIndex_UsingFullTextAlias_Match(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT)")
	runSQL(t, e, "CREATE INDEX idx_docs_body ON docs(body) USING FULLTEXT")

	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d1', '数据库 支持 全文 搜索')")
	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d2', '向量 检索')")

	res := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('全文 搜索')")
	if len(res.Rows) != 1 {
		t.Errorf("expected 1 row matching 全文 搜索, got %d", len(res.Rows))
	}
}

func TestIndex_Inverted_MatchChineseSubstring(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE docs (id VARCHAR(64) PRIMARY KEY, body TEXT)")
	runSQL(t, e, "CREATE INDEX idx_docs_body ON docs(body) USING INVERTED")

	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d1', '数据库系统支持向量检索和全文搜索')")
	runSQL(t, e, "INSERT INTO docs (id, body) VALUES ('d2', '图数据库支持最短路径查询')")

	res := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('全文搜索')")
	if len(res.Rows) != 1 {
		t.Errorf("expected 1 row matching 全文搜索, got %d", len(res.Rows))
	}

	res2 := runSQL(t, e, "SELECT * FROM docs WHERE MATCH(body) AGAINST ('向量检索')")
	if len(res2.Rows) != 1 {
		t.Errorf("expected 1 row matching 向量检索, got %d", len(res2.Rows))
	}
}

func TestIndex_UpdateDeleteMaintain(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64))")
	runSQL(t, e, "CREATE INDEX idx_users_name ON users(name) USING HASH")

	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'alice')")
	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u2', 'bob')")

	// UPDATE 后旧值不应命中，新值应命中
	runSQL(t, e, "UPDATE users SET name = 'carol' WHERE id = 'u1'")

	res := runSQL(t, e, "SELECT * FROM users WHERE name = 'alice'")
	if len(res.Rows) != 0 {
		t.Errorf("expected 0 alice rows after update, got %d", len(res.Rows))
	}
	res2 := runSQL(t, e, "SELECT * FROM users WHERE name = 'carol'")
	if len(res2.Rows) != 1 {
		t.Errorf("expected 1 carol row, got %d", len(res2.Rows))
	}

	// DELETE 后
	runSQL(t, e, "DELETE FROM users WHERE name = 'bob'")
	res3 := runSQL(t, e, "SELECT * FROM users WHERE name = 'bob'")
	if len(res3.Rows) != 0 {
		t.Errorf("expected 0 bob rows after delete, got %d", len(res3.Rows))
	}
}

func TestIndex_Backfill(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64))")
	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'alice')")
	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u2', 'bob')")

	// 索引创建在数据之后：应回填
	runSQL(t, e, "CREATE INDEX idx_users_name ON users(name) USING BTREE")

	res := runSQL(t, e, "SELECT * FROM users WHERE name = 'alice'")
	if len(res.Rows) != 1 {
		t.Errorf("expected 1 alice row after backfill, got %d", len(res.Rows))
	}
}

func TestIndex_UniqueRejectsDuplicateInsert(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, email VARCHAR(128))")
	runSQL(t, e, "CREATE UNIQUE INDEX idx_users_email ON users(email) USING HASH")
	runSQL(t, e, "INSERT INTO users (id, email) VALUES ('u1', 'a@example.com')")

	stmt, err := Parse("INSERT INTO users (id, email) VALUES ('u2', 'a@example.com')")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.Planner().Plan(stmt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(context.Background(), plan); err == nil {
		t.Fatal("expected duplicate unique index insert to fail")
	}

	res := runSQL(t, e, "SELECT * FROM users WHERE email = 'a@example.com'")
	if len(res.Rows) != 1 {
		t.Fatalf("expected original row only, got %d", len(res.Rows))
	}
}

func TestIndex_UniqueRejectsDuplicateUpdate(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, email VARCHAR(128))")
	runSQL(t, e, "CREATE UNIQUE INDEX idx_users_email ON users(email) USING BTREE")
	runSQL(t, e, "INSERT INTO users (id, email) VALUES ('u1', 'a@example.com')")
	runSQL(t, e, "INSERT INTO users (id, email) VALUES ('u2', 'b@example.com')")

	stmt, err := Parse("UPDATE users SET email = 'a@example.com' WHERE id = 'u2'")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.Planner().Plan(stmt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(context.Background(), plan); err == nil {
		t.Fatal("expected duplicate unique index update to fail")
	}

	res := runSQL(t, e, "SELECT * FROM users WHERE email = 'b@example.com'")
	if len(res.Rows) != 1 {
		t.Fatalf("expected u2 to keep original email, got %d rows", len(res.Rows))
	}
}

func TestIndex_UniqueBackfillRejectsDuplicates(t *testing.T) {
	e := setupIndexExecutor(t)

	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, email VARCHAR(128))")
	runSQL(t, e, "INSERT INTO users (id, email) VALUES ('u1', 'a@example.com')")
	runSQL(t, e, "INSERT INTO users (id, email) VALUES ('u2', 'a@example.com')")

	stmt, err := Parse("CREATE UNIQUE INDEX idx_users_email ON users(email) USING HASH")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.Planner().Plan(stmt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(context.Background(), plan); err == nil {
		t.Fatal("expected unique index backfill to fail")
	}

	shown := runSQL(t, e, "SHOW INDEXES FROM users")
	if len(shown.Rows) != 0 {
		t.Fatalf("expected failed index creation to be rolled back, got %d indexes", len(shown.Rows))
	}
}

// containsIndexScan 递归判断计划树中是否含 IndexScanNode
func containsIndexScan(p PlanNode) bool {
	if p == nil {
		return false
	}
	if _, ok := p.(*IndexScanNode); ok {
		return true
	}
	// 通过 String() 简单粗查 + 反射式遍历（避免为每个节点写案例）
	// 用类型断言处理已知带 Input 的节点
	type withInput interface{ input() PlanNode }
	_ = withInput(nil)

	switch n := p.(type) {
	case *FilterNode:
		return containsIndexScan(n.Input)
	case *ProjectNode:
		return containsIndexScan(n.Input)
	case *SortNode:
		return containsIndexScan(n.Input)
	case *LimitNode:
		return containsIndexScan(n.Input)
	case *AggregateNode:
		return containsIndexScan(n.Input)
	case *JoinNode:
		return containsIndexScan(n.Left) || containsIndexScan(n.Right)
	}
	return strings.Contains(p.String(), "IndexScan")
}
