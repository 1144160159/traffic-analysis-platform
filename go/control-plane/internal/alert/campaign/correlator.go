// Campaign Correlation Engine — 将独立告警关联为攻击活动 (Attack Campaign)
//
// 业务价值: 将孤立的告警聚合为有上下文的攻击链，帮助 SOC 分析师理解攻击全景
// 关联维度:
//   1. 时间关联 — 短时间窗口内密集告警
//   2. 空间关联 — 相同源/目标 IP 的告警
//   3. 行为关联 — 攻击链阶段递进（侦察→初始访问→执行→持久化→横向移动→外泄）
//   4. 社区关联 — 相同 community_id 的告警
package campaign

import (
	"context"
	"crypto/md5"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AttackPhase 攻击链阶段 (MITRE ATT&CK 映射)
type AttackPhase string

const (
	PhaseReconnaissance     AttackPhase = "reconnaissance"      // 侦察
	PhaseInitialAccess      AttackPhase = "initial_access"      // 初始访问
	PhaseExecution          AttackPhase = "execution"           // 执行
	PhasePersistence        AttackPhase = "persistence"         // 持久化
	PhasePrivilegeEscalation AttackPhase = "privilege_escalation" // 权限提升
	PhaseDefenseEvasion     AttackPhase = "defense_evasion"     // 防御规避
	PhaseCredentialAccess   AttackPhase = "credential_access"   // 凭证访问
	PhaseDiscovery          AttackPhase = "discovery"           // 发现
	PhaseLateralMovement    AttackPhase = "lateral_movement"    // 横向移动
	PhaseCollection         AttackPhase = "collection"          // 数据收集
	PhaseCommandControl     AttackPhase = "command_control"     // C2
	PhaseExfiltration       AttackPhase = "exfiltration"        // 数据外泄
	PhaseImpact             AttackPhase = "impact"              // 影响
)

// AlertType → AttackPhase 映射（基于检测规则语义）
var alertTypeToPhase = map[string]AttackPhase{
	"port_scan":          PhaseReconnaissance,
	"host_scan":          PhaseReconnaissance,
	"service_discovery":  PhaseDiscovery,
	"dns_recon":          PhaseReconnaissance,
	"brute_force":        PhaseCredentialAccess,
	"password_spray":     PhaseCredentialAccess,
	"exploit_attempt":    PhaseInitialAccess,
	"exploit_success":    PhaseExecution,
	"malware_download":   PhaseExecution,
	"malware_execution":  PhaseExecution,
	"c2_beacon":          PhaseCommandControl,
	"c2_dns_tunnel":      PhaseCommandControl,
	"c2_http_tunnel":     PhaseCommandControl,
	"privilege_escalation": PhasePrivilegeEscalation,
	"lateral_movement":   PhaseLateralMovement,
	"credential_dump":    PhaseCredentialAccess,
	"data_staging":       PhaseCollection,
	"data_exfiltration":  PhaseExfiltration,
	"dns_exfil":          PhaseExfiltration,
	"http_exfil":         PhaseExfiltration,
	"smb_movement":       PhaseLateralMovement,
	"rdp_movement":       PhaseLateralMovement,
	"ssh_movement":       PhaseLateralMovement,
	"persistence_mechanism": PhasePersistence,
	"defense_evasion":    PhaseDefenseEvasion,
	"ransomware_behavior": PhaseImpact,
	"data_destruction":   PhaseImpact,
}

// CampaignType 攻击活动类型
type CampaignType string

const (
	CampaignScanAndExploit     CampaignType = "scan_and_exploit"
	CampaignBruteForce         CampaignType = "brute_force"
	CampaignC2Communication    CampaignType = "c2_communication"
	CampaignDataExfiltration   CampaignType = "data_exfiltration"
	CampaignLateralMovement    CampaignType = "lateral_movement"
	CampaignRansomware         CampaignType = "ransomware"
	CampaignAPT                CampaignType = "apt"
	CampaignInsiderThreat      CampaignType = "insider_threat"
)

// CampaignPhaseSequence 已知攻击链序列模式
var knownAttackChains = map[CampaignType][]AttackPhase{
	CampaignScanAndExploit: {
		PhaseReconnaissance, PhaseInitialAccess, PhaseExecution,
	},
	CampaignBruteForce: {
		PhaseReconnaissance, PhaseCredentialAccess, PhaseLateralMovement,
	},
	CampaignC2Communication: {
		PhaseExecution, PhaseCommandControl,
	},
	CampaignDataExfiltration: {
		PhaseCollection, PhaseExfiltration,
	},
	CampaignLateralMovement: {
		PhaseCredentialAccess, PhaseLateralMovement, PhaseCollection, PhaseExfiltration,
	},
	CampaignRansomware: {
		PhaseInitialAccess, PhaseExecution, PhaseLateralMovement, PhaseImpact,
	},
	CampaignAPT: {
		PhaseReconnaissance, PhaseInitialAccess, PhaseExecution,
		PhasePersistence, PhaseDefenseEvasion, PhaseCredentialAccess,
		PhaseDiscovery, PhaseLateralMovement, PhaseCollection,
		PhaseCommandControl, PhaseExfiltration,
	},
	CampaignInsiderThreat: {
		PhaseCredentialAccess, PhaseDiscovery, PhaseCollection, PhaseExfiltration,
	},
}

// AlertInfo 用于关联的告警摘要
type AlertInfo struct {
	AlertID     string
	TenantID    string
	AlertType   string
	Severity    string
	SrcIP       string
	DstIP       string
	DstPort     uint32
	CommunityID string
	CampaignID  string // 已归属的 Campaign
	Timestamp   time.Time
	Score       float32
	Labels      []string
}

// CampaignEvent 攻击活动事件
type CampaignEvent struct {
	CampaignID   string
	CampaignType CampaignType
	Title        string
	Description  string
	Severity     string
	Score        float32
	TenantID     string
	AlertIDs     []string
	Phases       []AttackPhase
	PhaseProgress float32 // 0.0-1.0 攻击链完成度
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	AffectedIPs  []string
	CorrelationRules []string // 触发关联的规则
}

// CorrelationConfig 关联配置
type CorrelationConfig struct {
	TimeWindow          time.Duration // 时间窗口
	MaxAlertsPerCampaign int
	MinScoreForCampaign float32
	MinAlertsForCampaign int
	MinPhasesForChain   int // 最少阶段数以确认攻击链
}

// DefaultCorrelationConfig 默认关联配置
func DefaultCorrelationConfig() CorrelationConfig {
	return CorrelationConfig{
		TimeWindow:           24 * time.Hour,
		MaxAlertsPerCampaign: 1000,
		MinScoreForCampaign:  0.6,
		MinAlertsForCampaign: 3,
		MinPhasesForChain:    2,
	}
}

// Correlator 攻击活动关联引擎
type Correlator struct {
	config CorrelationConfig
	logger *zap.Logger
	mu     sync.RWMutex
	// 活跃告警缓冲区（最近 24h）
	alertBuffer []AlertInfo
	bufferSize  int
}

// NewCorrelator 创建关联引擎
func NewCorrelator(config CorrelationConfig, logger *zap.Logger) *Correlator {
	if config.MaxAlertsPerCampaign == 0 {
		config = DefaultCorrelationConfig()
	}
	return &Correlator{
		config:      config,
		logger:      logger,
		alertBuffer: make([]AlertInfo, 0, 10000),
		bufferSize:  10000,
	}
}

// Correlate 将告警关联为攻击活动
// 返回检测到的攻击活动列表
func (c *Correlator) Correlate(ctx context.Context, alerts []AlertInfo) []CampaignEvent {
	if len(alerts) < c.config.MinAlertsForCampaign {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-c.config.TimeWindow)

	// 清理过期告警
	c.pruneBuffer(cutoff)

	// 添加新告警到缓冲区
	for _, alert := range alerts {
		if alert.Timestamp.After(cutoff) {
			c.alertBuffer = append(c.alertBuffer, alert)
		}
	}

	// 执行多维度关联
	campaigns := c.multiDimCorrelate(alerts, now)

	c.logger.Info("Campaign correlation completed",
		zap.Int("input_alerts", len(alerts)),
		zap.Int("buffer_size", len(c.alertBuffer)),
		zap.Int("campaigns", len(campaigns)))

	return campaigns
}

// multiDimCorrelate 多维度关联
func (c *Correlator) multiDimCorrelate(alerts []AlertInfo, now time.Time) []CampaignEvent {
	// 维度1: 时间+源IP 关联（同一攻击者短时间内多目标）
	srcIPGroups := c.groupBySrcIP(alerts)
	// 维度2: 时间+目标IP 关联（同一目标被多方攻击）
	dstIPGroups := c.groupByDstIP(alerts)
	// 维度3: community_id 关联（同一会话触发多种告警）
	communityGroups := c.groupByCommunityID(alerts)
	// 维度4: 攻击链递进关联（多阶段攻击序列）
	phaseGroups := c.groupByAttackPhase(alerts)

	// 合并关联组
	mergedGroups := c.mergeGroups(srcIPGroups, dstIPGroups, communityGroups, phaseGroups)

	// 构建 CampaignEvent
	var campaigns []CampaignEvent
	for _, group := range mergedGroups {
		if len(group) < c.config.MinAlertsForCampaign {
			continue
		}
		campaign := c.buildCampaign(group)
		if campaign != nil {
			campaigns = append(campaigns, *campaign)
		}
	}

	return campaigns
}

// groupBySrcIP 按源IP分组
func (c *Correlator) groupBySrcIP(alerts []AlertInfo) [][]AlertInfo {
	groups := make(map[string][]AlertInfo)
	window := 1 * time.Hour // 1小时窗内同源IP告警

	for _, a := range alerts {
		if a.SrcIP == "" || a.SrcIP == "0.0.0.0" {
			continue
		}
		key := a.SrcIP
		// 检查是否在同一时间窗口内
		added := false
		for _, existing := range groups[key] {
			if absDuration(a.Timestamp.Sub(existing.Timestamp)) <= window {
				groups[key] = append(groups[key], a)
				added = true
				break
			}
		}
		if !added && len(groups[key]) == 0 {
			groups[key] = append(groups[key], a)
		}
	}

	var result [][]AlertInfo
	for _, g := range groups {
		if len(g) >= c.config.MinAlertsForCampaign {
			result = append(result, g)
		}
	}
	return result
}

// groupByDstIP 按目标IP分组
func (c *Correlator) groupByDstIP(alerts []AlertInfo) [][]AlertInfo {
	groups := make(map[string][]AlertInfo)
	window := 2 * time.Hour

	for _, a := range alerts {
		if a.DstIP == "" || a.DstIP == "0.0.0.0" {
			continue
		}
		key := a.DstIP
		added := false
		for _, existing := range groups[key] {
			if absDuration(a.Timestamp.Sub(existing.Timestamp)) <= window {
				groups[key] = append(groups[key], a)
				added = true
				break
			}
		}
		if !added && len(groups[key]) == 0 {
			groups[key] = append(groups[key], a)
		}
	}

	var result [][]AlertInfo
	for _, g := range groups {
		if len(g) >= c.config.MinAlertsForCampaign {
			result = append(result, g)
		}
	}
	return result
}

// groupByCommunityID 按community_id分组
func (c *Correlator) groupByCommunityID(alerts []AlertInfo) [][]AlertInfo {
	groups := make(map[string][]AlertInfo)
	for _, a := range alerts {
		if a.CommunityID == "" {
			continue
		}
		groups[a.CommunityID] = append(groups[a.CommunityID], a)
	}
	var result [][]AlertInfo
	for _, g := range groups {
		if len(g) >= 2 { // 同一会话至少2种告警
			result = append(result, g)
		}
	}
	return result
}

// groupByAttackPhase 按攻击链阶段分组
func (c *Correlator) groupByAttackPhase(alerts []AlertInfo) [][]AlertInfo {
	phaseMap := make(map[AttackPhase][]AlertInfo)
	for _, a := range alerts {
		phase := alertTypeToPhase[a.AlertType]
		if phase == "" {
			continue
		}
		phaseMap[phase] = append(phaseMap[phase], a)
	}

	// 检查已知攻击链
	var chains [][]AlertInfo
	for campaignType, phaseSeq := range knownAttackChains {
		var chainAlerts []AlertInfo
		matchedPhases := 0
		for _, phase := range phaseSeq {
			if alerts, ok := phaseMap[phase]; ok && len(alerts) > 0 {
				chainAlerts = append(chainAlerts, alerts...)
				matchedPhases++
			}
		}
		if matchedPhases >= c.config.MinPhasesForChain {
			c.logger.Info("Attack chain detected",
				zap.String("campaign_type", string(campaignType)),
				zap.Int("matched_phases", matchedPhases),
				zap.Int("total_alerts", len(chainAlerts)))
			chains = append(chains, chainAlerts)
		}
	}
	return chains
}

// mergeGroups 合并重叠的分组
func (c *Correlator) mergeGroups(groups ...[][]AlertInfo) [][]AlertInfo {
	// 使用 alertID 集合进行去重合并
	type group struct {
		alerts    []AlertInfo
		alertIDs  map[string]bool
	}

	var allGroups []group
	for _, gs := range groups {
		for _, g := range gs {
			ids := make(map[string]bool)
			for _, a := range g {
				ids[a.AlertID] = true
			}
			allGroups = append(allGroups, group{alerts: g, alertIDs: ids})
		}
	}

	// 合并有重叠 alertID 的组
	var merged [][]AlertInfo
	used := make(map[int]bool)
	for i := range allGroups {
		if used[i] {
			continue
		}
		mergedIDs := make(map[string]bool)
		for id := range allGroups[i].alertIDs {
			mergedIDs[id] = true
		}
		mergedAlerts := make([]AlertInfo, len(allGroups[i].alerts))
		copy(mergedAlerts, allGroups[i].alerts)
		used[i] = true

		// 迭代合并重叠组
		changed := true
		for changed {
			changed = false
			for j := range allGroups {
				if used[j] {
					continue
				}
				overlap := false
				for id := range allGroups[j].alertIDs {
					if mergedIDs[id] {
						overlap = true
						break
					}
				}
				if overlap {
					for id := range allGroups[j].alertIDs {
						mergedIDs[id] = true
					}
					mergedAlerts = append(mergedAlerts, allGroups[j].alerts...)
					used[j] = true
					changed = true
				}
			}
		}
		merged = append(merged, mergedAlerts)
	}
	return merged
}

// buildCampaign 从告警组构建 CampaignEvent
func (c *Correlator) buildCampaign(alerts []AlertInfo) *CampaignEvent {
	if len(alerts) == 0 {
		return nil
	}

	// 去重 + 排序
	seen := make(map[string]bool)
	var unique []AlertInfo
	for _, a := range alerts {
		if !seen[a.AlertID] {
			seen[a.AlertID] = true
			unique = append(unique, a)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Timestamp.Before(unique[j].Timestamp)
	})

	// 提取攻击阶段
	phases := c.extractPhases(unique)
	campaignType := c.identifyCampaignType(phases)

	// 计算综合评分
	score := c.calculateCampaignScore(unique, campaignType)

	// 生成 Campaign ID
	campaignID := c.generateCampaignID(unique)

	// 收集受影响的 IP
	affectedIPs := c.collectAffectedIPs(unique)

	// 计算攻击链完成度
	phaseProgress := c.calculatePhaseProgress(phases, campaignType)

	// 生成描述和标题
	title, desc := c.generateTitleAndDescription(campaignType, phases, unique)

	// 确定严重程度
	severity := c.determineCampaignSeverity(phaseProgress, score)

	return &CampaignEvent{
		CampaignID:    campaignID,
		CampaignType:  campaignType,
		Title:         title,
		Description:   desc,
		Severity:      severity,
		Score:         score,
		TenantID:      unique[0].TenantID,
		AlertIDs:      c.extractAlertIDs(unique),
		Phases:        phases,
		PhaseProgress: phaseProgress,
		StartTime:     unique[0].Timestamp,
		EndTime:       unique[len(unique)-1].Timestamp,
		Duration:      unique[len(unique)-1].Timestamp.Sub(unique[0].Timestamp),
		AffectedIPs:   affectedIPs,
	}
}

// extractPhases 提取攻击阶段
func (c *Correlator) extractPhases(alerts []AlertInfo) []AttackPhase {
	phaseSet := make(map[AttackPhase]bool)
	var phases []AttackPhase
	for _, a := range alerts {
		phase := alertTypeToPhase[a.AlertType]
		if phase != "" && !phaseSet[phase] {
			phaseSet[phase] = true
			phases = append(phases, phase)
		}
	}
	// 按攻击链顺序排序
	sort.Slice(phases, func(i, j int) bool {
		return c.phaseOrder(phases[i]) < c.phaseOrder(phases[j])
	})
	return phases
}

// phaseOrder 攻击阶段顺序权重
func (c *Correlator) phaseOrder(phase AttackPhase) int {
	order := map[AttackPhase]int{
		PhaseReconnaissance:      1,
		PhaseInitialAccess:       2,
		PhaseExecution:           3,
		PhasePersistence:         4,
		PhasePrivilegeEscalation: 5,
		PhaseDefenseEvasion:      6,
		PhaseCredentialAccess:    7,
		PhaseDiscovery:           8,
		PhaseLateralMovement:     9,
		PhaseCollection:          10,
		PhaseCommandControl:      11,
		PhaseExfiltration:        12,
		PhaseImpact:              13,
	}
	return order[phase]
}

// identifyCampaignType 识别攻击活动类型
func (c *Correlator) identifyCampaignType(phases []AttackPhase) CampaignType {
	phaseSet := make(map[AttackPhase]bool)
	for _, p := range phases {
		phaseSet[p] = true
	}

	// 按优先级匹配 (从高到低，匹配合并多个阶段的复杂攻击链优先)
	if phaseSet[PhaseImpact] {
		return CampaignRansomware
	}
	if (phaseSet[PhaseExfiltration] || phaseSet[PhaseCollection]) && (phaseSet[PhaseLateralMovement] || phaseSet[PhaseCredentialAccess]) {
		return CampaignAPT
	}
	if phaseSet[PhaseExfiltration] || phaseSet[PhaseCollection] {
		return CampaignDataExfiltration
	}
	if phaseSet[PhaseLateralMovement] {
		if phaseSet[PhaseCredentialAccess] {
			return CampaignLateralMovement
		}
		return CampaignLateralMovement
	}
	if phaseSet[PhaseCommandControl] {
		return CampaignC2Communication
	}
	if phaseSet[PhaseCredentialAccess] {
		return CampaignBruteForce
	}
	if phaseSet[PhaseReconnaissance] && phaseSet[PhaseInitialAccess] {
		return CampaignScanAndExploit
	}
	if phaseSet[PhaseReconnaissance] {
		return CampaignScanAndExploit
	}
	return CampaignScanAndExploit
}

// calculateCampaignScore 计算攻击活动综合评分
func (c *Correlator) calculateCampaignScore(alerts []AlertInfo, campaignType CampaignType) float32 {
	if len(alerts) == 0 {
		return 0
	}

	// 基础分 = 告警平均分的加权
	var totalScore float32
	for _, a := range alerts {
		totalScore += a.Score
	}
	avgScore := totalScore / float32(len(alerts))

	// 多样性加分（阶段越多，威胁越大）
	phases := c.extractPhases(alerts)
	phaseBonus := float32(len(phases)) * 0.05

	// 持续时间加分（长时间活动更可疑）
	duration := alerts[len(alerts)-1].Timestamp.Sub(alerts[0].Timestamp)
	durationBonus := float32(math.Min(float64(duration.Hours())/24.0, 1.0)) * 0.1

	// 数量加分
	countBonus := float32(math.Min(float64(len(alerts))/100.0, 1.0)) * 0.1

	score := avgScore + phaseBonus + durationBonus + countBonus
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// generateCampaignID 生成 Campaign ID
func (c *Correlator) generateCampaignID(alerts []AlertInfo) string {
	raw := fmt.Sprintf("%s:%d:%d",
		alerts[0].TenantID,
		alerts[0].Timestamp.Unix(),
		len(alerts))
	hash := md5.Sum([]byte(raw))
	return fmt.Sprintf("campaign-%s-%x", alerts[0].TenantID, hash[:8])
}

// collectAffectedIPs 收集受影响的 IP
func (c *Correlator) collectAffectedIPs(alerts []AlertInfo) []string {
	ipSet := make(map[string]bool)
	for _, a := range alerts {
		if a.SrcIP != "" && a.SrcIP != "0.0.0.0" {
			ipSet[a.SrcIP] = true
		}
		if a.DstIP != "" && a.DstIP != "0.0.0.0" {
			ipSet[a.DstIP] = true
		}
	}
	var ips []string
	for ip := range ipSet {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

// calculatePhaseProgress 计算攻击链完成度
func (c *Correlator) calculatePhaseProgress(phases []AttackPhase, campaignType CampaignType) float32 {
	expectedPhases := knownAttackChains[campaignType]
	if len(expectedPhases) == 0 {
		return 0
	}

	phaseSet := make(map[AttackPhase]bool)
	for _, p := range phases {
		phaseSet[p] = true
	}

	matched := 0
	for _, expected := range expectedPhases {
		if phaseSet[expected] {
			matched++
		}
	}
	return float32(matched) / float32(len(expectedPhases))
}

// generateTitleAndDescription 生成标题和描述
func (c *Correlator) generateTitleAndDescription(campaignType CampaignType, phases []AttackPhase, alerts []AlertInfo) (string, string) {
	phaseNames := make([]string, len(phases))
	for i, p := range phases {
		phaseNames[i] = string(p)
	}

	var title, desc string
	switch campaignType {
	case CampaignScanAndExploit:
		title = "扫描与漏洞利用攻击活动"
		desc = fmt.Sprintf("检测到针对 %d 个目标IP的扫描和漏洞利用活动，涉及阶段：%v",
			len(c.collectAffectedIPs(alerts)), phaseNames)
	case CampaignBruteForce:
		title = "暴力破解攻击活动"
		desc = fmt.Sprintf("检测到来自 %s 的暴力破解尝试，共 %d 次告警",
			alerts[0].SrcIP, len(alerts))
	case CampaignC2Communication:
		title = "C2 通信攻击活动"
		desc = fmt.Sprintf("检测到与 C2 服务器的持续通信，涉及阶段：%v", phaseNames)
	case CampaignDataExfiltration:
		title = "数据外泄攻击活动"
		desc = fmt.Sprintf("检测到数据外泄行为，涉及 %d 个目标，阶段：%v",
			len(c.collectAffectedIPs(alerts)), phaseNames)
	case CampaignLateralMovement:
		title = "横向移动攻击活动"
		desc = fmt.Sprintf("检测到内网横向移动行为，涉及 %d 个IP，阶段：%v",
			len(c.collectAffectedIPs(alerts)), phaseNames)
	case CampaignRansomware:
		title = "勒索软件攻击活动"
		desc = fmt.Sprintf("检测到疑似勒索软件攻击链，涉及阶段：%v", phaseNames)
	case CampaignAPT:
		title = "APT 高级持续威胁活动"
		desc = fmt.Sprintf("检测到多阶段 APT 攻击链，完成度 %.0f%%，涉及 %d 个IP，阶段：%v",
			c.calculatePhaseProgress(phases, campaignType)*100,
			len(c.collectAffectedIPs(alerts)), phaseNames)
	case CampaignInsiderThreat:
		title = "内部威胁活动"
		desc = fmt.Sprintf("检测到疑似内部威胁行为，涉及阶段：%v", phaseNames)
	default:
		title = "未知攻击活动"
		desc = fmt.Sprintf("检测到异常活动模式，共 %d 条告警", len(alerts))
	}

	return title, desc
}

// determineCampaignSeverity 确定攻击活动严重程度
func (c *Correlator) determineCampaignSeverity(phaseProgress, score float32) string {
	if phaseProgress >= 0.8 || score >= 0.9 {
		return "critical"
	}
	if phaseProgress >= 0.5 || score >= 0.7 {
		return "high"
	}
	if phaseProgress >= 0.3 || score >= 0.4 {
		return "medium"
	}
	return "low"
}

// extractAlertIDs 提取告警ID列表
func (c *Correlator) extractAlertIDs(alerts []AlertInfo) []string {
	ids := make([]string, len(alerts))
	for i, a := range alerts {
		ids[i] = a.AlertID
	}
	return ids
}

// pruneBuffer 清理过期告警
func (c *Correlator) pruneBuffer(cutoff time.Time) {
	var valid []AlertInfo
	for _, a := range c.alertBuffer {
		if a.Timestamp.After(cutoff) {
			valid = append(valid, a)
		}
	}
	c.alertBuffer = valid
}

// absDuration 返回 duration 的绝对值
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
