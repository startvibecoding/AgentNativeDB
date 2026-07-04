package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// Row 查询结果行
type Row struct {
	Values map[string]any
}

// Result 查询结果
type Result struct {
	Columns []string
	Rows    []Row
	// INSERT/UPDATE/DELETE 返回
	RowsAffected int64
}

// Executor 查询执行器
type Executor struct {
	engine   storage.Engine
	tables   *storage.TableManager
}

// NewExecutor 创建执行器
func NewExecutor(engine storage.Engine) *Executor {
	return &Executor{
		engine: engine,
		tables: storage.NewTableManager(engine),
	}
}

// Init 初始化执行器
func (e *Executor) Init(ctx context.Context) error {
	return e.tables.Init(ctx)
}

// Execute 执行查询计划
func (e *Executor) Execute(ctx context.Context, plan PlanNode) (*Result, error) {
	switch n := plan.(type) {
	case *ScanNode:
		return e.executeScan(ctx, n)
	case *FilterNode:
		return e.executeFilter(ctx, n)
	case *ProjectNode:
		return e.executeProject(ctx, n)
	case *SortNode:
		return e.executeSort(ctx, n)
	case *LimitNode:
		return e.executeLimit(ctx, n)
	case *AggregateNode:
		return e.executeAggregate(ctx, n)
	case *InsertPlan:
		return e.executeInsert(ctx, n)
	case *UpdatePlan:
		return e.executeUpdate(ctx, n)
	case *DeletePlan:
		return e.executeDelete(ctx, n)
	case *JoinNode:
		return e.executeJoin(ctx, n)
	case *CreateTablePlan:
		return e.executeCreateTable(ctx, n)
	case *DropTablePlan:
		return e.executeDropTable(ctx, n)
	case *AlterTablePlan:
		return e.executeAlterTable(ctx, n)
	case *ShowTablesPlan:
		return e.executeShowTables(ctx, n)
	case *DescribeTablePlan:
		return e.executeDescribeTable(ctx, n)
	default:
		return nil, fmt.Errorf("unsupported plan node: %T", plan)
	}
}

// ========== 扫描 ==========

func (e *Executor) executeScan(ctx context.Context, node *ScanNode) (*Result, error) {
	// 根据表名确定前缀
	prefix := e.tablePrefix(node.Table)
	if prefix == 0 {
		return nil, fmt.Errorf("unknown table: %s", node.Table)
	}

	start, end := storage.PrefixRange([]byte{prefix})
	iter, err := e.engine.Scan(ctx, start, end, storage.ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", node.Table, err)
	}
	defer iter.Close()

	var rows []Row
	for iter.Next() {
		_, val := iter.Item()

		// 解析 JSON 数据
		var values map[string]any
		if err := json.Unmarshal(val, &values); err != nil {
			continue
		}

		// 应用表别名前缀
		if node.Alias != "" {
			aliased := make(map[string]any, len(values))
			for k, v := range values {
				aliased[node.Alias+"."+k] = v
				aliased[k] = v // 保留原始名
			}
			rows = append(rows, Row{Values: aliased})
		} else {
			rows = append(rows, Row{Values: values})
		}
	}

	// 应用过滤条件
	if node.Filter != nil {
		rows = e.filterRows(rows, node.Filter)
	}

	columns := e.guessColumns(rows)

	return &Result{Columns: columns, Rows: rows}, nil
}

// ========== 过滤 ==========

func (e *Executor) executeFilter(ctx context.Context, node *FilterNode) (*Result, error) {
	result, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	result.Rows = e.filterRows(result.Rows, node.Filter)
	return result, nil
}

func (e *Executor) filterRows(rows []Row, filter Expression) []Row {
	var filtered []Row
	for _, row := range rows {
		if evalBool(filter, row.Values) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// ========== 投影 ==========

func (e *Executor) executeProject(ctx context.Context, node *ProjectNode) (*Result, error) {
	result, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	// 检查是否是 SELECT *
	if len(node.Columns) == 1 {
		if _, ok := node.Columns[0].Expr.(*StarExpr); ok {
			return result, nil
		}
	}

	var columns []string
	var projected []Row

	for _, row := range result.Rows {
		newRow := Row{Values: make(map[string]any)}
		for _, col := range node.Columns {
			val := evalExpr(col.Expr, row.Values)
			name := col.Alias
			if name == "" {
				name = col.Expr.String()
			}
			newRow.Values[name] = val
			if len(projected) == 0 {
				columns = append(columns, name)
			}
		}
		projected = append(projected, newRow)
	}

	return &Result{Columns: columns, Rows: projected}, nil
}

// ========== 排序 ==========

func (e *Executor) executeSort(ctx context.Context, node *SortNode) (*Result, error) {
	result, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(result.Rows, func(i, j int) bool {
		for _, oby := range node.Order {
			vi := evalExpr(oby.Expr, result.Rows[i].Values)
			vj := evalExpr(oby.Expr, result.Rows[j].Values)
			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}
			if oby.Ascending {
				return cmp < 0
			}
			return cmp > 0
		}
		return false
	})

	return result, nil
}

// ========== 分页 ==========

func (e *Executor) executeLimit(ctx context.Context, node *LimitNode) (*Result, error) {
	result, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	// Offset
	if node.Offset > 0 && node.Offset < len(result.Rows) {
		result.Rows = result.Rows[node.Offset:]
	} else if node.Offset >= len(result.Rows) {
		result.Rows = nil
	}

	// Limit
	if node.Limit > 0 && node.Limit < len(result.Rows) {
		result.Rows = result.Rows[:node.Limit]
	}

	return result, nil
}

// ========== 聚合 ==========

func (e *Executor) executeAggregate(ctx context.Context, node *AggregateNode) (*Result, error) {
	result, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	// 无 GROUP BY 则全部聚合为一行
	if len(node.GroupBy) == 0 {
		aggs := e.computeAggregates(node.Aggs, result.Rows)
		return &Result{
			Columns: aggs.Columns,
			Rows:    []Row{{Values: aggs.Rows[0].Values}},
		}, nil
	}

	// 按 GROUP BY 分组
	groups := make(map[string][]Row)
	var groupOrder []string

	for _, row := range result.Rows {
		key := groupKey(row.Values, node.GroupBy)
		if _, exists := groups[key]; !exists {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], row)
	}

	var finalRows []Row
	for _, key := range groupOrder {
		groupRows := groups[key]
		aggs := e.computeAggregates(node.Aggs, groupRows)
		if len(aggs.Rows) > 0 {
			// 添加分组键
			for _, gexpr := range node.GroupBy {
				if ident, ok := gexpr.(*IdentifierExpr); ok {
					val := evalExpr(ident, groupRows[0].Values)
					aggs.Rows[0].Values[ident.String()] = val
				}
			}

			// HAVING 过滤
			if node.Having != nil {
				if !evalBool(node.Having, aggs.Rows[0].Values) {
					continue
				}
			}

			finalRows = append(finalRows, aggs.Rows[0])
		}
	}

	columns := e.guessColumns(finalRows)
	return &Result{Columns: columns, Rows: finalRows}, nil
}

// ========== JOIN ==========

func (e *Executor) executeJoin(ctx context.Context, node *JoinNode) (*Result, error) {
	leftResult, err := e.Execute(ctx, node.Left)
	if err != nil {
		return nil, err
	}

	rightResult, err := e.Execute(ctx, node.Right)
	if err != nil {
		return nil, err
	}

	var rows []Row
	var columns []string

	// 合并列名
	columns = append(columns, leftResult.Columns...)
	for _, col := range rightResult.Columns {
		// 避免重复列名
		exists := false
		for _, lc := range leftResult.Columns {
			if lc == col {
				exists = true
				break
			}
		}
		if !exists {
			columns = append(columns, col)
		}
	}

	switch node.Type {
	case JoinInner:
		// 嵌套循环 JOIN
		for _, leftRow := range leftResult.Rows {
			for _, rightRow := range rightResult.Rows {
				merged := mergeRows(leftRow, rightRow)
				if node.On != nil && !evalBool(node.On, merged.Values) {
					continue
				}
				rows = append(rows, merged)
			}
		}

	case JoinLeft:
		for _, leftRow := range leftResult.Rows {
			matched := false
			for _, rightRow := range rightResult.Rows {
				merged := mergeRows(leftRow, rightRow)
				if node.On != nil && !evalBool(node.On, merged.Values) {
					continue
				}
				rows = append(rows, merged)
				matched = true
			}
			if !matched {
				// LEFT JOIN: 保留左侧行，右侧填 NULL
				nullRow := Row{Values: make(map[string]any)}
				for _, col := range rightResult.Columns {
					nullRow.Values[col] = nil
				}
				rows = append(rows, mergeRows(leftRow, nullRow))
			}
		}
	}

	return &Result{Columns: columns, Rows: rows}, nil
}

func mergeRows(left, right Row) Row {
	merged := Row{Values: make(map[string]any, len(left.Values)+len(right.Values))}
	for k, v := range left.Values {
		merged.Values[k] = v
	}
	for k, v := range right.Values {
		merged.Values[k] = v
	}
	return merged
}

func (e *Executor) computeAggregates(cols []SelectColumn, rows []Row) *Result {
	resultRow := Row{Values: make(map[string]any)}
	var columns []string

	for _, col := range cols {
		name := col.Alias
		if name == "" {
			name = col.Expr.String()
		}
		columns = append(columns, name)

		if funcExpr, ok := col.Expr.(*FuncCallExpr); ok {
			switch strings.ToUpper(funcExpr.Name) {
			case "COUNT":
				if len(funcExpr.Args) > 0 {
					if _, ok := funcExpr.Args[0].(*StarExpr); ok {
						resultRow.Values[name] = int64(len(rows))
					} else {
						count := int64(0)
						for _, row := range rows {
							v := evalExpr(funcExpr.Args[0], row.Values)
							if v != nil {
								count++
							}
						}
						resultRow.Values[name] = count
					}
				} else {
					resultRow.Values[name] = int64(len(rows))
				}
			case "SUM":
				sum := 0.0
				for _, row := range rows {
					v := evalExpr(funcExpr.Args[0], row.Values)
					sum += toFloat(v)
				}
				resultRow.Values[name] = sum
			case "MIN":
				var min any
				for _, row := range rows {
					v := evalExpr(funcExpr.Args[0], row.Values)
					if min == nil || compareValues(v, min) < 0 {
						min = v
					}
				}
				resultRow.Values[name] = min
			case "MAX":
				var max any
				for _, row := range rows {
					v := evalExpr(funcExpr.Args[0], row.Values)
					if max == nil || compareValues(v, max) > 0 {
						max = v
					}
				}
				resultRow.Values[name] = max
			case "AVG":
				sum := 0.0
				for _, row := range rows {
					v := evalExpr(funcExpr.Args[0], row.Values)
					sum += toFloat(v)
				}
				if len(rows) > 0 {
					resultRow.Values[name] = sum / float64(len(rows))
				}
			case "FIRST":
				if len(rows) > 0 && len(funcExpr.Args) > 0 {
					resultRow.Values[name] = evalExpr(funcExpr.Args[0], rows[0].Values)
				}
			case "LAST":
				if len(rows) > 0 && len(funcExpr.Args) > 0 {
					resultRow.Values[name] = evalExpr(funcExpr.Args[0], rows[len(rows)-1].Values)
				}
			default:
				// 自定义函数 - 暂时返回 nil
				resultRow.Values[name] = nil
			}
		} else {
			// 非聚合列，取第一行的值
			if len(rows) > 0 {
				resultRow.Values[name] = evalExpr(col.Expr, rows[0].Values)
			}
		}
	}

	return &Result{Columns: columns, Rows: []Row{resultRow}}
}

// ========== INSERT ==========

func (e *Executor) executeInsert(ctx context.Context, node *InsertPlan) (*Result, error) {
	table := node.Stmt.Table
	prefix := e.tablePrefix(table)
	if prefix == 0 {
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	var affected int64
	for _, values := range node.Stmt.Values {
		row := make(map[string]any)
		for i, col := range node.Stmt.Columns {
			if i < len(values) {
				row[col] = evalLiteral(values[i])
			}
		}

		// 生成 ID（如果未指定则自动生成）
		id, ok := row["id"].(string)
		if !ok || id == "" {
			id = util.NewUUID()
			row["id"] = id
		}

		data, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("marshal row: %w", err)
		}

		key := storage.EncodeKey(prefix, id)
		if err := e.engine.Set(ctx, key, data); err != nil {
			return nil, fmt.Errorf("insert: %w", err)
		}
		affected++
	}

	return &Result{RowsAffected: affected}, nil
}

// ========== UPDATE ==========

func (e *Executor) executeUpdate(ctx context.Context, node *UpdatePlan) (*Result, error) {
	// 先扫描获取匹配的行
	scanResult, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	table := node.Stmt.Table
	prefix := e.tablePrefix(table)
	if prefix == 0 {
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	var affected int64
	for _, row := range scanResult.Rows {
		// 应用 SET 子句
		for _, set := range node.Stmt.Set {
			val := evalExpr(set.Value, row.Values)
			row.Values[set.Column] = val
		}

		// 获取 ID
		id, ok := row.Values["id"].(string)
		if !ok {
			continue
		}

		data, err := json.Marshal(row.Values)
		if err != nil {
			continue
		}

		key := storage.EncodeKey(prefix, id)
		if err := e.engine.Set(ctx, key, data); err != nil {
			continue
		}
		affected++
	}

	return &Result{RowsAffected: affected}, nil
}

// ========== DELETE ==========

func (e *Executor) executeDelete(ctx context.Context, node *DeletePlan) (*Result, error) {
	scanResult, err := e.Execute(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	table := node.Stmt.Table
	prefix := e.tablePrefix(table)
	if prefix == 0 {
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	var affected int64
	for _, row := range scanResult.Rows {
		id, ok := row.Values["id"].(string)
		if !ok {
			continue
		}

		key := storage.EncodeKey(prefix, id)
		if err := e.engine.Delete(ctx, key); err != nil {
			continue
		}
		affected++
	}

	return &Result{RowsAffected: affected}, nil
}

// ========== 辅助函数 ==========

// tablePrefix 返回表名对应的存储前缀
func (e *Executor) tablePrefix(table string) byte {
	// 先从 TableManager 中查找
	if prefix, exists := e.tables.GetTablePrefix(table); exists {
		return prefix
	}

	// 兼容旧的硬编码表名
	switch strings.ToLower(table) {
	case "agent_sessions", "sessions":
		return storage.PrefixSession
	case "agent_memories", "memories":
		return storage.PrefixMemory
	case "agent_decisions", "decisions":
		return storage.PrefixDecision
	case "knowledge_entities", "entities":
		return storage.PrefixEntity
	case "knowledge_relations", "relations":
		return storage.PrefixRelation
	case "data_lineage", "lineage":
		return storage.PrefixLineage
	default:
		return 0
	}
}

// guessColumns 从行数据中猜测列名
func (e *Executor) guessColumns(rows []Row) []string {
	if len(rows) == 0 {
		return nil
	}
	cols := make([]string, 0, len(rows[0].Values))
	for k := range rows[0].Values {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// evalExpr 求值表达式
func evalExpr(expr Expression, vars map[string]any) any {
	switch e := expr.(type) {
	case *IdentifierExpr:
		name := e.String()
		if v, ok := vars[name]; ok {
			return v
		}
		// 尝试不带表前缀
		if v, ok := vars[e.Name]; ok {
			return v
		}
		return nil
	case *StringLiteralExpr:
		return e.Value
	case *IntegerLiteralExpr:
		return e.Value
	case *FloatLiteralExpr:
		return e.Value
	case *BoolLiteralExpr:
		return e.Value
	case *NullLiteralExpr:
		return nil
	case *StarExpr:
		return "*"
	case *BinaryExpr:
		left := evalExpr(e.Left, vars)
		right := evalExpr(e.Right, vars)
		return evalBinaryOp(e.Op, left, right)
	case *UnaryExpr:
		val := evalExpr(e.Expr, vars)
		return evalUnaryOp(e.Op, val)
	case *FuncCallExpr:
		return evalFuncCall(e, vars)
	default:
		return nil
	}
}

func evalBool(expr Expression, vars map[string]any) bool {
	val := evalExpr(expr, vars)
	return toBool(val)
}

func evalBinaryOp(op TokenType, left, right any) any {
	switch op {
	case TOKEN_EQ:
		return compareValues(left, right) == 0
	case TOKEN_NEQ:
		return compareValues(left, right) != 0
	case TOKEN_LT:
		return compareValues(left, right) < 0
	case TOKEN_LTE:
		return compareValues(left, right) <= 0
	case TOKEN_GT:
		return compareValues(left, right) > 0
	case TOKEN_GTE:
		return compareValues(left, right) >= 0
	case TOKEN_AND:
		return toBool(left) && toBool(right)
	case TOKEN_OR:
		return toBool(left) || toBool(right)
	case TOKEN_PLUS:
		return toFloat(left) + toFloat(right)
	case TOKEN_MINUS:
		return toFloat(left) - toFloat(right)
	case TOKEN_MULTIPLY:
		return toFloat(left) * toFloat(right)
	case TOKEN_DIVIDE:
		r := toFloat(right)
		if r == 0 {
			return nil
		}
		return toFloat(left) / r
	case TOKEN_MODULO:
		return float64(int64(toFloat(left)) % int64(toFloat(right)))
	case TOKEN_VECTOR_DIST:
		return vectorDistance(left, right)
	}
	return nil
}

func evalUnaryOp(op TokenType, val any) any {
	switch op {
	case TOKEN_MINUS:
		return -toFloat(val)
	case TOKEN_NOT:
		return !toBool(val)
	}
	return val
}

func evalFuncCall(expr *FuncCallExpr, vars map[string]any) any {
	// 简单的内置函数
	switch strings.ToUpper(expr.Name) {
	case "UPPER":
		if len(expr.Args) > 0 {
			v := evalExpr(expr.Args[0], vars)
			if s, ok := v.(string); ok {
				return strings.ToUpper(s)
			}
		}
	case "LOWER":
		if len(expr.Args) > 0 {
			v := evalExpr(expr.Args[0], vars)
			if s, ok := v.(string); ok {
				return strings.ToLower(s)
			}
		}
	case "LENGTH":
		if len(expr.Args) > 0 {
			v := evalExpr(expr.Args[0], vars)
			if s, ok := v.(string); ok {
				return int64(len(s))
			}
		}
	case "COALESCE":
		for _, arg := range expr.Args {
			v := evalExpr(arg, vars)
			if v != nil {
				return v
			}
		}
	}
	return nil
}

func evalLiteral(expr Expression) any {
	switch e := expr.(type) {
	case *StringLiteralExpr:
		return e.Value
	case *IntegerLiteralExpr:
		return e.Value
	case *FloatLiteralExpr:
		return e.Value
	case *BoolLiteralExpr:
		return e.Value
	case *NullLiteralExpr:
		return nil
	}
	return nil
}

func compareValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// 字符串比较
	if sa, ok := a.(string); ok {
		sb, sOk := b.(string)
		if sOk {
			return strings.Compare(sa, sb)
		}
		// string vs non-string: string 总是大于非 string
		return 1
	}
	if _, ok := b.(string); ok {
		return -1
	}

	// 数值比较
	fa := toFloat(a)
	fb := toFloat(b)
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func toBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		return s != ""
	}
	if n, ok := v.(int64); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return true
}

func groupKey(vars map[string]any, groupBy []Expression) string {
	parts := make([]string, len(groupBy))
	for i, expr := range groupBy {
		val := evalExpr(expr, vars)
		parts[i] = fmt.Sprintf("%v", val)
	}
	return strings.Join(parts, "\x00")
}

// vectorDistance 计算向量距离
func vectorDistance(left, right any) float64 {
	lv := toFloatSlice(left)
	rv := toFloatSlice(right)
	if len(lv) == 0 || len(rv) == 0 || len(lv) != len(rv) {
		return -1
	}
	// 余弦距离
	var dot, normA, normB float64
	for i := range lv {
		dot += lv[i] * rv[i]
		normA += lv[i] * lv[i]
		normB += rv[i] * rv[i]
	}
	if normA == 0 || normB == 0 {
		return 1.0
	}
	return 1.0 - dot/(math.Sqrt(normA)*math.Sqrt(normB))
}

// toFloatSlice 将 any 转为 []float64
func toFloatSlice(v any) []float64 {
	switch val := v.(type) {
	case []float32:
		fs := make([]float64, len(val))
		for i, f := range val {
			fs[i] = float64(f)
		}
		return fs
	case []float64:
		return val
	case []any:
		fs := make([]float64, len(val))
		for i, x := range val {
			fs[i] = toFloat(x)
		}
		return fs
	}
	return nil
}

// ========== DDL 执行 ==========

// executeCreateTable 执行 CREATE TABLE
func (e *Executor) executeCreateTable(ctx context.Context, node *CreateTablePlan) (*Result, error) {
	stmt := node.Stmt

	// 转换列定义
	columns := make([]storage.ColumnMeta, len(stmt.Columns))
	for i, col := range stmt.Columns {
		columns[i] = storage.ColumnMeta{
			Name:       col.Name,
			Type:       col.Type.Name,
			Length:     col.Type.Length,
			Nullable:   col.Nullable,
			PrimaryKey: col.PrimaryKey,
		}
		if col.Default != nil {
			columns[i].Default = evalLiteral(col.Default)
		}
	}

	if err := e.tables.CreateTable(ctx, stmt.Table, columns, stmt.IfNotExists); err != nil {
		return nil, err
	}

	return &Result{RowsAffected: 0}, nil
}

// executeDropTable 执行 DROP TABLE
func (e *Executor) executeDropTable(ctx context.Context, node *DropTablePlan) (*Result, error) {
	stmt := node.Stmt

	if err := e.tables.DropTable(ctx, stmt.Table, stmt.IfExists); err != nil {
		return nil, err
	}

	return &Result{RowsAffected: 0}, nil
}

// executeAlterTable 执行 ALTER TABLE
func (e *Executor) executeAlterTable(ctx context.Context, node *AlterTablePlan) (*Result, error) {
	stmt := node.Stmt

	var action storage.AlterAction
	switch a := stmt.Action.(type) {
	case *AddColumnAction:
		action = &storage.AddColumnAction{
			Column: storage.ColumnMeta{
				Name:       a.Column.Name,
				Type:       a.Column.Type.Name,
				Length:     a.Column.Type.Length,
				Nullable:   a.Column.Nullable,
				PrimaryKey: a.Column.PrimaryKey,
			},
		}
	case *DropColumnAction:
		action = &storage.DropColumnAction{Column: a.Column}
	case *ModifyColumnAction:
		action = &storage.ModifyColumnAction{
			Column: storage.ColumnMeta{
				Name:       a.Column.Name,
				Type:       a.Column.Type.Name,
				Length:     a.Column.Type.Length,
				Nullable:   a.Column.Nullable,
				PrimaryKey: a.Column.PrimaryKey,
			},
		}
	default:
		return nil, fmt.Errorf("unsupported alter action: %T", stmt.Action)
	}

	if err := e.tables.AlterTable(ctx, stmt.Table, action); err != nil {
		return nil, err
	}

	return &Result{RowsAffected: 0}, nil
}

// executeShowTables 执行 SHOW TABLES
func (e *Executor) executeShowTables(ctx context.Context, node *ShowTablesPlan) (*Result, error) {
	tables := e.tables.ListTables()

	var rows []Row
	for _, name := range tables {
		rows = append(rows, Row{Values: map[string]any{"table_name": name}})
	}

	return &Result{
		Columns: []string{"table_name"},
		Rows:    rows,
	}, nil
}

// executeDescribeTable 执行 DESCRIBE TABLE
func (e *Executor) executeDescribeTable(ctx context.Context, node *DescribeTablePlan) (*Result, error) {
	stmt := node.Stmt

	meta, exists := e.tables.GetTable(stmt.Table)
	if !exists {
		return nil, fmt.Errorf("table %s does not exist", stmt.Table)
	}

	var rows []Row
	for _, col := range meta.Columns {
		row := Row{Values: map[string]any{
			"field":   col.Name,
			"type":    col.Type,
			"null":    col.Nullable,
			"key":     col.PrimaryKey,
			"default": col.Default,
		}}
		if col.Length > 0 {
			row.Values["type"] = fmt.Sprintf("%s(%d)", col.Type, col.Length)
		}
		rows = append(rows, row)
	}

	return &Result{
		Columns: []string{"field", "type", "null", "key", "default"},
		Rows:    rows,
	}, nil
}

