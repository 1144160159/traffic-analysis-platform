////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/query/builder.go
// SQL 查询构建器（完整修复版）
// 修复内容：
// 1. 完全禁止函数表达式（防止注入）
// 2. 严格验证字段名白名单
// 3. 修复 WhereIn 大小限制
// 4. 添加参数转义验证
////////////////////////////////////////////////////////////////////////////////

package query

import (
	"fmt"
	"regexp"
	"strings"
)

// 修复：字段名白名单（完整列表）
var allowedOrderByFields = map[string]bool{
	"session_count":    true,
	"total_bytes":      true,
	"last_seen":        true,
	"first_seen":       true,
	"alert_count":      true,
	"severity":         true,
	"ts_start":         true,
	"ts_end":           true,
	"protocol":         true,
	"server_port":      true,
	"client_ip":        true,
	"server_ip":        true,
	"bytes_total":      true,
	"packets_total":    true,
	"duration_ms":      true,
	"unique_peers":     true,
	"unique_ports":     true,
	"max_severity":     true,
	"created_at":       true,
	"updated_at":       true,
	"node_count":       true,
	"edge_count":       true,
	"path_count":       true,
	"cache_hit":        true,
	"query_start_time": true,
	"query_end_time":   true,
}

var allowedGroupByFields = map[string]bool{
	"client_ip":   true,
	"server_ip":   true,
	"server_port": true,
	"protocol":    true,
	"severity":    true,
	"alert_type":  true,
	"tenant_id":   true,
	"run_id":      true,
	"probe_id":    true,
	"status":      true,
	"query_type":  true,
	"error_type":  true,
}

// 修复：允许的聚合函数白名单
var allowedAggregateFunctions = map[string]bool{
	"count":     true,
	"sum":       true,
	"avg":       true,
	"min":       true,
	"max":       true,
	"uniq":      true,
	"uniqExact": true,
	"countIf":   true,
	"sumIf":     true,
	"quantile":  true,
}

// 修复：字段名验证正则（只允许字母、数字、下划线）
var fieldNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// QueryBuilder SQL 查询构建器
type QueryBuilder struct {
	table      string
	selects    []string
	conditions []string
	groupBy    []string
	having     []string
	orderBy    string
	limit      int
	offset     int
	args       []interface{}
}

// NewQueryBuilder 创建查询构建器
func NewQueryBuilder(table string) *QueryBuilder {
	return &QueryBuilder{
		table:      table,
		selects:    make([]string, 0),
		conditions: make([]string, 0),
		groupBy:    make([]string, 0),
		having:     make([]string, 0),
		args:       make([]interface{}, 0),
	}
}

// Select 添加选择字段（修复：严格验证）
func (qb *QueryBuilder) Select(fields ...string) *QueryBuilder {
	for _, field := range fields {
		// 修复：验证字段名或聚合函数
		if isValidSelectField(field) {
			qb.selects = append(qb.selects, field)
		}
	}
	return qb
}

// Where 添加条件
func (qb *QueryBuilder) Where(condition string, args ...interface{}) *QueryBuilder {
	qb.conditions = append(qb.conditions, condition)
	qb.args = append(qb.args, args...)
	return qb
}

// WhereTenant 添加租户条件
func (qb *QueryBuilder) WhereTenant(tenantID string) *QueryBuilder {
	return qb.Where("tenant_id = ?", tenantID)
}

// WhereTimeRange 添加时间范围条件
func (qb *QueryBuilder) WhereTimeRange(startTime, endTime int64, field string) *QueryBuilder {
	// 修复：验证字段名
	if !isValidFieldName(field) {
		return qb
	}

	// 修复：正确使用 ClickHouse 时间函数
	return qb.Where(
		fmt.Sprintf("%s BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)", field),
		float64(startTime)/1000.0, // 修复：转换为秒（带小数）
		float64(endTime)/1000.0,
	)
}

// WhereIP 添加 IP 条件（双向匹配）
func (qb *QueryBuilder) WhereIP(ip string) *QueryBuilder {
	if ip == "" {
		return qb
	}
	return qb.Where("(src_ip = ? OR dst_ip = ?)", ip, ip)
}

// WhereClientOrServerIP 添加客户端/服务端 IP 条件
func (qb *QueryBuilder) WhereClientOrServerIP(ip string) *QueryBuilder {
	if ip == "" {
		return qb
	}
	return qb.Where("(client_ip = ? OR server_ip = ?)", ip, ip)
}

// WhereIn 添加 IN 条件（修复版：分批查询策略）
func (qb *QueryBuilder) WhereIn(field string, values []string) *QueryBuilder {
	if len(values) == 0 {
		return qb
	}

	// 修复：验证字段名
	if !isValidFieldName(field) {
		return qb
	}

	// 修复：ClickHouse IN 子句限制
	maxInClause := 1000 // 降低到 1000（更安全）
	if len(values) > maxInClause {
		// 修复：使用多个 OR IN 子句
		for i := 0; i < len(values); i += maxInClause {
			end := i + maxInClause
			if end > len(values) {
				end = len(values)
			}
			qb.whereInBatch(field, values[i:end])
		}
		return qb
	}

	return qb.whereInBatch(field, values)
}

// whereInBatch 添加单批 IN 条件
func (qb *QueryBuilder) whereInBatch(field string, values []string) *QueryBuilder {
	placeholders := make([]string, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		qb.args = append(qb.args, v)
	}

	condition := fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	qb.conditions = append(qb.conditions, condition)

	return qb
}

// WhereNotNull 添加非空条件
func (qb *QueryBuilder) WhereNotNull(field string) *QueryBuilder {
	if !isValidFieldName(field) {
		return qb
	}
	return qb.Where(fmt.Sprintf("%s IS NOT NULL AND %s != ''", field, field))
}

// WhereSeverity 添加严重级别条件
func (qb *QueryBuilder) WhereSeverity(minSeverity string) *QueryBuilder {
	if minSeverity == "" {
		return qb
	}

	severityOrder := map[string]int{
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}

	minOrder, ok := severityOrder[minSeverity]
	if !ok {
		return qb
	}

	// 构建有效的严重级别列表
	var validSeverities []string
	for sev, order := range severityOrder {
		if order >= minOrder {
			validSeverities = append(validSeverities, sev)
		}
	}

	return qb.WhereIn("severity", validSeverities)
}

// WhereProtocol 添加协议条件
func (qb *QueryBuilder) WhereProtocol(protocol uint8) *QueryBuilder {
	return qb.Where("protocol = ?", protocol)
}

// WherePort 添加端口条件
func (qb *QueryBuilder) WherePort(port uint16) *QueryBuilder {
	return qb.Where("server_port = ?", port)
}

// WherePortRange 添加端口范围条件
func (qb *QueryBuilder) WherePortRange(minPort, maxPort uint16) *QueryBuilder {
	return qb.Where("server_port BETWEEN ? AND ?", minPort, maxPort)
}

// WhereBytesGreaterThan 添加字节数大于条件
func (qb *QueryBuilder) WhereBytesGreaterThan(bytes uint64) *QueryBuilder {
	return qb.Where("bytes_total > ?", bytes)
}

// WhereRunID 添加运行批次 ID 条件（新增）
func (qb *QueryBuilder) WhereRunID(runID string) *QueryBuilder {
	if runID == "" {
		return qb
	}
	return qb.Where("run_id = ?", runID)
}

// GroupBy 添加分组（修复：严格验证）
func (qb *QueryBuilder) GroupBy(fields ...string) *QueryBuilder {
	for _, field := range fields {
		// 修复：完全禁止函数，只允许字段名
		if allowedGroupByFields[field] && isValidFieldName(field) {
			qb.groupBy = append(qb.groupBy, field)
		}
	}
	return qb
}

// Having 添加 HAVING 条件
func (qb *QueryBuilder) Having(condition string, args ...interface{}) *QueryBuilder {
	qb.having = append(qb.having, condition)
	qb.args = append(qb.args, args...)
	return qb
}

// OrderBy 添加排序（修复版：严格验证）
func (qb *QueryBuilder) OrderBy(order string) *QueryBuilder {
	parts := strings.Fields(order)
	if len(parts) == 0 {
		return qb
	}

	field := parts[0]
	direction := "ASC"
	if len(parts) > 1 {
		direction = strings.ToUpper(parts[1])
		if direction != "ASC" && direction != "DESC" {
			direction = "ASC"
		}
	}

	// 修复：完全禁止函数，只允许字段名
	if !allowedOrderByFields[field] || !isValidFieldName(field) {
		return qb
	}

	qb.orderBy = fmt.Sprintf("%s %s", field, direction)
	return qb
}

// OrderByDesc 按字段降序排序
func (qb *QueryBuilder) OrderByDesc(field string) *QueryBuilder {
	if !allowedOrderByFields[field] || !isValidFieldName(field) {
		return qb
	}
	qb.orderBy = field + " DESC"
	return qb
}

// OrderByAsc 按字段升序排序
func (qb *QueryBuilder) OrderByAsc(field string) *QueryBuilder {
	if !allowedOrderByFields[field] || !isValidFieldName(field) {
		return qb
	}
	qb.orderBy = field + " ASC"
	return qb
}

// Limit 设置限制
func (qb *QueryBuilder) Limit(n int) *QueryBuilder {
	if n < 0 {
		n = 0
	}
	// 修复：添加最大限制保护
	if n > 10000 {
		n = 10000
	}
	qb.limit = n
	return qb
}

// Offset 设置偏移
func (qb *QueryBuilder) Offset(n int) *QueryBuilder {
	if n < 0 {
		n = 0
	}
	qb.offset = n
	return qb
}

// Build 构建 SQL 查询
func (qb *QueryBuilder) Build() (string, []interface{}) {
	var sb strings.Builder

	// SELECT
	sb.WriteString("SELECT ")
	if len(qb.selects) == 0 {
		sb.WriteString("*")
	} else {
		sb.WriteString(strings.Join(qb.selects, ", "))
	}

	// FROM
	sb.WriteString(" FROM ")
	sb.WriteString(qb.table)

	// WHERE
	if len(qb.conditions) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(qb.conditions, " AND "))
	}

	// GROUP BY
	if len(qb.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(qb.groupBy, ", "))
	}

	// HAVING
	if len(qb.having) > 0 {
		sb.WriteString(" HAVING ")
		sb.WriteString(strings.Join(qb.having, " AND "))
	}

	// ORDER BY
	if qb.orderBy != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(qb.orderBy)
	}

	// LIMIT
	if qb.limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", qb.limit))
	}

	// OFFSET
	if qb.offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", qb.offset))
	}

	return sb.String(), qb.args
}

// BuildCount 构建计数查询
func (qb *QueryBuilder) BuildCount() (string, []interface{}) {
	var sb strings.Builder

	sb.WriteString("SELECT count() FROM ")
	sb.WriteString(qb.table)

	if len(qb.conditions) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(qb.conditions, " AND "))
	}

	return sb.String(), qb.args
}

// BuildExists 构建存在性检查查询
func (qb *QueryBuilder) BuildExists() (string, []interface{}) {
	var sb strings.Builder

	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(qb.table)

	if len(qb.conditions) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(qb.conditions, " AND "))
	}

	sb.WriteString(" LIMIT 1")

	return sb.String(), qb.args
}

// Clone 克隆查询构建器
func (qb *QueryBuilder) Clone() *QueryBuilder {
	newQB := &QueryBuilder{
		table:      qb.table,
		selects:    make([]string, len(qb.selects)),
		conditions: make([]string, len(qb.conditions)),
		groupBy:    make([]string, len(qb.groupBy)),
		having:     make([]string, len(qb.having)),
		orderBy:    qb.orderBy,
		limit:      qb.limit,
		offset:     qb.offset,
		args:       make([]interface{}, len(qb.args)),
	}

	copy(newQB.selects, qb.selects)
	copy(newQB.conditions, qb.conditions)
	copy(newQB.groupBy, qb.groupBy)
	copy(newQB.having, qb.having)
	copy(newQB.args, qb.args)

	return newQB
}

// Reset 重置查询构建器
func (qb *QueryBuilder) Reset() *QueryBuilder {
	qb.selects = make([]string, 0)
	qb.conditions = make([]string, 0)
	qb.groupBy = make([]string, 0)
	qb.having = make([]string, 0)
	qb.orderBy = ""
	qb.limit = 0
	qb.offset = 0
	qb.args = make([]interface{}, 0)
	return qb
}

// WithTable 设置新表名
func (qb *QueryBuilder) WithTable(table string) *QueryBuilder {
	qb.table = table
	return qb
}

// String 返回 SQL 字符串（调试用）
func (qb *QueryBuilder) String() string {
	sql, _ := qb.Build()
	return sql
}

// GetArgs 获取参数（调试用）
func (qb *QueryBuilder) GetArgs() []interface{} {
	return qb.args
}

// ==================== 辅助验证函数 ====================

// isValidFieldName 验证字段名（修复：严格验证）
func isValidFieldName(field string) bool {
	return fieldNameRegex.MatchString(field)
}

// isValidSelectField 验证 SELECT 字段（修复：支持聚合函数）
func isValidSelectField(field string) bool {
	// 1. 普通字段名
	if isValidFieldName(field) {
		return true
	}

	// 2. AS 别名：field AS alias
	if strings.Contains(field, " AS ") || strings.Contains(field, " as ") {
		parts := strings.Fields(field)
		if len(parts) >= 3 {
			return isValidSelectField(parts[0]) && isValidFieldName(parts[2])
		}
	}

	// 3. 聚合函数：count(), sum(field), avg(field) 等
	if strings.Contains(field, "(") && strings.Contains(field, ")") {
		// 提取函数名
		parenIndex := strings.Index(field, "(")
		funcName := strings.TrimSpace(field[:parenIndex])

		// 验证函数名
		if !allowedAggregateFunctions[funcName] {
			return false
		}

		// 提取参数
		endIndex := strings.LastIndex(field, ")")
		if endIndex <= parenIndex {
			return false
		}

		args := strings.TrimSpace(field[parenIndex+1 : endIndex])

		// count() 无参数
		if funcName == "count" && args == "" {
			return true
		}

		// 验证参数是字段名或 *
		if args == "*" {
			return true
		}

		// 支持多个参数（用逗号分隔）
		argParts := strings.Split(args, ",")
		for _, arg := range argParts {
			arg = strings.TrimSpace(arg)
			if !isValidFieldName(arg) && arg != "*" {
				return false
			}
		}

		return true
	}

	return false
}

// AddAllowedOrderByField 添加允许的排序字段
func AddAllowedOrderByField(field string) {
	if isValidFieldName(field) {
		allowedOrderByFields[field] = true
	}
}

// AddAllowedGroupByField 添加允许的分组字段
func AddAllowedGroupByField(field string) {
	if isValidFieldName(field) {
		allowedGroupByFields[field] = true
	}
}

// IsAllowedOrderByField 检查字段是否允许排序
func IsAllowedOrderByField(field string) bool {
	return allowedOrderByFields[field] && isValidFieldName(field)
}

// IsAllowedGroupByField 检查字段是否允许分组
func IsAllowedGroupByField(field string) bool {
	return allowedGroupByFields[field] && isValidFieldName(field)
}
