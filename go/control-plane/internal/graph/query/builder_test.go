package query

import (
	"strings"
	"testing"
)

// TestQueryBuilderBasic 验证基本 SQL 构建
func TestQueryBuilderBasic(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").
		Select("client_ip", "server_ip", "count() as session_count").
		WhereTenant("test-tenant").
		Where("run_id = ?", "realtime").
		GroupBy("client_ip", "server_ip").
		OrderByDesc("session_count").
		Limit(10)

	sql, args := qb.Build()

	if !strings.Contains(sql, "FROM traffic.sessions") {
		t.Errorf("Missing FROM clause: %s", sql)
	}
	if !strings.Contains(sql, "WHERE") {
		t.Errorf("Missing WHERE clause: %s", sql)
	}
	if !strings.Contains(sql, "GROUP BY") {
		t.Errorf("Missing GROUP BY: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY") {
		t.Errorf("Missing ORDER BY: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT 10") {
		t.Errorf("Missing LIMIT: %s", sql)
	}

	if len(args) < 2 {
		t.Errorf("Expected at least 2 args, got %d", len(args))
	}
}

// TestQueryBuilderSQLInjection 验证 SQL 注入防护
func TestQueryBuilderSQLInjection(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").
		OrderBy("session_count; DROP TABLE sessions;--")

	sql, _ := qb.Build()

	// SQL injection attempt should be rejected
	if strings.Contains(strings.ToUpper(sql), "DROP") {
		t.Errorf("SQL injection not prevented: %s", sql)
	}
	if strings.Contains(sql, ";") {
		t.Errorf("SQL injection semicolon not prevented: %s", sql)
	}
	if strings.Contains(sql, "--") {
		t.Errorf("SQL injection comment not prevented: %s", sql)
	}

	// 合法字段名不应受影响
	qb2 := NewQueryBuilder("traffic.sessions").OrderByDesc("session_count")
	sql2, _ := qb2.Build()
	if !strings.Contains(sql2, "ORDER BY session_count DESC") {
		t.Errorf("Valid ORDER BY rejected: %s", sql2)
	}
}

// TestQueryBuilderFieldValidation 验证字段名白名单
func TestQueryBuilderFieldValidation(t *testing.T) {
	tests := []struct {
		field   string
		allowed bool
	}{
		{"session_count", true},
		{"total_bytes", true},
		{"last_seen", true},
		{"client_ip", true},
		{"server_ip", true},
		{"invalid_field", false},
		{"1=1", false},
		{"session_count--", false},
		{"session_count; DROP", false},
	}

	for _, tt := range tests {
		result := allowedOrderByFields[tt.field] && isValidFieldName(tt.field)
		if result != tt.allowed {
			t.Errorf("Field %q: allowed=%v, expected=%v", tt.field, result, tt.allowed)
		}
	}
}

// TestQueryBuilderGroupByValidation 验证 GROUP BY 白名单
func TestQueryBuilderGroupByValidation(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").
		GroupBy("client_ip", "invalid_hack; DROP")

	sql, _ := qb.Build()

	if strings.Contains(strings.ToUpper(sql), "DROP") {
		t.Error("SQL injection in GROUP BY not prevented")
	}

	// Only "client_ip" should pass validation
	if strings.Count(sql, "GROUP BY") == 1 {
		groupPart := sql[strings.Index(sql, "GROUP BY")+9:]
		if !strings.Contains(groupPart, "client_ip") {
			t.Error("Valid field missing from GROUP BY")
		}
	}
}

// TestQueryBuilderAggregateValidation 验证聚合函数白名单
func TestQueryBuilderAggregateValidation(t *testing.T) {
	tests := []struct {
		expr    string
		allowed bool
	}{
		{"count()", true},
		{"sum(total_bytes)", true},
		{"avg(duration_ms)", true},
		{"uniq(client_ip)", true},
		{"exec(rm -rf /)", false},
		{"DROP TABLE", false},
		{"sleep(10)", false},
	}

	for _, tt := range tests {
		result := isValidSelectField(tt.expr)
		if result != tt.allowed {
			t.Errorf("Expression %q: allowed=%v, expected=%v", tt.expr, result, tt.allowed)
		}
	}
}

// TestQueryBuilderWhereIn 验证 IN 子句分批
func TestQueryBuilderWhereIn(t *testing.T) {
	values := make([]string, 2500) // 超过默认 1000 限制
	for i := range values {
		values[i] = "ip-" + string(rune('0'+i%10))
	}

	qb := NewQueryBuilder("traffic.sessions").
		WhereIn("client_ip", values)

	sql, args := qb.Build()

	// Should be split into 3 batches (1000 + 1000 + 500)
	if strings.Count(sql, "IN (") < 2 {
		t.Errorf("Expected at least 2 IN clauses, got: %s", sql)
	}

	if len(args) != 2500 {
		t.Errorf("Expected 2500 args, got %d", len(args))
	}
}

// TestQueryBuilderBuildCount 验证 COUNT 查询
func TestQueryBuilderBuildCount(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").
		WhereTenant("test-tenant").
		WhereTimeRange(1717718400000, 1717804800000, "ts_end")

	sql, _ := qb.BuildCount()

	if !strings.Contains(sql, "SELECT count()") {
		t.Errorf("Expected SELECT count(), got: %s", sql)
	}
	if !strings.Contains(sql, "WHERE") {
		t.Errorf("Missing WHERE in count query: %s", sql)
	}
}

// TestQueryBuilderClone 验证克隆
func TestQueryBuilderClone(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").
		Select("client_ip").
		WhereTenant("t1").
		Limit(5)

	clone := qb.Clone()

	// Clone should have same SQL
	sql1, _ := qb.Build()
	sql2, _ := clone.Build()
	if sql1 != sql2 {
		t.Errorf("Clone mismatch:\n  original: %s\n  clone:    %s", sql1, sql2)
	}

	// Modifying clone should not affect original
	clone.Where("extra = ?", "value")
	sql1After, _ := qb.Build()
	if sql1 != sql1After {
		t.Error("Original was modified after clone change")
	}
}

// TestQueryBuilderLimit 验证限制保护
func TestQueryBuilderLimit(t *testing.T) {
	qb := NewQueryBuilder("traffic.sessions").Limit(50000) // exceeds max
	sql, _ := qb.Build()

	if strings.Contains(sql, "LIMIT 50000") {
		t.Error("Limit exceeding max should be capped")
	}
	if !strings.Contains(sql, "LIMIT 10000") {
		t.Errorf("Expected LIMIT 10000, got: %s", sql)
	}
}

// TestIsValidFieldName 验证字段名校验
func TestIsValidFieldName(t *testing.T) {
	tests := []struct {
		field string
		valid bool
	}{
		{"session_count", true},
		{"total_bytes", true},
		{"client_ip", true},
		{"", false},
		{"1field", false},
		{"field-name", false},
		{"field name", false},
		{"field;drop", false},
		{"field(name)", false},
	}

	for _, tt := range tests {
		result := isValidFieldName(tt.field)
		if result != tt.valid {
			t.Errorf("isValidFieldName(%q) = %v, want %v", tt.field, result, tt.valid)
		}
	}
}
