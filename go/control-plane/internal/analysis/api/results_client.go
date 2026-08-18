// Package api §20 运行详情"结果"页签的 ClickHouse 读侧客户端:
// GET /runs/{id}/results 经 CH HTTP 接口读取 analysis_run_results(阶段结果)
// 与 analysis_detections(每输入×检测器处置);未配置时 fail-closed 503。
// 查询一律用 CH 命名参数({name:String} + param_name URL 参数),杜绝字符串拼接注入。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RunResultsClient CH HTTP 只读客户端(JSONEachRow)。
type RunResultsClient struct {
	baseURL  string
	user     string
	password string
	client   *http.Client
}

// NewRunResultsClient 装配 CH 只读客户端;baseURL 如 http://clickhouse-1.middleware.svc:8123。
func NewRunResultsClient(baseURL, user, password string, timeout time.Duration) *RunResultsClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &RunResultsClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		user:     user,
		password: password,
		client:   &http.Client{Timeout: timeout},
	}
}

// Query 执行只读 SQL(JSONEachRow),返回逐行 map。
// SQL 中使用 {name:String} 占位,params 提供值(经 CH param_name 参数化,无拼接)。
func (c *RunResultsClient) Query(ctx context.Context, sql string, params map[string]string) ([]map[string]interface{}, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("clickhouse results reader is not configured")
	}
	for name, value := range params {
		if !safeParam(value) {
			return nil, fmt.Errorf("invalid query parameter %q", name)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", strings.NewReader(sql))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-ClickHouse-User", c.user)
	req.Header.Set("X-ClickHouse-Key", c.password)
	q := req.URL.Query()
	q.Set("database", "traffic")
	q.Set("default_format", "JSONEachRow")
	for name, value := range params {
		q.Set("param_"+name, value)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse query status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var rows []map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return nil, fmt.Errorf("clickhouse row decode: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// safeParam 参数白名单:短 ASCII 标识(租户/run_id 形态),拒绝引号与空白。
func safeParam(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
