////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/persistence/alert.go
// 修复版：
// 1. 修复证据 ID 生成可能重复问题（使用索引 + 哈希）
// 2. 添加完整 Proto 映射、转换方法、辅助函数
////////////////////////////////////////////////////////////////////////////////

package persistence

import (
	"crypto/md5"
	"fmt"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// Alert 告警持久化对象
// 对应 ClickHouse 表 traffic.alerts_local
//
// DDL 参考：
// CREATE TABLE traffic.alerts_local (
//
//	tenant_id       String,
//	alert_id        String,
//	dedup_fingerprint String,
//	community_id    String,
//	session_id      String,
//	campaign_id     String,
//	src_ip          String,
//	dst_ip          String,
//	src_port        UInt16,
//	dst_port        UInt16,
//	protocol        UInt8,
//	alert_type      String,
//	labels          Array(String),
//	score           Float32,
//	severity        String,
//	first_seen      DateTime64(3),
//	last_seen       DateTime64(3),
//	count           Int32,
//	status          String,
//	assignee        String,
//	updated_ts      DateTime64(3),
//	model_version   String,
//	rule_version    String,
//	feature_set_id  String,
//	evidence_ids    Array(String),
//	event_id        String
//
// ) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/alerts_local', '{replica}', updated_ts)
// PARTITION BY toDate(first_seen)
// ORDER BY (tenant_id, first_seen, community_id, alert_id)
// TTL first_seen + INTERVAL 30 DAY;
type Alert struct {
	// ==================== 租户与标识 ====================
	// tenant_id: 租户标识，用于数据隔离
	TenantID string `json:"tenant_id" ch:"tenant_id"`

	// alert_id: 告警唯一标识（UUID）
	AlertID string `json:"alert_id" ch:"alert_id"`

	// dedup_fingerprint: 去重指纹，用于告警聚合
	// 计算方式：MD5(tenant_id + alert_type + src_ip + dst_ip + dst_port + severity + time_bucket)
	Fingerprint string `json:"fingerprint" ch:"dedup_fingerprint"`

	// ==================== 关联标识 ====================
	// community_id: 会话标识（Community ID v1 标准）
	CommunityID string `json:"community_id" ch:"community_id"`

	// session_id: 内部会话标识
	SessionID string `json:"session_id" ch:"session_id"`

	// campaign_id: 战役标识（由 CEP 生成，可选）
	CampaignID string `json:"campaign_id" ch:"campaign_id"`

	// ==================== 网络五元组 ====================
	// src_ip: 源 IP 地址
	SrcIP string `json:"src_ip" ch:"src_ip"`

	// dst_ip: 目的 IP 地址
	DstIP string `json:"dst_ip" ch:"dst_ip"`

	// src_port: 源端口
	SrcPort uint16 `json:"src_port" ch:"src_port"`

	// dst_port: 目的端口
	DstPort uint16 `json:"dst_port" ch:"dst_port"`

	// protocol: 协议号（6=TCP, 17=UDP, 1=ICMP）
	Protocol uint8 `json:"protocol" ch:"protocol"`

	// ==================== 分类信息 ====================
	// alert_type: 告警类型（如 "malware", "ddos", "scan" 等）
	AlertType string `json:"alert_type" ch:"alert_type"`

	// attack_phase: 由显式 MITRE/phase 标签优先、检测类型语义次优推导的规范攻击阶段。
	// 它是查询投影字段，不写回原始 alerts 事件表。
	AttackPhase string `json:"attack_phase,omitempty" ch:"-"`

	// labels: 标签列表
	Labels []string `json:"labels" ch:"labels"`

	// score: 置信度分数（0.0 - 1.0）
	Score float32 `json:"score" ch:"score"`

	// severity: 严重程度（critical, high, medium, low, info）
	Severity string `json:"severity" ch:"severity"`

	// ==================== 时间信息 ====================
	// first_seen: 首次发现时间
	FirstSeen time.Time `json:"first_seen" ch:"first_seen"`

	// last_seen: 最后发现时间
	LastSeen time.Time `json:"last_seen" ch:"last_seen"`

	// count: 聚合次数（去重后的事件计数）
	Count int32 `json:"count" ch:"count"`

	// ==================== 状态管理 ====================
	// status: 告警状态（new, triage, assigned, closed）
	Status string `json:"status" ch:"status"`

	// assignee: 分配给的处理人
	Assignee string `json:"assignee,omitempty" ch:"assignee"`

	// updated_ts: 更新时间戳（用于 ReplacingMergeTree 版本控制）
	UpdatedTs time.Time `json:"updated_ts" ch:"updated_ts"`

	// ==================== 版本信息 ====================
	// model_version: 检测模型版本
	ModelVersion string `json:"model_version" ch:"model_version"`

	// rule_version: 检测规则版本
	RuleVersion string `json:"rule_version" ch:"rule_version"`

	// feature_set_id: 特征集标识
	FeatureSetID string `json:"feature_set_id" ch:"feature_set_id"`

	// ==================== 证据与追踪 ====================
	// evidence_ids: 关联的证据 ID 列表
	EvidenceIDs []string `json:"evidence_ids,omitempty" ch:"evidence_ids"`

	// event_id: 原始事件 ID（用于追溯）
	EventID string `json:"event_id" ch:"event_id"`

	// ==================== 扩展字段（不存储到 ClickHouse）====================
	// arkime_link: Arkime 查询链接（运行时生成）
	ArkimeLink string `json:"arkime_link,omitempty" ch:"-"`

	// evidence_count: 证据数量（运行时计算）
	EvidenceCount int `json:"evidence_count,omitempty" ch:"-"`
}

// Proto 与 Alert 字段映射关系：
//
// Proto DetectionEvent Field    -> Alert Field
// ─────────────────────────────────────────────────────────
// header.tenant_id             -> TenantID
// header.event_id              -> EventID
// header.feature_set_id        -> FeatureSetID
// header.probe_id              -> (不存储，仅日志)
// header.run_id                -> (不存储，仅日志)
// header.event_ts              -> (用于计算 FirstSeen/LastSeen)
// detection_id                 -> (不存储，使用生成的 AlertID)
// community_id                 -> CommunityID
// session_id                   -> SessionID
// flow_id                      -> (不存储，通过 community_id 关联)
// tuple.src_ip                 -> SrcIP
// tuple.dst_ip                 -> DstIP
// tuple.src_port               -> SrcPort
// tuple.dst_port               -> DstPort
// tuple.protocol               -> Protocol
// detection_type               -> AlertType
// labels                       -> Labels
// score                        -> Score
// severity                     -> Severity
// ts_start                     -> (用于计算时间范围)
// ts_end                       -> (用于计算时间范围)
// model_id                     -> (不存储)
// model_version                -> ModelVersion
// rule_id                      -> (不存储)
// rule_version                 -> RuleVersion
// evidence                     -> (转换为 EvidenceIDs)

// NewAlertFromProto 从 Proto DetectionEvent 创建 Alert
// 修复：使用索引 + 哈希生成唯一证据 ID
func NewAlertFromProto(detection *pb.DetectionBatch, alertID, fingerprint string, count int32, firstSeen, lastSeen time.Time) *Alert {
	if detection == nil {
		return nil
	}

	// Extract from first behavior/business in batch
	tenantID := detection.GetTenantId()
	eventID := detection.GetBatchId()
	featureSetID := ""
	srcIP, dstIP := "", ""
	var srcPort, dstPort uint16
	var protocol uint8
	communityID := ""
	sessionID := ""
	alertType := ""
	severity := ""
	modelVersion := ""
	ruleVersion := ""
	var score float32
	var labels []string
	var evidenceList []*pb.Evidence

	if len(detection.Behaviors) > 0 {
		b := detection.Behaviors[0]
		if b.Header != nil {
			eventID = b.Header.GetEventId()
			featureSetID = b.Header.GetFeatureSetId()
		}
		communityID = b.GetCommunityId()
		alertType = b.GetObjectType()
		severity = b.GetTopLabel()
		score = b.GetTopScore()
		labels = b.GetLabels()
	} else if len(detection.Businesses) > 0 {
		bu := detection.Businesses[0]
		if bu.Header != nil {
			eventID = bu.Header.GetEventId()
			featureSetID = bu.Header.GetFeatureSetId()
		}
		communityID = bu.GetCommunityId()
		sessionID = bu.GetSessionId()
		alertType = bu.GetDetectionType()
		modelVersion = bu.GetModelVersion()
		ruleVersion = bu.GetRuleVersion()
	}

	if labels == nil {
		labels = []string{}
	}

	evidenceIDs := generateEvidenceIDs(eventID, evidenceList)

	return &Alert{
		TenantID:    tenantID,
		AlertID:     alertID,
		Fingerprint: fingerprint,
		CommunityID: communityID,
		SessionID:   sessionID,
		CampaignID:  "",

		SrcIP:    srcIP,
		DstIP:    dstIP,
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: protocol,

		AlertType: alertType,
		Labels:    labels,
		Score:     score,
		Severity:  severity,

		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
		Count:     count,

		Status:    "new",
		Assignee:  "",
		UpdatedTs: time.Now(),

		ModelVersion: modelVersion,
		RuleVersion:  ruleVersion,
		FeatureSetID: featureSetID,

		EvidenceIDs: evidenceIDs,
		EventID:     eventID,
	}
}

// generateEvidenceIDs 生成唯一的证据 ID 列表
// 修复：使用索引和哈希确保 ID 唯一性
func generateEvidenceIDs(eventID string, evidenceEntries []*pb.Evidence) []string {
	if len(evidenceEntries) == 0 {
		return []string{}
	}

	evidenceIDs := make([]string, 0, len(evidenceEntries))
	seen := make(map[string]bool) // 用于去重

	for idx, ev := range evidenceEntries {
		if ev == nil {
			continue
		}

		// 生成唯一 ID：eventID + 索引 + key + value 的哈希
		// 格式：eventID:idx:hash
		var evidenceID string

		if ev.Type != "" {
			// 使用 MD5 哈希保证唯一性（即使 key:value 相同，索引不同也不会重复）
			data := fmt.Sprintf("%s:%d:%s:%s", eventID, idx, ev.Type, ev.Summary)
			hash := md5.Sum([]byte(data))
			evidenceID = fmt.Sprintf("%s:%d:%x", eventID, idx, hash[:8]) // 使用前8字节
		} else {
			// 没有 key 的情况，仅使用索引
			data := fmt.Sprintf("%s:%d:%s", eventID, idx, ev.Summary)
			hash := md5.Sum([]byte(data))
			evidenceID = fmt.Sprintf("%s:%d:%x", eventID, idx, hash[:8])
		}

		// 检查是否重复（理论上不会重复，但双重保险）
		if !seen[evidenceID] {
			seen[evidenceID] = true
			evidenceIDs = append(evidenceIDs, evidenceID)
		}
	}

	return evidenceIDs
}

// GenerateEvidenceID 生成单个证据 ID（公开方法，供外部使用）
// 格式：eventID:idx:hash
func GenerateEvidenceID(eventID string, index int, key, value string) string {
	data := fmt.Sprintf("%s:%d:%s:%s", eventID, index, key, value)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%s:%d:%x", eventID, index, hash[:8])
}

// ToProto 将 Alert 转换为 Proto Alert（用于 API 响应或事件发布）
func (a *Alert) ToProto() *pb.Alert {
	if a == nil {
		return nil
	}

	return &pb.Alert{
		AlertId:          a.AlertID,
		TenantId:         a.TenantID,
		DedupFingerprint: a.Fingerprint,
		CommunityId:      a.CommunityID,
		SessionId:        a.SessionID,
		CampaignId:       a.CampaignID,
		AlertType:        a.AlertType,
		Labels:           a.Labels,
		Score:            a.Score,
		Severity:         pb.Severity(pb.Severity_value[a.Severity]),
		FirstSeen:        a.FirstSeen.UnixMilli(),
		LastSeen:         a.LastSeen.UnixMilli(),
		Count:            a.Count,
		Status:           pb.AlertStatus(pb.AlertStatus_value[a.Status]),
		Assignee:         a.Assignee,
		UpdatedTs:        a.UpdatedTs.UnixMilli(),
		ModelVersion:     a.ModelVersion,
		RuleVersion:      a.RuleVersion,
		FeatureSetId:     a.FeatureSetID,
		EvidenceIds:      a.EvidenceIDs,
		EventId:          a.EventID,
		SrcIp:            a.SrcIP,
		DstIp:            a.DstIP,
		SrcPort:          uint32(a.SrcPort),
		DstPort:          uint32(a.DstPort),
		Protocol:         uint32(a.Protocol),
	}
}

// Clone 深拷贝 Alert
func (a *Alert) Clone() *Alert {
	if a == nil {
		return nil
	}

	clone := *a

	// 深拷贝 slice 字段
	if a.Labels != nil {
		clone.Labels = make([]string, len(a.Labels))
		copy(clone.Labels, a.Labels)
	}

	if a.EvidenceIDs != nil {
		clone.EvidenceIDs = make([]string, len(a.EvidenceIDs))
		copy(clone.EvidenceIDs, a.EvidenceIDs)
	}

	return &clone
}

// UpdateFromDedup 从去重结果更新 Alert
func (a *Alert) UpdateFromDedup(count int64, firstSeen, lastSeen int64) {
	a.Count = int32(count)
	if firstSeen > 0 {
		a.FirstSeen = time.UnixMilli(firstSeen)
	}
	if lastSeen > 0 {
		a.LastSeen = time.UnixMilli(lastSeen)
	}
	a.UpdatedTs = time.Now()
}

// SetStatus 设置状态并更新时间戳
func (a *Alert) SetStatus(status string) {
	a.Status = status
	a.UpdatedTs = time.Now()
}

// SetAssignee 设置分配人并更新状态
func (a *Alert) SetAssignee(assignee string) {
	a.Assignee = assignee
	if assignee != "" && a.Status != "closed" {
		a.Status = "assigned"
	}
	a.UpdatedTs = time.Now()
}

// AddEvidenceID 添加证据 ID（带去重检查）
func (a *Alert) AddEvidenceID(evidenceID string) {
	if evidenceID == "" {
		return
	}
	// 检查是否已存在
	for _, id := range a.EvidenceIDs {
		if id == evidenceID {
			return
		}
	}
	a.EvidenceIDs = append(a.EvidenceIDs, evidenceID)
}

// AddEvidenceIDs 批量添加证据 ID（带去重检查）
func (a *Alert) AddEvidenceIDs(evidenceIDs []string) {
	for _, id := range evidenceIDs {
		a.AddEvidenceID(id)
	}
}

// RemoveEvidenceID 移除证据 ID
func (a *Alert) RemoveEvidenceID(evidenceID string) {
	if evidenceID == "" {
		return
	}
	newIDs := make([]string, 0, len(a.EvidenceIDs))
	for _, id := range a.EvidenceIDs {
		if id != evidenceID {
			newIDs = append(newIDs, id)
		}
	}
	a.EvidenceIDs = newIDs
}

// HasEvidenceID 检查是否包含指定的证据 ID
func (a *Alert) HasEvidenceID(evidenceID string) bool {
	for _, id := range a.EvidenceIDs {
		if id == evidenceID {
			return true
		}
	}
	return false
}

// AddLabel 添加标签
func (a *Alert) AddLabel(label string) {
	if label == "" {
		return
	}
	// 检查是否已存在
	for _, l := range a.Labels {
		if l == label {
			return
		}
	}
	a.Labels = append(a.Labels, label)
}

// RemoveLabel 移除标签
func (a *Alert) RemoveLabel(label string) {
	if label == "" {
		return
	}
	newLabels := make([]string, 0, len(a.Labels))
	for _, l := range a.Labels {
		if l != label {
			newLabels = append(newLabels, l)
		}
	}
	a.Labels = newLabels
}

// HasLabel 检查是否有指定标签
func (a *Alert) HasLabel(label string) bool {
	for _, l := range a.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// GetProtocolName 获取协议名称
func (a *Alert) GetProtocolName() string {
	return ProtocolToName(a.Protocol)
}

// ProtocolToName 将协议号转换为名称
func ProtocolToName(protocol uint8) string {
	switch protocol {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 47:
		return "GRE"
	case 50:
		return "ESP"
	case 51:
		return "AH"
	case 58:
		return "ICMPv6"
	case 89:
		return "OSPF"
	case 132:
		return "SCTP"
	default:
		return fmt.Sprintf("PROTO_%d", protocol)
	}
}

// NameToProtocol 将协议名称转换为协议号
func NameToProtocol(name string) uint8 {
	switch name {
	case "ICMP":
		return 1
	case "TCP":
		return 6
	case "UDP":
		return 17
	case "GRE":
		return 47
	case "ESP":
		return 50
	case "AH":
		return 51
	case "ICMPv6":
		return 58
	case "OSPF":
		return 89
	case "SCTP":
		return 132
	default:
		return 0
	}
}

// GetSeverityLevel 获取严重程度等级（用于排序）
func (a *Alert) GetSeverityLevel() int {
	return SeverityToLevel(a.Severity)
}

// SeverityToLevel 将严重程度转换为数字等级
func SeverityToLevel(severity string) int {
	switch severity {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// LevelToSeverity 将数字等级转换为严重程度
func LevelToSeverity(level int) string {
	switch level {
	case 5:
		return "critical"
	case 4:
		return "high"
	case 3:
		return "medium"
	case 2:
		return "low"
	case 1:
		return "info"
	default:
		return "unknown"
	}
}

// IsOpen 检查告警是否处于打开状态
func (a *Alert) IsOpen() bool {
	return a.Status != "closed"
}

// IsClosed 检查告警是否已关闭
func (a *Alert) IsClosed() bool {
	return a.Status == "closed"
}

// IsAssigned 检查告警是否已分配
func (a *Alert) IsAssigned() bool {
	return a.Status == "assigned" && a.Assignee != ""
}

// Duration 获取告警持续时间
func (a *Alert) Duration() time.Duration {
	return a.LastSeen.Sub(a.FirstSeen)
}

// Age 获取告警年龄（从首次发现到现在）
func (a *Alert) Age() time.Duration {
	return time.Since(a.FirstSeen)
}

// SinceLastSeen 获取距离最后发现的时间
func (a *Alert) SinceLastSeen() time.Duration {
	return time.Since(a.LastSeen)
}

// IsStale 检查告警是否过期（超过指定时间未更新）
func (a *Alert) IsStale(threshold time.Duration) bool {
	return time.Since(a.LastSeen) > threshold
}

// Validate 验证 Alert 字段
func (a *Alert) Validate() error {
	if a.TenantID == "" {
		return &AlertValidationError{Field: "tenant_id", Message: "tenant_id is required"}
	}
	if a.AlertID == "" {
		return &AlertValidationError{Field: "alert_id", Message: "alert_id is required"}
	}
	if a.AlertType == "" {
		return &AlertValidationError{Field: "alert_type", Message: "alert_type is required"}
	}
	if a.Severity == "" {
		return &AlertValidationError{Field: "severity", Message: "severity is required"}
	}
	if !isValidSeverity(a.Severity) {
		return &AlertValidationError{Field: "severity", Message: "invalid severity value: " + a.Severity}
	}
	if !isValidStatus(a.Status) {
		return &AlertValidationError{Field: "status", Message: "invalid status value: " + a.Status}
	}
	if a.Score < 0 || a.Score > 1 {
		return &AlertValidationError{Field: "score", Message: "score must be between 0 and 1"}
	}
	if a.FirstSeen.IsZero() {
		return &AlertValidationError{Field: "first_seen", Message: "first_seen is required"}
	}
	if a.LastSeen.IsZero() {
		return &AlertValidationError{Field: "last_seen", Message: "last_seen is required"}
	}
	if a.LastSeen.Before(a.FirstSeen) {
		return &AlertValidationError{Field: "last_seen", Message: "last_seen cannot be before first_seen"}
	}
	return nil
}

// AlertValidationError 告警验证错误
type AlertValidationError struct {
	Field   string
	Message string
}

func (e *AlertValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// isValidSeverity 验证严重程度
func isValidSeverity(severity string) bool {
	switch severity {
	case "critical", "high", "medium", "low", "info":
		return true
	default:
		return false
	}
}

// isValidStatus 验证状态
func isValidStatus(status string) bool {
	switch status {
	case "new", "triage", "assigned", "closed":
		return true
	default:
		return false
	}
}

// ValidSeverities 返回所有有效的严重程度列表
func ValidSeverities() []string {
	return []string{"critical", "high", "medium", "low", "info"}
}

// ValidStatuses 返回所有有效的状态列表
func ValidStatuses() []string {
	return []string{"new", "triage", "assigned", "closed"}
}

// AlertBatch 告警批次（用于批量操作）
type AlertBatch struct {
	Alerts    []*Alert
	TenantID  string
	BatchID   string
	CreatedAt time.Time
}

// NewAlertBatch 创建告警批次
func NewAlertBatch(tenantID, batchID string) *AlertBatch {
	return &AlertBatch{
		Alerts:    make([]*Alert, 0),
		TenantID:  tenantID,
		BatchID:   batchID,
		CreatedAt: time.Now(),
	}
}

// Add 添加告警到批次
func (b *AlertBatch) Add(alert *Alert) {
	if alert != nil {
		b.Alerts = append(b.Alerts, alert)
	}
}

// AddAll 批量添加告警
func (b *AlertBatch) AddAll(alerts []*Alert) {
	for _, alert := range alerts {
		b.Add(alert)
	}
}

// Size 获取批次大小
func (b *AlertBatch) Size() int {
	return len(b.Alerts)
}

// IsEmpty 检查批次是否为空
func (b *AlertBatch) IsEmpty() bool {
	return len(b.Alerts) == 0
}

// IsFull 检查批次是否已满
func (b *AlertBatch) IsFull(maxSize int) bool {
	return len(b.Alerts) >= maxSize
}

// Clear 清空批次
func (b *AlertBatch) Clear() {
	b.Alerts = b.Alerts[:0]
}

// GetAlertIDs 获取所有告警 ID
func (b *AlertBatch) GetAlertIDs() []string {
	ids := make([]string, len(b.Alerts))
	for i, alert := range b.Alerts {
		ids[i] = alert.AlertID
	}
	return ids
}

// FilterBySeverity 按严重程度过滤
func (b *AlertBatch) FilterBySeverity(severity string) []*Alert {
	result := make([]*Alert, 0)
	for _, alert := range b.Alerts {
		if alert.Severity == severity {
			result = append(result, alert)
		}
	}
	return result
}

// FilterByStatus 按状态过滤
func (b *AlertBatch) FilterByStatus(status string) []*Alert {
	result := make([]*Alert, 0)
	for _, alert := range b.Alerts {
		if alert.Status == status {
			result = append(result, alert)
		}
	}
	return result
}

// GroupBySeverity 按严重程度分组
func (b *AlertBatch) GroupBySeverity() map[string][]*Alert {
	groups := make(map[string][]*Alert)
	for _, alert := range b.Alerts {
		groups[alert.Severity] = append(groups[alert.Severity], alert)
	}
	return groups
}

// GroupByStatus 按状态分组
func (b *AlertBatch) GroupByStatus() map[string][]*Alert {
	groups := make(map[string][]*Alert)
	for _, alert := range b.Alerts {
		groups[alert.Status] = append(groups[alert.Status], alert)
	}
	return groups
}

// GroupByAlertType 按告警类型分组
func (b *AlertBatch) GroupByAlertType() map[string][]*Alert {
	groups := make(map[string][]*Alert)
	for _, alert := range b.Alerts {
		groups[alert.AlertType] = append(groups[alert.AlertType], alert)
	}
	return groups
}

// SortByLastSeen 按最后发现时间排序（降序）
func (b *AlertBatch) SortByLastSeen() {
	// 使用简单的冒泡排序（对于小批次足够）
	n := len(b.Alerts)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if b.Alerts[j].LastSeen.Before(b.Alerts[j+1].LastSeen) {
				b.Alerts[j], b.Alerts[j+1] = b.Alerts[j+1], b.Alerts[j]
			}
		}
	}
}

// SortBySeverity 按严重程度排序（降序）
func (b *AlertBatch) SortBySeverity() {
	n := len(b.Alerts)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if b.Alerts[j].GetSeverityLevel() < b.Alerts[j+1].GetSeverityLevel() {
				b.Alerts[j], b.Alerts[j+1] = b.Alerts[j+1], b.Alerts[j]
			}
		}
	}
}

// Stats 获取批次统计信息
func (b *AlertBatch) Stats() *AlertBatchStats {
	stats := &AlertBatchStats{
		Total:      len(b.Alerts),
		BySeverity: make(map[string]int),
		ByStatus:   make(map[string]int),
		ByType:     make(map[string]int),
	}

	for _, alert := range b.Alerts {
		stats.BySeverity[alert.Severity]++
		stats.ByStatus[alert.Status]++
		stats.ByType[alert.AlertType]++
	}

	return stats
}

// AlertBatchStats 批次统计
type AlertBatchStats struct {
	Total      int            `json:"total"`
	BySeverity map[string]int `json:"by_severity"`
	ByStatus   map[string]int `json:"by_status"`
	ByType     map[string]int `json:"by_type"`
}
