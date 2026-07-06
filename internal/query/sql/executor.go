package sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/startvibecoding/AgentNativeDB/internal/query/sql/index"
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
	engine  storage.Engine
	tables  *storage.TableManager
	indexes *index.Manager
	planner *Planner
}

// NewExecutor 创建执行器
func NewExecutor(engine storage.Engine) *Executor {
	e := &Executor{
		engine:  engine,
		tables:  storage.NewTableManager(engine),
		indexes: index.NewManager(engine),
	}
	e.planner = NewPlannerWithCatalog(&executorCatalog{ex: e})
	return e
}

// Planner 返回与本执行器共享索引目录的计划器
func (e *Executor) Planner() *Planner {
	return e.planner
}

// executorCatalog 适配 index.Manager 为 IndexCatalog
type executorCatalog struct {
	ex *Executor
}

func (c *executorCatalog) FindByColumn(table, column string) []IndexInfo {
	metas := c.ex.indexes.FindByColumn(table, column)
	out := make([]IndexInfo, 0, len(metas))
	for _, m := range metas {
		out = append(out, IndexInfo{
			Name:   m.Name,
			Table:  m.Table,
			Column: m.Column,
			Type:   string(m.Type),
		})
	}
	return out
}

// Init 初始化执行器
func (e *Executor) Init(ctx context.Context) error {
	if err := e.tables.Init(ctx); err != nil {
		return err
	}
	return e.indexes.Init(ctx)
}

// Indexes 暴露索引管理器（供测试/外部使用）
func (e *Executor) Indexes() *index.Manager {
	return e.indexes
}

// Execute 执行查询计划
func (e *Executor) Execute(ctx context.Context, plan PlanNode) (*Result, error) {
	switch n := plan.(type) {
	case *ScanNode:
		return e.executeScan(ctx, n)
	case *IndexScanNode:
		return e.executeIndexScan(ctx, n)
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
	case *CreateIndexPlan:
		return e.executeCreateIndex(ctx, n)
	case *DropIndexPlan:
		return e.executeDropIndex(ctx, n)
	case *ShowIndexesPlan:
		return e.executeShowIndexes(ctx, n)
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
	meta, _ := e.tables.GetTable(table)
	if !isUserTableMeta(meta) {
		meta = nil
	}

	var affected int64
	for _, values := range node.Stmt.Values {
		row := make(map[string]any)
		columns := node.Stmt.Columns
		if len(columns) == 0 && meta != nil {
			columns = make([]string, 0, len(meta.Columns))
			for _, col := range meta.Columns {
				columns = append(columns, col.Name)
			}
		}
		if len(columns) != len(values) {
			return nil, fmt.Errorf("INSERT 值数量与列数量不匹配")
		}
		for i, col := range columns {
			if i < len(values) {
				row[col] = evalLiteral(values[i])
			}
		}

		// 生成 ID（如果未指定则自动生成）
		id := ""
		if v, ok := row["id"]; ok && v != nil {
			id = rowIDString(v)
		}
		if id == "" && tableHasColumn(meta, "id") {
			id = util.NewUUID()
			row["id"] = id
		}
		if meta != nil {
			var err error
			id, err = e.validateRowForWrite(ctx, meta, row, "", true)
			if err != nil {
				return nil, err
			}
		}
		if id == "" {
			id = util.NewUUID()
			row["id"] = id
		}

		data, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("marshal row: %w", err)
		}

		indexOps, err := e.indexes.InsertRowOps(ctx, table, row, id)
		if err != nil {
			return nil, fmt.Errorf("index insert: %w", err)
		}

		key := storage.EncodeKey(prefix, id)
		ops := make([]storage.WriteOp, 0, 1+len(indexOps))
		ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: key, Value: data})
		ops = append(ops, indexOps...)
		if err := e.engine.BatchWrite(ctx, ops); err != nil {
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
	meta, _ := e.tables.GetTable(table)
	if !isUserTableMeta(meta) {
		meta = nil
	}

	var affected int64
	for _, row := range scanResult.Rows {
		// 保存旧行用于索引更新
		oldRow := make(map[string]any, len(row.Values))
		for k, v := range row.Values {
			oldRow[k] = v
		}

		// 应用 SET 子句
		for _, set := range node.Stmt.Set {
			val := evalExpr(set.Value, row.Values)
			row.Values[set.Column] = val
		}

		// 获取行 ID
		rowIDColumn := "id"
		if meta != nil {
			rowIDColumn = rowIDColumnForMeta(meta)
		}
		oldID := rowIDString(oldRow[rowIDColumn])
		if oldID == "" {
			continue
		}
		id := rowIDString(row.Values[rowIDColumn])
		if meta != nil {
			var err error
			id, err = e.validateRowForWrite(ctx, meta, row.Values, oldID, false)
			if err != nil {
				return nil, err
			}
		}
		if id == "" {
			continue
		}

		data, err := json.Marshal(row.Values)
		if err != nil {
			continue
		}

		var indexOps []storage.WriteOp
		if id == oldID {
			indexOps, err = e.indexes.UpdateRowOps(ctx, table, oldRow, row.Values, id)
		} else {
			indexOps = e.indexes.DeleteRowOps(table, oldRow, oldID)
			insertOps, err := e.indexes.InsertRowOps(ctx, table, row.Values, id)
			if err == nil {
				indexOps = append(indexOps, insertOps...)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("index update: %w", err)
		}

		key := storage.EncodeKey(prefix, id)
		ops := make([]storage.WriteOp, 0, 2+len(indexOps))
		if id != oldID {
			ops = append(ops, storage.WriteOp{Type: storage.OpDelete, Key: storage.EncodeKey(prefix, oldID)})
		}
		ops = append(ops, storage.WriteOp{Type: storage.OpPut, Key: key, Value: data})
		ops = append(ops, indexOps...)
		if err := e.engine.BatchWrite(ctx, ops); err != nil {
			return nil, fmt.Errorf("update: %w", err)
		}
		affected++
	}

	return &Result{RowsAffected: affected}, nil
}

func (e *Executor) validateRowForWrite(ctx context.Context, meta *storage.TableMetadata, row map[string]any, oldID string, insert bool) (string, error) {
	columns := make(map[string]storage.ColumnMeta, len(meta.Columns))
	for _, col := range meta.Columns {
		columns[col.Name] = col
		if insert {
			if _, ok := row[col.Name]; !ok && col.Default != nil {
				row[col.Name] = col.Default
			}
		}
	}

	for name := range row {
		if _, ok := columns[name]; !ok {
			return "", fmt.Errorf("列 %s 不存在于表 %s", name, meta.Name)
		}
	}

	for _, col := range meta.Columns {
		val, exists := row[col.Name]
		if !exists {
			val = nil
		}
		if (col.PrimaryKey || !col.Nullable) && val == nil {
			constraint := "NOT NULL"
			if col.PrimaryKey {
				constraint = "PRIMARY KEY"
			}
			return "", fmt.Errorf("%s 约束失败: %s.%s", constraint, meta.Name, col.Name)
		}
		if val == nil {
			continue
		}
		if err := validateColumnValue(meta.Name, col, val); err != nil {
			return "", err
		}
	}

	rowIDColumn := rowIDColumnForMeta(meta)
	rowID := rowIDString(row[rowIDColumn])
	if rowID == "" {
		return "", fmt.Errorf("PRIMARY KEY 约束失败: %s.%s", meta.Name, rowIDColumn)
	}
	if rowID != oldID {
		key := storage.EncodeKey(meta.Prefix, rowID)
		if _, err := e.engine.Get(ctx, key); err == nil {
			return "", fmt.Errorf("PRIMARY KEY 约束失败: %s.%s", meta.Name, rowIDColumn)
		} else if !errors.Is(err, storage.ErrKeyNotFound) {
			return "", fmt.Errorf("检查 PRIMARY KEY 约束: %w", err)
		}
	}
	return rowID, nil
}

func validateColumnValue(table string, col storage.ColumnMeta, val any) error {
	switch strings.ToUpper(col.Type) {
	case "INT", "INTEGER":
		switch n := val.(type) {
		case int, int32, int64:
			return nil
		case float32:
			if math.Trunc(float64(n)) == float64(n) {
				return nil
			}
		case float64:
			if math.Trunc(n) == n {
				return nil
			}
		}
	case "FLOAT":
		if isNumeric(val) {
			return nil
		}
	case "BOOL", "BOOLEAN":
		if _, ok := val.(bool); ok {
			return nil
		}
	case "VARCHAR", "STRING", "TEXT":
		s, ok := val.(string)
		if !ok {
			break
		}
		if strings.EqualFold(col.Type, "VARCHAR") && col.Length > 0 && len(s) > col.Length {
			return fmt.Errorf("VARCHAR 长度约束失败: %s.%s 最大长度 %d", table, col.Name, col.Length)
		}
		return nil
	default:
		return nil
	}
	return fmt.Errorf("类型约束失败: %s.%s 需要 %s", table, col.Name, col.Type)
}

func tableHasColumn(meta *storage.TableMetadata, name string) bool {
	if meta == nil {
		return false
	}
	for _, col := range meta.Columns {
		if col.Name == name {
			return true
		}
	}
	return false
}

func isUserTableMeta(meta *storage.TableMetadata) bool {
	return meta != nil && meta.Prefix >= 0x30
}

func rowIDColumnForMeta(meta *storage.TableMetadata) string {
	for _, col := range meta.Columns {
		if col.PrimaryKey {
			return col.Name
		}
	}
	return "id"
}

func rowIDString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
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

		indexOps := e.indexes.DeleteRowOps(table, row.Values, id)
		key := storage.EncodeKey(prefix, id)
		ops := make([]storage.WriteOp, 0, 1+len(indexOps))
		ops = append(ops, storage.WriteOp{Type: storage.OpDelete, Key: key})
		ops = append(ops, indexOps...)
		if err := e.engine.BatchWrite(ctx, ops); err != nil {
			return nil, fmt.Errorf("delete: %w", err)
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
		ri := int64(toFloat(right))
		if ri == 0 {
			return nil
		}
		return float64(int64(toFloat(left)) % ri)
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

	// bool 相同类型直接比较
	if ba, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			switch {
			case ba == bb:
				return 0
			case !ba:
				return -1
			default:
				return 1
			}
		}
	}

	// 数值 vs 数值
	if isNumeric(a) && isNumeric(b) {
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

	// 数值 与 数字字符串混合：尝试统一成浮点
	if isNumeric(a) {
		if s, ok := b.(string); ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return compareFloat(toFloat(a), f)
			}
		}
		return -1 // 数值 小于 非可转换字符串
	}
	if isNumeric(b) {
		if s, ok := a.(string); ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return compareFloat(f, toFloat(b))
			}
		}
		return 1
	}

	// 字符串 vs 字符串
	if sa, ok := a.(string); ok {
		if sb, ok := b.(string); ok {
			return strings.Compare(sa, sb)
		}
	}

	// fallback: 按字符串化后比较
	return strings.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int32, int64, float32, float64:
		return true
	}
	return false
}

func compareFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
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
		if a.Column.Default != nil {
			action.(*storage.AddColumnAction).Column.Default = evalLiteral(a.Column.Default)
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
		if a.Column.Default != nil {
			action.(*storage.ModifyColumnAction).Column.Default = evalLiteral(a.Column.Default)
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

// ========== IndexScan 与 索引 DDL ==========

// executeIndexScan 使用二级索引获取候选 rowID，再回主键读取行数据
func (e *Executor) executeIndexScan(ctx context.Context, node *IndexScanNode) (*Result, error) {
	// 选索引
	var meta *index.Meta
	if node.IndexName != "" {
		meta, _ = e.indexes.Get(node.IndexName)
	}
	if meta == nil && node.Column != "" {
		metas := e.indexes.FindByColumn(node.Table, node.Column)
		if len(metas) > 0 {
			for _, m := range metas {
				switch node.Op {
				case IndexOpMatch:
					if m.Type == index.TypeInverted {
						meta = m
					}
				case IndexOpRange:
					if m.Type == index.TypeBTree {
						meta = m
					}
				case IndexOpEqual:
					if m.Type == index.TypeHash {
						meta = m
					}
				}
				if meta != nil {
					break
				}
			}
			if meta == nil {
				meta = metas[0]
			}
		}
	}
	if meta == nil {
		return e.executeScan(ctx, &ScanNode{Table: node.Table, Alias: node.Alias, Filter: node.Filter})
	}

	var rowIDs []string
	var err error
	switch node.Op {
	case IndexOpEqual:
		rowIDs, err = e.indexes.LookupEqual(ctx, meta, node.Equal)
	case IndexOpRange:
		rowIDs, err = e.indexes.LookupRange(ctx, meta, node.Low, node.High, node.IncludeLow, node.IncludeHigh)
	case IndexOpMatch:
		rowIDs, err = e.indexes.Match(ctx, meta, node.MatchQuery)
	}
	if err != nil {
		return nil, err
	}

	prefix := e.tablePrefix(node.Table)
	if prefix == 0 {
		return nil, fmt.Errorf("unknown table: %s", node.Table)
	}

	seen := make(map[string]struct{}, len(rowIDs))
	var rows []Row
	for _, id := range rowIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		key := storage.EncodeKey(prefix, id)
		val, err := e.engine.Get(ctx, key)
		if err != nil {
			continue
		}
		var values map[string]any
		if err := json.Unmarshal(val, &values); err != nil {
			continue
		}

		if node.Alias != "" {
			aliased := make(map[string]any, len(values))
			for k, v := range values {
				aliased[node.Alias+"."+k] = v
				aliased[k] = v
			}
			values = aliased
		}
		rows = append(rows, Row{Values: values})
	}

	if node.Filter != nil {
		rows = e.filterRows(rows, node.Filter)
	}

	columns := e.guessColumns(rows)
	return &Result{Columns: columns, Rows: rows}, nil
}
func (e *Executor) executeCreateIndex(ctx context.Context, node *CreateIndexPlan) (*Result, error) {
	stmt := node.Stmt

	// 校验表存在
	if _, ok := e.tables.GetTable(stmt.Table); !ok && e.tablePrefix(stmt.Table) == 0 {
		return nil, fmt.Errorf("table %s does not exist", stmt.Table)
	}

	meta := index.Meta{
		Name:   stmt.Name,
		Table:  stmt.Table,
		Column: stmt.Column,
		Type:   index.Type(strings.ToUpper(stmt.IndexType)),
		Unique: stmt.Unique,
	}
	if meta.Unique && meta.Type == index.TypeInverted {
		return nil, fmt.Errorf("unique inverted index is not supported")
	}
	if err := e.indexes.Create(ctx, meta, stmt.IfNotExists); err != nil {
		return nil, err
	}

	// 回填：全表扫描将已有行写入索引
	prefix := e.tablePrefix(stmt.Table)
	if prefix != 0 {
		start, end := storage.PrefixRange([]byte{prefix})
		iter, err := e.engine.Scan(ctx, start, end, storage.ScanOptions{})
		if err == nil {
			var rows []map[string]any
			var ids []string
			for iter.Next() {
				k, v := iter.Item()
				// 跳过二级索引条目（key 中含 0x00 分隔符）
				if len(k) > 1 && bytesContainsZero(k[1:]) {
					continue
				}
				var row map[string]any
				if err := json.Unmarshal(v, &row); err != nil {
					continue
				}
				id, _ := row["id"].(string)
				if id == "" {
					continue
				}
				rows = append(rows, row)
				ids = append(ids, id)
			}
			iter.Close()
			metaPtr, _ := e.indexes.Get(stmt.Name)
			if metaPtr != nil && len(rows) > 0 {
				if err := e.indexes.RebuildFromRows(ctx, metaPtr, rows, ids); err != nil {
					_ = e.indexes.Drop(ctx, stmt.Name, true)
					return nil, fmt.Errorf("backfill index: %w", err)
				}
			}
		}
	}

	return &Result{RowsAffected: 0}, nil
}

// executeDropIndex 执行 DROP INDEX
func (e *Executor) executeDropIndex(ctx context.Context, node *DropIndexPlan) (*Result, error) {
	if err := e.indexes.Drop(ctx, node.Stmt.Name, node.Stmt.IfExists); err != nil {
		return nil, err
	}
	return &Result{RowsAffected: 0}, nil
}

// executeShowIndexes 执行 SHOW INDEXES
func (e *Executor) executeShowIndexes(ctx context.Context, node *ShowIndexesPlan) (*Result, error) {
	var metas []*index.Meta
	if node.Stmt.Table != "" {
		metas = e.indexes.ListByTable(node.Stmt.Table)
	} else {
		metas = e.indexes.ListAll()
	}
	rows := make([]Row, 0, len(metas))
	for _, m := range metas {
		rows = append(rows, Row{Values: map[string]any{
			"name":   m.Name,
			"table":  m.Table,
			"column": m.Column,
			"type":   string(m.Type),
			"unique": m.Unique,
		}})
	}
	return &Result{
		Columns: []string{"name", "table", "column", "type", "unique"},
		Rows:    rows,
	}, nil
}

// bytesContainsZero 检查是否含 0x00 分隔符（用于区分主键 vs 索引项）
func bytesContainsZero(b []byte) bool {
	for _, c := range b {
		if c == 0x00 {
			return true
		}
	}
	return false
}
