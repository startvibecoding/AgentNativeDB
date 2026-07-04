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
	IndexName string
	Key       string // 索引查找的值
	Filter    Expression
}

func (n *IndexScanNode) planNode() {}
func (n *IndexScanNode) String() string { return fmt.Sprintf("IndexScan(%s, %s)", n.Table, n.IndexName) }

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
type Planner struct{}

// NewPlanner 创建计划器
func NewPlanner() *Planner {
	return &Planner{}
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
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}
}

func (p *Planner) planSelect(s *SelectStmt) (PlanNode, error) {
	if s.From == nil {
		return nil, fmt.Errorf("SELECT without FROM is not supported")
	}

	// 1. 扫描节点
	var node PlanNode = &ScanNode{
		Table: s.From.Name,
		Alias: s.From.Alias,
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

	// 2. WHERE 过滤（谓词下推）
	if s.Where != nil {
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
