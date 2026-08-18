// Package service 5 字段 cron 解析(§76.45.1 TriggerKind=CRON_WINDOW 支撑):
// minute hour day-of-month month day-of-week(0=周日),支持 *,列表,范围,步进。
// 不支持秒/年/DST 双写;超出范围返回错误(fail-closed,不猜测)。
package service

import (
	"fmt"
	"time"
)

// cronField 单字段解析结果(命中集合)。
type cronField struct {
	values map[int]bool
	step   int
}

// parseCronField 解析 "*"|"a"|"a,b"|"a-b"|"*/n"|"a-b/n"(范围内值)。
func parseCronField(field string, minV, maxV int) (cronField, error) {
	if field == "" {
		return cronField{}, fmt.Errorf("empty cron field")
	}
	values := map[int]bool{}
	step := 1
	body := field
	if i := indexByte(body, '/'); i >= 0 {
		if _, err := fmt.Sscanf(body[i+1:], "%d", &step); err != nil || step <= 0 {
			return cronField{}, fmt.Errorf("cron step must be a positive integer in %q", field)
		}
		body = body[:i]
	}
	addRange := func(a, b int) error {
		if a < minV || b > maxV || a > b {
			return fmt.Errorf("cron range %d-%d out of [%d,%d]", a, b, minV, maxV)
		}
		for v := a; v <= b; v += step {
			values[v] = true
		}
		return nil
	}
	switch {
	case body == "*":
		if err := addRange(minV, maxV); err != nil {
			return cronField{}, err
		}
	case indexByte(body, '-') >= 0:
		var a, b int
		if _, err := fmt.Sscanf(body, "%d-%d", &a, &b); err != nil {
			return cronField{}, fmt.Errorf("cron range malformed: %q", field)
		}
		if err := addRange(a, b); err != nil {
			return cronField{}, err
		}
	default:
		for _, part := range splitComma(body) {
			var v int
			if _, err := fmt.Sscanf(part, "%d", &v); err != nil || v < minV || v > maxV {
				return cronField{}, fmt.Errorf("cron value %q out of [%d,%d]", part, minV, maxV)
			}
			values[v] = true
		}
	}
	return cronField{values: values, step: step}, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}

// CronSchedule 解析后的 5 字段 cron。
type CronSchedule struct {
	minute, hour, dom, month, dow cronField
}

// ParseCron 解析 5 字段 cron(分 时 日 月 周)。
func ParseCron(spec string) (*CronSchedule, error) {
	fields := splitSpace(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron spec must have 5 fields, got %d: %q", len(fields), spec)
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, err
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, err
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, err
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, err
	}
	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, err
	}
	return &CronSchedule{minute: minute, hour: hour, dom: dom, month: month, dow: dow}, nil
}

func splitSpace(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (c *CronSchedule) matches(t time.Time) bool {
	// 日与周的语义:两者都被约束时取并集(标准 cron 语义)。
	domOK := c.dom.values[t.Day()]
	dowOK := c.dow.values[int(t.Weekday())]
	dayOK := domOK || dowOK
	return c.minute.values[t.Minute()] && c.hour.values[t.Hour()] && dayOK && c.month.values[int(t.Month())]
}

// NextFire 返回 after 之后(不含)的第一个触发时刻(UTC;调度时区由调用方平移)。
func (c *CronSchedule) NextFire(after time.Time) (time.Time, error) {
	t := after.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if c.matches(t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron has no occurrence within one year")
}

// PrevFireOrNow 返回 after 之前(含)最近的触发时刻(滚动窗口对齐用)。
func (c *CronSchedule) PrevFireOrNow(after time.Time) (time.Time, error) {
	t := after.Truncate(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if c.matches(t) {
			return t, nil
		}
		t = t.Add(-time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron has no occurrence within one year before %v", after)
}
