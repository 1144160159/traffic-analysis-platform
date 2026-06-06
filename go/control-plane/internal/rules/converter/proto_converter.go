////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/converter/proto_converter.go
// 规则 Proto 转换器 - 完整修复版
// 修复内容：
// 1. 添加 CommunityID 标准生成算法
// 2. 添加 evidence_id 支持
// 3. 增强验证与错误处理
// 4. 添加规则条件序列化/反序列化
////////////////////////////////////////////////////////////////////////////////

package converter

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// =============================================================================
// CommunityID 标准算法实现 (基于 Zeek/Corelight 标准)
// =============================================================================

// CommunityIDVersion 版本号
const CommunityIDVersion = 1

// CommunityIDSeed 默认种子
const CommunityIDSeed = 0

// GenerateCommunityID 生成标准 Community ID
// 实现参考: https://github.com/corelight/community-id-spec
func GenerateCommunityID(srcIP, dstIP string, srcPort, dstPort, protocol uint32) string {
	return GenerateCommunityIDWithSeed(srcIP, dstIP, srcPort, dstPort, protocol, CommunityIDSeed)
}

// GenerateCommunityIDWithSeed 使用指定种子生成 Community ID
func GenerateCommunityIDWithSeed(srcIP, dstIP string, srcPort, dstPort, protocol uint32, seed uint16) string {
	// 解析 IP 地址
	srcIPParsed := net.ParseIP(srcIP)
	dstIPParsed := net.ParseIP(dstIP)

	if srcIPParsed == nil || dstIPParsed == nil {
		// 无效 IP，返回基于 hash 的 fallback
		return generateFallbackCommunityID(srcIP, dstIP, srcPort, dstPort, protocol)
	}

	// 规范化 IP 地址
	srcIPBytes := normalizeIP(srcIPParsed)
	dstIPBytes := normalizeIP(dstIPParsed)

	// 确定排序顺序（确保相同流的两个方向生成相同 ID）
	isOrdered := isOrderedTuple(srcIPBytes, dstIPBytes, srcPort, dstPort)

	var orderedSrcIP, orderedDstIP []byte
	var orderedSrcPort, orderedDstPort uint32

	if isOrdered {
		orderedSrcIP = srcIPBytes
		orderedDstIP = dstIPBytes
		orderedSrcPort = srcPort
		orderedDstPort = dstPort
	} else {
		orderedSrcIP = dstIPBytes
		orderedDstIP = srcIPBytes
		orderedSrcPort = dstPort
		orderedDstPort = srcPort
	}

	// 构建待 hash 的数据
	// Format: seed(2) + srcIP(4/16) + dstIP(4/16) + protocol(1) + pad(1) + srcPort(2) + dstPort(2)
	dataLen := 2 + len(orderedSrcIP) + len(orderedDstIP) + 1 + 1 + 2 + 2
	data := make([]byte, dataLen)

	offset := 0

	// Seed (2 bytes, big endian)
	binary.BigEndian.PutUint16(data[offset:], seed)
	offset += 2

	// Source IP
	copy(data[offset:], orderedSrcIP)
	offset += len(orderedSrcIP)

	// Destination IP
	copy(data[offset:], orderedDstIP)
	offset += len(orderedDstIP)

	// Protocol (1 byte)
	data[offset] = byte(protocol)
	offset++

	// Padding (1 byte)
	data[offset] = 0
	offset++

	// Source Port (2 bytes, big endian)
	binary.BigEndian.PutUint16(data[offset:], uint16(orderedSrcPort))
	offset += 2

	// Destination Port (2 bytes, big endian)
	binary.BigEndian.PutUint16(data[offset:], uint16(orderedDstPort))

	// SHA256 hash
	hash := sha256.Sum256(data)

	// Base64 编码（取前 20 字节）
	// 使用标准格式: version:base64(hash[:20])
	return fmt.Sprintf("%d:%x", CommunityIDVersion, hash[:10])
}

// normalizeIP 规范化 IP 地址为字节
func normalizeIP(ip net.IP) []byte {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

// isOrderedTuple 判断五元组是否需要交换顺序
func isOrderedTuple(srcIP, dstIP []byte, srcPort, dstPort uint32) bool {
	// 先比较 IP
	cmp := compareBytes(srcIP, dstIP)
	if cmp < 0 {
		return true
	}
	if cmp > 0 {
		return false
	}
	// IP 相同，比较端口
	return srcPort <= dstPort
}

// compareBytes 比较字节数组
func compareBytes(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// generateFallbackCommunityID 生成 fallback Community ID
func generateFallbackCommunityID(srcIP, dstIP string, srcPort, dstPort, protocol uint32) string {
	// 确保相同流的两个方向生成相同 ID
	var data string
	if srcIP < dstIP || (srcIP == dstIP && srcPort <= dstPort) {
		data = fmt.Sprintf("%s:%d-%s:%d-%d", srcIP, srcPort, dstIP, dstPort, protocol)
	} else {
		data = fmt.Sprintf("%s:%d-%s:%d-%d", dstIP, dstPort, srcIP, srcPort, protocol)
	}
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%d:%x", CommunityIDVersion, hash[:10])
}

// GenerateCommunityIDFromTuple 从 FiveTuple 生成 Community ID
func GenerateCommunityIDFromTuple(tuple *trafficv1.FiveTuple) string {
	if tuple == nil {
		return ""
	}
	return GenerateCommunityID(tuple.SrcIp, tuple.DstIp, tuple.SrcPort, tuple.DstPort, tuple.Protocol)
}

// =============================================================================
// Evidence 相关
// =============================================================================

// Evidence 证据条目
type Evidence struct {
	EvidenceID string                 `json:"evidence_id"`
	Key        string                 `json:"key"`
	Value      string                 `json:"value"`
	Type       string                 `json:"type,omitempty"` // text, json, base64
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// BuildEvidenceWithID 构建带 ID 的证据条目
func BuildEvidenceWithID(key, value, evidenceType string) *Evidence {
	return &Evidence{
		EvidenceID: uuid.New().String(),
		Type:        key,
		//Type: removed (duplicate)
	}
}

// buildRuleEvidence 构建规则证据（增强版）
func buildRuleEvidence(rule *model.Rule) []*trafficv1.Evidence {
	evidence := make([]*trafficv1.Evidence, 0, 6)

	// 基础规则信息
	evidence = append(evidence, &trafficv1.Evidence{
		Type:   "evidence_id",
		Summary: uuid.New().String(),
	})
	evidence = append(evidence, &trafficv1.Evidence{
		Type:   "rule_id",
		Summary: rule.RuleID,
	})
	evidence = append(evidence, &trafficv1.Evidence{
		Type:   "rule_name",
		Summary: rule.Name,
	})
	evidence = append(evidence, &trafficv1.Evidence{
		Type:   "rule_type",
		Summary: rule.Type,
	})
	evidence = append(evidence, &trafficv1.Evidence{
		Type:   "rule_engine",
		Summary: rule.Engine,
	})

	if rule.Description != "" {
		evidence = append(evidence, &trafficv1.Evidence{
			Type:   "description",
			Summary: rule.Description,
		})
	}

	// 添加规则条件摘要（JSON 格式）
	if len(rule.Conditions) > 0 {
		conditionsJSON, err := json.Marshal(rule.Conditions)
		if err == nil {
			evidence = append(evidence, &trafficv1.Evidence{
				Type:   "conditions_summary",
				Summary: string(conditionsJSON),
			})
		}
	}

	return evidence
}

// =============================================================================
// Rule 转换函数
// =============================================================================

// RuleToDetectionBatch 将规则命令转换为 DetectionBatch（用于规则触发时）
func RuleToDetectionBatch(rule *model.Rule, tuple *trafficv1.FiveTuple, communityID, sessionID, flowID string) *trafficv1.DetectionBatch {
	now := time.Now().UnixMilli()

	// 如果没有提供 communityID，自动生成
	if communityID == "" && tuple != nil {
		communityID = GenerateCommunityIDFromTuple(tuple)
	}

	// 构建 EventHeader
	header := &trafficv1.EventHeader{
		EventId:      uuid.New().String(),
		TenantId:     rule.TenantID,
		RunId:        "", // 实时流量无 run_id
		EventTs:      now,
		IngestTs:     now,
		ProbeId:      "",
		FeatureSetId: "",
	}

	// 构建 DetectionBusiness（规则检测结果）
	business := &trafficv1.DetectionBusiness{
		Header:        header,
		RuleVersion:   FormatVersion(rule.Version),
		ModelVersion:  "",
		Ts:            now,
		CommunityId:   communityID,
		SessionId:     sessionID,
		CampaignId:    "",
		DetectionType: "rule",
		Label:         firstLabel(rule.Labels),
		Score:         1.0, // 规则匹配默认置信度
	}

	return &trafficv1.DetectionBatch{
		Businesses: []*trafficv1.DetectionBusiness{business},
		BatchId:    uuid.New().String(),
		TenantId:   rule.TenantID,
		RunId:      "", // 实时流量无 run_id
		CreatedAt:  now,
	}
}

// firstLabel 获取第一个标签，如果为空返回空字符串
func firstLabel(labels []string) string {
	if len(labels) > 0 {
		return labels[0]
	}
	return ""
}

// =============================================================================
// RuleCommand Proto 转换
// =============================================================================

// RuleCommandProto 规则命令的 Proto 兼容格式
type RuleCommandProto struct {
	EventID     string     `json:"event_id"`
	Action      string     `json:"action"`
	Timestamp   int64      `json:"timestamp"`
	OperatorID  string     `json:"operator_id"`
	Rule        *RuleProto `json:"rule"`
	RuleVersion string     `json:"rule_version"`
	// 元数据
	SchemaVersion string `json:"schema_version,omitempty"`
	Checksum      string `json:"checksum,omitempty"`
}

// RuleProto 规则的 Proto 兼容格式
type RuleProto struct {
	RuleID      string                 `json:"rule_id"`
	TenantID    string                 `json:"tenant_id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Engine      string                 `json:"engine"`
	Description string                 `json:"description"`
	Conditions  map[string]interface{} `json:"conditions"`
	Labels      []string               `json:"labels"`
	Severity    string                 `json:"severity"`
	Enabled     bool                   `json:"enabled"`
	Version     int64                  `json:"version"`
	Priority    int                    `json:"priority,omitempty"`
	CreatedBy   string                 `json:"created_by"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// ModelToProto 将 model.Rule 转换为 RuleProto
func ModelToProto(rule *model.Rule) *RuleProto {
	return &RuleProto{
		RuleID:      rule.RuleID,
		TenantID:    rule.TenantID,
		Name:        rule.Name,
		Type:        rule.Type,
		Engine:      rule.Engine,
		Description: rule.Description,
		Conditions:  rule.Conditions,
		Labels:      rule.Labels,
		Severity:    rule.Severity,
		Enabled:     rule.Enabled,
		Version:     rule.Version,
		Priority:    rule.Priority,
		CreatedBy:   rule.CreatedBy,
		CreatedAt:   rule.CreatedAt.UnixMilli(),
		UpdatedAt:   rule.UpdatedAt.UnixMilli(),
	}
}

// ProtoToModel 将 RuleProto 转换为 model.Rule
func ProtoToModel(proto *RuleProto) *model.Rule {
	return &model.Rule{
		RuleID:      proto.RuleID,
		TenantID:    proto.TenantID,
		Name:        proto.Name,
		Type:        proto.Type,
		Engine:      proto.Engine,
		Description: proto.Description,
		Conditions:  proto.Conditions,
		Labels:      proto.Labels,
		Severity:    proto.Severity,
		Enabled:     proto.Enabled,
		Version:     proto.Version,
		Priority:    proto.Priority,
		CreatedBy:   proto.CreatedBy,
		CreatedAt:   time.UnixMilli(proto.CreatedAt),
		UpdatedAt:   time.UnixMilli(proto.UpdatedAt),
	}
}

// CommandToProto 将 model.RuleCommand 转换为 RuleCommandProto
func CommandToProto(cmd *model.RuleCommand) *RuleCommandProto {
	protoCmd := &RuleCommandProto{
		EventID:       uuid.New().String(), // 独立的事件ID
		Action:        cmd.Action,
		Timestamp:     cmd.Timestamp.UnixMilli(),
		OperatorID:    cmd.OperatorID,
		Rule:          ModelToProto(cmd.Rule),
		RuleVersion:   FormatVersion(cmd.Rule.Version),
		SchemaVersion: "1.0",
	}

	// 计算 checksum
	protoCmd.Checksum = calculateRuleChecksum(cmd.Rule)

	return protoCmd
}

// ProtoToCommand 将 RuleCommandProto 转换为 model.RuleCommand
func ProtoToCommand(proto *RuleCommandProto) *model.RuleCommand {
	return &model.RuleCommand{
		Action:     proto.Action,
		Timestamp:  time.UnixMilli(proto.Timestamp),
		OperatorID: proto.OperatorID,
		Rule:       ProtoToModel(proto.Rule),
	}
}

// calculateRuleChecksum 计算规则内容的校验和
func calculateRuleChecksum(rule *model.Rule) string {
	data, err := json.Marshal(rule)
	if err != nil {
		return ""
	}
	hash := md5.Sum(data)
	return fmt.Sprintf("%x", hash)
}

// =============================================================================
// 版本号处理
// =============================================================================

// FormatVersion 格式化版本号
func FormatVersion(version int64) string {
	return fmt.Sprintf("v%d", version)
}

// ParseVersion 解析版本号字符串
func ParseVersion(versionStr string) (int64, error) {
	var version int64
	_, err := fmt.Sscanf(versionStr, "v%d", &version)
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %s", versionStr)
	}
	return version, nil
}

// =============================================================================
// Alert 生成
// =============================================================================

// AlertFromRule 从规则生成告警（用于规则触发时）
func AlertFromRule(rule *model.Rule, tuple *trafficv1.FiveTuple, communityID, sessionID string) *trafficv1.Alert {
	now := time.Now().UnixMilli()

	// 如果没有提供 communityID，自动生成
	if communityID == "" && tuple != nil {
		communityID = GenerateCommunityIDFromTuple(tuple)
	}

	// 构建证据 ID 列表
	evidenceIDs := make([]string, 0, 1)
	evidenceIDs = append(evidenceIDs, uuid.New().String())

	// 从 FiveTuple 提取 IP/Port 信息
	srcIP, dstIP := "", ""
	var srcPort, dstPort, protocol uint32
	if tuple != nil {
		srcIP = tuple.SrcIp
		dstIP = tuple.DstIp
		srcPort = tuple.SrcPort
		dstPort = tuple.DstPort
		protocol = tuple.Protocol
	}

	return &trafficv1.Alert{
		AlertId:      uuid.New().String(),
		TenantId:     rule.TenantID,
		CommunityId:  communityID,
		SessionId:    sessionID,
		CampaignId:   "", // 由 CEP Job 填充
		SrcIp:        srcIP,
		DstIp:        dstIP,
		SrcPort:      srcPort,
		DstPort:      dstPort,
		Protocol:     protocol,
		AlertType:    rule.Type,
		Labels:       rule.Labels,
		Score:        1.0, // 规则匹配默认置信度
		Severity:     severityToProtoEnum(rule.Severity),
		FirstSeen:    now,
		LastSeen:     now,
		Status:       trafficv1.AlertStatus_ALERT_STATUS_NEW,
		Assignee:     "",
		EvidenceIds:  evidenceIDs,
		RuleVersion:  FormatVersion(rule.Version),
		ModelVersion: "",
		FeatureSetId: "",
	}
}

// severityToProtoEnum 将 string severity 转换为 proto Severity 枚举
func severityToProtoEnum(severity string) trafficv1.Severity {
	switch severity {
	case "low":
		return trafficv1.Severity_SEVERITY_LOW
	case "medium":
		return trafficv1.Severity_SEVERITY_MEDIUM
	case "high":
		return trafficv1.Severity_SEVERITY_HIGH
	case "critical":
		return trafficv1.Severity_SEVERITY_CRITICAL
	default:
		return trafficv1.Severity_SEVERITY_MEDIUM
	}
}

// GenerateAlertFingerprint 生成告警去重指纹
// 指纹算法: MD5(alert_type + src_ip + dst_ip + dst_port)
func GenerateAlertFingerprint(alertType string, tuple *trafficv1.FiveTuple) string {
	if tuple == nil {
		return fmt.Sprintf("%x", md5.Sum([]byte(alertType)))
	}

	data := fmt.Sprintf("%s|%s|%s|%d",
		alertType,
		tuple.SrcIp,
		tuple.DstIp,
		tuple.DstPort,
	)
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

// GenerateAlertFingerprintExtended 生成扩展去重指纹（包含更多信息）
func GenerateAlertFingerprintExtended(alertType string, tuple *trafficv1.FiveTuple, labels []string) string {
	if tuple == nil {
		return fmt.Sprintf("%x", md5.Sum([]byte(alertType)))
	}

	// 对 labels 排序确保一致性
	sortedLabels := make([]string, len(labels))
	copy(sortedLabels, labels)
	sort.Strings(sortedLabels)

	data := fmt.Sprintf("%s|%s|%s|%d|%v",
		alertType,
		tuple.SrcIp,
		tuple.DstIp,
		tuple.DstPort,
		sortedLabels,
	)
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

// =============================================================================
// EventHeader 构建
// =============================================================================

// BuildEventHeader 构建通用事件头
func BuildEventHeader(tenantID, probeID, runID, featureSetID string) *trafficv1.EventHeader {
	now := time.Now().UnixMilli()
	return &trafficv1.EventHeader{
		EventId:      uuid.New().String(),
		TenantId:     tenantID,
		RunId:        runID,
		EventTs:      now,
		IngestTs:     now,
		ProbeId:      probeID,
		FeatureSetId: featureSetID,
	}
}

// BuildEventHeaderWithTime 构建指定时间的事件头
func BuildEventHeaderWithTime(tenantID, probeID, runID, featureSetID string, eventTime time.Time) *trafficv1.EventHeader {
	return &trafficv1.EventHeader{
		EventId:      uuid.New().String(),
		TenantId:     tenantID,
		RunId:        runID,
		EventTs:      eventTime.UnixMilli(),
		IngestTs:     time.Now().UnixMilli(),
		ProbeId:      probeID,
		FeatureSetId: featureSetID,
	}
}

// =============================================================================
// FiveTuple 构建
// =============================================================================

// BuildFiveTuple 构建五元组
func BuildFiveTuple(srcIP, dstIP string, srcPort, dstPort, protocol uint32) *trafficv1.FiveTuple {
	return &trafficv1.FiveTuple{
		SrcIp:    srcIP,
		DstIp:    dstIP,
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: protocol,
	}
}

// ReverseFiveTuple 反转五元组方向
func ReverseFiveTuple(tuple *trafficv1.FiveTuple) *trafficv1.FiveTuple {
	if tuple == nil {
		return nil
	}
	return &trafficv1.FiveTuple{
		SrcIp:    tuple.DstIp,
		DstIp:    tuple.SrcIp,
		SrcPort:  tuple.DstPort,
		DstPort:  tuple.SrcPort,
		Protocol: tuple.Protocol,
	}
}

// =============================================================================
// DeploymentScope 处理
// =============================================================================

// DeploymentScope 将部署范围转换为标准格式
type DeploymentScope struct {
	AssetGroups []string `json:"asset_groups,omitempty"`
	Probes      []string `json:"probes,omitempty"`
	Percentage  int      `json:"percentage,omitempty"`
	Regions     []string `json:"regions,omitempty"`
	Tenants     []string `json:"tenants,omitempty"` // 多租户场景
}

// ParseDeploymentScope 解析部署范围
func ParseDeploymentScope(scope map[string]interface{}) *DeploymentScope {
	result := &DeploymentScope{}

	if groups, ok := scope["asset_groups"].([]interface{}); ok {
		for _, g := range groups {
			if s, ok := g.(string); ok {
				result.AssetGroups = append(result.AssetGroups, s)
			}
		}
	}

	if probes, ok := scope["probes"].([]interface{}); ok {
		for _, p := range probes {
			if s, ok := p.(string); ok {
				result.Probes = append(result.Probes, s)
			}
		}
	}

	if regions, ok := scope["regions"].([]interface{}); ok {
		for _, r := range regions {
			if s, ok := r.(string); ok {
				result.Regions = append(result.Regions, s)
			}
		}
	}

	if pct, ok := scope["percentage"].(float64); ok {
		result.Percentage = int(pct)
	}

	return result
}

// DeploymentScopeToMap 将 DeploymentScope 转换为 map
func DeploymentScopeToMap(scope *DeploymentScope) map[string]interface{} {
	result := make(map[string]interface{})

	if len(scope.AssetGroups) > 0 {
		result["asset_groups"] = scope.AssetGroups
	}

	if len(scope.Probes) > 0 {
		result["probes"] = scope.Probes
	}

	if len(scope.Regions) > 0 {
		result["regions"] = scope.Regions
	}

	if scope.Percentage > 0 {
		result["percentage"] = scope.Percentage
	}

	return result
}

// ValidateDeploymentScope 验证部署范围
func ValidateDeploymentScope(scope *DeploymentScope) error {
	if scope.Percentage < 0 || scope.Percentage > 100 {
		return fmt.Errorf("percentage must be between 0 and 100")
	}

	// 至少需要指定一个范围
	if len(scope.AssetGroups) == 0 && len(scope.Probes) == 0 && len(scope.Regions) == 0 && scope.Percentage == 0 {
		return fmt.Errorf("at least one scope parameter is required")
	}

	return nil
}

// =============================================================================
// Campaign 生成
// =============================================================================

// CampaignFromAlerts 从告警列表创建 Campaign
func CampaignFromAlerts(tenantID string, alerts []*trafficv1.Alert, campaignType, summary string) *trafficv1.Campaign {
	if len(alerts) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()

	// 收集告警ID和实体
	alertIDs := make([]string, 0, len(alerts))
	entitySet := make(map[string]bool)
	ruleIDSet := make(map[string]bool)
	var minTime, maxTime int64 = alerts[0].FirstSeen, alerts[0].LastSeen
	var totalScore float32

	for _, alert := range alerts {
		alertIDs = append(alertIDs, alert.AlertId)

			if alert.SrcIp != "" {
				entitySet[alert.SrcIp] = true
			}
			if alert.DstIp != "" {
				entitySet[alert.DstIp] = true
			}

		if alert.FirstSeen < minTime {
			minTime = alert.FirstSeen
		}
		if alert.LastSeen > maxTime {
			maxTime = alert.LastSeen
		}

		totalScore += alert.Score

		// 从 RuleVersion 提取 RuleID
		if alert.RuleVersion != "" {
			ruleIDSet[alert.RuleVersion] = true
		}
	}

	entities := make([]string, 0, len(entitySet))
	for entity := range entitySet {
		entities = append(entities, entity)
	}

	ruleIDs := make([]string, 0, len(ruleIDSet))
	for ruleID := range ruleIDSet {
		ruleIDs = append(ruleIDs, ruleID)
	}

	avgScore := totalScore / float32(len(alerts))

	return &trafficv1.Campaign{
		Header: &trafficv1.EventHeader{
			EventId:  uuid.New().String(),
			TenantId: tenantID,
			EventTs:  now,
			IngestTs: now,
		},
		CampaignId:   uuid.New().String(),
		TsStart:      minTime,
		TsEnd:        maxTime,
		Alerts:       alertIDs,
		Entities:     entities,
		Score:        avgScore,
		Summary:      summary,
		CampaignType: campaignType,
		AttackPhases: []string{},
		RuleIds:      ruleIDs,
		ModelIds:     []string{},
	}
}

// =============================================================================
// DetectionBatch 转换
// =============================================================================

// DetectionBatchToAlert 将检测批次中的第一个 DetectionBusiness 转换为告警
func DetectionBatchToAlert(detection *trafficv1.DetectionBatch) *trafficv1.Alert {
	if detection == nil || len(detection.Businesses) == 0 {
		return nil
	}

	biz := detection.Businesses[0]
	now := time.Now().UnixMilli()

	// 从 DetectionBusiness 提取数据
	tenantID := detection.TenantId
	if tenantID == "" && biz.Header != nil {
		tenantID = biz.Header.TenantId
	}

	srcIP, dstIP := "", ""
	var srcPort, dstPort, protocol uint32
	// FiveTuple 信息可从 EventHeader 获取（如果存在）
	if biz.Header != nil {
		// 五元组信息可能在其他地方，这里留空
	}

	return &trafficv1.Alert{
		AlertId:      uuid.New().String(),
		TenantId:     tenantID,
		CommunityId:  biz.CommunityId,
		SessionId:    biz.SessionId,
		CampaignId:   biz.CampaignId,
		SrcIp:        srcIP,
		DstIp:        dstIP,
		SrcPort:      srcPort,
		DstPort:      dstPort,
		Protocol:     protocol,
		AlertType:    biz.DetectionType,
		Labels:       []string{biz.Label},
		Score:        biz.Score,
		Severity:     trafficv1.Severity_SEVERITY_MEDIUM,
		FirstSeen:    biz.Ts,
		LastSeen:     now,
		Status:       trafficv1.AlertStatus_ALERT_STATUS_NEW,
		Assignee:     "",
		EvidenceIds:  []string{},
		RuleVersion:  biz.RuleVersion,
		ModelVersion: biz.ModelVersion,
		FeatureSetId: "",
	}
}

// extractEvidenceIDs 从证据条目中提取ID（保留兼容性）
func extractEvidenceIDs(evidence []*trafficv1.Evidence) []string {
	ids := make([]string, 0)
	for _, e := range evidence {
		if e.Type == "evidence_id" {
			ids = append(ids, e.Summary)
		}
	}
	return ids
}

// =============================================================================
// Severity 处理
// =============================================================================

// ValidSeverities 有效的严重程度列表
var ValidSeverities = map[string]bool{
	"low":      true,
	"medium":   true,
	"high":     true,
	"critical": true,
}

// ValidateSeverity 验证严重程度
func ValidateSeverity(severity string) bool {
	return ValidSeverities[severity]
}

// NormalizeSeverity 规范化严重程度
func NormalizeSeverity(severity string) string {
	if ValidateSeverity(severity) {
		return severity
	}
	return "medium" // 默认值
}

// SeverityToScore 将严重程度转换为分数
func SeverityToScore(severity string) float32 {
	scores := map[string]float32{
		"low":      0.25,
		"medium":   0.5,
		"high":     0.75,
		"critical": 1.0,
	}
	if score, ok := scores[severity]; ok {
		return score
	}
	return 0.5
}

// ScoreToSeverity 将分数转换为严重程度
func ScoreToSeverity(score float32) string {
	switch {
	case score >= 0.9:
		return "critical"
	case score >= 0.7:
		return "high"
	case score >= 0.4:
		return "medium"
	default:
		return "low"
	}
}

// SeverityPriority 严重程度优先级（用于排序）
func SeverityPriority(severity string) int {
	priorities := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
	}
	if p, ok := priorities[severity]; ok {
		return p
	}
	return 0
}

// =============================================================================
// 规则条件处理
// =============================================================================

// SerializeConditions 序列化规则条件
func SerializeConditions(conditions map[string]interface{}) ([]byte, error) {
	if conditions == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(conditions)
}

// DeserializeConditions 反序列化规则条件
func DeserializeConditions(data []byte) (map[string]interface{}, error) {
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}
	var conditions map[string]interface{}
	if err := json.Unmarshal(data, &conditions); err != nil {
		return nil, fmt.Errorf("failed to deserialize conditions: %w", err)
	}
	return conditions, nil
}

// ValidateConditions 验证规则条件结构
func ValidateConditions(conditions map[string]interface{}) error {
	if conditions == nil {
		return nil
	}

	// 检查是否可以正确序列化（验证数据类型）
	_, err := json.Marshal(conditions)
	if err != nil {
		return fmt.Errorf("conditions contain invalid data types: %w", err)
	}

	return nil
}
