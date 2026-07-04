package sql

import (
	"fmt"
)

// PlanNode 查询计划节点接口
type PlanNode interface {
	fmt.Stringer
	planNode()
}

// ScanNode 全表扫描
type ScanNode struct {
	Table     string
	Alias     string
	Filter    Expression // 下推的过滤条件
}

func (n *ScanNode) planNode() {}
func (n *ScanNode) String() string { return fmt.Sprintf("Scan(%s)", n.Table) }

// IndexScanNode 索引扫描
type IndexScanNode struct {
	Table     string
	Alias     string
	IndexName string          // 可选：直接指定索引名（否则根据 Column 自动选）
	Column    string          // 相关列
	Op        IndexScanOp     // 扫描方式
	Equal     any             // Op=Equal 时使用
	Low       any             // Op=Range 时使用
	High      any             // Op=Range 时使用
	IncludeLow  bool
	IncludeHigh bool
	MatchQuery  string          // Op=Match 时使用（全文查询字串）
	Filter      Expression      // 回流时的额外过滤
}

// IndexScanOp 索引扫描方式
type IndexScanOp int

const (
	IndexOpEqual IndexScanOp = iota
	IndexOpRange
	IndexOpMatch
)

func (n *IndexScanNode) planNode()      {}
func (n *IndexScanNode) String() string {
	return fmt.Sprintf("IndexScan(%s.%s)", n.Table, n.Column)
}

// FilterNode 过滤
type FilterNode struct {
	Input  PlanNode
	Filter Expression
}

func (n *FilterNode) planNode() {}
func (n *FilterNode) String() string { return fmt.Sprintf("Filter(%s)", n.Filter) }

// ProjectNode 投影（列选择）
type ProjectNode struct {
	Input   PlanNode
	Columns []SelectColumn
}

func (n *ProjectNode) planNode() {}
func (n *ProjectNode) String() string { return "Project" }

// SortNode 排序
type SortNode struct {
	Input PlanNode
	Order []OrderByExpr
}

func (n *SortNode) planNode() {}
func (n *SortNode) String() string { return "Sort" }

// LimitNode 分页
type LimitNode struct {
	Input  PlanNode
	Limit  int
	Offset int
}

func (n *LimitNode) planNode() {}
func (n *LimitNode) String() string { return fmt.Sprintf("Limit(%d, %d)", n.Limit, n.Offset) }

// AggregateNode 聚合
type AggregateNode struct {
	Input    PlanNode
	Aggs     []SelectColumn // 聚合表达式
	GroupBy  []Expression
	Having   Expression
}

func (n *AggregateNode) planNode() {}
func (n *AggregateNode) String() string { return "Aggregate" }

// Planner 查询计划器
type Planner struct {
	catalog IndexCatalog // 可选：写入后可向量/索引选择
}

// IndexCatalog 供 Planner 选择索引使用。与实际 index.Manager 解耦。
type IndexCatalog interface {
	FindByColumn(table, column string) []IndexInfo
}

// IndexInfo 计划器看到的索引信息
type IndexInfo struct {
	Name   string
	Table  string
	Column string
	Type   string // "HASH" | "BTREE" | "INVERTED"
}

// NewPlanner 创建计划器
func NewPlanner() *Planner {
	return &Planner{}
}

// NewPlannerWithCatalog 创建带索引目录的计划器
func NewPlannerWithCatalog(catalog IndexCatalog) *Planner {
	return &Planner{catalog: catalog}
}

// Plan 生成查询计划
func (p *Planner) Plan(stmt Statement) (PlanNode, error) {
	switch s := stmt.(type) {
	case *SelectStmt:
		return p.planSelect(s)
	case *InsertStmt:
		return p.planInsert(s)
	case *UpdateStmt:
		return p.planUpdate(s)
	case *DeleteStmt:
		return p.planDelete(s)
	case *CreateTableStmt:
		return p.planCreateTable(s)
	case *DropTableStmt:
		return p.planDropTable(s)
	case *AlterTableStmt:
		return p.planAlterTable(s)
	case *ShowTablesStmt:
		return p.planShowTables(s)
	case *DescribeTableStmt:
		return p.planDescribeTable(s)
	case *CreateIndexStmt:
		return &CreateIndexPlan{Stmt: s}, nil
	case *DropIndexStmt:
		return &DropIndexPlan{Stmt: s}, nil
	case *ShowIndexesStmt:
		return &ShowIndexesPlan{Stmt: s}, nil
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}
}

func (p *Planner) planSelect(s *SelectStmt) (PlanNode, error) {
	if s.From == nil {
		return nil, fmt.Errorf("SELECT without FROM is not supported")
	}

	// 1. 扫描节点（尝试用索引）
	var node PlanNode
	var consumedWhere Expression // 已被索引消耗的谓词
	if len(s.Joins) == 0 && p.catalog != nil && s.Where != nil {
		if is, remain := p.tryIndexScan(s.From, s.Where); is != nil {
			node = is
			consumedWhere = remain
		}
	}
	if node == nil {
		node = &ScanNode{
			Table: s.From.Name,
			Alias: s.From.Alias,
		}
	}

	// 2. JOIN
	for _, join := range s.Joins {
		node = &JoinNode{
			Left:  node,
			Right: &ScanNode{Table: join.Table.Name, Alias: join.Table.Alias},
			Type:  join.Type,
			On:    join.On,
		}
	}

	// 3. WHERE 过滤（谓词下推）
	if consumedWhere != nil {
		node = &FilterNode{
			Input:  node,
			Filter: consumedWhere,
		}
	} else if s.Where != nil && !isIndexNode(node) {
		node = &FilterNode{
			Input:  node,
			Filter: s.Where,
		}
	}

	// 3. 聚合
	hasAgg := p.hasAggregates(s.Columns)
	if s.GroupBy != nil || hasAgg {
		node = &AggregateNode{
			Input:   node,
			Aggs:    s.Columns,
			GroupBy: s.GroupBy,
			Having:  s.Having,
		}
		// 聚合后不需要额外的 Project（聚合已包含投影）
	} else {
		// 4. 投影（仅非聚合查询）
		node = &ProjectNode{
			Input:   node,
			Columns: s.Columns,
		}
	}

	// 5. 排序
	if s.OrderBy != nil {
		node = &SortNode{
			Input: node,
			Order: s.OrderBy,
		}
	}

	// 6. 分页
	if s.Limit != nil || s.Offset != nil {
		limit := 0
		offset := 0
		if s.Limit != nil {
			limit = *s.Limit
		}
		if s.Offset != nil {
			offset = *s.Offset
		}
		node = &LimitNode{
			Input:  node,
			Limit:  limit,
			Offset: offset,
		}
	}

	return node, nil
}

func (p *Planner) planInsert(s *InsertStmt) (PlanNode, error) {
	// INSERT 直接在执行器中处理
	return &InsertPlan{Stmt: s}, nil
}

func (p *Planner) planUpdate(s *UpdateStmt) (PlanNode, error) {
	var node PlanNode = &ScanNode{Table: s.Table}
	if s.Where != nil {
		node = &FilterNode{Input: node, Filter: s.Where}
	}
	return &UpdatePlan{Input: node, Stmt: s}, nil
}

func (p *Planner) planDelete(s *DeleteStmt) (PlanNode, error) {
	var node PlanNode = &ScanNode{Table: s.Table}
	if s.Where != nil {
		node = &FilterNode{Input: node, Filter: s.Where}
	}
	return &DeletePlan{Input: node, Stmt: s}, nil
}

func (p *Planner) hasAggregates(cols []SelectColumn) bool {
	for _, col := range cols {
		if p.isAggregateExpr(col.Expr) {
			return true
		}
	}
	return false
}

func (p *Planner) isAggregateExpr(expr Expression) bool {
	switch e := expr.(type) {
	case *FuncCallExpr:
		if IsAggregate(lookupKeyword(e.Name)) {
			return true
		}
	}
	return false
}

func lookupKeyword(name string) TokenType {
	if tt, ok := keywords[name]; ok {
		return tt
	}
	return TOKEN_IDENT
}

// InsertPlan INSERT 计划
type InsertPlan struct {
	Stmt *InsertStmt
}

func (n *InsertPlan) planNode() {}
func (n *InsertPlan) String() string { return "Insert" }

// UpdatePlan UPDATE 计划
type UpdatePlan struct {
	Input PlanNode
	Stmt  *UpdateStmt
}

func (n *UpdatePlan) planNode() {}
func (n *UpdatePlan) String() string { return "Update" }

// DeletePlan DELETE 计划
type DeletePlan struct {
	Input PlanNode
	Stmt  *DeleteStmt
}

func (n *DeletePlan) planNode() {}
func (n *DeletePlan) String() string { return "Delete" }

// JoinNode JOIN 计划
type JoinNode struct {
	Left  PlanNode
	Right PlanNode
	Type  JoinType
	On    Expression
}

func (n *JoinNode) planNode() {}
func (n *JoinNode) String() string { return fmt.Sprintf("%s JOIN", n.Type) }

// ========== DDL 计划节点 ==========

// CreateTablePlan CREATE TABLE 计划
type CreateTablePlan struct {
	Stmt *CreateTableStmt
}

func (n *CreateTablePlan) planNode() {}
func (n *CreateTablePlan) String() string { return "CreateTable" }

// DropTablePlan DROP TABLE 计划
type DropTablePlan struct {
	Stmt *DropTableStmt
}

func (n *DropTablePlan) planNode() {}
func (n *DropTablePlan) String() string { return "DropTable" }

// AlterTablePlan ALTER TABLE 计划
type AlterTablePlan struct {
	Stmt *AlterTableStmt
}

func (n *AlterTablePlan) planNode() {}
func (n *AlterTablePlan) String() string { return "AlterTable" }

// ShowTablesPlan SHOW TABLES 计划
type ShowTablesPlan struct{}

func (n *ShowTablesPlan) planNode() {}
func (n *ShowTablesPlan) String() string { return "ShowTables" }

// DescribeTablePlan DESCRIBE TABLE 计划
type DescribeTablePlan struct {
	Stmt *DescribeTableStmt
}

func (n *DescribeTablePlan) planNode() {}
func (n *DescribeTablePlan) String() string { return "DescribeTable" }

// CreateIndexPlan CREATE INDEX 计划
type CreateIndexPlan struct {
	Stmt *CreateIndexStmt
}

func (n *CreateIndexPlan) planNode()      {}
func (n *CreateIndexPlan) String() string { return "CreateIndex" }

// DropIndexPlan DROP INDEX 计划
type DropIndexPlan struct {
	Stmt *DropIndexStmt
}

func (n *DropIndexPlan) planNode()      {}
func (n *DropIndexPlan) String() string { return "DropIndex" }

// ShowIndexesPlan SHOW INDEXES 计划
type ShowIndexesPlan struct {
	Stmt *ShowIndexesStmt
}

func (n *ShowIndexesPlan) planNode()      {}
func (n *ShowIndexesPlan) String() string { return "ShowIndexes" }

// ========== DDL 计划生成 ==========

func (p *Planner) planCreateTable(s *CreateTableStmt) (PlanNode, error) {
	return &CreateTablePlan{Stmt: s}, nil
}

func (p *Planner) planDropTable(s *DropTableStmt) (PlanNode, error) {
	return &DropTablePlan{Stmt: s}, nil
}

func (p *Planner) planAlterTable(s *AlterTableStmt) (PlanNode, error) {
	return &AlterTablePlan{Stmt: s}, nil
}

func (p *Planner) planShowTables(s *ShowTablesStmt) (PlanNode, error) {
	return &ShowTablesPlan{}, nil
}

func (p *Planner) planDescribeTable(s *DescribeTableStmt) (PlanNode, error) {
	return &DescribeTablePlan{Stmt: s}, nil
}

// ========== 索引选择 ==========

// tryIndexScan 尝试将 WHERE 分解为 (indexScan, remainingFilter)。
// 目前支持：
//   - col = literal        → Hash / BTree 等值
//   - col <op> literal     → BTree 范围（<, <=, >, >=）
//   - low <= col <= high   → BTree 范围
//   - BETWEEN              → BTree 范围
//   - MATCH(col) AGAINST   → Inverted
//   - AND 组合：从中挑选一个可用索引子谓词，其他作为 remain
func (p *Planner) tryIndexScan(from *TableRef, where Expression) (PlanNode, Expression) {
	conjuncts := splitAnd(where)
	for i, c := range conjuncts {
		if node := p.matchIndexPredicate(from, c); node != nil {
			// 剩余谓词
			rest := make([]Expression, 0, len(conjuncts)-1)
			rest = append(rest, conjuncts[:i]...)
			rest = append(rest, conjuncts[i+1:]...)
			return node, joinAnd(rest)
		}
	}
	return nil, where
}

func (p *Planner) matchIndexPredicate(from *TableRef, e Expression) PlanNode {
	// MATCH(col) AGAINST ('...')
	if m, ok := e.(*MatchExpr); ok && len(m.Columns) > 0 {
		col := m.Columns[0]
		if idx := p.pickIndex(from.Name, col, "MATCH"); idx != nil {
			return &IndexScanNode{
				Table:      from.Name,
				Alias:      from.Alias,
				IndexName:  idx.Name,
				Column:     col,
				Op:         IndexOpMatch,
				MatchQuery: m.Query,
			}
		}
		return nil
	}

	// col = literal
	if b, ok := e.(*BinaryExpr); ok {
		if col, lit, op, swapped := extractColLiteral(b); col != "" {
			// 只支持匹配 from 表（无别名限定或匹配别名）的列
			_ = swapped
			switch op {
			case TOKEN_EQ:
				if idx := p.pickIndex(from.Name, col, "EQ"); idx != nil {
					return &IndexScanNode{
						Table: from.Name, Alias: from.Alias,
						IndexName: idx.Name, Column: col,
						Op: IndexOpEqual, Equal: lit,
					}
				}
			case TOKEN_LT, TOKEN_LTE, TOKEN_GT, TOKEN_GTE:
				if idx := p.pickIndex(from.Name, col, "RANGE"); idx != nil && idx.Type != "HASH" {
					node := &IndexScanNode{
						Table: from.Name, Alias: from.Alias,
						IndexName: idx.Name, Column: col,
						Op: IndexOpRange,
					}
					// 若 col 在左侧则 op 保持；若右侧则反转
					effOp := op
					if swapped {
						effOp = flipCompareOp(op)
					}
					switch effOp {
					case TOKEN_LT:
						node.High = lit
						node.IncludeHigh = false
					case TOKEN_LTE:
						node.High = lit
						node.IncludeHigh = true
					case TOKEN_GT:
						node.Low = lit
						node.IncludeLow = false
					case TOKEN_GTE:
						node.Low = lit
						node.IncludeLow = true
					}
					return node
				}
			}
		}
	}

	// BETWEEN
	if be, ok := e.(*BetweenExpr); ok && !be.Not {
		if id, colOk := be.Expr.(*IdentifierExpr); colOk {
			col := id.Name
			low := evalLiteral(be.Low)
			high := evalLiteral(be.High)
			if idx := p.pickIndex(from.Name, col, "RANGE"); idx != nil && idx.Type != "HASH" {
				return &IndexScanNode{
					Table: from.Name, Alias: from.Alias,
					IndexName: idx.Name, Column: col,
					Op:  IndexOpRange,
					Low: low, High: high,
					IncludeLow: true, IncludeHigh: true,
				}
			}
		}
	}
	return nil
}

// pickIndex 从目录中挑一个最匹配用途的索引
// usage: "EQ" | "RANGE" | "MATCH"
func (p *Planner) pickIndex(table, column, usage string) *IndexInfo {
	if p.catalog == nil {
		return nil
	}
	metas := p.catalog.FindByColumn(table, column)
	if len(metas) == 0 {
		return nil
	}
	prefer := ""
	switch usage {
	case "EQ":
		prefer = "HASH"
	case "RANGE":
		prefer = "BTREE"
	case "MATCH":
		prefer = "INVERTED"
	}
	// 优先匹配偏好类型
	for i := range metas {
		if metas[i].Type == prefer {
			return &metas[i]
		}
	}
	// EQ / RANGE 可以互相回退到 BTREE / HASH（MATCH 不能）
	if usage == "MATCH" {
		return nil
	}
	for i := range metas {
		if metas[i].Type == "BTREE" || metas[i].Type == "HASH" {
			return &metas[i]
		}
	}
	return nil
}

// splitAnd 将 (a AND b AND c) 拆成 [a, b, c]
func splitAnd(e Expression) []Expression {
	if b, ok := e.(*BinaryExpr); ok && b.Op == TOKEN_AND {
		return append(splitAnd(b.Left), splitAnd(b.Right)...)
	}
	return []Expression{e}
}

// joinAnd 将 [a, b, c] 组回 (a AND b AND c)；空返回 nil
func joinAnd(es []Expression) Expression {
	if len(es) == 0 {
		return nil
	}
	acc := es[0]
	for i := 1; i < len(es); i++ {
		acc = &BinaryExpr{Op: TOKEN_AND, Left: acc, Right: es[i]}
	}
	return acc
}

// extractColLiteral 从 col <op> literal 或 literal <op> col 中提取列名和字面量
// 返回 (column, literalValue, op, swapped)
func extractColLiteral(b *BinaryExpr) (string, any, TokenType, bool) {
	if id, ok := b.Left.(*IdentifierExpr); ok {
		if lit, ok := literalOf(b.Right); ok {
			return id.Name, lit, b.Op, false
		}
	}
	if id, ok := b.Right.(*IdentifierExpr); ok {
		if lit, ok := literalOf(b.Left); ok {
			return id.Name, lit, b.Op, true
		}
	}
	return "", nil, 0, false
}

// literalOf 判断表达式是否是字面量并返回值
func literalOf(e Expression) (any, bool) {
	switch x := e.(type) {
	case *StringLiteralExpr:
		return x.Value, true
	case *IntegerLiteralExpr:
		return x.Value, true
	case *FloatLiteralExpr:
		return x.Value, true
	case *BoolLiteralExpr:
		return x.Value, true
	case *NullLiteralExpr:
		return nil, true
	}
	return nil, false
}

// flipCompareOp 反转比较运算符（用于 literal <op> col 形式）
func flipCompareOp(op TokenType) TokenType {
	switch op {
	case TOKEN_LT:
		return TOKEN_GT
	case TOKEN_LTE:
		return TOKEN_GTE
	case TOKEN_GT:
		return TOKEN_LT
	case TOKEN_GTE:
		return TOKEN_LTE
	}
	return op
}

// isIndexNode 检查节点是否是 IndexScanNode
func isIndexNode(n PlanNode) bool {
	_, ok := n.(*IndexScanNode)
	return ok
}
