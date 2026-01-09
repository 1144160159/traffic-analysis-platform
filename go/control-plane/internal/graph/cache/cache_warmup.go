////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/cache/cache_warmup.go
// 缓存预热服务（完整修复版）
// 修复内容：
// 1. 修复 W1：解决循环依赖（注入 GraphQuery）
// 2. 修复 W2：在 handler 中调用 UpdateHotIP
// 3. 修复 W3：添加预热成功率统计
////////////////////////////////////////////////////////////////////////////////

package cache

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// WarmupService 缓存预热服务
type WarmupService struct {
	db         *sql.DB
	cache      *GraphCache
	logger     *zap.Logger
	interval   time.Duration
	stopChan   chan struct{}
	graphQuery GraphQueryInterface // 修复 W1：使用接口避免循环依赖
}

// GraphQueryInterface 图查询接口（避免循环依赖）
type GraphQueryInterface interface {
	Explore(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (interface{}, error)
}

// NewWarmupService 创建缓存预热服务（修复版）
func NewWarmupService(
	db *sql.DB,
	cache *GraphCache,
	graphQuery GraphQueryInterface, // 修复 W1：注入 GraphQuery
	logger *zap.Logger,
	interval time.Duration,
) *WarmupService {
	if interval == 0 {
		interval = 1 * time.Hour
	}

	return &WarmupService{
		db:         db,
		cache:      cache,
		graphQuery: graphQuery,
		logger:     logger,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动缓存预热服务
func (s *WarmupService) Start() {
	go s.run()
}

// Stop 停止缓存预热服务
func (s *WarmupService) Stop() {
	close(s.stopChan)
}

// run 后台运行
func (s *WarmupService) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// 启动时立即执行一次
	s.warmupAll()

	for {
		select {
		case <-s.stopChan:
			s.logger.Info("Cache warmup service stopped")
			return
		case <-ticker.C:
			s.warmupAll()
		}
	}
}

// warmupAll 预热所有租户的热点 IP（修复 W3：添加统计）
func (s *WarmupService) warmupAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tenants, err := s.getTenants(ctx)
	if err != nil {
		s.logger.Error("Failed to get tenants for warmup", zap.Error(err))
		return
	}

	var successCount, failCount int

	for _, tenantID := range tenants {
		if err := s.warmupTenant(ctx, tenantID); err != nil {
			s.logger.Error("Failed to warmup tenant",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			failCount++
		} else {
			successCount++
		}
	}

	// 修复 W3：记录统计信息
	s.logger.Info("Cache warmup cycle completed",
		zap.Int("total_tenants", len(tenants)),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))
}

// getTenants 获取所有租户列表
func (s *WarmupService) getTenants(ctx context.Context) ([]string, error) {
	query := `SELECT tenant_id FROM tenants WHERE status = 'active'`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenants := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			continue
		}
		tenants = append(tenants, tenantID)
	}

	return tenants, rows.Err()
}

// warmupTenant 预热单个租户的热点 IP（修复 W1：使用 GraphQuery）
func (s *WarmupService) warmupTenant(ctx context.Context, tenantID string) error {
	hotIPs, err := s.getHotIPs(ctx, tenantID, 50)
	if err != nil {
		return err
	}

	if len(hotIPs) == 0 {
		s.logger.Debug("No hot IPs to warmup",
			zap.String("tenant_id", tenantID))
		return nil
	}

	s.logger.Info("Starting cache warmup for tenant",
		zap.String("tenant_id", tenantID),
		zap.Int("hot_ip_count", len(hotIPs)))

	depth := 1
	endTime := time.Now().UnixMilli()
	startTime := endTime - 24*3600*1000
	runID := "realtime"

	var successCount, failCount int

	for _, ip := range hotIPs {
		// 修复 W1：调用注入的 GraphQuery
		_, err := s.graphQuery.Explore(ctx, tenantID, ip, depth, startTime, endTime, runID)
		if err != nil {
			s.logger.Warn("Failed to warmup IP",
				zap.String("tenant_id", tenantID),
				zap.String("ip", ip),
				zap.Error(err))
			failCount++
			continue
		}

		successCount++

		// 限流：避免压垮数据库
		time.Sleep(100 * time.Millisecond)
	}

	s.logger.Info("Tenant warmup completed",
		zap.String("tenant_id", tenantID),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	return nil
}

// getHotIPs 获取热点 IP 列表
func (s *WarmupService) getHotIPs(ctx context.Context, tenantID string, limit int) ([]string, error) {
	query := `
		SELECT ip
		FROM graph_hot_ips
		WHERE tenant_id = $1
		ORDER BY priority DESC, last_query_at DESC
		LIMIT $2
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ips := make([]string, 0, limit)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			continue
		}
		ips = append(ips, ip)
	}

	return ips, rows.Err()
}

// UpdateHotIP 更新热点 IP（修复 W2：独立函数供 handler 调用）
func UpdateHotIP(ctx context.Context, db *sql.DB, tenantID, ip string) error {
	query := `
		INSERT INTO graph_hot_ips (tenant_id, ip, query_count, last_query_at, priority, warmed_up)
		VALUES ($1, $2, 1, NOW(), 0, FALSE)
		ON CONFLICT (tenant_id, ip)
		DO UPDATE SET
			query_count = graph_hot_ips.query_count + 1,
			last_query_at = NOW(),
			priority = CASE
				WHEN graph_hot_ips.query_count + 1 > 100 THEN 3
				WHEN graph_hot_ips.query_count + 1 > 50 THEN 2
				WHEN graph_hot_ips.query_count + 1 > 10 THEN 1
				ELSE 0
			END,
			updated_at = NOW()
	`

	_, err := db.ExecContext(ctx, query, tenantID, ip)
	return err
}
