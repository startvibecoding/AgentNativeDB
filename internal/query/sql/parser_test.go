package sql

import (
	"testing"
)

func TestParser_SelectStar(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s, ok := stmt.(*SelectStmt)
	if !ok {
		t.Fatalf("expected *SelectStmt, got %T", stmt)
	}
	if s.From == nil || s.From.Name != "users" {
		t.Fatalf("expected FROM users, got %v", s.From)
	}
	if len(s.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(s.Columns))
	}
	if _, ok := s.Columns[0].Expr.(*StarExpr); !ok {
		t.Fatalf("expected StarExpr, got %T", s.Columns[0].Expr)
	}
}

func TestParser_SelectColumns(t *testing.T) {
	stmt, err := Parse("SELECT id, name, age FROM users")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(s.Columns))
	}

	expectedNames := []string{"id", "name", "age"}
	for i, name := range expectedNames {
		ident, ok := s.Columns[i].Expr.(*IdentifierExpr)
		if !ok {
			t.Fatalf("column %d: expected IdentifierExpr, got %T", i, s.Columns[i].Expr)
		}
		if ident.Name != name {
			t.Fatalf("column %d: expected %q, got %q", i, name, ident.Name)
		}
	}
}

func TestParser_SelectWithAlias(t *testing.T) {
	stmt, err := Parse("SELECT name AS username FROM users u")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if s.Columns[0].Alias != "username" {
		t.Fatalf("expected alias 'username', got %q", s.Columns[0].Alias)
	}
	if s.From.Alias != "u" {
		t.Fatalf("expected table alias 'u', got %q", s.From.Alias)
	}
}

func TestParser_Where(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users WHERE age > 18 AND status = 'active'")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if s.Where == nil {
		t.Fatal("expected WHERE clause")
	}

	bin, ok := s.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", s.Where)
	}
	if bin.Op != TOKEN_AND {
		t.Fatalf("expected AND, got %s", bin.Op)
	}
}

func TestParser_OrderBy(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users ORDER BY name ASC, age DESC")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.OrderBy) != 2 {
		t.Fatalf("expected 2 ORDER BY, got %d", len(s.OrderBy))
	}
	if !s.OrderBy[0].Ascending {
		t.Fatal("expected first to be ASC")
	}
	if s.OrderBy[1].Ascending {
		t.Fatal("expected second to be DESC")
	}
}

func TestParser_LimitOffset(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users LIMIT 10 OFFSET 20")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if s.Limit == nil || *s.Limit != 10 {
		t.Fatalf("expected LIMIT 10, got %v", s.Limit)
	}
	if s.Offset == nil || *s.Offset != 20 {
		t.Fatalf("expected OFFSET 20, got %v", s.Offset)
	}
}

func TestParser_GroupBy(t *testing.T) {
	stmt, err := Parse("SELECT department, COUNT(*) FROM employees GROUP BY department")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.GroupBy) != 1 {
		t.Fatalf("expected 1 GROUP BY, got %d", len(s.GroupBy))
	}
}

func TestParser_AggregateFunctions(t *testing.T) {
	tests := []struct {
		input string
		name  string
	}{
		{"SELECT COUNT(*) FROM t", "COUNT"},
		{"SELECT SUM(amount) FROM t", "SUM"},
		{"SELECT MIN(price) FROM t", "MIN"},
		{"SELECT MAX(price) FROM t", "MAX"},
		{"SELECT AVG(score) FROM t", "AVG"},
	}

	for _, tt := range tests {
		stmt, err := Parse(tt.input)
		if err != nil {
			t.Errorf("parse %q: %v", tt.input, err)
			continue
		}
		s := stmt.(*SelectStmt)
		funcExpr, ok := s.Columns[0].Expr.(*FuncCallExpr)
		if !ok {
			t.Errorf("%q: expected FuncCallExpr, got %T", tt.input, s.Columns[0].Expr)
			continue
		}
		if funcExpr.Name != tt.name {
			t.Errorf("%q: expected %s, got %s", tt.input, tt.name, funcExpr.Name)
		}
	}
}

func TestParser_Insert(t *testing.T) {
	stmt, err := Parse("INSERT INTO users (name, age) VALUES ('Alice', 30), ('Bob', 25)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	ins, ok := stmt.(*InsertStmt)
	if !ok {
		t.Fatalf("expected *InsertStmt, got %T", stmt)
	}
	if ins.Table != "users" {
		t.Fatalf("expected table 'users', got %q", ins.Table)
	}
	if len(ins.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(ins.Columns))
	}
	if len(ins.Values) != 2 {
		t.Fatalf("expected 2 value rows, got %d", len(ins.Values))
	}
}

func TestParser_Update(t *testing.T) {
	stmt, err := Parse("UPDATE users SET name = 'Charlie', age = 35 WHERE id = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	upd, ok := stmt.(*UpdateStmt)
	if !ok {
		t.Fatalf("expected *UpdateStmt, got %T", stmt)
	}
	if upd.Table != "users" {
		t.Fatalf("expected table 'users', got %q", upd.Table)
	}
	if len(upd.Set) != 2 {
		t.Fatalf("expected 2 SET clauses, got %d", len(upd.Set))
	}
	if upd.Where == nil {
		t.Fatal("expected WHERE clause")
	}
}

func TestParser_Delete(t *testing.T) {
	stmt, err := Parse("DELETE FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	del, ok := stmt.(*DeleteStmt)
	if !ok {
		t.Fatalf("expected *DeleteStmt, got %T", stmt)
	}
	if del.Table != "users" {
		t.Fatalf("expected table 'users', got %q", del.Table)
	}
	if del.Where == nil {
		t.Fatal("expected WHERE clause")
	}
}

func TestParser_ComplexWhere(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE (a > 1 OR b < 2) AND c = 'x'")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if s.Where == nil {
		t.Fatal("expected WHERE")
	}

	// 应该是 AND
	bin, ok := s.Where.(*BinaryExpr)
	if !ok || bin.Op != TOKEN_AND {
		t.Fatalf("expected top-level AND, got %T %s", s.Where, bin.Op)
	}
}

func TestParser_InExpression(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE id IN (1, 2, 3)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	inExpr, ok := s.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected InExpr, got %T", s.Where)
	}
	if len(inExpr.Values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(inExpr.Values))
	}
}

func TestParser_BetweenExpression(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE age BETWEEN 18 AND 65")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	between, ok := s.Where.(*BetweenExpr)
	if !ok {
		t.Fatalf("expected BetweenExpr, got %T", s.Where)
	}
	if between.Not {
		t.Fatal("expected NOT=false")
	}
}

func TestParser_NotInExpression(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE id NOT IN (1, 2)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	inExpr, ok := s.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected InExpr, got %T", s.Where)
	}
	if !inExpr.Not {
		t.Fatal("expected NOT=true")
	}
}

func TestParser_IsNull(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE name IS NULL")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	isNull, ok := s.Where.(*IsNullExpr)
	if !ok {
		t.Fatalf("expected IsNullExpr, got %T", s.Where)
	}
	if isNull.Not {
		t.Fatal("expected NOT=false")
	}
}

func TestParser_IsNotNull(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE name IS NOT NULL")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	isNull, ok := s.Where.(*IsNullExpr)
	if !ok {
		t.Fatalf("expected IsNullExpr, got %T", s.Where)
	}
	if !isNull.Not {
		t.Fatal("expected NOT=true")
	}
}

func TestParser_TableDotColumn(t *testing.T) {
	stmt, err := Parse("SELECT u.name, u.age FROM users u WHERE u.id = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	ident, ok := s.Columns[0].Expr.(*IdentifierExpr)
	if !ok {
		t.Fatalf("expected IdentifierExpr, got %T", s.Columns[0].Expr)
	}
	if ident.Table != "u" || ident.Name != "name" {
		t.Fatalf("expected u.name, got %s.%s", ident.Table, ident.Name)
	}
}

func TestParser_NegativeNumber(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE balance > -100.5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	bin, ok := s.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", s.Where)
	}

	_, ok = bin.Right.(*UnaryExpr)
	if !ok {
		// 可能被解析为 float
		_, ok2 := bin.Right.(*FloatLiteralExpr)
		if !ok2 {
			t.Fatalf("expected negative number expression, got %T", bin.Right)
		}
	}
}

func TestParser_Errors(t *testing.T) {
	tests := []string{
		"",
		"SELECT",
		"SELECT * FROM",
		"INSERT INTO",
		"UPDATE SET",
		"DELETE",
	}

	for _, input := range tests {
		_, err := Parse(input)
		if err == nil {
			t.Errorf("expected error for %q, got nil", input)
		}
	}
}

func BenchmarkParser(b *testing.B) {
	input := `SELECT u.name, COUNT(*) as cnt FROM users u WHERE u.age > 18 AND u.status = 'active' GROUP BY u.name ORDER BY cnt DESC LIMIT 10`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(input)
	}
}
