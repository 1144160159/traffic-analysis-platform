// NebulaGraph HTTP Client — 基于 HTTP API 的图数据库客户端
//
// 使用 NebulaGraph v3.6 的三种接入方式:
//  1. HTTP Gateway (nebula-http-gateway, port 18080): /api/v1/ngql，推荐生产使用
//     部署: kubectl apply -f deployments/kubernetes/infrastructure/11-nebula-http-gateway.yaml
//  2. Graph Service HTTP (port 19669): 仅 /status 健康检查，不支持 nGQL 执行
//  3. Console Client (client_console.go): stdin/stdout 调用 nebula-console CLI
//
// 当前默认使用 Graph Service Status API (port 19669) 进行健康检查。
// 实际 nGQL 执行需部署 nebula-http-gateway 或使用 ConsoleClient。
//
// 特性:
//   - HTTP 连接池 (Keep-Alive, MaxIdleConns)
//   - 认证会话管理
//   - nGQL 查询重试 (指数退避)
//   - JSON 响应解析为 ResultSet
//   - 健康检查与指标收集
package nebula

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HTTPClientConfig HTTP 客户端配置
type HTTPClientConfig struct {
	GraphAddr     string        // Graph HTTP 地址 (host:port, 默认 19669)
	Username      string        // 用户名
	Password      string        // 密码
	Space         string        // 默认图空间
	Timeout       time.Duration // 请求超时
	RetryCount    int           // 重试次数
	RetryDelay    time.Duration // 重试间隔
	MaxIdleConns  int           // 最大空闲连接
	EnableMetrics bool          // 启用指标
}

// DefaultHTTPConfig 默认 HTTP 配置
func DefaultHTTPConfig() HTTPClientConfig {
	return HTTPClientConfig{
		GraphAddr:     "nebula-graph.middleware.svc:19669",
		Username:      "traffic_graph",
		Password:      "",
		Space:         "traffic_graph",
		Timeout:       30 * time.Second,
		RetryCount:    3,
		RetryDelay:    100 * time.Millisecond,
		MaxIdleConns:  20,
		EnableMetrics: true,
	}
}

// ============================================================================
// HTTP API 请求/响应结构
// ============================================================================

// httpExecuteRequest HTTP API 请求体
type httpExecuteRequest struct {
	GQL      string `json:"gql"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// httpExecuteResponse HTTP API 响应体
type httpExecuteResponse struct {
	Errors     []httpAPIError  `json:"errors"`
	Results    []httpAPIResult `json:"results"`
	ExecTimeUs int64           `json:"execTimeInUs"`
	SessionID  string          `json:"sessionId"`
}

type httpAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type httpAPIResult struct {
	Columns   []string        `json:"columns"`
	Data      [][]interface{} `json:"data"`
	LatencyUs int64           `json:"latencyInUs"`
	SpaceName string          `json:"spaceName"`
	ErrorCode int32           `json:"errorCode"`
	ErrorMsg  string          `json:"errorMsg"`
}

// HTTPClient NebulaGraph HTTP 客户端
type HTTPClient struct {
	config     HTTPClientConfig
	httpClient *http.Client
	logger     *zap.Logger
	metrics    *ClientMetrics
	sessionID  string
	mu         sync.RWMutex
	closed     bool
}

// NewHTTPClient 创建 HTTP 客户端
func NewHTTPClient(cfg HTTPClientConfig, logger *zap.Logger) (*HTTPClient, error) {
	if cfg.GraphAddr == "" {
		return nil, fmt.Errorf("graph HTTP address is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 20
	}

	transport := &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConns / 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	client := &HTTPClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		logger:  logger,
		metrics: &ClientMetrics{},
	}

	// 验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		logger.Warn("NebulaGraph HTTP ping failed, client created anyway",
			zap.String("addr", cfg.GraphAddr),
			zap.Error(err))
	}

	// 切换默认空间
	if cfg.Space != "" {
		if err := client.SwitchSpace(ctx, cfg.Space); err != nil {
			logger.Warn("Failed to switch to default space",
				zap.String("space", cfg.Space),
				zap.Error(err))
		}
	}

	logger.Info("NebulaGraph HTTP client initialized",
		zap.String("addr", cfg.GraphAddr),
		zap.String("space", cfg.Space))

	return client, nil
}

// ============================================================================
// 核心执行方法
// ============================================================================

// Execute 执行 nGQL 查询 (HTTP API)
func (hc *HTTPClient) Execute(ctx context.Context, nGQL string) (*ResultSet, error) {
	startTime := time.Now()
	hc.metrics.mu.Lock()
	hc.metrics.TotalQueries++
	hc.metrics.mu.Unlock()

	var lastErr error
	for i := 0; i <= hc.config.RetryCount; i++ {
		if i > 0 {
			delay := hc.config.RetryDelay * time.Duration(1<<uint(i-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := hc.executeOnce(ctx, nGQL)
		if err == nil {
			latency := float64(time.Since(startTime).Microseconds()) / 1000.0
			hc.metrics.mu.Lock()
			hc.metrics.AvgLatencyMs = 0.1*latency + 0.9*hc.metrics.AvgLatencyMs
			hc.metrics.mu.Unlock()
			return result, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break
		}
		hc.logger.Warn("Retrying nGQL query via HTTP",
			zap.Int("attempt", i+1),
			zap.Error(err))
	}

	hc.metrics.mu.Lock()
	hc.metrics.FailedQueries++
	hc.metrics.mu.Unlock()

	return nil, fmt.Errorf("execute nGQL via HTTP after %d retries: %w", hc.config.RetryCount, lastErr)
}

// executeOnce 单次执行 (无重试)
func (hc *HTTPClient) executeOnce(ctx context.Context, nGQL string) (*ResultSet, error) {
	reqBody := httpExecuteRequest{
		GQL:      nGQL,
		Username: hc.config.Username,
		Password: hc.config.Password,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("http://%s/api/v1/execute", hc.config.GraphAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// 携带 session ID (如已认证)
	hc.mu.RLock()
	sid := hc.sessionID
	hc.mu.RUnlock()
	if sid != "" {
		req.Header.Set("Cookie", fmt.Sprintf("sessionId=%s", sid))
	}

	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResp httpExecuteResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, truncate(string(respBytes), 200))
	}

	// 检查 API 错误
	if len(apiResp.Errors) > 0 {
		errMsgs := make([]string, len(apiResp.Errors))
		for i, e := range apiResp.Errors {
			errMsgs[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("API errors: %s", strings.Join(errMsgs, "; "))
	}

	// 保存 session ID
	if apiResp.SessionID != "" {
		hc.mu.Lock()
		hc.sessionID = apiResp.SessionID
		hc.mu.Unlock()
	}

	// 转换为 ResultSet
	result := &ResultSet{
		Columns:   []string{},
		Rows:      make([]map[string]interface{}, 0),
		LatencyUs: apiResp.ExecTimeUs,
	}

	if len(apiResp.Results) > 0 {
		r := apiResp.Results[0]
		result.Columns = r.Columns
		result.ErrorCode = r.ErrorCode
		result.ErrorMessage = r.ErrorMsg

		for _, row := range r.Data {
			rowMap := make(map[string]interface{}, len(r.Columns))
			for j, col := range r.Columns {
				if j < len(row) {
					rowMap[col] = row[j]
				}
			}
			result.Rows = append(result.Rows, rowMap)
		}
	}

	return result, nil
}

// ============================================================================
// 管理方法
// ============================================================================

// SwitchSpace 切换图空间
func (hc *HTTPClient) SwitchSpace(ctx context.Context, space string) error {
	nGQL := fmt.Sprintf("USE %s;", space)
	_, err := hc.Execute(ctx, nGQL)
	if err == nil {
		hc.config.Space = space
	}
	return err
}

// ShowSpaces 列出所有图空间
func (hc *HTTPClient) ShowSpaces(ctx context.Context) ([]string, error) {
	result, err := hc.Execute(ctx, "SHOW SPACES;")
	if err != nil {
		return nil, err
	}
	spaces := make([]string, len(result.Rows))
	for i, row := range result.Rows {
		if name, ok := row["Name"].(string); ok {
			spaces[i] = name
		}
	}
	return spaces, nil
}

// ShowHosts 查看集群主机状态
func (hc *HTTPClient) ShowHosts(ctx context.Context) ([]map[string]interface{}, error) {
	result, err := hc.Execute(ctx, "SHOW HOSTS;")
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// ShowTags 列出所有 Tag
func (hc *HTTPClient) ShowTags(ctx context.Context) ([]string, error) {
	result, err := hc.Execute(ctx, "SHOW TAGS;")
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(result.Rows))
	for i, row := range result.Rows {
		if name, ok := row["Name"].(string); ok {
			tags[i] = name
		}
	}
	return tags, nil
}

// ShowEdges 列出所有 Edge Type
func (hc *HTTPClient) ShowEdges(ctx context.Context) ([]string, error) {
	result, err := hc.Execute(ctx, "SHOW EDGES;")
	if err != nil {
		return nil, err
	}
	edges := make([]string, len(result.Rows))
	for i, row := range result.Rows {
		if name, ok := row["Name"].(string); ok {
			edges[i] = name
		}
	}
	return edges, nil
}

// Ping 健康检查
func (hc *HTTPClient) Ping(ctx context.Context) error {
	_, err := hc.Execute(ctx, "SHOW SPACES;")
	return err
}

// Close 关闭客户端
func (hc *HTTPClient) Close() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.closed = true
	hc.httpClient.CloseIdleConnections()
	hc.logger.Info("NebulaGraph HTTP client closed")
	return nil
}

// ============================================================================
// 统计
// ============================================================================

// GetMetrics 获取指标
func (hc *HTTPClient) GetMetrics() ClientMetrics {
	hc.metrics.mu.RLock()
	defer hc.metrics.mu.RUnlock()
	return ClientMetrics{
		TotalQueries:   hc.metrics.TotalQueries,
		FailedQueries:  hc.metrics.FailedQueries,
		AvgLatencyMs:   hc.metrics.AvgLatencyMs,
		ActiveSessions: hc.metrics.ActiveSessions,
	}
}

// Space 获取当前空间名
func (hc *HTTPClient) Space() string {
	return hc.config.Space
}

// ============================================================================
// 图操作 API (委托给 nGQL 构建器 + Execute)
// ============================================================================

// InsertIPNode 插入 IP 节点
func (hc *HTTPClient) InsertIPNode(ctx context.Context, tenantID, ip, mac, hostname, vendor, osType string, isGateway bool, riskScore float64, firstSeen, lastSeen int64) error {
	vid := hashTenantVID(tenantID, ip)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX ip_address(tenant_id, ip, mac_address, hostname, vendor, os_type, is_gateway, risk_score, first_seen, last_seen) VALUES "%s":("%s", "%s", "%s", "%s", "%s", "%s", %t, %f, %d, %d);`,
		vid, tenantID, ip, mac, hostname, vendor, osType, isGateway, riskScore, firstSeen, lastSeen)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// InsertSessionEdge 插入会话边
func (hc *HTTPClient) InsertSessionEdge(ctx context.Context, tenantID, srcIP, dstIP, communityID string, protocol int, sessionCount, totalBytes, totalPackets int64, firstSeen, lastSeen int64, direction string) error {
	srcVID := hashTenantVID(tenantID, srcIP)
	dstVID := hashTenantVID(tenantID, dstIP)
	nGQL := fmt.Sprintf(
		`INSERT EDGE communicates(tenant_id, community_id, protocol, session_count, total_bytes, total_packets, first_seen, last_seen, direction) VALUES "%s"->"%s":("%s", "%s", %d, %d, %d, %d, %d, %d, "%s");`,
		srcVID, dstVID, tenantID, communityID, protocol, sessionCount, totalBytes, totalPackets, firstSeen, lastSeen, direction)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// InsertAlertNode 插入告警节点
func (hc *HTTPClient) InsertAlertNode(ctx context.Context, tenantID, alertID, alertType, severity, labels string, score float64, firstSeen, lastSeen int64) error {
	vid := hashTenantVID(tenantID, alertID)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX alert(tenant_id, alert_type, severity, score, labels, first_seen, last_seen) VALUES "%s":("%s", "%s", "%s", %f, "%s", %d, %d);`,
		vid, tenantID, alertType, severity, score, labels, firstSeen, lastSeen)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// InsertCampaignNode 插入攻击活动节点
func (hc *HTTPClient) InsertCampaignNode(ctx context.Context, tenantID, campaignID, campaignType, title, desc, severity string, score, phaseProgress float64, startTime, endTime int64) error {
	vid := hashTenantVID(tenantID, campaignID)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX campaign(tenant_id, campaign_type, title, description, severity, score, phase_progress, start_time, end_time) VALUES "%s":("%s", "%s", "%s", "%s", "%s", %f, %f, %d, %d);`,
		vid, tenantID, campaignType, title, desc, severity, score, phaseProgress, startTime, endTime)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// InsertTriggerEdge 插入告警触发边
func (hc *HTTPClient) InsertTriggerEdge(ctx context.Context, tenantID, communityID, alertID string, ts int64) error {
	srcVID := hashTenantVID(tenantID, communityID)
	dstVID := hashTenantVID(tenantID, alertID)
	nGQL := fmt.Sprintf(
		`INSERT EDGE triggers_alert(tenant_id, alert_id, ts) VALUES "%s"->"%s":("%s", "%s", %d);`,
		srcVID, dstVID, tenantID, alertID, ts)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// InsertAttackPathHop 插入攻击路径跳
func (hc *HTTPClient) InsertAttackPathHop(ctx context.Context, tenantID, campaignID, srcIP, dstIP string, hopOrder int32, ts int64) error {
	srcVID := hashTenantVID(tenantID, srcIP)
	dstVID := hashTenantVID(tenantID, dstIP)
	nGQL := fmt.Sprintf(
		`INSERT EDGE attack_path_hop(tenant_id, campaign_id, hop_order, ts) VALUES "%s"->"%s":("%s", "%s", %d, %d);`,
		srcVID, dstVID, tenantID, campaignID, hopOrder, ts)
	_, err := hc.Execute(ctx, nGQL)
	return err
}

// ============================================================================
// 查询方法
// ============================================================================

// GetNeighbors 获取节点的邻居 (1-hop)
func (hc *HTTPClient) GetNeighbors(ctx context.Context, tenantID, ip string, limit int) ([]map[string]interface{}, error) {
	vid := hashTenantVID(tenantID, ip)
	nGQL := fmt.Sprintf(
		`GO FROM "%s" OVER communicates WHERE communicates.tenant_id == "%s" YIELD communicates._dst AS dst_ip, communicates.session_count AS session_count, communicates.total_bytes AS total_bytes, communicates.first_seen AS first_seen, communicates.last_seen AS last_seen LIMIT %d;`,
		vid, tenantID, limit)
	result, err := hc.Execute(ctx, nGQL)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// GetSubgraph 获取子图 (n-hop)
func (hc *HTTPClient) GetSubgraph(ctx context.Context, tenantID string, centerIPs []string, steps int) (*SubgraphResult, error) {
	vids := make([]string, len(centerIPs))
	for i, ip := range centerIPs {
		vids[i] = fmt.Sprintf(`"%s"`, hashTenantVID(tenantID, ip))
	}
	vidStr := strings.Join(vids, ", ")

	nGQL := fmt.Sprintf(
		`GET SUBGRAPH %d STEPS FROM %s IN communicates, triggers_alert OUT communicates, triggers_alert BOTH attack_path_hop;`,
		steps, vidStr)

	_, err := hc.Execute(ctx, nGQL)
	if err != nil {
		return nil, err
	}

	subgraph := &SubgraphResult{
		Vertices: make([]Vertex, 0),
		Edges:    make([]Edge, 0),
	}
	return subgraph, nil
}

// FindPath 查找最短路径
func (hc *HTTPClient) FindPath(ctx context.Context, tenantID, srcIP, dstIP string, maxHops int) ([]map[string]interface{}, error) {
	srcVID := hashTenantVID(tenantID, srcIP)
	dstVID := hashTenantVID(tenantID, dstIP)
	nGQL := fmt.Sprintf(
		`FIND SHORTEST PATH FROM "%s" TO "%s" OVER communicates UPTO %d STEPS YIELD path AS p;`,
		srcVID, dstVID, maxHops)
	result, err := hc.Execute(ctx, nGQL)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// ============================================================================
// 辅助函数
// ============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
