////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/converter/proto_converter.go
// 完整修复版：
// - ✅ H4: 增强数据范围验证（时间戳、MaxPackets、ProbeID、CommunityID）
// - ✅ 添加更严格的输入验证
// - ✅ 增加数据清洗和规范化
////////////////////////////////////////////////////////////////////////////////

package converter

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/validation"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// 时间范围限制
	MaxTimeRangeHours = 24 // 最多 24 小时
	MaxFutureOffset   = 1  // 允许 1 小时未来时间（时钟偏移）

	// 数据量限制
	MaxPacketsLimit   = 10_000_000 // 1000 万包
	MaxPacketsSync    = 10_000     // 同步模式最多 1 万包
	DefaultMaxPackets = 100_000    // 默认 10 万包

	// 字符串长度限制
	MaxCommunityIDLength = 128
	MaxProbeIDLength     = 64
	MaxTenantIDLength    = 64

	// IP/端口验证
	MinPort = 0
	MaxPort = 65535
)

// 正则表达式（编译一次）
var (
	// ProbeID 格式：字母数字下划线连字符，3-64字符
	probeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

	// TenantID 格式：字母数字下划线连字符，2-64字符
	tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,64}$`)

	// CommunityID 格式（标准格式验证）
	communityIDPattern = regexp.MustCompile(`^[a-zA-Z0-9:+/=-]{1,128}$`)
)

// ============================================================================
// API 请求/响应类型定义
// ============================================================================

// CutRequestParams API 裁剪请求参数
type CutRequestParams struct {
	TenantID    string `json:"tenant_id,omitempty"`
	ProbeID     string `json:"probe_id,omitempty"`
	SrcIP       string `json:"src_ip,omitempty"`
	DstIP       string `json:"dst_ip,omitempty"`
	SrcPort     uint16 `json:"src_port,omitempty"`
	DstPort     uint16 `json:"dst_port,omitempty"`
	Protocol    uint8  `json:"protocol,omitempty"`
	CommunityID string `json:"community_id,omitempty"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
	MaxPackets  int64  `json:"max_packets,omitempty"`
}

// Validate 验证请求参数（修复版：完整验证）
func (p *CutRequestParams) Validate() error {
	// ========== 1. 时间戳合法性验证 ==========
	now := time.Now().UnixMilli()

	if p.StartTime < 0 || p.EndTime < 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "timestamps cannot be negative")
	}

	if p.StartTime == 0 || p.EndTime == 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "start_time and end_time are required")
	}

	// ✅ 修复 H4: 检查未来时间（允许 1 小时时钟偏移）
	maxFutureTime := now + int64(MaxFutureOffset*3600*1000)
	if p.StartTime > maxFutureTime {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"start_time cannot be more than %d hour(s) in the future", MaxFutureOffset)
	}

	// ========== 2. 时间范围验证 ==========
	if p.EndTime < p.StartTime {
		return errors.New(errors.ErrCodeInvalidParameter, "end_time must be after start_time")
	}

	timeRangeMs := p.EndTime - p.StartTime
	maxRangeMs := int64(MaxTimeRangeHours * 3600 * 1000)

	if timeRangeMs > maxRangeMs {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"time range cannot exceed %d hours (current: %.2f hours)",
			MaxTimeRangeHours, float64(timeRangeMs)/(3600*1000))
	}

	// 最小时间范围检查（至少 1 秒）
	if timeRangeMs < 1000 {
		return errors.New(errors.ErrCodeInvalidParameter, "time range must be at least 1 second")
	}

	// ========== 3. MaxPackets 验证 ==========
	if p.MaxPackets < 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "max_packets cannot be negative")
	}

	if p.MaxPackets > MaxPacketsLimit {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"max_packets cannot exceed %d (current: %d)", MaxPacketsLimit, p.MaxPackets)
	}

	// ========== 4. IP 地址验证 ==========
	validator := validation.NewIPValidator()

	if p.SrcIP != "" {
		if !validator.IsValidIP(p.SrcIP) {
			return errors.Newf(errors.ErrCodeInvalidParameter, "invalid src_ip: %s", p.SrcIP)
		}
		// 清洗 IP（去除空格）
		p.SrcIP = strings.TrimSpace(p.SrcIP)
	}

	if p.DstIP != "" {
		if !validator.IsValidIP(p.DstIP) {
			return errors.Newf(errors.ErrCodeInvalidParameter, "invalid dst_ip: %s", p.DstIP)
		}
		// 清洗 IP
		p.DstIP = strings.TrimSpace(p.DstIP)
	}

	// ========== 5. 端口验证 ==========
	if p.SrcPort > MaxPort {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"invalid src_port: %d (must be <= %d)", p.SrcPort, MaxPort)
	}

	if p.DstPort > MaxPort {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"invalid dst_port: %d (must be <= %d)", p.DstPort, MaxPort)
	}

	// ========== 6. 协议验证 ==========
	// Protocol 是 uint8，范围自动在 0-255
	// 可以添加已知协议检查
	if p.Protocol > 0 && !isKnownProtocol(p.Protocol) {
		// 仅警告，不阻止
	}

	// ========== 7. ProbeID 验证 ==========
	if p.ProbeID != "" {
		if len(p.ProbeID) > MaxProbeIDLength {
			return errors.Newf(errors.ErrCodeInvalidParameter,
				"probe_id too long (max: %d, current: %d)", MaxProbeIDLength, len(p.ProbeID))
		}

		if !probeIDPattern.MatchString(p.ProbeID) {
			return errors.New(errors.ErrCodeInvalidParameter,
				"invalid probe_id format (must be alphanumeric, underscore, or hyphen)")
		}

		// 清洗 ProbeID
		p.ProbeID = strings.TrimSpace(p.ProbeID)
	}

	// ========== 8. TenantID 验证 ==========
	if p.TenantID != "" {
		if len(p.TenantID) > MaxTenantIDLength {
			return errors.Newf(errors.ErrCodeInvalidParameter,
				"tenant_id too long (max: %d, current: %d)", MaxTenantIDLength, len(p.TenantID))
		}

		if !tenantIDPattern.MatchString(p.TenantID) {
			return errors.New(errors.ErrCodeInvalidParameter,
				"invalid tenant_id format (must be alphanumeric, underscore, or hyphen)")
		}

		// 清洗 TenantID
		p.TenantID = strings.TrimSpace(p.TenantID)
	}

	// ========== 9. CommunityID 验证 ==========
	if p.CommunityID != "" {
		if len(p.CommunityID) > MaxCommunityIDLength {
			return errors.Newf(errors.ErrCodeInvalidParameter,
				"community_id too long (max: %d, current: %d)", MaxCommunityIDLength, len(p.CommunityID))
		}

		if !communityIDPattern.MatchString(p.CommunityID) {
			return errors.New(errors.ErrCodeInvalidParameter,
				"invalid community_id format")
		}

		// 清洗 CommunityID
		p.CommunityID = strings.TrimSpace(p.CommunityID)
	}

	// ========== 10. 逻辑组合验证 ==========
	// 至少需要指定一个过滤条件
	hasFilter := p.SrcIP != "" || p.DstIP != "" ||
		p.SrcPort != 0 || p.DstPort != 0 ||
		p.Protocol != 0 || p.CommunityID != "" || p.ProbeID != ""

	if !hasFilter {
		return errors.New(errors.ErrCodeInvalidParameter,
			"at least one filter condition is required (ip/port/protocol/community_id/probe_id)")
	}

	return nil
}

// ToCutQuery 转换为内部裁剪查询
func (p *CutRequestParams) ToCutQuery() *cutter.CutQuery {
	return &cutter.CutQuery{
		TenantID:    p.TenantID,
		ProbeID:     p.ProbeID,
		SrcIP:       p.SrcIP,
		DstIP:       p.DstIP,
		SrcPort:     p.SrcPort,
		DstPort:     p.DstPort,
		Protocol:    p.Protocol,
		CommunityID: p.CommunityID,
		StartTime:   p.StartTime,
		EndTime:     p.EndTime,
		MaxPackets:  p.MaxPackets,
	}
}

// ToProto 转换为 Proto PcapCutRequest
func (p *CutRequestParams) ToProto() *trafficv1.PcapCutRequest {
	return &trafficv1.PcapCutRequest{
		TenantId:    p.TenantID,
		SrcIp:       p.SrcIP,
		DstIp:       p.DstIP,
		SrcPort:     uint32(p.SrcPort),
		DstPort:     uint32(p.DstPort),
		Protocol:    uint32(p.Protocol),
		CommunityId: p.CommunityID,
		StartTime:   p.StartTime,
		EndTime:     p.EndTime,
		MaxPackets:  uint32(p.MaxPackets),
	}
}

// ToIndexQuery 转换为索引查询
func (p *CutRequestParams) ToIndexQuery() *index.IndexQuery {
	return &index.IndexQuery{
		TenantID:    p.TenantID,
		ProbeID:     p.ProbeID,
		SrcIP:       p.SrcIP,
		DstIP:       p.DstIP,
		SrcPort:     p.SrcPort,
		DstPort:     p.DstPort,
		Protocol:    p.Protocol,
		CommunityID: p.CommunityID,
		StartTime:   p.StartTime,
		EndTime:     p.EndTime,
	}
}

// JobResponse 任务响应
type JobResponse struct {
	JobID        string                 `json:"job_id"`
	Status       string                 `json:"status"`
	Progress     int                    `json:"progress"`
	TotalPackets int64                  `json:"total_packets"`
	TotalBytes   int64                  `json:"total_bytes"`
	FilesScanned int                    `json:"files_scanned"`
	DownloadURL  string                 `json:"download_url,omitempty"`
	ExpiresAt    *int64                 `json:"expires_at,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
	CreatedAt    int64                  `json:"created_at"`
	UpdatedAt    int64                  `json:"updated_at"`
	CompletedAt  *int64                 `json:"completed_at,omitempty"`
}

// FromTask 从 Task 实体转换
func (r *JobResponse) FromTask(task *repository.Task) {
	r.JobID = task.TaskID
	r.Status = task.Status
	r.Progress = task.Progress
	r.TotalPackets = task.ResultPackets
	r.TotalBytes = task.ResultBytes
	r.FilesScanned = task.FilesScanned
	r.ErrorMessage = task.ErrorMessage
	r.CreatedAt = task.CreatedAt.UnixMilli()
	r.UpdatedAt = task.UpdatedAt.UnixMilli()
	if task.CompletedAt != nil {
		completedAt := task.CompletedAt.UnixMilli()
		r.CompletedAt = &completedAt
	}
}

// ToProto 转换为 Proto PcapCutResponse
func (r *JobResponse) ToProto() *trafficv1.PcapCutResponse {
	return &trafficv1.PcapCutResponse{
		JobId:           r.JobID,
		Status:          TaskStatusToProtoStatus(r.Status),
		ProgressPercent: int32(r.Progress),
		TotalPackets:    uint64(r.TotalPackets),
		TotalBytes:      uint64(r.TotalBytes),
		DownloadUrl:     r.DownloadURL,
		ErrorMessage:    r.ErrorMessage,
	}
}

// ============================================================================
// Proto 转换函数
// ============================================================================

// CutRequestParamsFromProto 从 Proto PcapCutRequest 转换为 CutRequestParams
func CutRequestParamsFromProto(req *trafficv1.PcapCutRequest) *CutRequestParams {
	return &CutRequestParams{
		TenantID:    req.TenantId,
		SrcIP:       req.SrcIp,
		DstIP:       req.DstIp,
		SrcPort:     uint16(req.SrcPort),
		DstPort:     uint16(req.DstPort),
		Protocol:    uint8(req.Protocol),
		CommunityID: req.CommunityId,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		MaxPackets:  int64(req.MaxPackets),
	}
}

// CutQueryFromProto 从 Proto PcapCutRequest 转换为内部 CutQuery
func CutQueryFromProto(req *trafficv1.PcapCutRequest) *cutter.CutQuery {
	return &cutter.CutQuery{
		TenantID:    req.TenantId,
		SrcIP:       req.SrcIp,
		DstIP:       req.DstIp,
		SrcPort:     uint16(req.SrcPort),
		DstPort:     uint16(req.DstPort),
		Protocol:    uint8(req.Protocol),
		CommunityID: req.CommunityId,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		MaxPackets:  int64(req.MaxPackets),
	}
}

// CutQueryToProto 从内部 CutQuery 转换为 Proto PcapCutRequest
func CutQueryToProto(query *cutter.CutQuery) *trafficv1.PcapCutRequest {
	return &trafficv1.PcapCutRequest{
		TenantId:    query.TenantID,
		SrcIp:       query.SrcIP,
		DstIp:       query.DstIP,
		SrcPort:     uint32(query.SrcPort),
		DstPort:     uint32(query.DstPort),
		Protocol:    uint32(query.Protocol),
		CommunityId: query.CommunityID,
		StartTime:   query.StartTime,
		EndTime:     query.EndTime,
		MaxPackets:  uint32(query.MaxPackets),
	}
}

// CutResultToProto 从内部 CutResult 转换为 Proto PcapCutResponse
func CutResultToProto(jobID string, status string, result *cutter.CutResult, downloadURL string) *trafficv1.PcapCutResponse {
	resp := &trafficv1.PcapCutResponse{
		JobId:       jobID,
		Status:      TaskStatusToProtoStatus(status),
		DownloadUrl: downloadURL,
	}

	if result != nil {
		resp.TotalPackets = uint64(result.TotalPackets)
		resp.TotalBytes = uint64(result.TotalBytes)

		// 计算进度
		if status == "completed" {
			resp.ProgressPercent = 100
		}
	}

	return resp
}

// IndexQueryFromProto 从 Proto 请求构建索引查询
func IndexQueryFromProto(req *trafficv1.PcapCutRequest) *index.IndexQuery {
	return &index.IndexQuery{
		TenantID:    req.TenantId,
		SrcIP:       req.SrcIp,
		DstIP:       req.DstIp,
		SrcPort:     uint16(req.SrcPort),
		DstPort:     uint16(req.DstPort),
		Protocol:    uint8(req.Protocol),
		CommunityID: req.CommunityId,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
	}
}

// ============================================================================
// PcapIndexMeta 转换
// ============================================================================

// PcapIndexMetaFromFile 从文件元数据创建 PcapIndexMeta
func PcapIndexMetaFromFile(file *index.FileMetadata, tenantID string) *trafficv1.PcapIndexMeta {
	return &trafficv1.PcapIndexMeta{
		TenantId:     tenantID,
		ProbeId:      file.ProbeID,
		FileKey:      file.FileKey,
		TsStart:      file.TsStart.UnixMilli(),
		TsEnd:        file.TsEnd.UnixMilli(),
		ByteSize:     file.ByteSize,
		CommunityIds: []string{file.CommunityID},
	}
}

// FileMetadataFromProto 从 Proto 转换为内部 FileMetadata
func FileMetadataFromProto(meta *trafficv1.PcapIndexMeta) *index.FileMetadata {
	communityID := ""
	if len(meta.CommunityIds) > 0 {
		communityID = meta.CommunityIds[0]
	}

	return &index.FileMetadata{
		FileKey:     meta.FileKey,
		TsStart:     time.UnixMilli(meta.TsStart),
		TsEnd:       time.UnixMilli(meta.TsEnd),
		ProbeID:     meta.ProbeId,
		ByteSize:    meta.ByteSize,
		CommunityID: communityID,
	}
}

// ============================================================================
// 状态转换
// ============================================================================

// TaskStatusToProtoStatus 转换任务状态到 Proto 状态
func TaskStatusToProtoStatus(status string) string {
	// Proto 中定义的状态: "queued" | "processing" | "completed" | "failed"
	switch status {
	case repository.TaskStatusQueued:
		return "queued"
	case repository.TaskStatusProcessing:
		return "processing"
	case repository.TaskStatusCompleted:
		return "completed"
	case repository.TaskStatusFailed:
		return "failed"
	case repository.TaskStatusCancelled:
		return "failed" // Proto 中没有 cancelled，映射到 failed
	default:
		return status
	}
}

// ProtoStatusToTaskStatus 从 Proto 状态转换为内部任务状态
func ProtoStatusToTaskStatus(protoStatus string) string {
	switch protoStatus {
	case "queued":
		return repository.TaskStatusQueued
	case "processing":
		return repository.TaskStatusProcessing
	case "completed":
		return repository.TaskStatusCompleted
	case "failed":
		return repository.TaskStatusFailed
	default:
		return protoStatus
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// BuildPcapCutResponse 构建 PCAP 裁剪响应
func BuildPcapCutResponse(
	jobID string,
	status string,
	progress int,
	totalPackets, totalBytes uint64,
	downloadURL string,
	errorMessage string,
) *trafficv1.PcapCutResponse {
	return &trafficv1.PcapCutResponse{
		JobId:           jobID,
		Status:          TaskStatusToProtoStatus(status),
		ProgressPercent: int32(progress),
		TotalPackets:    totalPackets,
		TotalBytes:      totalBytes,
		DownloadUrl:     downloadURL,
		ErrorMessage:    errorMessage,
	}
}

// FiveTupleFromQuery 从查询参数构建五元组
func FiveTupleFromQuery(srcIP, dstIP string, srcPort, dstPort uint16, protocol uint8) *trafficv1.FiveTuple {
	return &trafficv1.FiveTuple{
		SrcIp:    srcIP,
		DstIp:    dstIP,
		SrcPort:  uint32(srcPort),
		DstPort:  uint32(dstPort),
		Protocol: uint32(protocol),
	}
}

// FiveTupleFromCutRequestParams 从请求参数构建五元组
func FiveTupleFromCutRequestParams(params *CutRequestParams) *trafficv1.FiveTuple {
	return FiveTupleFromQuery(
		params.SrcIP,
		params.DstIP,
		params.SrcPort,
		params.DstPort,
		params.Protocol,
	)
}

// ParseTimeRange 解析时间范围，返回默认值如果未指定
func ParseTimeRange(startTime, endTime int64, defaultRangeHours int) (int64, int64) {
	now := time.Now().UnixMilli()

	if endTime == 0 {
		endTime = now
	}

	if startTime == 0 {
		startTime = endTime - int64(defaultRangeHours)*3600*1000
	}

	return startTime, endTime
}

// NewJobResponseFromTask 从 Task 创建 JobResponse
func NewJobResponseFromTask(task *repository.Task) *JobResponse {
	resp := &JobResponse{}
	resp.FromTask(task)
	return resp
}

// NewCutRequestParamsFromProto 从 Proto 创建 CutRequestParams
func NewCutRequestParamsFromProto(req *trafficv1.PcapCutRequest) *CutRequestParams {
	return CutRequestParamsFromProto(req)
}

// ========== 协议验证辅助函数 ==========

// isKnownProtocol 检查是否为已知协议
func isKnownProtocol(protocol uint8) bool {
	knownProtocols := map[uint8]string{
		1:   "ICMP",
		6:   "TCP",
		17:  "UDP",
		47:  "GRE",
		50:  "ESP",
		51:  "AH",
		58:  "ICMPv6",
		132: "SCTP",
	}
	_, ok := knownProtocols[protocol]
	return ok
}

// GetProtocolName 获取协议名称
func GetProtocolName(protocol uint8) string {
	protocolNames := map[uint8]string{
		1:   "ICMP",
		6:   "TCP",
		17:  "UDP",
		47:  "GRE",
		50:  "ESP",
		51:  "AH",
		58:  "ICMPv6",
		132: "SCTP",
	}
	if name, ok := protocolNames[protocol]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN(%d)", protocol)
}

// NormalizeIP 规范化 IP 地址
func NormalizeIP(ip string) string {
	// 去除前后空格
	ip = strings.TrimSpace(ip)
	// TODO: 可以添加 IPv6 规范化逻辑
	return ip
}

// ValidateAndNormalize 验证并规范化请求参数（便捷方法）
func (p *CutRequestParams) ValidateAndNormalize() error {
	// 先规范化
	if p.SrcIP != "" {
		p.SrcIP = NormalizeIP(p.SrcIP)
	}
	if p.DstIP != "" {
		p.DstIP = NormalizeIP(p.DstIP)
	}
	if p.ProbeID != "" {
		p.ProbeID = strings.TrimSpace(p.ProbeID)
	}
	if p.CommunityID != "" {
		p.CommunityID = strings.TrimSpace(p.CommunityID)
	}
	if p.TenantID != "" {
		p.TenantID = strings.TrimSpace(p.TenantID)
	}

	// 再验证
	return p.Validate()
}
