package sql

import (
	"fmt"
	"strconv"
)

// Parser 递归下降 SQL 解析器
type Parser struct {
	tokens []Token
	pos    int
}

// Parse 解析 SQL 语句
func Parse(input string) (Statement, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, fmt.Errorf("lexer error: %w", err)
	}

	p := &Parser{tokens: tokens}
	return p.parseStatement()
}

// ParseExpr 解析表达式
func ParseExpr(input string) (Expression, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, fmt.Errorf("lexer error: %w", err)
	}
	p := &Parser{tokens: tokens}
	return p.parseOrExpr()
}

func (p *Parser) parseStatement() (Statement, error) {
	tok := p.peek()
	switch tok.Type {
	case TOKEN_SELECT:
		return p.parseSelect()
	case TOKEN_INSERT:
		return p.parseInsert()
	case TOKEN_UPDATE:
		return p.parseUpdate()
	case TOKEN_DELETE:
		return p.parseDelete()
	case TOKEN_CREATE:
		return p.parseCreate()
	case TOKEN_DROP:
		return p.parseDrop()
	case TOKEN_ALTER:
		return p.parseAlterTable()
	case TOKEN_SHOW:
		return p.parseShow()
	case TOKEN_DESCRIBE:
		return p.parseDescribeTable()
	default:
		// 检查 DESC 作为 DESCRIBE 的别名
		if tok.Type == TOKEN_DESC {
			return p.parseDescribeTable()
		}
		return nil, fmt.Errorf("unexpected token %s at position %d, expected SELECT/INSERT/UPDATE/DELETE/CREATE/DROP/ALTER/SHOW/DESCRIBE", tok.Type, tok.Pos)
	}
}

// ========== SELECT ==========

func (p *Parser) parseSelect() (*SelectStmt, error) {
	stmt := &SelectStmt{}

	// SELECT
	p.expect(TOKEN_SELECT)

	// DISTINCT?
	if p.peek().Type == TOKEN_IDENT && p.peek().Literal == "DISTINCT" {
		stmt.Distinct = true
		p.advance()
	}

	// 列列表
	cols, err := p.parseSelectColumns()
	if err != nil {
		return nil, err
	}
	stmt.Columns = cols

	// FROM?
	if p.peek().Type == TOKEN_FROM {
		p.advance()
		table, err := p.parseTableRef()
		if err != nil {
			return nil, err
		}
		stmt.From = table
	}

	// JOIN?
	for {
		tok := p.peek()
		if tok.Type == TOKEN_JOIN {
			join, err := p.parseJoin(JoinInner)
			if err != nil {
				return nil, err
			}
			stmt.Joins = append(stmt.Joins, join)
		} else if tok.Type == TOKEN_LEFT {
			p.advance()
			if p.peek().Type == TOKEN_JOIN {
				join, err := p.parseJoin(JoinLeft)
				if err != nil {
					return nil, err
				}
				stmt.Joins = append(stmt.Joins, join)
			} else {
				return nil, fmt.Errorf("expected JOIN after LEFT at position %d", tok.Pos)
			}
		} else {
			break
		}
	}

	// WHERE?
	if p.peek().Type == TOKEN_WHERE {
		p.advance()
		where, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}

	// GROUP BY?
	if p.peek().Type == TOKEN_GROUP {
		p.advance()
		p.expect(TOKEN_BY)
		groupBy, err := p.parseExprList()
		if err != nil {
			return nil, err
		}
		stmt.GroupBy = groupBy
	}

	// HAVING?
	if p.peek().Type == TOKEN_IDENT && p.peek().Literal == "HAVING" {
		p.advance()
		having, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		stmt.Having = having
	}

	// ORDER BY?
	if p.peek().Type == TOKEN_ORDER {
		p.advance()
		p.expect(TOKEN_BY)
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		stmt.OrderBy = orderBy
	}

	// LIMIT?
	if p.peek().Type == TOKEN_LIMIT {
		p.advance()
		n, err := p.parseIntLiteral()
		if err != nil {
			return nil, err
		}
		limit := int(n)
		stmt.Limit = &limit
	}

	// OFFSET?
	if p.peek().Type == TOKEN_OFFSET {
		p.advance()
		n, err := p.parseIntLiteral()
		if err != nil {
			return nil, err
		}
		offset := int(n)
		stmt.Offset = &offset
	}

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

func (p *Parser) parseJoin(joinType JoinType) (JoinClause, error) {
	join := JoinClause{Type: joinType}

	if err := p.expect(TOKEN_JOIN); err != nil {
		return join, err
	}

	table, err := p.parseTableRef()
	if err != nil {
		return join, err
	}
	join.Table = table

	// ON
	if p.peek().Type == TOKEN_ON {
		p.advance()
		on, err := p.parseOrExpr()
		if err != nil {
			return join, err
		}
		join.On = on
	}

	return join, nil
}

func (p *Parser) parseSelectColumns() ([]SelectColumn, error) {
	var cols []SelectColumn

	// SELECT *
	if p.peek().Type == TOKEN_MULTIPLY {
		p.advance()
		cols = append(cols, SelectColumn{Expr: &StarExpr{}})
		return cols, nil
	}

	for {
		expr, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}

		col := SelectColumn{Expr: expr}

		// AS?
		if p.peek().Type == TOKEN_AS {
			p.advance()
			alias, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			col.Alias = alias
		} else if p.peek().Type == TOKEN_IDENT {
			// 隐式别名
			col.Alias = p.peek().Literal
			p.advance()
		}

		cols = append(cols, col)

		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance() // 跳过逗号
	}

	return cols, nil
}

func (p *Parser) parseTableRef() (*TableRef, error) {
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	ref := &TableRef{Name: name}

	// AS? 或隐式别名
	if p.peek().Type == TOKEN_AS {
		p.advance()
		alias, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		ref.Alias = alias
	} else if p.peek().Type == TOKEN_IDENT && !IsKeyword(p.peek().Literal) {
		ref.Alias = p.peek().Literal
		p.advance()
	}

	return ref, nil
}

// ========== INSERT ==========

func (p *Parser) parseInsert() (*InsertStmt, error) {
	stmt := &InsertStmt{}

	p.expect(TOKEN_INSERT)
	p.expect(TOKEN_INTO)

	// 表名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	// 可选列列表
	if p.peek().Type == TOKEN_LPAREN {
		p.advance()
		cols, err := p.parseIdentList()
		if err != nil {
			return nil, err
		}
		stmt.Columns = cols
		p.expect(TOKEN_RPAREN)
	}

	// VALUES
	p.expect(TOKEN_VALUES)

	// 值列表
	for {
		p.expect(TOKEN_LPAREN)
		vals, err := p.parseExprList()
		if err != nil {
			return nil, err
		}
		stmt.Values = append(stmt.Values, vals)
		p.expect(TOKEN_RPAREN)

		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}

	return stmt, nil
}

// ========== UPDATE ==========

func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	stmt := &UpdateStmt{}

	p.expect(TOKEN_UPDATE)

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	p.expect(TOKEN_SET)

	// SET 子句
	for {
		col, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		p.expect(TOKEN_EQ)
		val, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		stmt.Set = append(stmt.Set, SetClause{Column: col, Value: val})

		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}

	// WHERE?
	if p.peek().Type == TOKEN_WHERE {
		p.advance()
		where, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}

	return stmt, nil
}

// ========== DELETE ==========

func (p *Parser) parseDelete() (*DeleteStmt, error) {
	stmt := &DeleteStmt{}

	p.expect(TOKEN_DELETE)
	p.expect(TOKEN_FROM)

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	// WHERE?
	if p.peek().Type == TOKEN_WHERE {
		p.advance()
		where, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}

	return stmt, nil
}

// ========== CREATE TABLE ==========

func (p *Parser) parseCreateTable() (*CreateTableStmt, error) {
	stmt := &CreateTableStmt{}

	p.expect(TOKEN_CREATE)
	p.expect(TOKEN_TABLE)

	// IF NOT EXISTS?
	if p.peek().Type == TOKEN_IF {
		p.advance()
		p.expect(TOKEN_NOT)
		p.expect(TOKEN_EXISTS)
		stmt.IfNotExists = true
	}

	// 表名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	// 列定义列表
	p.expect(TOKEN_LPAREN)

	for {
		col, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		stmt.Columns = append(stmt.Columns, col)

		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}

	p.expect(TOKEN_RPAREN)

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

func (p *Parser) parseColumnDef() (ColumnDef, error) {
	col := ColumnDef{Nullable: true}

	// 列名
	name, err := p.expectIdent()
	if err != nil {
		return col, err
	}
	col.Name = name

	// 列类型
	colType, err := p.parseColumnType()
	if err != nil {
		return col, err
	}
	col.Type = colType

	// 列约束
	for {
		tok := p.peek()
		switch tok.Type {
		case TOKEN_NOT:
			p.advance()
			p.expect(TOKEN_NULL)
			col.Nullable = false
		case TOKEN_PRIMARY:
			p.advance()
			p.expect(TOKEN_KEY)
			col.PrimaryKey = true
			col.Nullable = false
		case TOKEN_DEFAULT:
			p.advance()
			defaultVal, err := p.parseOrExpr()
			if err != nil {
				return col, err
			}
			col.Default = defaultVal
		default:
			return col, nil
		}
	}
}

func (p *Parser) parseColumnType() (ColumnType, error) {
	tok := p.peek()
	var colType ColumnType

	switch tok.Type {
	case TOKEN_INT, TOKEN_INTEGER_TYPE:
		p.advance()
		colType.Name = "INT"
	case TOKEN_VARCHAR:
		p.advance()
		colType.Name = "VARCHAR"
		// 可选的长度
		if p.peek().Type == TOKEN_LPAREN {
			p.advance()
			len, err := p.parseIntLiteral()
			if err != nil {
				return colType, err
			}
			colType.Length = int(len)
			p.expect(TOKEN_RPAREN)
		}
	case TOKEN_TEXT:
		p.advance()
		colType.Name = "TEXT"
	case TOKEN_STRING_TYPE:
		p.advance()
		colType.Name = "STRING"
	case TOKEN_FLOAT_TYPE:
		p.advance()
		colType.Name = "FLOAT"
	case TOKEN_BOOL, TOKEN_BOOLEAN:
		p.advance()
		colType.Name = "BOOL"
	default:
		return colType, fmt.Errorf("expected column type, got %s at position %d", tok.Type, tok.Pos)
	}

	return colType, nil
}

// ========== DROP TABLE ==========

func (p *Parser) parseDropTable() (*DropTableStmt, error) {
	stmt := &DropTableStmt{}

	p.expect(TOKEN_DROP)
	p.expect(TOKEN_TABLE)

	// IF EXISTS?
	if p.peek().Type == TOKEN_IF {
		p.advance()
		p.expect(TOKEN_EXISTS)
		stmt.IfExists = true
	}

	// 表名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ========== ALTER TABLE ==========

func (p *Parser) parseAlterTable() (*AlterTableStmt, error) {
	stmt := &AlterTableStmt{}

	p.expect(TOKEN_ALTER)
	p.expect(TOKEN_TABLE)

	// 表名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = name

	// 操作类型
	tok := p.peek()
	switch tok.Type {
	case TOKEN_ADD:
		p.advance()
		if p.peek().Type == TOKEN_COLUMN {
			p.advance()
		}
		col, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		stmt.Action = &AddColumnAction{Column: col}
	case TOKEN_DROP:
		p.advance()
		if p.peek().Type == TOKEN_COLUMN {
			p.advance()
		}
		colName, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		stmt.Action = &DropColumnAction{Column: colName}
	case TOKEN_MODIFY:
		p.advance()
		if p.peek().Type == TOKEN_COLUMN {
			p.advance()
		}
		col, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		stmt.Action = &ModifyColumnAction{Column: col}
	default:
		return nil, fmt.Errorf("expected ADD/DROP/MODIFY after ALTER TABLE, got %s at position %d", tok.Type, tok.Pos)
	}

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ========== SHOW TABLES ==========

func (p *Parser) parseShowTables() (*ShowTablesStmt, error) {
	p.expect(TOKEN_SHOW)
	p.expect(TOKEN_TABLES)

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &ShowTablesStmt{}, nil
}

// ========== 分派：CREATE / DROP / SHOW ==========

func (p *Parser) parseCreate() (Statement, error) {
	// 前看后续 token 判断是 TABLE 还是 [UNIQUE] [FULLTEXT] INDEX
	save := p.pos
	p.advance() // CREATE
	for p.peek().Type == TOKEN_UNIQUE || p.peek().Type == TOKEN_FULLTEXT {
		p.advance()
	}
	if p.peek().Type == TOKEN_INDEX {
		p.pos = save
		return p.parseCreateIndex()
	}
	p.pos = save
	return p.parseCreateTable()
}

func (p *Parser) parseDrop() (Statement, error) {
	save := p.pos
	p.advance() // DROP
	tok := p.peek()
	if tok.Type == TOKEN_INDEX {
		p.pos = save
		return p.parseDropIndex()
	}
	p.pos = save
	return p.parseDropTable()
}

func (p *Parser) parseShow() (Statement, error) {
	save := p.pos
	p.advance() // SHOW
	tok := p.peek()
	if tok.Type == TOKEN_INDEXES || tok.Type == TOKEN_INDEX {
		p.pos = save
		return p.parseShowIndexes()
	}
	p.pos = save
	return p.parseShowTables()
}

// ========== CREATE INDEX ==========

func (p *Parser) parseCreateIndex() (*CreateIndexStmt, error) {
	stmt := &CreateIndexStmt{IndexType: "BTREE"}

	p.expect(TOKEN_CREATE)
	for {
		switch p.peek().Type {
		case TOKEN_UNIQUE:
			p.advance()
			stmt.Unique = true
		case TOKEN_FULLTEXT:
			p.advance()
			stmt.IndexType = "INVERTED"
		default:
			goto expectIndex
		}
	}
expectIndex:
	if err := p.expect(TOKEN_INDEX); err != nil {
		return nil, err
	}

	// IF NOT EXISTS?
	if p.peek().Type == TOKEN_IF {
		p.advance()
		if err := p.expect(TOKEN_NOT); err != nil {
			return nil, err
		}
		if err := p.expect(TOKEN_EXISTS); err != nil {
			return nil, err
		}
		stmt.IfNotExists = true
	}

	// 索引名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Name = name

	// ON <table>
	if err := p.expect(TOKEN_ON); err != nil {
		return nil, err
	}
	table, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Table = table

	// (<column>)
	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}
	col, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Column = col
	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	// [USING HASH|BTREE|INVERTED|FULLTEXT]
	if p.peek().Type == TOKEN_USING {
		p.advance()
		typTok := p.peek()
		switch typTok.Type {
		case TOKEN_HASH:
			stmt.IndexType = "HASH"
		case TOKEN_BTREE:
			stmt.IndexType = "BTREE"
		case TOKEN_INVERTED:
			stmt.IndexType = "INVERTED"
		case TOKEN_FULLTEXT:
			stmt.IndexType = "INVERTED"
		default:
			return nil, fmt.Errorf("expected HASH/BTREE/INVERTED/FULLTEXT after USING, got %s at position %d", typTok.Type, typTok.Pos)
		}
		p.advance()
	}

	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}
	return stmt, nil
}

// ========== DROP INDEX ==========

func (p *Parser) parseDropIndex() (*DropIndexStmt, error) {
	stmt := &DropIndexStmt{}
	p.expect(TOKEN_DROP)
	if err := p.expect(TOKEN_INDEX); err != nil {
		return nil, err
	}
	if p.peek().Type == TOKEN_IF {
		p.advance()
		if err := p.expect(TOKEN_EXISTS); err != nil {
			return nil, err
		}
		stmt.IfExists = true
	}
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	stmt.Name = name
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}
	return stmt, nil
}

// ========== SHOW INDEXES ==========

func (p *Parser) parseShowIndexes() (*ShowIndexesStmt, error) {
	stmt := &ShowIndexesStmt{}
	p.expect(TOKEN_SHOW)
	tok := p.peek()
	if tok.Type != TOKEN_INDEXES && tok.Type != TOKEN_INDEX {
		return nil, fmt.Errorf("expected INDEXES, got %s at position %d", tok.Type, tok.Pos)
	}
	p.advance()
	if p.peek().Type == TOKEN_FROM {
		p.advance()
		name, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		stmt.Table = name
	}
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}
	return stmt, nil
}

// ========== DESCRIBE TABLE ==========

func (p *Parser) parseDescribeTable() (*DescribeTableStmt, error) {
	tok := p.peek()
	if tok.Type == TOKEN_DESCRIBE {
		p.advance()
	} else if tok.Type == TOKEN_DESC {
		p.advance()
	} else {
		return nil, fmt.Errorf("expected DESCRIBE or DESC, got %s at position %d", tok.Type, tok.Pos)
	}

	// 表名
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	// 可选的分号
	if p.peek().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	return &DescribeTableStmt{Table: name}, nil
}

// ========== 表达式解析（优先级递增） ==========

// OR
func (p *Parser) parseOrExpr() (Expression, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TOKEN_OR {
		p.advance()
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: TOKEN_OR, Left: left, Right: right}
	}

	return left, nil
}

// AND
func (p *Parser) parseAndExpr() (Expression, error) {
	left, err := p.parseNotExpr()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TOKEN_AND {
		p.advance()
		right, err := p.parseNotExpr()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: TOKEN_AND, Left: left, Right: right}
	}

	return left, nil
}

// NOT
func (p *Parser) parseNotExpr() (Expression, error) {
	if p.peek().Type == TOKEN_NOT {
		p.advance()
		expr, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: TOKEN_NOT, Expr: expr}, nil
	}
	return p.parseComparison()
}

// 比较运算
func (p *Parser) parseComparison() (Expression, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}

	tok := p.peek()

	// IS [NOT] NULL
	if tok.Type == TOKEN_IS {
		p.advance()
		if p.peek().Type == TOKEN_NOT {
			p.advance()
			p.expect(TOKEN_NULL)
			return &IsNullExpr{Expr: left, Not: true}, nil
		}
		p.expect(TOKEN_NULL)
		return &IsNullExpr{Expr: left}, nil
	}

	// [NOT] IN
	if tok.Type == TOKEN_IN || (tok.Type == TOKEN_NOT && p.peekAt(1).Type == TOKEN_IN) {
		not := false
		if tok.Type == TOKEN_NOT {
			not = true
			p.advance()
		}
		p.expect(TOKEN_IN)
		p.expect(TOKEN_LPAREN)

		// 检查子查询
		if p.peek().Type == TOKEN_SELECT {
			subStmt, err := p.parseSelect()
			if err != nil {
				return nil, err
			}
			p.expect(TOKEN_RPAREN)
			return &InExpr{Expr: left, Values: []Expression{&SubqueryExpr{Stmt: subStmt}}, Not: not}, nil
		}

		vals, err := p.parseExprList()
		if err != nil {
			return nil, err
		}
		p.expect(TOKEN_RPAREN)
		return &InExpr{Expr: left, Values: vals, Not: not}, nil
	}

	// [NOT] BETWEEN
	if tok.Type == TOKEN_BETWEEN || (tok.Type == TOKEN_NOT && p.peekAt(1).Type == TOKEN_BETWEEN) {
		not := false
		if tok.Type == TOKEN_NOT {
			not = true
			p.advance()
		}
		p.expect(TOKEN_BETWEEN)
		low, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		p.expect(TOKEN_AND)
		high, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		return &BetweenExpr{Expr: left, Low: low, High: high, Not: not}, nil
	}

	// [NOT] LIKE
	if tok.Type == TOKEN_LIKE || (tok.Type == TOKEN_NOT && p.peekAt(1).Type == TOKEN_LIKE) {
		not := false
		if tok.Type == TOKEN_NOT {
			not = true
			p.advance()
		}
		p.expect(TOKEN_LIKE)
		pattern, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		return &LikeExpr{Expr: left, Pattern: pattern, Not: not}, nil
	}

	// 比较运算符
	if IsComparisonOp(tok.Type) {
		p.advance()
		right, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: tok.Type, Left: left, Right: right}, nil
	}

	// 向量距离运算符 <->
	if tok.Type == TOKEN_VECTOR_DIST {
		p.advance()
		right, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: TOKEN_VECTOR_DIST, Left: left, Right: right}, nil
	}

	return left, nil
}

// 加减
func (p *Parser) parseAddSub() (Expression, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TOKEN_PLUS || p.peek().Type == TOKEN_MINUS {
		op := p.peek().Type
		p.advance()
		right, err := p.parseMulDiv()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}

	return left, nil
}

// 乘除
func (p *Parser) parseMulDiv() (Expression, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TOKEN_MULTIPLY || p.peek().Type == TOKEN_DIVIDE || p.peek().Type == TOKEN_MODULO {
		op := p.peek().Type
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}

	return left, nil
}

// 一元运算
func (p *Parser) parseUnary() (Expression, error) {
	tok := p.peek()

	if tok.Type == TOKEN_MINUS {
		p.advance()
		expr, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: TOKEN_MINUS, Expr: expr}, nil
	}

	return p.parsePrimary()
}

// 基本表达式
func (p *Parser) parsePrimary() (Expression, error) {
	tok := p.peek()

	switch tok.Type {
	case TOKEN_IDENT:
		return p.parseIdentOrFunc()
	case TOKEN_MATCH:
		return p.parseMatchExpr()
	case TOKEN_STRING:
		p.advance()
		return &StringLiteralExpr{Value: tok.Literal}, nil
	case TOKEN_INTEGER:
		p.advance()
		n, err := strconv.ParseInt(tok.Literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q at position %d", tok.Literal, tok.Pos)
		}
		return &IntegerLiteralExpr{Value: n}, nil
	case TOKEN_FLOAT:
		p.advance()
		f, err := strconv.ParseFloat(tok.Literal, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float %q at position %d", tok.Literal, tok.Pos)
		}
		return &FloatLiteralExpr{Value: f}, nil
	case TOKEN_TRUE:
		p.advance()
		return &BoolLiteralExpr{Value: true}, nil
	case TOKEN_FALSE:
		p.advance()
		return &BoolLiteralExpr{Value: false}, nil
	case TOKEN_NULL:
		p.advance()
		return &NullLiteralExpr{}, nil
	case TOKEN_LPAREN:
		p.advance()
		// 检查子查询
		if p.peek().Type == TOKEN_SELECT {
			subStmt, err := p.parseSelect()
			if err != nil {
				return nil, err
			}
			p.expect(TOKEN_RPAREN)
			return &SubqueryExpr{Stmt: subStmt}, nil
		}
		expr, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		p.expect(TOKEN_RPAREN)
		return expr, nil
	case TOKEN_MULTIPLY:
		p.advance()
		return &StarExpr{}, nil
	// 聚合函数关键字
	case TOKEN_COUNT, TOKEN_SUM, TOKEN_MIN, TOKEN_MAX, TOKEN_AVG, TOKEN_FIRST, TOKEN_LAST:
		return p.parseAggregateFunc()
	default:
		return nil, fmt.Errorf("unexpected token %s at position %d", tok.Type, tok.Pos)
	}
}

func (p *Parser) parseIdentOrFunc() (Expression, error) {
	name := p.peek().Literal
	p.advance()

	// 检查是否是函数调用 (name(...))
	if p.peek().Type == TOKEN_LPAREN {
		return p.parseFuncCall(name)
	}

	// 检查是否是 table.column
	if p.peek().Type == TOKEN_DOT {
		p.advance()
		colName, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		return &IdentifierExpr{Name: colName, Table: name}, nil
	}

	return &IdentifierExpr{Name: name}, nil
}

func (p *Parser) parseFuncCall(name string) (Expression, error) {
	p.expect(TOKEN_LPAREN)

	distinct := false
	if p.peek().Type == TOKEN_IDENT && p.peek().Literal == "DISTINCT" {
		distinct = true
		p.advance()
	}

	var args []Expression
	if p.peek().Type != TOKEN_RPAREN {
		var err error
		args, err = p.parseExprList()
		if err != nil {
			return nil, err
		}
	}

	p.expect(TOKEN_RPAREN)
	return &FuncCallExpr{Name: name, Args: args, Distinct: distinct}, nil
}

func (p *Parser) parseAggregateFunc() (Expression, error) {
	name := p.peek().Literal
	p.advance()

	p.expect(TOKEN_LPAREN)

	distinct := false
	if p.peek().Type == TOKEN_IDENT && p.peek().Literal == "DISTINCT" {
		distinct = true
		p.advance()
	}

	var args []Expression
	if p.peek().Type != TOKEN_RPAREN {
		if p.peek().Type == TOKEN_MULTIPLY {
			p.advance()
			args = []Expression{&StarExpr{}}
		} else {
			var err error
			args, err = p.parseExprList()
			if err != nil {
				return nil, err
			}
		}
	}

	p.expect(TOKEN_RPAREN)
	return &FuncCallExpr{Name: name, Args: args, Distinct: distinct}, nil
}

// ========== 辅助方法 ==========

func (p *Parser) parseExprList() ([]Expression, error) {
	var exprs []Expression
	for {
		expr, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}
	return exprs, nil
}

func (p *Parser) parseIdentList() ([]string, error) {
	var names []string
	for {
		name, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}
	return names, nil
}

func (p *Parser) parseOrderBy() ([]OrderByExpr, error) {
	var order []OrderByExpr
	for {
		expr, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}

		oby := OrderByExpr{Expr: expr, Ascending: true}

		if p.peek().Type == TOKEN_ASC {
			p.advance()
		} else if p.peek().Type == TOKEN_DESC {
			oby.Ascending = false
			p.advance()
		}

		order = append(order, oby)

		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}
	return order, nil
}

func (p *Parser) parseIntLiteral() (int64, error) {
	tok := p.peek()
	if tok.Type != TOKEN_INTEGER {
		return 0, fmt.Errorf("expected integer, got %s at position %d", tok.Type, tok.Pos)
	}
	p.advance()
	return strconv.ParseInt(tok.Literal, 10, 64)
}

func (p *Parser) expectIdent() (string, error) {
	tok := p.peek()
	if tok.Type != TOKEN_IDENT {
		return "", fmt.Errorf("expected identifier, got %s at position %d", tok.Type, tok.Pos)
	}
	p.advance()
	return tok.Literal, nil
}

func (p *Parser) expect(tt TokenType) error {
	tok := p.peek()
	if tok.Type != tt {
		return fmt.Errorf("expected %s, got %s at position %d", tt, tok.Type, tok.Pos)
	}
	p.advance()
	return nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekAt(offset int) Token {
	pos := p.pos + offset
	if pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[pos]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	p.pos++
	return tok
}

// ========== MATCH ... AGAINST ==========

// parseMatchExpr 解析 MATCH(col1[, col2 ...]) AGAINST ('query')
func (p *Parser) parseMatchExpr() (Expression, error) {
	if err := p.expect(TOKEN_MATCH); err != nil {
		return nil, err
	}
	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}
	var cols []string
	for {
		name, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		cols = append(cols, name)
		if p.peek().Type != TOKEN_COMMA {
			break
		}
		p.advance()
	}
	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}
	if err := p.expect(TOKEN_AGAINST); err != nil {
		return nil, err
	}
	if err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}
	tok := p.peek()
	if tok.Type != TOKEN_STRING {
		return nil, fmt.Errorf("expected string literal after AGAINST(, got %s at position %d", tok.Type, tok.Pos)
	}
	p.advance()
	if err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}
	return &MatchExpr{Columns: cols, Query: tok.Literal}, nil
}
