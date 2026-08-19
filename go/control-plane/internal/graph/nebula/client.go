// NebulaGraph 客户端共享类型与工具函数
//
// 说明: 仓库内真实生产链路使用 nebula-go SDK(见 workbench_store.go)或
// HTTP/Console 适配(client_http.go / client_console.go)。早期手写的 thrift
// 伪客户端(authenticate 返回模拟 sessionID、executeRaw 恒返回空结果集)已被
// 删除,防止生产误用导致图数据静默返回空。本文件仅保留跨实现共享的类型:
//   - hashVID / hashTenantVID: 确定性 VID 派生(HTTP 适配复用)
//   - ResultSet / SubgraphResult / Vertex / Edge: 查询结果容器
//   - ClientMetrics: 指标容器(HTTP/Console 适配复用)
//   - isRetryableError: 可重试错误判定(HTTP 适配复用)
package nebula

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strings"
	"sync"
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

// ClientMetrics 客户端指标
type ClientMetrics struct {
	TotalQueries   int64
	FailedQueries  int64
	AvgLatencyMs   float64
	ActiveSessions int
	mu             sync.RWMutex
}

// Client 两种传输适配(HTTP 网关 / 本地 console)的共同抽象。
// 上层只依赖该接口,不绑定具体传输实现(依赖倒转);
// HTTPClient 与 ConsoleClient 均显式实现(见各自文件的编译期断言)。
// 写入类操作(Insert* 系列)目前仅 HTTPClient 提供,保留在其具体类型上。
type Client interface {
	Execute(ctx context.Context, nGQL string) (*ResultSet, error)
	Ping(ctx context.Context) error
	Close() error
	GetMetrics() ClientMetrics
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
