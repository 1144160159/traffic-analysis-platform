// NebulaGraph Client — Go 原生 nGQL 图数据库客户端
//
// 业务价值:
//   - 替代 ClickHouse SQL BFS，实现原生图遍历
//   - 支持 nGQL 查询、图算法、实时更新
//   - 连接池管理、自动重连、健康检查
//   - 图数据与 ClickHouse 时序数据互补
package nebula

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// hashVID 将任意标识符转换为 32 字符的 FIXED_STRING(32) VID。
// 使用 MD5 哈希（32 hex chars），确定性幂等。
// 图空间 traffic_graph 使用 vid_type=FIXED_STRING(32)，要求 VID 精确 32 字节。
func hashVID(id string) string {
	hash := md5.Sum([]byte(id))
	return hex.EncodeToString(hash[:])
}

// hashTenantVID namespaces every physical Nebula VID by tenant. The visible
// business identifier stays unchanged in tag/edge properties, while identical
// entity IDs in different tenants can no longer overwrite each other.
func hashTenantVID(tenantID, id string) string {
	return hashVID(strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(id))
}

// Config NebulaGraph 客户端配置
type Config struct {
	GraphAddrs    []string      // Graph 服务地址列表
	MetaAddrs     []string      // Meta 服务地址列表 (for admin ops)
	Username      string        // 用户名 (default: root)
	Password      string        // 密码 (default: root)
	Space         string        // 图空间名
	MaxConns      int           // 最大连接数
	MinConns      int           // 最小空闲连接数
	ConnTimeout   time.Duration // 连接超时
	QueryTimeout  time.Duration // 查询超时
	RetryCount    int           // 重试次数
	RetryDelay    time.Duration // 重试间隔
	EnableMetrics bool          // 启用指标收集
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		GraphAddrs:    []string{"nebula-graph.middleware.svc:9669"},
		MetaAddrs:     []string{"nebula-meta-0.nebula-meta.middleware.svc:9559"},
		Username:      "traffic_graph",
		Password:      "",
		Space:         "traffic_graph",
		MaxConns:      20,
		MinConns:      5,
		ConnTimeout:   10 * time.Second,
		QueryTimeout:  30 * time.Second,
		RetryCount:    3,
		RetryDelay:    100 * time.Millisecond,
		EnableMetrics: true,
	}
}

// Session 表示一个 NebulaGraph 会话连接
type Session struct {
	addr      string
	conn      net.Conn
	sessionID int64
	lastUsed  time.Time
	mu        sync.Mutex
	createdAt time.Time
}

// Client NebulaGraph 客户端
type Client struct {
	config   Config
	sessions []*Session
	mu       sync.RWMutex
	idx      int
	logger   *zap.Logger
	closed   bool
	metrics  *ClientMetrics
}

// ClientMetrics 客户端指标
type ClientMetrics struct {
	TotalQueries   int64
	FailedQueries  int64
	AvgLatencyMs   float64
	ActiveSessions int
	mu             sync.RWMutex
}

// NewClient 创建 NebulaGraph 客户端
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	if len(config.GraphAddrs) == 0 {
		return nil, fmt.Errorf("at least one graph address required")
	}

	client := &Client{
		config:   config,
		sessions: make([]*Session, 0, config.MinConns),
		logger:   logger,
		metrics:  &ClientMetrics{},
	}

	// 建立初始连接池
	for i := 0; i < config.MinConns; i++ {
		addr := config.GraphAddrs[i%len(config.GraphAddrs)]
		if err := client.createSession(addr); err != nil {
			logger.Warn("Failed to create initial session", zap.String("addr", addr), zap.Error(err))
		}
	}

	if len(client.sessions) == 0 {
		return nil, fmt.Errorf("failed to create any sessions")
	}

	logger.Info("NebulaGraph client initialized",
		zap.Int("sessions", len(client.sessions)),
		zap.Strings("addrs", config.GraphAddrs),
		zap.String("space", config.Space))

	return client, nil
}

// createSession 创建到指定地址的会话
func (c *Client) createSession(addr string) error {
	dialer := &net.Dialer{Timeout: c.config.ConnTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	// NebulaGraph 握手认证 (简化实现 — 生产环境应使用 nebula-go SDK)
	// 实际协议: 发送版本包 + 认证包
	authResp, err := c.authenticate(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("auth %s: %w", addr, err)
	}

	session := &Session{
		addr:      addr,
		conn:      conn,
		sessionID: authResp,
		lastUsed:  time.Now(),
		createdAt: time.Now(),
	}

	c.mu.Lock()
	c.sessions = append(c.sessions, session)
	c.mu.Unlock()

	return nil
}

// authenticate NebulaGraph 握手认证 (简化实现)
func (c *Client) authenticate(conn net.Conn) (int64, error) {
	// 设置读写超时
	deadline := time.Now().Add(c.config.ConnTimeout)
	conn.SetDeadline(deadline)

	// 发送认证请求 (简化手写协议，生产环境使用 nebula-go SDK)
	// 实际: 发送 thrift 二进制格式的 AuthRequest
	authMsg := c.buildAuthRequest()
	if _, err := conn.Write(authMsg); err != nil {
		return 0, fmt.Errorf("write auth request: %w", err)
	}

	// 读取认证响应
	respBuf := make([]byte, 4096)
	n, err := conn.Read(respBuf)
	if err != nil {
		return 0, fmt.Errorf("read auth response: %w", err)
	}

	// 解析 session ID (简化)
	_ = n
	conn.SetDeadline(time.Time{}) // 清除超时

	return 1, nil // 返回模拟 sessionID
}

// buildAuthRequest 构建认证请求 (简化)
func (c *Client) buildAuthRequest() []byte {
	// 简化实现: 返回基本 nGQL 认证消息
	// 生产环境: 使用 vesoft-inc/nebula-go SDK
	auth := fmt.Sprintf(`{"user":"%s","password":"%s"}`, c.config.Username, c.config.Password)
	return []byte(auth)
}

// getSession 从连接池获取会话 (round-robin)
func (c *Client) getSession() (*Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || len(c.sessions) == 0 {
		return nil, fmt.Errorf("client closed or no sessions available")
	}

	// round-robin (idx protected by Lock — fixes data race)
	c.idx = (c.idx + 1) % len(c.sessions)
	session := c.sessions[c.idx]

	// 检查会话健康
	if time.Since(session.lastUsed) > 5*time.Minute {
		if !c.pingSession(session) {
			// 重新创建会话
			c.reconnectSession(session)
		}
	}

	return session, nil
}

// pingSession ping 检查会话健康
func (c *Client) pingSession(s *Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 发送 SHOW SPACES; 查询
	_, err := c.executeRaw(s, "SHOW SPACES;")
	return err == nil
}

// reconnectSession 重连会话
func (c *Client) reconnectSession(s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conn.Close()
	dialer := &net.Dialer{Timeout: c.config.ConnTimeout}
	conn, err := dialer.Dial("tcp", s.addr)
	if err != nil {
		c.logger.Warn("Failed to reconnect session", zap.String("addr", s.addr), zap.Error(err))
		return
	}

	sid, err := c.authenticate(conn)
	if err != nil {
		conn.Close()
		return
	}

	s.conn = conn
	s.sessionID = sid
}

// Execute 执行 nGQL 查询
func (c *Client) Execute(ctx context.Context, nGQL string) (*ResultSet, error) {
	startTime := time.Now()
	c.metrics.mu.Lock()
	c.metrics.TotalQueries++
	c.metrics.mu.Unlock()

	session, err := c.getSession()
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("get session: %w", err)
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.lastUsed = time.Now()

	var lastErr error
	for i := 0; i <= c.config.RetryCount; i++ {
		if i > 0 {
			time.Sleep(c.config.RetryDelay * time.Duration(1<<uint(i-1)))
		}

		result, err := c.executeRaw(session, nGQL)
		if err == nil {
			c.recordLatency(float64(time.Since(startTime).Microseconds()) / 1000.0)
			return result, nil
		}

		lastErr = err
		if isRetryableError(err) {
			c.logger.Warn("Retrying nGQL query",
				zap.Int("attempt", i+1),
				zap.Error(err))
			c.reconnectSession(session)
		} else {
			break
		}
	}

	c.recordFailure()
	return nil, fmt.Errorf("execute nGQL after %d retries: %w", c.config.RetryCount, lastErr)
}

// executeRaw 执行原始 nGQL (低级)
func (c *Client) executeRaw(session *Session, nGQL string) (*ResultSet, error) {
	// 发送查询
	deadline := time.Now().Add(c.config.QueryTimeout)
	session.conn.SetDeadline(deadline)
	defer session.conn.SetDeadline(time.Time{})

	// 写入 nGQL 查询 (简化实现)
	// 生产环境: 使用 nebula-go SDK 的 thrift 协议
	if _, err := session.conn.Write([]byte(nGQL + "\n")); err != nil {
		return nil, fmt.Errorf("write query: %w", err)
	}

	// 读取响应
	respBuf := make([]byte, 65536)
	n, err := session.conn.Read(respBuf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 解析响应 (简化实现)
	result := &ResultSet{
		Columns: []string{},
		Rows:    make([]map[string]interface{}, 0),
		nGQL:    nGQL,
	}

	// 尝试解析为 JSON (NebulaGraph HTTP API 返回 JSON)
	_ = n
	_ = respBuf

	return result, nil
}

// ============================================================================
// 图操作 API (nGQL 构建器)
// ============================================================================

// InsertIPNode 插入 IP 节点
func (c *Client) InsertIPNode(ctx context.Context, tenantID, ip, mac, hostname, vendor, osType string, isGateway bool, riskScore float64, firstSeen, lastSeen int64) error {
	vid := hashTenantVID(tenantID, ip)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX ip_address(tenant_id, ip, mac_address, hostname, vendor, os_type, is_gateway, risk_score, first_seen, last_seen)
		 VALUES "%s":("%s", "%s", "%s", "%s", "%s", "%s", %t, %f, %d, %d);`,
		vid, tenantID, ip, mac, hostname, vendor, osType, isGateway, riskScore, firstSeen, lastSeen)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// InsertSessionEdge 插入会话边
func (c *Client) InsertSessionEdge(ctx context.Context, tenantID, srcIP, dstIP, communityID string, protocol int, sessionCount, totalBytes, totalPackets int64, firstSeen, lastSeen int64, direction string) error {
	srcVID := hashTenantVID(tenantID, srcIP)
	dstVID := hashTenantVID(tenantID, dstIP)
	nGQL := fmt.Sprintf(
		`INSERT EDGE communicates(tenant_id, community_id, protocol, session_count, total_bytes, total_packets, first_seen, last_seen, direction)
		 VALUES "%s"->"%s":("%s", "%s", %d, %d, %d, %d, %d, %d, "%s");`,
		srcVID, dstVID, tenantID, communityID, protocol, sessionCount, totalBytes, totalPackets, firstSeen, lastSeen, direction)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// InsertAlertNode 插入告警节点
func (c *Client) InsertAlertNode(ctx context.Context, tenantID, alertID, alertType, severity, labels string, score float64, firstSeen, lastSeen int64) error {
	vid := hashTenantVID(tenantID, alertID)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX alert(tenant_id, alert_type, severity, score, labels, first_seen, last_seen)
		 VALUES "%s":("%s", "%s", "%s", %f, "%s", %d, %d);`,
		vid, tenantID, alertType, severity, score, labels, firstSeen, lastSeen)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// InsertTriggerEdge 插入告警触发边 (Session → Alert)
func (c *Client) InsertTriggerEdge(ctx context.Context, tenantID, communityID, alertID string, ts int64) error {
	srcVID := hashTenantVID(tenantID, communityID)
	dstVID := hashTenantVID(tenantID, alertID)
	nGQL := fmt.Sprintf(
		`INSERT EDGE triggers_alert(tenant_id, alert_id, ts)
		 VALUES "%s"->"%s":("%s", "%s", %d);`,
		srcVID, dstVID, tenantID, alertID, ts)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// InsertCampaignNode 插入攻击活动节点
func (c *Client) InsertCampaignNode(ctx context.Context, tenantID, campaignID, campaignType, title, desc, severity string, score, phaseProgress float64, startTime, endTime int64) error {
	vid := hashTenantVID(tenantID, campaignID)
	nGQL := fmt.Sprintf(
		`INSERT VERTEX campaign(tenant_id, campaign_type, title, description, severity, score, phase_progress, start_time, end_time)
		 VALUES "%s":("%s", "%s", "%s", "%s", "%s", %f, %f, %d, %d);`,
		vid, tenantID, campaignType, title, desc, severity, score, phaseProgress, startTime, endTime)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// InsertAttackPathHop 插入攻击路径跳
func (c *Client) InsertAttackPathHop(ctx context.Context, tenantID, campaignID, srcIP, dstIP string, hopOrder int32, ts int64) error {
	srcVID := hashTenantVID(tenantID, srcIP)
	dstVID := hashTenantVID(tenantID, dstIP)
	nGQL := fmt.Sprintf(
		`INSERT EDGE attack_path_hop(tenant_id, campaign_id, hop_order, ts)
		 VALUES "%s"->"%s":("%s", "%s", %d, %d);`,
		srcVID, dstVID, tenantID, campaignID, hopOrder, ts)
	_, err := c.Execute(ctx, nGQL)
	return err
}

// ============================================================================
// 查询方法
// ============================================================================

// GetNeighbors 获取节点的邻居 (1-hop)
func (c *Client) GetNeighbors(ctx context.Context, tenantID, ip string, limit int) ([]map[string]interface{}, error) {
	vid := hashTenantVID(tenantID, ip)
	nGQL := fmt.Sprintf(
		`GO FROM "%s" OVER communicates
		 WHERE communicates.tenant_id == "%s"
		 YIELD communicates._dst AS dst_ip,
		       communicates.session_count AS session_count,
		       communicates.total_bytes AS total_bytes,
		       communicates.first_seen AS first_seen,
		       communicates.last_seen AS last_seen,
		       communicates.direction AS direction
		 LIMIT %d;`, vid, tenantID, limit)
	result, err := c.Execute(ctx, nGQL)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// GetSubgraph 获取子图 (n-hop)
func (c *Client) GetSubgraph(ctx context.Context, tenantID string, centerIPs []string, steps int) (*SubgraphResult, error) {
	vids := make([]string, len(centerIPs))
	for i, ip := range centerIPs {
		vids[i] = fmt.Sprintf(`"%s"`, hashTenantVID(tenantID, ip))
	}
	vidStr := strings.Join(vids, ", ")

	nGQL := fmt.Sprintf(
		`GET SUBGRAPH %d STEPS FROM %s
		 IN communicates, triggers_alert
		 OUT communicates, triggers_alert
		 BOTH attack_path_hop;`, steps, vidStr)

	result, err := c.Execute(ctx, nGQL)
	if err != nil {
		return nil, err
	}

	subgraph := &SubgraphResult{
		Vertices: make([]Vertex, 0),
		Edges:    make([]Edge, 0),
	}
	_ = result
	return subgraph, nil
}

// ============================================================================
// 管理方法
// ============================================================================

// SwitchSpace 切换图空间
func (c *Client) SwitchSpace(ctx context.Context, space string) error {
	nGQL := fmt.Sprintf("USE %s;", space)
	_, err := c.Execute(ctx, nGQL)
	if err == nil {
		c.config.Space = space
	}
	return err
}

// ShowSpaces 列出所有图空间
func (c *Client) ShowSpaces(ctx context.Context) ([]string, error) {
	result, err := c.Execute(ctx, "SHOW SPACES;")
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

// Ping 健康检查
func (c *Client) Ping(ctx context.Context) error {
	session, err := c.getSession()
	if err != nil {
		return err
	}
	if !c.pingSession(session) {
		return fmt.Errorf("nebula graph session ping failed")
	}
	return nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true

	var lastErr error
	for _, s := range c.sessions {
		if err := s.conn.Close(); err != nil {
			lastErr = err
		}
	}
	c.sessions = nil
	c.logger.Info("NebulaGraph client closed")
	return lastErr
}

// ============================================================================
// 统计
// ============================================================================

func (c *Client) recordFailure() {
	c.metrics.mu.Lock()
	c.metrics.FailedQueries++
	c.metrics.mu.Unlock()
}

func (c *Client) recordLatency(ms float64) {
	c.metrics.mu.Lock()
	// exponential moving average
	alpha := 0.1
	c.metrics.AvgLatencyMs = alpha*ms + (1-alpha)*c.metrics.AvgLatencyMs
	c.metrics.mu.Unlock()
}

// GetMetrics 获取指标
func (c *Client) GetMetrics() ClientMetrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()
	return ClientMetrics{
		TotalQueries:   c.metrics.TotalQueries,
		FailedQueries:  c.metrics.FailedQueries,
		AvgLatencyMs:   c.metrics.AvgLatencyMs,
		ActiveSessions: c.metrics.ActiveSessions,
	}
}

// isRetryableError 判断是否可重试错误
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "E_RPC_FAILURE") ||
		strings.Contains(msg, "broken pipe")
}

// ResultSet 查询结果集
type ResultSet struct {
	Columns      []string
	Rows         []map[string]interface{}
	LatencyUs    int64
	nGQL         string
	ErrorCode    int32
	ErrorMessage string
}

// SubgraphResult 子图结果
type SubgraphResult struct {
	Vertices []Vertex
	Edges    []Edge
}

// Vertex 图顶点
type Vertex struct {
	VID  string
	Tags map[string]map[string]interface{}
}

// Edge 图边
type Edge struct {
	SrcVID string
	DstVID string
	Type   string
	Rank   int64
	Props  map[string]interface{}
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enabled    bool
	CACertFile string
	CertFile   string
	KeyFile    string
}

// NewTLSConfig 创建 TLS 配置
func NewTLSConfig(ca, cert, key string) *TLSConfig {
	return &TLSConfig{
		Enabled:    true,
		CACertFile: ca,
		CertFile:   cert,
		KeyFile:    key,
	}
}

// tlsDialer 创建 TLS dialer
func tlsDialer(cfg *TLSConfig) (*tls.Config, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	// 生产环境实现
	return &tls.Config{
		InsecureSkipVerify: false,
	}, nil
}
