// Package sqlbuild 提供最小化、参数化的 SQL 构造助手,消化散落在各
// repository/handler 中的 "SELECT ... WHERE " + strings.Join(...) 字符串拼接。
//
// 安全边界:本包只负责结构化拼接,**不负责注入防护**——占位符与实参仍由
// 调用方配对传入(database/sql 参数化);标识符(表名/列名)如需动态传入,
// 调用方必须先经 sqlIdent 类白名单校验(见 internal/alert/api 的用法)。
package sqlbuild

import (
	"strconv"
	"strings"
)

// Placeholder 占位符方言。
type Placeholder int

const (
	// PlaceholderDollar PostgreSQL 风格: $1, $2, ...
	PlaceholderDollar Placeholder = iota
	// PlaceholderQuestion ClickHouse 风格: ?, ?, ...
	PlaceholderQuestion
)

// Builder 参数化 SQL 构造器。
type Builder struct {
	style Placeholder
}

// New 按方言创建构造器。
func New(style Placeholder) *Builder {
	return &Builder{style: style}
}

// Placeholder 返回第 n(1-based)个占位符文本。
func (b *Builder) Placeholder(n int) string {
	if b.style == PlaceholderQuestion {
		return "?"
	}
	return "$" + strconv.Itoa(n)
}

// CountFrom 生成 "SELECT COUNT(*) FROM <table>"。
func (b *Builder) CountFrom(table string) string {
	return "SELECT COUNT(*) FROM " + table
}

// Where 拼接条件:无条件时返回空串,有条件时返回 " WHERE a AND b"。
func Where(conditions []string) string {
	conditions = nonEmpty(conditions)
	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

// Eq 生成 "<column> = <placeholder>"。
func (b *Builder) Eq(column string, n int) string {
	return column + " = " + b.Placeholder(n)
}

// ILike 生成 "<column> ILIKE <placeholder>"(PG 方言)。
func (b *Builder) ILike(column string, n int) string {
	return column + " ILIKE " + b.Placeholder(n)
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
