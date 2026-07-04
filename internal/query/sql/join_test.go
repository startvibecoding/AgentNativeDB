package sql

import (
	"testing"
)

func TestParser_Join(t *testing.T) {
	stmt, err := Parse("SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Joins) != 1 {
		t.Fatalf("expected 1 join, got %d", len(s.Joins))
	}
	if s.Joins[0].Type != JoinInner {
		t.Fatalf("expected INNER join, got %s", s.Joins[0].Type)
	}
	if s.Joins[0].Table.Name != "orders" {
		t.Fatalf("expected orders table, got %s", s.Joins[0].Table.Name)
	}
	if s.Joins[0].On == nil {
		t.Fatal("expected ON clause")
	}
}

func TestParser_LeftJoin(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users u LEFT JOIN orders o ON u.id = o.user_id")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Joins) != 1 {
		t.Fatalf("expected 1 join, got %d", len(s.Joins))
	}
	if s.Joins[0].Type != JoinLeft {
		t.Fatalf("expected LEFT join, got %s", s.Joins[0].Type)
	}
}

func TestParser_MultipleJoins(t *testing.T) {
	stmt, err := Parse("SELECT * FROM a JOIN b ON a.id = b.a_id JOIN c ON b.id = c.b_id")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Joins) != 2 {
		t.Fatalf("expected 2 joins, got %d", len(s.Joins))
	}
}

func TestParser_SubqueryInWhere(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users WHERE id IN (SELECT user_id FROM orders)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	inExpr, ok := s.Where.(*InExpr)
	if !ok {
		t.Fatalf("expected InExpr, got %T", s.Where)
	}
	if len(inExpr.Values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(inExpr.Values))
	}
	if _, ok := inExpr.Values[0].(*SubqueryExpr); !ok {
		t.Fatalf("expected SubqueryExpr, got %T", inExpr.Values[0])
	}
}

func TestParser_JoinWithWhere(t *testing.T) {
	stmt, err := Parse("SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id WHERE o.amount > 100")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Joins) != 1 {
		t.Fatalf("expected 1 join, got %d", len(s.Joins))
	}
	if s.Where == nil {
		t.Fatal("expected WHERE clause")
	}
}

func TestParser_JoinWithOrderBy(t *testing.T) {
	stmt, err := Parse("SELECT * FROM a JOIN b ON a.id = b.a_id ORDER BY a.name")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if len(s.Joins) != 1 {
		t.Fatalf("expected 1 join, got %d", len(s.Joins))
	}
	if len(s.OrderBy) != 1 {
		t.Fatalf("expected 1 order by, got %d", len(s.OrderBy))
	}
}

func TestLexer_VectorDist(t *testing.T) {
	lexer := NewLexer("embedding <-> '[0.1, 0.2]'")
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}

	found := false
	for _, tok := range tokens {
		if tok.Type == TOKEN_VECTOR_DIST {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected <-> token")
	}
}

func TestParser_VectorDistance(t *testing.T) {
	stmt, err := Parse("SELECT * FROM t WHERE embedding <-> '[0.1, 0.2]' < 0.5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := stmt.(*SelectStmt)
	if s.Where == nil {
		t.Fatal("expected WHERE clause")
	}
}
