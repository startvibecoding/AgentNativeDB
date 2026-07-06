package sql

import (
	"testing"
)

func TestLexer_BasicSelect(t *testing.T) {
	input := "SELECT * FROM users WHERE id = 1"
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}

	expected := []TokenType{
		TOKEN_SELECT, TOKEN_MULTIPLY, TOKEN_FROM, TOKEN_IDENT,
		TOKEN_WHERE, TOKEN_IDENT, TOKEN_EQ, TOKEN_INTEGER, TOKEN_EOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}

	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token %d: expected %s, got %s (literal=%q)", i, exp, tokens[i].Type, tokens[i].Literal)
		}
	}
}

func TestLexer_StringLiteral(t *testing.T) {
	input := "SELECT name FROM users WHERE name = 'hello world'"
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}

	// 找到字符串 token
	var strToken *Token
	for i := range tokens {
		if tokens[i].Type == TOKEN_STRING {
			strToken = &tokens[i]
			break
		}
	}

	if strToken == nil {
		t.Fatal("expected string token")
	}
	if strToken.Literal != "hello world" {
		t.Fatalf("expected 'hello world', got %q", strToken.Literal)
	}
}

func TestLexer_Operators(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"=", TOKEN_EQ},
		{"!=", TOKEN_NEQ},
		{"<>", TOKEN_NEQ},
		{"<", TOKEN_LT},
		{"<=", TOKEN_LTE},
		{">", TOKEN_GT},
		{">=", TOKEN_GTE},
		{"+", TOKEN_PLUS},
		{"-", TOKEN_MINUS},
		{"*", TOKEN_MULTIPLY},
		{"/", TOKEN_DIVIDE},
		{"%", TOKEN_MODULO},
	}

	for _, tt := range tests {
		lexer := NewLexer(tt.input)
		tokens, err := lexer.Tokenize()
		if err != nil {
			t.Errorf("tokenize %q: %v", tt.input, err)
			continue
		}
		if tokens[0].Type != tt.want {
			t.Errorf("%q: expected %s, got %s", tt.input, tt.want, tokens[0].Type)
		}
	}
}

func TestLexer_Keywords(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"SELECT", TOKEN_SELECT},
		{"select", TOKEN_SELECT},
		{"FROM", TOKEN_FROM},
		{"WHERE", TOKEN_WHERE},
		{"INSERT", TOKEN_INSERT},
		{"UPDATE", TOKEN_UPDATE},
		{"DELETE", TOKEN_DELETE},
		{"ORDER", TOKEN_ORDER},
		{"GROUP", TOKEN_GROUP},
		{"LIMIT", TOKEN_LIMIT},
		{"AND", TOKEN_AND},
		{"OR", TOKEN_OR},
		{"NOT", TOKEN_NOT},
		{"NULL", TOKEN_NULL},
		{"TRUE", TOKEN_TRUE},
		{"FALSE", TOKEN_FALSE},
		{"COUNT", TOKEN_COUNT},
		{"SUM", TOKEN_SUM},
		{"FULLTEXT", TOKEN_FULLTEXT},
	}

	for _, tt := range tests {
		lexer := NewLexer(tt.input)
		tokens, err := lexer.Tokenize()
		if err != nil {
			t.Errorf("tokenize %q: %v", tt.input, err)
			continue
		}
		if tokens[0].Type != tt.want {
			t.Errorf("%q: expected %s, got %s", tt.input, tt.want, tokens[0].Type)
		}
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		input   string
		wantTyp TokenType
		wantLit string
	}{
		{"123", TOKEN_INTEGER, "123"},
		{"-5", TOKEN_INTEGER, "-5"},
		{"3.14", TOKEN_FLOAT, "3.14"},
		{"-2.5", TOKEN_FLOAT, "-2.5"},
	}

	for _, tt := range tests {
		lexer := NewLexer(tt.input)
		tokens, err := lexer.Tokenize()
		if err != nil {
			t.Errorf("tokenize %q: %v", tt.input, err)
			continue
		}
		if tokens[0].Type != tt.wantTyp {
			t.Errorf("%q: expected type %s, got %s", tt.input, tt.wantTyp, tokens[0].Type)
		}
		if tokens[0].Literal != tt.wantLit {
			t.Errorf("%q: expected literal %q, got %q", tt.input, tt.wantLit, tokens[0].Literal)
		}
	}
}

func TestLexer_ComplexQuery(t *testing.T) {
	input := `SELECT u.name, COUNT(*) as cnt 
FROM users u 
WHERE u.age > 18 AND u.status = 'active'
GROUP BY u.name
ORDER BY cnt DESC
LIMIT 10`

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}

	if tokens[len(tokens)-1].Type != TOKEN_EOF {
		t.Fatalf("expected EOF at end, got %s", tokens[len(tokens)-1].Type)
	}

	// 检查一些关键 token
	selectFound := false
	fromFound := false
	whereFound := false
	groupFound := false
	orderFound := false
	limitFound := false

	for _, tok := range tokens {
		switch tok.Type {
		case TOKEN_SELECT:
			selectFound = true
		case TOKEN_FROM:
			fromFound = true
		case TOKEN_WHERE:
			whereFound = true
		case TOKEN_GROUP:
			groupFound = true
		case TOKEN_ORDER:
			orderFound = true
		case TOKEN_LIMIT:
			limitFound = true
		}
	}

	if !selectFound || !fromFound || !whereFound || !groupFound || !orderFound || !limitFound {
		t.Fatal("missing expected keywords in complex query")
	}
}

func TestLexer_UnterminatedString(t *testing.T) {
	input := "SELECT * FROM users WHERE name = 'unterminated"
	lexer := NewLexer(input)
	_, err := lexer.Tokenize()
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
}

func BenchmarkLexer(b *testing.B) {
	input := `SELECT u.name, COUNT(*) FROM users u WHERE u.age > 18 AND u.status = 'active' GROUP BY u.name ORDER BY COUNT(*) DESC LIMIT 10`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		lexer.Tokenize()
	}
}
