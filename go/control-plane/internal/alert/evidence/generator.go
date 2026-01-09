// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/evidence/generator.go
// 修复版：修复 ClickHouse 数组字段扫描、并发执行证据生成、完善错误处理、支持批量操作
// 主要修复：hexFreq 和 hexRatio 数组字段直接扫描，无需 JSON 解析
// //////////////////////////////////////////////////////////////////////////////
package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EvidenceType 证据类型
type EvidenceType string

const (
	EvidenceTypeStat        EvidenceType = "stat"        // 统计特征
	EvidenceTypeSequence    EvidenceType = "sequence"    // 序列特征
	EvidenceTypeFingerprint EvidenceType = "fingerprint" // 指纹特征
	EvidenceTypeGraph       EvidenceType = "graph"       // 关系图
	EvidenceTypePcap        EvidenceType = "pcap"        // PCAP片段
)

// Evidence 证据结构
type Evidence struct {
	TenantID         string                 `json:"tenant_id"`
	EvidenceID       string                 `json:"evidence_id"`
	AlertID          string                 `json:"alert_id"`
	Timestamp        time.Time              `json:"timestamp"`
	Type             EvidenceType           `json:"type"`
	Summary          string                 `json:"summary"`
	Metrics          map[string]interface{} `json:"metrics,omitempty"`
	SnippetRef       map[string]string      `json:"snippet_ref,omitempty"`
	ArkimeLink       string                 `json:"arkime_link,omitempty"`
	Confidence       float32                `json:"confidence"`
	EventID          string                 `json:"event_id"`
	VisualizationURL string                 `json:"visualization_url,omitempty"`
}

// GeneratorConfig 证据生成器配置
type GeneratorConfig struct {
	VisualBaseURL     string        // 可视化前端地址
	QueryTimeout      time.Duration // 查询超时时间
	ConcurrencyLimit  int           // 并发限制
	EnableStatFeature bool          // 是否启用统计特征
	EnableSeqFeature  bool          // 是否启用序列特征
	EnableFPFeature   bool          // 是否启用指纹特征
	EnableArkime      bool          // 是否启用 Arkime
}

// DefaultGeneratorConfig 默认配置
func DefaultGeneratorConfig() *GeneratorConfig {
	return &GeneratorConfig{
		VisualBaseURL:     "http://localhost:3000",
		QueryTimeout:      5 * time.Second,
		ConcurrencyLimit:  4,
		EnableStatFeature: true,
		EnableSeqFeature:  true,
		EnableFPFeature:   true,
		EnableArkime:      true,
	}
}

// Generator 证据生成器
type Generator struct {
	chClient      *storage.ClickHouseClient
	arkimeLinkGen *arkime.LinkGenerator
	logger        *zap.Logger
	config        *GeneratorConfig
}

// NewGenerator 创建证据生成器
func NewGenerator(
	chClient *storage.ClickHouseClient,
	arkimeGen *arkime.LinkGenerator,
	visualBaseURL string,
	logger *zap.Logger,
) *Generator {
	config := DefaultGeneratorConfig()
	if visualBaseURL != "" {
		config.VisualBaseURL = visualBaseURL
	}
	return &Generator{
		chClient:      chClient,
		arkimeLinkGen: arkimeGen,
		logger:        logger,
		config:        config,
	}
}

// NewGeneratorWithConfig 创建带配置的证据生成器
func NewGeneratorWithConfig(
	chClient *storage.ClickHouseClient,
	arkimeGen *arkime.LinkGenerator,
	config *GeneratorConfig,
	logger *zap.Logger,
) *Generator {
	if config == nil {
		config = DefaultGeneratorConfig()
	}
	return &Generator{
		chClient:      chClient,
		arkimeLinkGen: arkimeGen,
		logger:        logger,
		config:        config,
	}
}

// evidenceResult 证据生成结果
type evidenceResult struct {
	evidence *Evidence
	err      error
	typ      EvidenceType
}

// GenerateForAlert 为告警生成证据（并发执行）
func (g *Generator) GenerateForAlert(ctx context.Context, alert *persistence.Alert) ([]*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.generate_for_alert")
	defer span.End()

	if alert == nil {
		return nil, fmt.Errorf("alert is nil")
	}

	// 创建带超时的 context
	queryCtx, cancel := context.WithTimeout(ctx, g.config.QueryTimeout)
	defer cancel()

	// 使用 channel 收集结果
	resultChan := make(chan evidenceResult, 4)
	var wg sync.WaitGroup

	// 1. 并发生成统计特征证据
	if g.config.EnableStatFeature && alert.CommunityID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evidence, err := g.generateStatEvidence(queryCtx, alert)
			resultChan <- evidenceResult{evidence: evidence, err: err, typ: EvidenceTypeStat}
		}()
	}

	// 2. 并发生成序列特征证据
	if g.config.EnableSeqFeature && alert.CommunityID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evidence, err := g.generateSequenceEvidence(queryCtx, alert)
			resultChan <- evidenceResult{evidence: evidence, err: err, typ: EvidenceTypeSequence}
		}()
	}

	// 3. 并发生成指纹证据
	if g.config.EnableFPFeature && alert.CommunityID != "" && alert.SessionID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evidence, err := g.generateFingerprintEvidence(queryCtx, alert)
			resultChan <- evidenceResult{evidence: evidence, err: err, typ: EvidenceTypeFingerprint}
		}()
	}

	// 4. 生成 Arkime 链接证据（不需要查询数据库，同步执行）
	var arkimeEvidence *Evidence
	if g.config.EnableArkime && g.arkimeLinkGen != nil {
		arkimeEvidence = g.generateArkimeEvidence(alert)
	}

	// 等待所有并发任务完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	evidences := make([]*Evidence, 0, 4)
	var errors []error

	for result := range resultChan {
		if result.err != nil {
			g.logger.Warn("Failed to generate evidence",
				zap.String("alert_id", alert.AlertID),
				zap.String("type", string(result.typ)),
				zap.Error(result.err))
			errors = append(errors, result.err)
		} else if result.evidence != nil {
			evidences = append(evidences, result.evidence)
		}
	}

	// 添加 Arkime 证据
	if arkimeEvidence != nil {
		evidences = append(evidences, arkimeEvidence)
	}

	g.logger.Info("Generated evidence for alert",
		zap.String("alert_id", alert.AlertID),
		zap.Int("evidence_count", len(evidences)),
		zap.Int("error_count", len(errors)))

	return evidences, nil
}

// GenerateForAlertsBatch 批量为多个告警生成证据
func (g *Generator) GenerateForAlertsBatch(ctx context.Context, alerts []*persistence.Alert) (map[string][]*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.generate_for_alerts_batch")
	defer span.End()

	if len(alerts) == 0 {
		return nil, nil
	}

	result := make(map[string][]*Evidence)
	var mu sync.Mutex

	// 使用 semaphore 限制并发
	sem := make(chan struct{}, g.config.ConcurrencyLimit)
	var wg sync.WaitGroup

	for _, alert := range alerts {
		wg.Add(1)
		go func(a *persistence.Alert) {
			defer wg.Done()

			// 获取 semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			evidences, err := g.GenerateForAlert(ctx, a)
			if err != nil {
				g.logger.Warn("Failed to generate evidence for alert",
					zap.String("alert_id", a.AlertID),
					zap.Error(err))
				return
			}

			mu.Lock()
			result[a.AlertID] = evidences
			mu.Unlock()
		}(alert)
	}

	wg.Wait()

	return result, nil
}

// generateStatEvidence 生成统计特征证据
func (g *Generator) generateStatEvidence(ctx context.Context, alert *persistence.Alert) (*Evidence, error) {
	if alert.CommunityID == "" {
		return nil, nil
	}

	// 查询feature_stat表
	query := `
		SELECT 
			pps, bps, up_down_ratio,
			pktlen_mean, pktlen_std,
			iat_mean_ms, iat_std_ms,
			active_mean_ms, idle_mean_ms,
			tcp_flag_syn_cnt, tcp_flag_ack_cnt
		FROM traffic.feature_stat
		WHERE tenant_id = ?
		  AND community_id = ?
		ORDER BY ts DESC
		LIMIT 1
	`

	row := g.chClient.QueryRow(ctx, query, alert.TenantID, alert.CommunityID)

	var pps, bps, upDownRatio float32
	var pktlenMean, pktlenStd, iatMean, iatStd float32
	var activeMean, idleMean float32
	var synCnt, ackCnt uint16

	if err := row.Scan(
		&pps, &bps, &upDownRatio,
		&pktlenMean, &pktlenStd,
		&iatMean, &iatStd,
		&activeMean, &idleMean,
		&synCnt, &ackCnt,
	); err != nil {
		// 如果没有找到特征数据，返回nil而不是错误
		return nil, nil
	}

	metrics := map[string]interface{}{
		"pps":           pps,
		"bps":           bps,
		"up_down_ratio": upDownRatio,
		"pktlen_mean":   pktlenMean,
		"pktlen_std":    pktlenStd,
		"iat_mean_ms":   iatMean,
		"iat_std_ms":    iatStd,
		"active_ms":     activeMean,
		"idle_ms":       idleMean,
		"syn_count":     synCnt,
		"ack_count":     ackCnt,
	}

	// 生成摘要
	summary := g.generateStatSummary(pps, bps, upDownRatio)

	return &Evidence{
		TenantID:   alert.TenantID,
		EvidenceID: uuid.New().String(),
		AlertID:    alert.AlertID,
		Timestamp:  time.Now(),
		Type:       EvidenceTypeStat,
		Summary:    summary,
		Metrics:    metrics,
		Confidence: 0.8,
		EventID:    alert.EventID,
		VisualizationURL: fmt.Sprintf("%s/evidence/stat/%s/%s",
			g.config.VisualBaseURL, alert.TenantID, alert.CommunityID),
	}, nil
}

// generateStatSummary 生成统计特征摘要
func (g *Generator) generateStatSummary(pps, bps, upDownRatio float32) string {
	var summary string

	// 根据流量特征生成描述
	if pps > 10000 {
		summary = fmt.Sprintf("高速率流量 - PPS: %.0f, BPS: %.2f Mbps", pps, bps/1000000)
	} else if pps > 1000 {
		summary = fmt.Sprintf("中等速率流量 - PPS: %.0f, BPS: %.2f Kbps", pps, bps/1000)
	} else {
		summary = fmt.Sprintf("低速率流量 - PPS: %.0f, BPS: %.2f bps", pps, bps)
	}

	// 添加上下行比例信息
	if upDownRatio > 10 {
		summary += " (大量上行流量，可能为数据外泄)"
	} else if upDownRatio < 0.1 {
		summary += " (大量下行流量，可能为下载行为)"
	}

	return summary
}

// generateSequenceEvidence 生成序列特征证据
func (g *Generator) generateSequenceEvidence(ctx context.Context, alert *persistence.Alert) (*Evidence, error) {
	if alert.CommunityID == "" {
		return nil, nil
	}

	// 查询feature_seq表
	query := `
		SELECT 
			pktlen_seq_hash, iat_seq_hash,
			wavelet_releng_fwd, wavelet_releng_bwd,
			wavelet_entropy_fwd, wavelet_entropy_bwd,
			wavelet_detail_mean_fwd, wavelet_detail_mean_bwd,
			wavelet_detail_std_fwd, wavelet_detail_std_bwd
		FROM traffic.feature_seq
		WHERE tenant_id = ?
		  AND community_id = ?
		ORDER BY ts_end DESC
		LIMIT 1
	`

	row := g.chClient.QueryRow(ctx, query, alert.TenantID, alert.CommunityID)

	var pktlenHash, iatHash string
	var waveletFwd, waveletBwd, entropyFwd, entropyBwd float32
	var detailMeanFwd, detailMeanBwd, detailStdFwd, detailStdBwd float32

	if err := row.Scan(
		&pktlenHash, &iatHash,
		&waveletFwd, &waveletBwd,
		&entropyFwd, &entropyBwd,
		&detailMeanFwd, &detailMeanBwd,
		&detailStdFwd, &detailStdBwd,
	); err != nil {
		// 如果没有找到特征数据，返回nil
		return nil, nil
	}

	metrics := map[string]interface{}{
		"pktlen_seq_hash":         pktlenHash,
		"iat_seq_hash":            iatHash,
		"wavelet_energy_fwd":      waveletFwd,
		"wavelet_energy_bwd":      waveletBwd,
		"wavelet_entropy_fwd":     entropyFwd,
		"wavelet_entropy_bwd":     entropyBwd,
		"wavelet_detail_mean_fwd": detailMeanFwd,
		"wavelet_detail_mean_bwd": detailMeanBwd,
		"wavelet_detail_std_fwd":  detailStdFwd,
		"wavelet_detail_std_bwd":  detailStdBwd,
	}

	// 生成摘要
	summary := g.generateSequenceSummary(pktlenHash, entropyFwd, entropyBwd)

	return &Evidence{
		TenantID:   alert.TenantID,
		EvidenceID: uuid.New().String(),
		AlertID:    alert.AlertID,
		Timestamp:  time.Now(),
		Type:       EvidenceTypeSequence,
		Summary:    summary,
		Metrics:    metrics,
		Confidence: 0.75,
		EventID:    alert.EventID,
		VisualizationURL: fmt.Sprintf("%s/evidence/sequence/%s/%s",
			g.config.VisualBaseURL, alert.TenantID, alert.CommunityID),
	}, nil
}

// generateSequenceSummary 生成序列特征摘要
func (g *Generator) generateSequenceSummary(pktlenHash string, entropyFwd, entropyBwd float32) string {
	hashPrefix := pktlenHash
	if len(hashPrefix) > 8 {
		hashPrefix = hashPrefix[:8]
	}

	summary := fmt.Sprintf("序列特征 - 包长哈希: %s, 熵值: fwd=%.2f/bwd=%.2f",
		hashPrefix, entropyFwd, entropyBwd)

	// 根据熵值判断加密特征
	if entropyFwd > 7.5 && entropyBwd > 7.5 {
		summary += " (高熵值，可能为加密流量)"
	} else if entropyFwd > 6.0 || entropyBwd > 6.0 {
		summary += " (中等熵值，可能为压缩或部分加密)"
	}

	return summary
}

// generateFingerprintEvidence 生成指纹证据（修复版：正确处理数组字段）
func (g *Generator) generateFingerprintEvidence(ctx context.Context, alert *persistence.Alert) (*Evidence, error) {
	if alert.CommunityID == "" || alert.SessionID == "" {
		return nil, nil
	}

	// 修复：一次查询获取所有字段（包括数组）
	query := `
		SELECT 
			is_encrypted, tls_version, ja3,
			sni_hash, cert_sha256, cert_is_self_signed, pubkey_len,
			hex_freq, hex_ratio,
			entropy_payload, chi_square_bfd
		FROM traffic.feature_fp
		WHERE tenant_id = ?
		  AND community_id = ?
		  AND session_id = ?
		ORDER BY ts DESC
		LIMIT 1
	`

	row := g.chClient.QueryRow(ctx, query, alert.TenantID, alert.CommunityID, alert.SessionID)

	var isEncrypted uint8
	var tlsVersion, ja3, sniHash, certSha256 string
	var certIsSelfSigned uint8
	var pubkeyLen uint16
	var entropyPayload, chiSquareBfd float32

	// 修复：直接扫描数组字段到 Go 切片
	var hexFreq, hexRatio []float32

	if err := row.Scan(
		&isEncrypted, &tlsVersion, &ja3,
		&sniHash, &certSha256, &certIsSelfSigned, &pubkeyLen,
		&hexFreq, &hexRatio, // ClickHouse Go Driver 自动处理数组
		&entropyPayload, &chiSquareBfd,
	); err != nil {
		// 如果没有找到特征数据，返回nil
		g.logger.Debug("No fingerprint features found",
			zap.String("community_id", alert.CommunityID),
			zap.String("session_id", alert.SessionID),
			zap.Error(err))
		return nil, nil
	}

	// 构建 metrics
	metrics := map[string]interface{}{
		"is_encrypted":        isEncrypted == 1,
		"tls_version":         tlsVersion,
		"ja3":                 ja3,
		"sni_hash":            sniHash,
		"cert_sha256":         certSha256,
		"cert_is_self_signed": certIsSelfSigned == 1,
		"pubkey_len":          pubkeyLen,
		"entropy_payload":     entropyPayload,
		"chi_square_bfd":      chiSquareBfd,
	}

	// 添加数组字段（如果有数据）
	if len(hexFreq) > 0 {
		metrics["hex_freq"] = hexFreq
	}
	if len(hexRatio) > 0 {
		metrics["hex_ratio"] = hexRatio
	}

	// 生成摘要
	summary := g.generateFingerprintSummary(isEncrypted == 1, tlsVersion, ja3, certIsSelfSigned == 1)

	return &Evidence{
		TenantID:   alert.TenantID,
		EvidenceID: uuid.New().String(),
		AlertID:    alert.AlertID,
		Timestamp:  time.Now(),
		Type:       EvidenceTypeFingerprint,
		Summary:    summary,
		Metrics:    metrics,
		Confidence: 0.85,
		EventID:    alert.EventID,
		VisualizationURL: fmt.Sprintf("%s/evidence/fingerprint/%s/%s",
			g.config.VisualBaseURL, alert.TenantID, alert.SessionID),
	}, nil
}

// generateFingerprintSummary 生成指纹特征摘要
func (g *Generator) generateFingerprintSummary(isEncrypted bool, tlsVersion, ja3 string, isSelfSigned bool) string {
	var summary string

	if isEncrypted {
		summary = fmt.Sprintf("加密流量 - TLS: %s", tlsVersion)
		if ja3 != "" {
			ja3Short := ja3
			if len(ja3Short) > 16 {
				ja3Short = ja3Short[:16] + "..."
			}
			summary += fmt.Sprintf(", JA3: %s", ja3Short)
		}
		if isSelfSigned {
			summary += " (自签名证书，可能为恶意)"
		}
	} else {
		summary = "非加密流量"
	}

	return summary
}

// generateArkimeEvidence 生成Arkime链接证据
func (g *Generator) generateArkimeEvidence(alert *persistence.Alert) *Evidence {
	if g.arkimeLinkGen == nil {
		return nil
	}

	// 生成Arkime链接
	var arkimeLink string
	if alert.CommunityID != "" {
		arkimeLink = g.arkimeLinkGen.GenerateSessionLink(
			alert.CommunityID,
			alert.FirstSeen,
			alert.LastSeen,
		)
	} else if alert.SrcIP != "" && alert.DstIP != "" {
		arkimeLink = g.arkimeLinkGen.GenerateTupleLink(
			alert.SrcIP,
			alert.DstIP,
			alert.SrcPort,
			alert.DstPort,
			alert.Protocol,
			alert.FirstSeen,
			alert.LastSeen,
		)
	}

	if arkimeLink == "" {
		return nil
	}

	// 生成所有相关链接
	links := g.arkimeLinkGen.GenerateAlertLinks(
		alert.CommunityID,
		alert.SrcIP,
		alert.DstIP,
		alert.SrcPort,
		alert.DstPort,
		alert.Protocol,
		alert.FirstSeen,
		alert.LastSeen,
	)

	snippetRef := map[string]string{
		"session_link": links.SessionLink,
	}
	if links.SrcIPLink != "" {
		snippetRef["src_ip_link"] = links.SrcIPLink
	}
	if links.DstIPLink != "" {
		snippetRef["dst_ip_link"] = links.DstIPLink
	}
	if links.ConnectionsLink != "" {
		snippetRef["connections_link"] = links.ConnectionsLink
	}
	if links.SPIViewLink != "" {
		snippetRef["spi_view_link"] = links.SPIViewLink
	}

	summary := fmt.Sprintf("PCAP可用 - 会话 %s 到 %s",
		alert.FirstSeen.Format("2006-01-02 15:04:05"),
		alert.LastSeen.Format("2006-01-02 15:04:05"))

	return &Evidence{
		TenantID:   alert.TenantID,
		EvidenceID: uuid.New().String(),
		AlertID:    alert.AlertID,
		Timestamp:  time.Now(),
		Type:       EvidenceTypePcap,
		Summary:    summary,
		SnippetRef: snippetRef,
		ArkimeLink: arkimeLink,
		Confidence: 1.0,
		EventID:    alert.EventID,
	}
}

// SaveEvidence 保存证据到ClickHouse
func (g *Generator) SaveEvidence(ctx context.Context, evidence *Evidence) error {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.save_evidence")
	defer span.End()

	if evidence == nil {
		return fmt.Errorf("evidence is nil")
	}

	metricsJSON, err := json.Marshal(evidence.Metrics)
	if err != nil {
		metricsJSON = []byte("{}")
		g.logger.Warn("Failed to marshal metrics", zap.Error(err))
	}

	snippetRefJSON, err := json.Marshal(evidence.SnippetRef)
	if err != nil {
		snippetRefJSON = []byte("{}")
		g.logger.Warn("Failed to marshal snippet_ref", zap.Error(err))
	}

	query := `
		INSERT INTO traffic.evidence_local (
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	err = g.chClient.Exec(ctx, query,
		evidence.TenantID,
		evidence.EvidenceID,
		evidence.AlertID,
		evidence.Timestamp,
		string(evidence.Type),
		evidence.Summary,
		string(metricsJSON),
		string(snippetRefJSON),
		evidence.ArkimeLink,
		evidence.Confidence,
		evidence.EventID,
	)

	if err != nil {
		return fmt.Errorf("failed to save evidence: %w", err)
	}

	g.logger.Debug("Evidence saved",
		zap.String("evidence_id", evidence.EvidenceID),
		zap.String("alert_id", evidence.AlertID))

	return nil
}

// SaveEvidenceBatch 批量保存证据
func (g *Generator) SaveEvidenceBatch(ctx context.Context, evidences []*Evidence) error {
	if len(evidences) == 0 {
		return nil
	}

	ctx, span := otel.StartSpan(ctx, "evidence_generator.save_evidence_batch")
	defer span.End()

	successCount := 0
	errorCount := 0
	var lastError error

	for _, evidence := range evidences {
		if err := g.SaveEvidence(ctx, evidence); err != nil {
			g.logger.Error("Failed to save evidence",
				zap.String("evidence_id", evidence.EvidenceID),
				zap.Error(err))
			errorCount++
			lastError = err
			// 继续处理其他证据
		} else {
			successCount++
		}
	}

	g.logger.Info("Batch save evidence completed",
		zap.Int("total", len(evidences)),
		zap.Int("success", successCount),
		zap.Int("errors", errorCount))

	if errorCount > 0 {
		return fmt.Errorf("failed to save %d/%d evidences, last error: %w", errorCount, len(evidences), lastError)
	}

	return nil
}

// SaveEvidenceBatchConcurrent 并发批量保存证据
func (g *Generator) SaveEvidenceBatchConcurrent(ctx context.Context, evidences []*Evidence) *BatchSaveResult {
	if len(evidences) == 0 {
		return &BatchSaveResult{TotalCount: 0}
	}

	ctx, span := otel.StartSpan(ctx, "evidence_generator.save_evidence_batch_concurrent")
	defer span.End()

	result := &BatchSaveResult{
		TotalCount: len(evidences),
		FailedIDs:  make([]string, 0),
		Errors:     make(map[string]string),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, g.config.ConcurrencyLimit)

	for _, evidence := range evidences {
		wg.Add(1)
		go func(ev *Evidence) {
			defer wg.Done()

			// 获取 semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := g.SaveEvidence(ctx, ev); err != nil {
				mu.Lock()
				result.FailedCount++
				result.FailedIDs = append(result.FailedIDs, ev.EvidenceID)
				result.Errors[ev.EvidenceID] = err.Error()
				mu.Unlock()
			} else {
				mu.Lock()
				result.SuccessCount++
				mu.Unlock()
			}
		}(evidence)
	}

	wg.Wait()

	return result
}

// BatchSaveResult 批量保存结果
type BatchSaveResult struct {
	TotalCount   int               `json:"total_count"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	FailedIDs    []string          `json:"failed_ids,omitempty"`
	Errors       map[string]string `json:"errors,omitempty"`
}

// GetEvidenceByAlertID 根据告警ID获取证据列表
func (g *Generator) GetEvidenceByAlertID(ctx context.Context, tenantID, alertID string) ([]*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.get_evidence_by_alert_id")
	defer span.End()

	query := `
		SELECT 
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		FROM traffic.evidence
		WHERE tenant_id = ? AND alert_id = ?
		ORDER BY ts DESC
	`

	rows, err := g.chClient.Query(ctx, query, tenantID, alertID)
	if err != nil {
		return nil, fmt.Errorf("failed to query evidence: %w", err)
	}
	defer rows.Close()

	var evidences []*Evidence
	for rows.Next() {
		var e Evidence
		var metricsJSON, snippetRefJSON string
		var evidenceType string

		if err := rows.Scan(
			&e.TenantID, &e.EvidenceID, &e.AlertID, &e.Timestamp,
			&evidenceType, &e.Summary, &metricsJSON, &snippetRefJSON, &e.ArkimeLink,
			&e.Confidence, &e.EventID,
		); err != nil {
			g.logger.Error("Failed to scan evidence row", zap.Error(err))
			continue
		}

		e.Type = EvidenceType(evidenceType)

		// 解析JSON字段
		if metricsJSON != "" && metricsJSON != "{}" {
			if err := json.Unmarshal([]byte(metricsJSON), &e.Metrics); err != nil {
				g.logger.Warn("Failed to unmarshal metrics", zap.Error(err))
			}
		}

		if snippetRefJSON != "" && snippetRefJSON != "{}" {
			if err := json.Unmarshal([]byte(snippetRefJSON), &e.SnippetRef); err != nil {
				g.logger.Warn("Failed to unmarshal snippet_ref", zap.Error(err))
			}
		}

		evidences = append(evidences, &e)
	}

	return evidences, nil
}

// GetEvidenceByID 根据ID获取单个证据
func (g *Generator) GetEvidenceByID(ctx context.Context, tenantID, evidenceID string) (*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.get_evidence_by_id")
	defer span.End()

	query := `
		SELECT 
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		FROM traffic.evidence
		WHERE tenant_id = ? AND evidence_id = ?
		LIMIT 1
	`

	row := g.chClient.QueryRow(ctx, query, tenantID, evidenceID)

	var e Evidence
	var metricsJSON, snippetRefJSON string
	var evidenceType string

	if err := row.Scan(
		&e.TenantID, &e.EvidenceID, &e.AlertID, &e.Timestamp,
		&evidenceType, &e.Summary, &metricsJSON, &snippetRefJSON, &e.ArkimeLink,
		&e.Confidence, &e.EventID,
	); err != nil {
		return nil, fmt.Errorf("failed to get evidence: %w", err)
	}

	e.Type = EvidenceType(evidenceType)

	// 解析JSON字段
	if metricsJSON != "" && metricsJSON != "{}" {
		json.Unmarshal([]byte(metricsJSON), &e.Metrics)
	}

	if snippetRefJSON != "" && snippetRefJSON != "{}" {
		json.Unmarshal([]byte(snippetRefJSON), &e.SnippetRef)
	}

	return &e, nil
}

// DeleteEvidenceByAlertID 删除告警相关的所有证据
func (g *Generator) DeleteEvidenceByAlertID(ctx context.Context, tenantID, alertID string) error {
	ctx, span := otel.StartSpan(ctx, "evidence_generator.delete_evidence_by_alert_id")
	defer span.End()

	// 注意：ClickHouse 删除是异步的，使用 ALTER TABLE DELETE
	query := `
		ALTER TABLE traffic.evidence_local DELETE 
		WHERE tenant_id = ? AND alert_id = ?
	`

	err := g.chClient.Exec(ctx, query, tenantID, alertID)
	if err != nil {
		return fmt.Errorf("failed to delete evidence: %w", err)
	}

	g.logger.Info("Evidence deleted",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))

	return nil
}

// SetConfig 更新配置
func (g *Generator) SetConfig(config *GeneratorConfig) {
	if config != nil {
		g.config = config
	}
}

// GetConfig 获取当前配置
func (g *Generator) GetConfig() *GeneratorConfig {
	return g.config
}
