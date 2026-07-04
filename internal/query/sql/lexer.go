package sql

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType 词法单元类型
type TokenType int

const (
	// 特殊
	TOKEN_EOF TokenType = iota
	TOKEN_ILLEGAL

	// 字面量
	TOKEN_IDENT   // 标识符（表名、列名）
	TOKEN_STRING  // 'string'
	TOKEN_INTEGER // 123
	TOKEN_FLOAT   // 1.23

	// 关键字
	TOKEN_SELECT
	TOKEN_INSERT
	TOKEN_UPDATE
	TOKEN_DELETE
	TOKEN_FROM
	TOKEN_WHERE
	TOKEN_AND
	TOKEN_OR
	TOKEN_NOT
	TOKEN_IN
	TOKEN_IS
	TOKEN_NULL
	TOKEN_AS
	TOKEN_ORDER
	TOKEN_BY
	TOKEN_ASC
	TOKEN_DESC
	TOKEN_GROUP
	TOKEN_LIMIT
	TOKEN_OFFSET
	TOKEN_SET
	TOKEN_INTO
	TOKEN_VALUES
	TOKEN_CREATE
	TOKEN_TABLE
	TOKEN_INDEX
	TOKEN_DROP
	TOKEN_JOIN
	TOKEN_LEFT
	TOKEN_ON
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_LIKE
	TOKEN_BETWEEN
	TOKEN_EXISTS
	TOKEN_COUNT
	TOKEN_SUM
	TOKEN_MIN
	TOKEN_MAX
	TOKEN_AVG
	TOKEN_FIRST
	TOKEN_LAST

	// DDL 关键字
	TOKEN_ALTER      // ALTER
	TOKEN_SHOW       // SHOW
	TOKEN_DESCRIBE   // DESCRIBE
	TOKEN_ADD        // ADD
	TOKEN_COLUMN      // COLUMN
	TOKEN_MODIFY     // MODIFY
	TOKEN_PRIMARY    // PRIMARY
	TOKEN_KEY        // KEY
	TOKEN_IF         // IF
	TOKEN_TABLES     // TABLES
	TOKEN_INT        // INT
	TOKEN_INTEGER_TYPE // INTEGER
	TOKEN_VARCHAR    // VARCHAR
	TOKEN_TEXT       // TEXT
	TOKEN_FLOAT_TYPE // FLOAT
	TOKEN_BOOL       // BOOL
	TOKEN_BOOLEAN    // BOOLEAN
	TOKEN_DEFAULT    // DEFAULT

	// 运算符
	TOKEN_EQ        // =
	TOKEN_NEQ       // != 或 <>
	TOKEN_LT        // <
	TOKEN_LTE       // <=
	TOKEN_GT        // >
	TOKEN_GTE       // >=
	TOKEN_PLUS      // +
	TOKEN_MINUS     // -
	TOKEN_MULTIPLY  // *
	TOKEN_DIVIDE    // /
	TOKEN_MODULO    // %
	TOKEN_COMMA     // ,
	TOKEN_SEMICOLON // ;
	TOKEN_LPAREN    // (
	TOKEN_RPAREN    // )
	TOKEN_DOT       // .

	// 向量运算符
	TOKEN_VECTOR_DIST // <->
)

// Token 词法单元
type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

var keywords = map[string]TokenType{
	"SELECT":  TOKEN_SELECT,
	"INSERT":  TOKEN_INSERT,
	"UPDATE":  TOKEN_UPDATE,
	"DELETE":  TOKEN_DELETE,
	"FROM":    TOKEN_FROM,
	"WHERE":   TOKEN_WHERE,
	"AND":     TOKEN_AND,
	"OR":      TOKEN_OR,
	"NOT":     TOKEN_NOT,
	"IN":      TOKEN_IN,
	"IS":      TOKEN_IS,
	"NULL":    TOKEN_NULL,
	"AS":      TOKEN_AS,
	"ORDER":   TOKEN_ORDER,
	"BY":      TOKEN_BY,
	"ASC":     TOKEN_ASC,
	"DESC":    TOKEN_DESC,
	"GROUP":   TOKEN_GROUP,
	"LIMIT":   TOKEN_LIMIT,
	"OFFSET":  TOKEN_OFFSET,
	"SET":     TOKEN_SET,
	"INTO":    TOKEN_INTO,
	"VALUES":  TOKEN_VALUES,
	"CREATE":  TOKEN_CREATE,
	"TABLE":   TOKEN_TABLE,
	"INDEX":   TOKEN_INDEX,
	"DROP":    TOKEN_DROP,
	"JOIN":    TOKEN_JOIN,
	"LEFT":    TOKEN_LEFT,
	"ON":      TOKEN_ON,
	"TRUE":    TOKEN_TRUE,
	"FALSE":   TOKEN_FALSE,
	"LIKE":    TOKEN_LIKE,
	"BETWEEN": TOKEN_BETWEEN,
	"EXISTS":  TOKEN_EXISTS,
	"COUNT":   TOKEN_COUNT,
	"SUM":     TOKEN_SUM,
	"MIN":     TOKEN_MIN,
	"MAX":     TOKEN_MAX,
	"AVG":     TOKEN_AVG,
	"FIRST":   TOKEN_FIRST,
	"LAST":    TOKEN_LAST,

	// DDL 关键字
	"ALTER":     TOKEN_ALTER,
	"SHOW":      TOKEN_SHOW,
	"DESCRIBE":  TOKEN_DESCRIBE,
	"ADD":       TOKEN_ADD,
	"COLUMN":    TOKEN_COLUMN,
	"MODIFY":    TOKEN_MODIFY,
	"PRIMARY":   TOKEN_PRIMARY,
	"KEY":       TOKEN_KEY,
	"IF":        TOKEN_IF,
	"TABLES":    TOKEN_TABLES,
	"INT":       TOKEN_INT,
	"INTEGER":   TOKEN_INTEGER_TYPE,
	"VARCHAR":   TOKEN_VARCHAR,
	"TEXT":      TOKEN_TEXT,
	"FLOAT":     TOKEN_FLOAT_TYPE,
	"BOOL":      TOKEN_BOOL,
	"BOOLEAN":   TOKEN_BOOLEAN,
	"DEFAULT":   TOKEN_DEFAULT,
}

func (t TokenType) String() string {
	switch {
	case t == TOKEN_EOF:
		return "EOF"
	case t == TOKEN_ILLEGAL:
		return "ILLEGAL"
	case t <= TOKEN_FLOAT:
		return []string{"IDENT", "STRING", "INTEGER", "FLOAT"}[t-TOKEN_IDENT]
	case t <= TOKEN_LAST:
		return []string{
			"SELECT", "INSERT", "UPDATE", "DELETE", "FROM", "WHERE",
			"AND", "OR", "NOT", "IN", "IS", "NULL", "AS", "ORDER", "BY",
			"ASC", "DESC", "GROUP", "LIMIT", "OFFSET", "SET", "INTO",
			"VALUES", "CREATE", "TABLE", "INDEX", "DROP", "JOIN", "LEFT",
			"ON", "TRUE", "FALSE", "LIKE", "BETWEEN", "EXISTS",
			"COUNT", "SUM", "MIN", "MAX", "AVG", "FIRST", "LAST",
		}[t-TOKEN_SELECT]
	case t <= TOKEN_DEFAULT:
		return []string{
			"ALTER", "SHOW", "DESCRIBE", "ADD", "COLUMN", "MODIFY",
			"PRIMARY", "KEY", "IF", "TABLES", "INT", "INTEGER",
			"VARCHAR", "TEXT", "FLOAT", "BOOL", "BOOLEAN", "DEFAULT",
		}[t-TOKEN_ALTER]
	case t <= TOKEN_DOT:
		return []string{
			"=", "!=", "<", "<=", ">", ">=", "+", "-", "*", "/",
			"%", ",", ";", "(", ")", ".",
		}[t-TOKEN_EQ]
	case t == TOKEN_VECTOR_DIST:
		return "<->"
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// Lexer 词法分析器
type Lexer struct {
	input   string
	pos     int
	start   int
	tokens  []Token
}

// NewLexer 创建词法分析器
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: strings.TrimSpace(input),
	}
}

// Tokenize 将输入拆分为词法单元
func (l *Lexer) Tokenize() ([]Token, error) {
	for {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			l.emit(TOKEN_EOF)
			return l.tokens, nil
		}

		ch := l.input[l.pos]

		switch {
		case ch == '\'' || ch == '"':
			if err := l.readString(); err != nil {
				return nil, err
			}
		case isDigit(ch):
			l.readNumber()
		case isAlpha(ch) || ch == '_':
			l.readIdent()
		case ch == '<' && l.peek(1) == '-' && l.peek(2) == '>':
			// <-> 向量距离运算符
			l.pos += 3
			l.emit(TOKEN_VECTOR_DIST)
		case ch == '<' && l.peek(1) == '>':
			l.pos += 2
			l.emit(TOKEN_NEQ)
		case ch == '<' && l.peek(1) == '=':
			l.pos += 2
			l.emit(TOKEN_LTE)
		case ch == '>' && l.peek(1) == '=':
			l.pos += 2
			l.emit(TOKEN_GTE)
		case ch == '!' && l.peek(1) == '=':
			l.pos += 2
			l.emit(TOKEN_NEQ)
		case ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]):
			// 负数
			l.readNumber()
		default:
			l.readOperator()
		}
	}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\n' || l.input[l.pos] == '\r') {
		l.pos++
	}
	l.start = l.pos
}

func (l *Lexer) readString() error {
	quote := l.input[l.pos]
	l.pos++ // 跳过引号
	start := l.pos
	for l.pos < len(l.input) {
		if l.input[l.pos] == quote {
			if l.pos+1 < len(l.input) && l.input[l.pos+1] == quote {
				// 转义引号
				l.pos += 2
				continue
			}
			l.tokens = append(l.tokens, Token{Type: TOKEN_STRING, Literal: l.input[start:l.pos], Pos: start})
			l.pos++ // 跳过结束引号
			l.start = l.pos
			return nil
		}
		l.pos++
	}
	return fmt.Errorf("unterminated string at position %d", start)
}

func (l *Lexer) readNumber() {
	start := l.pos
	isFloat := false

	if l.input[l.pos] == '-' {
		l.pos++
	}

	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}

	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		isFloat = true
		l.pos++
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}

	literal := l.input[start:l.pos]
	if isFloat {
		l.tokens = append(l.tokens, Token{Type: TOKEN_FLOAT, Literal: literal, Pos: start})
	} else {
		l.tokens = append(l.tokens, Token{Type: TOKEN_INTEGER, Literal: literal, Pos: start})
	}
	l.start = l.pos
}

func (l *Lexer) readIdent() {
	start := l.pos
	for l.pos < len(l.input) && (isAlphaNum(l.input[l.pos]) || l.input[l.pos] == '_') {
		l.pos++
	}
	literal := l.input[start:l.pos]

	// 检查是否是关键字
	upper := strings.ToUpper(literal)
	if tt, ok := keywords[upper]; ok {
		l.tokens = append(l.tokens, Token{Type: tt, Literal: upper, Pos: start})
	} else {
		l.tokens = append(l.tokens, Token{Type: TOKEN_IDENT, Literal: literal, Pos: start})
	}
	l.start = l.pos
}

func (l *Lexer) readOperator() {
	ch := l.input[l.pos]
	pos := l.pos
	l.pos++

	switch ch {
	case '=':
		l.emit(TOKEN_EQ)
	case '<':
		l.emit(TOKEN_LT)
	case '>':
		l.emit(TOKEN_GT)
	case '+':
		l.emit(TOKEN_PLUS)
	case '-':
		// 可能是 -> 向量运算符的一部分，也可能是减号
		l.emit(TOKEN_MINUS)
	case '*':
		l.emit(TOKEN_MULTIPLY)
	case '/':
		l.emit(TOKEN_DIVIDE)
	case '%':
		l.emit(TOKEN_MODULO)
	case ',':
		l.emit(TOKEN_COMMA)
	case ';':
		l.emit(TOKEN_SEMICOLON)
	case '(':
		l.emit(TOKEN_LPAREN)
	case ')':
		l.emit(TOKEN_RPAREN)
	case '.':
		l.emit(TOKEN_DOT)
	default:
		l.tokens = append(l.tokens, Token{Type: TOKEN_ILLEGAL, Literal: string(ch), Pos: pos})
	}
}

func (l *Lexer) emit(tt TokenType) {
	l.tokens = append(l.tokens, Token{Type: tt, Literal: l.input[l.start:l.pos], Pos: l.start})
}

func (l *Lexer) peek(offset int) byte {
	pos := l.pos + offset
	if pos >= len(l.input) {
		return 0
	}
	return l.input[pos]
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isAlphaNum(ch byte) bool {
	return isAlpha(ch) || isDigit(ch)
}

// IsKeyword 检查标识符是否是关键字
func IsKeyword(s string) bool {
	_, ok := keywords[strings.ToUpper(s)]
	return ok
}

// IsAggregate 检查是否是聚合函数
func IsAggregate(tt TokenType) bool {
	switch tt {
	case TOKEN_COUNT, TOKEN_SUM, TOKEN_MIN, TOKEN_MAX, TOKEN_AVG, TOKEN_FIRST, TOKEN_LAST:
		return true
	}
	return false
}

// TokenTypeName 返回 token 类型的名称，用于解析器报错
func TokenTypeName(tt TokenType) string {
	return tt.String()
}

// IsComparisonOp 判断是否是比较运算符
func IsComparisonOp(tt TokenType) bool {
	switch tt {
	case TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_LTE, TOKEN_GT, TOKEN_GTE:
		return true
	}
	return false
}

// IsArithmeticOp 判断是否是算术运算符
func IsArithmeticOp(tt TokenType) bool {
	switch tt {
	case TOKEN_PLUS, TOKEN_MINUS, TOKEN_MULTIPLY, TOKEN_DIVIDE, TOKEN_MODULO:
		return true
	}
	return false
}

// suppress unused warning
var _ = unicode.IsSpace
