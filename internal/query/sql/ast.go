package sql

import "fmt"

// AST 节点定义

// Statement SQL 语句接口
type Statement interface {
	stmtNode()
	String() string
}

// Expression 表达式接口
type Expression interface {
	exprNode()
	String() string
}

// ========== 语句节点 ==========

// SelectStmt SELECT 语句
type SelectStmt struct {
	Distinct  bool
	Columns   []SelectColumn
	From      *TableRef
	Joins     []JoinClause
	Where     Expression
	GroupBy   []Expression
	Having    Expression
	OrderBy   []OrderByExpr
	Limit     *int
	Offset    *int
}

func (s *SelectStmt) stmtNode() {}
func (s *SelectStmt) String() string { return "SELECT ..." }

// JoinClause JOIN 子句
type JoinClause struct {
	Type  JoinType
	Table *TableRef
	On    Expression
}

// JoinType JOIN 类型
type JoinType string

const (
	JoinInner JoinType = "INNER"
	JoinLeft  JoinType = "LEFT"
)

// SubqueryExpr 子查询表达式
type SubqueryExpr struct {
	Stmt *SelectStmt
}

func (e *SubqueryExpr) exprNode() {}
func (e *SubqueryExpr) String() string { return "(SELECT ...)" }

// InsertStmt INSERT 语句
type InsertStmt struct {
	Table   string
	Columns []string
	Values  [][]Expression
}

func (s *InsertStmt) stmtNode() {}
func (s *InsertStmt) String() string { return "INSERT ..." }

// UpdateStmt UPDATE 语句
type UpdateStmt struct {
	Table string
	Set   []SetClause
	Where Expression
}

func (s *UpdateStmt) stmtNode() {}
func (s *UpdateStmt) String() string { return "UPDATE ..." }

// DeleteStmt DELETE 语句
type DeleteStmt struct {
	Table string
	Where Expression
}

func (s *DeleteStmt) stmtNode() {}
func (s *DeleteStmt) String() string { return "DELETE ..." }

// CreateTableStmt CREATE TABLE 语句
type CreateTableStmt struct {
	IfNotExists bool
	Table       string
	Columns     []ColumnDef
}

func (s *CreateTableStmt) stmtNode() {}
func (s *CreateTableStmt) String() string { return "CREATE TABLE ..." }

// DropTableStmt DROP TABLE 语句
type DropTableStmt struct {
	IfExists bool
	Table    string
}

func (s *DropTableStmt) stmtNode() {}
func (s *DropTableStmt) String() string { return "DROP TABLE ..." }

// AlterTableStmt ALTER TABLE 语句
type AlterTableStmt struct {
	Table  string
	Action AlterAction
}

func (s *AlterTableStmt) stmtNode() {}
func (s *AlterTableStmt) String() string { return "ALTER TABLE ..." }

// AlterAction 变更操作类型
type AlterAction interface {
	alterActionNode()
	String() string
}

// AddColumnAction ADD COLUMN 操作
type AddColumnAction struct {
	Column ColumnDef
}

func (a *AddColumnAction) alterActionNode() {}
func (a *AddColumnAction) String() string { return "ADD COLUMN ..." }

// DropColumnAction DROP COLUMN 操作
type DropColumnAction struct {
	Column string
}

func (a *DropColumnAction) alterActionNode() {}
func (a *DropColumnAction) String() string { return "DROP COLUMN ..." }

// ModifyColumnAction MODIFY COLUMN 操作
type ModifyColumnAction struct {
	Column ColumnDef
}

func (a *ModifyColumnAction) alterActionNode() {}
func (a *ModifyColumnAction) String() string { return "MODIFY COLUMN ..." }

// ShowTablesStmt SHOW TABLES 语句
type ShowTablesStmt struct{}

func (s *ShowTablesStmt) stmtNode() {}
func (s *ShowTablesStmt) String() string { return "SHOW TABLES" }

// DescribeTableStmt DESCRIBE/DESC 语句
type DescribeTableStmt struct {
	Table string
}

func (s *DescribeTableStmt) stmtNode() {}
func (s *DescribeTableStmt) String() string { return "DESCRIBE ..." }

// ColumnDef 列定义
type ColumnDef struct {
	Name       string
	Type       ColumnType
	Nullable   bool
	Default    Expression
	PrimaryKey bool
}

// ColumnType 列类型
type ColumnType struct {
	Name   string // INT, VARCHAR, TEXT, FLOAT, BOOL, etc.
	Length int    // VARCHAR 长度，0 表示不限
}

// ========== SELECT 子句 ==========

// SelectColumn SELECT 列
type SelectColumn struct {
	Expr Expression
	Alias string // AS 别名
}

// TableRef 表引用
type TableRef struct {
	Name  string
	Alias string
}

// OrderByExpr ORDER BY 表达式
type OrderByExpr struct {
	Expr      Expression
	Ascending bool // true=ASC, false=DESC
}

// SetClause UPDATE SET 子句
type SetClause struct {
	Column string
	Value  Expression
}

// ========== 表达式节点 ==========

// IdentifierExpr 标识符（列名、表名）
type IdentifierExpr struct {
	Name  string
	Table string // 可选的表限定符（table.column）
}

func (e *IdentifierExpr) exprNode() {}
func (e *IdentifierExpr) String() string {
	if e.Table != "" {
		return e.Table + "." + e.Name
	}
	return e.Name
}

// StringLiteralExpr 字符串字面量
type StringLiteralExpr struct {
	Value string
}

func (e *StringLiteralExpr) exprNode() {}
func (e *StringLiteralExpr) String() string { return "'" + e.Value + "'" }

// IntegerLiteralExpr 整数字面量
type IntegerLiteralExpr struct {
	Value int64
}

func (e *IntegerLiteralExpr) exprNode() {}
func (e *IntegerLiteralExpr) String() string { return fmt.Sprintf("%d", e.Value) }

// FloatLiteralExpr 浮点数字面量
type FloatLiteralExpr struct {
	Value float64
}

func (e *FloatLiteralExpr) exprNode() {}
func (e *FloatLiteralExpr) String() string { return fmt.Sprintf("%g", e.Value) }

// BoolLiteralExpr 布尔字面量
type BoolLiteralExpr struct {
	Value bool
}

func (e *BoolLiteralExpr) exprNode() {}
func (e *BoolLiteralExpr) String() string {
	if e.Value {
		return "TRUE"
	}
	return "FALSE"
}

// NullLiteralExpr NULL 字面量
type NullLiteralExpr struct{}

func (e *NullLiteralExpr) exprNode() {}
func (e *NullLiteralExpr) String() string { return "NULL" }

// BinaryExpr 二元表达式
type BinaryExpr struct {
	Op    TokenType
	Left  Expression
	Right Expression
}

func (e *BinaryExpr) exprNode() {}
func (e *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", e.Left, e.Op, e.Right)
}

// UnaryExpr 一元表达式
type UnaryExpr struct {
	Op   TokenType
	Expr Expression
}

func (e *UnaryExpr) exprNode() {}
func (e *UnaryExpr) String() string {
	return fmt.Sprintf("(%s %s)", e.Op, e.Expr)
}

// FuncCallExpr 函数调用
type FuncCallExpr struct {
	Name      string
	Args      []Expression
	Distinct  bool // COUNT(DISTINCT ...)
}

func (e *FuncCallExpr) exprNode() {}
func (e *FuncCallExpr) String() string {
	if len(e.Args) == 1 {
		if _, ok := e.Args[0].(*StarExpr); ok {
			return e.Name + "(*)"
		}
	}
	return e.Name + "(...)"
}

// StarExpr * 通配符
type StarExpr struct{}

func (e *StarExpr) exprNode() {}
func (e *StarExpr) String() string { return "*" }

// InExpr IN 表达式
type InExpr struct {
	Expr   Expression
	Values []Expression
	Not    bool // NOT IN
}

func (e *InExpr) exprNode() {}
func (e *InExpr) String() string { return "IN (...)" }

// BetweenExpr BETWEEN 表达式
type BetweenExpr struct {
	Expr Expression
	Low  Expression
	High Expression
	Not  bool
}

func (e *BetweenExpr) exprNode() {}
func (e *BetweenExpr) String() string { return "BETWEEN ... AND ..." }

// LikeExpr LIKE 表达式
type LikeExpr struct {
	Expr    Expression
	Pattern Expression
	Not     bool
}

func (e *LikeExpr) exprNode() {}
func (e *LikeExpr) String() string { return "LIKE ..." }

// IsNullExpr IS NULL 表达式
type IsNullExpr struct {
	Expr Expression
	Not  bool // IS NOT NULL
}

func (e *IsNullExpr) exprNode() {}
func (e *IsNullExpr) String() string {
	if e.Not {
		return "IS NOT NULL"
	}
	return "IS NULL"
}
