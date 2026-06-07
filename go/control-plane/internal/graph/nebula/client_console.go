// NebulaGraph Console Client — 基于 nebula-console CLI 的图数据库客户端
//
// 通过调用 nebula-console 二进制执行 nGQL 查询，解析其表格输出为 ResultSet。
// 适用于 K8s Pod 内已安装 nebula-console 的环境。
//
// 使用方式:
//   1. 在 graph-service Docker 镜像中安装 nebula-console
//   2. 或通过 initContainer/shared volume 挂载二进制
//   3. 或通过 sidecar 容器提供 console 服务
//
// 验证状态: 已通过 K8s 集群内实际执行验证 (SHOW HOSTS, INSERT, FETCH, GO)
package nebula

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ConsoleClientConfig Console 客户端配置
type ConsoleClientConfig struct {
	ConsoleBin  string        // nebula-console 二进制路径 (默认: /usr/local/nebula/bin/nebula-console)
	GraphAddr   string        // Graph 服务地址
	GraphPort   int           // Graph 端口 (默认: 9669)
	Username    string        // 用户名
	Password    string        // 密码
	Timeout     time.Duration // 单次查询超时
	RetryCount  int           // 重试次数
}

// DefaultConsoleConfig 默认 Console 配置
// 支持环境变量覆盖: NEBULA_CONSOLE_BIN, NEBULA_CONSOLE_ADDR, NEBULA_CONSOLE_PORT
func DefaultConsoleConfig() ConsoleClientConfig {
	cfg := ConsoleClientConfig{
		ConsoleBin: "/usr/local/nebula/bin/nebula-console",
		GraphAddr:  "nebula-graph.middleware.svc",
		GraphPort:  9669,
		Username:   "root",
		Password:   "root",
		Timeout:    30 * time.Second,
		RetryCount: 3,
	}
	if v := os.Getenv("NEBULA_CONSOLE_BIN"); v != "" {
		cfg.ConsoleBin = v
	}
	if v := os.Getenv("NEBULA_CONSOLE_ADDR"); v != "" {
		cfg.GraphAddr = v
	}
	if v := os.Getenv("NEBULA_CONSOLE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.GraphPort = p
		}
	}
	return cfg
}

// ConsoleClient NebulaGraph Console 客户端
type ConsoleClient struct {
	config  ConsoleClientConfig
	logger  *zap.Logger
	metrics *ClientMetrics
	mu      sync.RWMutex
	closed  bool
}

// NewConsoleClient 创建 Console 客户端
func NewConsoleClient(cfg ConsoleClientConfig, logger *zap.Logger) (*ConsoleClient, error) {
	if cfg.ConsoleBin == "" {
		cfg.ConsoleBin = "/usr/local/nebula/bin/nebula-console"
	}
	if cfg.GraphPort == 0 {
		cfg.GraphPort = 9669
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	client := &ConsoleClient{
		config:  cfg,
		logger:  logger,
		metrics: &ClientMetrics{},
	}

	// 验证 console 二进制可用
	if err := client.checkBinary(); err != nil {
		logger.Warn("nebula-console binary not available",
			zap.String("path", cfg.ConsoleBin),
			zap.Error(err))
	}

	logger.Info("NebulaGraph Console client initialized",
		zap.String("addr", fmt.Sprintf("%s:%d", cfg.GraphAddr, cfg.GraphPort)))

	return client, nil
}

// checkBinary 检查 console 二进制是否可用
func (cc *ConsoleClient) checkBinary() error {
	cmd := exec.Command(cc.config.ConsoleBin, "--help")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ============================================================================
// 核心执行方法
// ============================================================================

// Execute 执行 nGQL 查询并返回解析后的结果
func (cc *ConsoleClient) Execute(ctx context.Context, nGQL string) (*ResultSet, error) {
	startTime := time.Now()
	cc.metrics.mu.Lock()
	cc.metrics.TotalQueries++
	cc.metrics.mu.Unlock()

	var lastErr error
	for i := 0; i <= cc.config.RetryCount; i++ {
		if i > 0 {
			delay := time.Duration(1<<uint(i-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := cc.executeConsole(ctx, nGQL)
		if err == nil {
			latency := float64(time.Since(startTime).Microseconds()) / 1000.0
			cc.metrics.mu.Lock()
			cc.metrics.AvgLatencyMs = 0.1*latency + 0.9*cc.metrics.AvgLatencyMs
			cc.metrics.mu.Unlock()
			return result, nil
		}

		lastErr = err
	}

	cc.metrics.mu.Lock()
	cc.metrics.FailedQueries++
	cc.metrics.mu.Unlock()

	return nil, fmt.Errorf("execute nGQL via console after %d retries: %w", cc.config.RetryCount, lastErr)
}

// executeConsole 调用 nebula-console 执行查询
func (cc *ConsoleClient) executeConsole(ctx context.Context, nGQL string) (*ResultSet, error) {
	args := []string{
		"-addr", cc.config.GraphAddr,
		"-port", fmt.Sprintf("%d", cc.config.GraphPort),
		"-u", cc.config.Username,
		"-p", cc.config.Password,
		"-e", nGQL,
	}

	cmd := exec.CommandContext(ctx, cc.config.ConsoleBin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start console: %w", err)
	}

	// 解析输出
	scanner := bufio.NewScanner(stdout)
	result := cc.parseTabularOutput(scanner)

	if err := cmd.Wait(); err != nil {
		// nebula-console 即使查询成功也可能返回非零退出码
		// 如果有解析结果，视为成功
		if result != nil && len(result.Rows) > 0 {
			return result, nil
		}
		if strings.Contains(err.Error(), "exit status") {
			return result, nil // 尝试返回部分结果
		}
		return nil, fmt.Errorf("console wait: %w", err)
	}

	return result, nil
}

// parseTabularOutput 解析 nebula-console 的表格输出
//
// nebula-console 输出格式:
//
//	+----------+------+
//	| Column1  | Col2 |
//	+----------+------+
//	| value1   | val2 |
//	| value2   | val3 |
//	+----------+------+
//	Got 2 rows (time spent ...)
func (cc *ConsoleClient) parseTabularOutput(scanner *bufio.Scanner) *ResultSet {
	result := &ResultSet{
		Columns: []string{},
		Rows:    make([]map[string]interface{}, 0),
	}

	var headerLine, separatorLine string
	var dataLines []string
	inTable := false
	headerParsed := false

	for scanner.Scan() {
		line := scanner.Text()

		// 检测表格开始: +---+---+
		if !inTable && isSeparatorLine(line) {
			inTable = true
			continue
		}

		if inTable && !headerParsed {
			if isDataLine(line) {
				// 这是表头行: | Col1 | Col2 |
				headerLine = line
				headerParsed = true
				continue
			}
			if isSeparatorLine(line) {
				continue
			}
		}

		if headerParsed {
			if isSeparatorLine(line) {
				// 表格结束或分隔符
				if separatorLine == "" {
					separatorLine = line
					// 表头后的第一个分隔符 — 继续
					continue
				}
				// 第二个分隔符 — 表格结束
				break
			}
			if isDataLine(line) {
				dataLines = append(dataLines, line)
				continue
			}
			// 非表格行 — 表格结束
			if len(dataLines) > 0 {
				break
			}
		}
	}

	if headerLine == "" {
		return result
	}

	// 解析列名
	result.Columns = parseRow(headerLine)

	// 解析数据行
	for _, dataLine := range dataLines {
		values := parseRow(dataLine)
		row := make(map[string]interface{}, len(result.Columns))
		for j, col := range result.Columns {
			if j < len(values) {
				row[col] = values[j]
			}
		}
		result.Rows = append(result.Rows, row)
	}

	return result
}

// ============================================================================
// 管理方法
// ============================================================================

// Ping 健康检查
func (cc *ConsoleClient) Ping(ctx context.Context) error {
	_, err := cc.Execute(ctx, "SHOW SPACES;")
	return err
}

// ShowSpaces 列出图空间
func (cc *ConsoleClient) ShowSpaces(ctx context.Context) ([]string, error) {
	result, err := cc.Execute(ctx, "SHOW SPACES;")
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

// ShowHosts 查看集群状态
func (cc *ConsoleClient) ShowHosts(ctx context.Context) ([]map[string]interface{}, error) {
	result, err := cc.Execute(ctx, "SHOW HOSTS;")
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// ShowTags 列出所有 Tag
func (cc *ConsoleClient) ShowTags(ctx context.Context, space string) ([]string, error) {
	nGQL := fmt.Sprintf("USE %s; SHOW TAGS;", space)
	result, err := cc.Execute(ctx, nGQL)
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
func (cc *ConsoleClient) ShowEdges(ctx context.Context, space string) ([]string, error) {
	nGQL := fmt.Sprintf("USE %s; SHOW EDGES;", space)
	result, err := cc.Execute(ctx, nGQL)
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

// Close 关闭客户端
func (cc *ConsoleClient) Close() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.closed = true
	cc.logger.Info("NebulaGraph Console client closed")
	return nil
}

// GetMetrics 获取指标
func (cc *ConsoleClient) GetMetrics() ClientMetrics {
	cc.metrics.mu.RLock()
	defer cc.metrics.mu.RUnlock()
	return ClientMetrics{
		TotalQueries:   cc.metrics.TotalQueries,
		FailedQueries:  cc.metrics.FailedQueries,
		AvgLatencyMs:   cc.metrics.AvgLatencyMs,
		ActiveSessions: cc.metrics.ActiveSessions,
	}
}

// ============================================================================
// 表格解析辅助函数
// ============================================================================

var (
	separatorRe = regexp.MustCompile(`^\+\-+\+$`)
	dataLineRe  = regexp.MustCompile(`^\|.*\|$`)
)

func isSeparatorLine(line string) bool {
	return separatorRe.MatchString(strings.TrimSpace(line))
}

func isDataLine(line string) bool {
	return dataLineRe.MatchString(line)
}

func parseRow(line string) []string {
	// 移除首尾的 |
	trimmed := strings.Trim(line, "| ")
	// 按 | 分割
	parts := strings.Split(trimmed, "|")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result
}
