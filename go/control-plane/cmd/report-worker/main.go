// Command report-worker 人读报告生成 worker(§10.3/76.44.3):
// analysis.report.requests.v1 → 读取冻结机器摘要(经 analysis-service API,
// 零 PG 凭证)→ 确定性 HTML 渲染 → MinIO 上传 → ApplyWorkerReceipt(VERIFYING)
// → VerifyAndConfirm(AVAILABLE)。失败不回退 Run;幂等经 report_id 与对象 sha。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/segmentio/kafka-go"

	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type reportRequest struct {
	ReportID         string `json:"report_id"`
	RunID            string `json:"run_id"`
	TenantID         string `json:"tenant_id"`
	SummarySHA256    string `json:"summary_sha256"`
	TemplateRevision string `json:"template_revision"`
	Locale           string `json:"locale"`
}

type summaryPayload struct {
	Data struct {
		RunID               string          `json:"RunID"`
		RunState            string          `json:"RunState"`
		FindingConclusion   string          `json:"FindingConclusion"`
		RiskSeverity        string          `json:"RiskSeverity"`
		Completeness        string          `json:"Completeness"`
		IntegrityState      string          `json:"IntegrityState"`
		ExecutionSpecSHA256 string          `json:"ExecutionSpecSHA256"`
		WindowStartMs       int64           `json:"WindowStartMs"`
		WindowEndMs         int64           `json:"WindowEndMs"`
		KeyFindings         json.RawMessage `json:"KeyFindings"`
		Limitations         json.RawMessage `json:"Limitations"`
		EvidenceEntries     json.RawMessage `json:"EvidenceEntries"`
		SummarySHA256       string          `json:"SummarySHA256"`
	} `json:"data"`
}

var reportTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>分析报告 {{.Data.RunID}}</title></head>
<body>
<h1>统一分析任务调度中心 · 人读报告</h1>
<h2>执行摘要</h2>
<table border="1">
<tr><th>运行</th><td>{{.Data.RunID}}</td></tr>
<tr><th>运行状态</th><td>{{.Data.RunState}}</td></tr>
<tr><th>机器结论</th><td>{{.Data.FindingConclusion}}</td></tr>
<tr><th>风险级别</th><td>{{.Data.RiskSeverity}}</td></tr>
<tr><th>完整性</th><td>{{.Data.Completeness}}</td></tr>
<tr><th>完整性校验</th><td>{{.Data.IntegrityState}}</td></tr>
<tr><th>执行规格哈希</th><td>{{.Data.ExecutionSpecSHA256}}</td></tr>
<tr><th>窗口</th><td>{{.Data.WindowStartMs}} ~ {{.Data.WindowEndMs}}</td></tr>
</table>
<h2>关键发现</h2><pre>{{.KeyFindingsPretty}}</pre>
<h2>限制</h2><pre>{{.LimitationsPretty}}</pre>
<h2>证据条目</h2><pre>{{.EvidencePretty}}</pre>
<h2>附录:模型/特征/规则版本</h2><p>见运行技术详情与计划冻结修订(execution_spec_sha256 绑定)。</p>
<p><small>本报告由冻结机器摘要确定性渲染(摘要 sha256:{{.Data.SummarySHA256}}),生成时间 {{.GeneratedAt}}。</small></p>
</body></html>`))

func main() {
	log := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, "[report-worker] "+format+"\n", args...)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	brokers := strings.Split(strings.TrimSpace(os.Getenv("KAFKA_BROKERS")), ",")
	if len(brokers) == 0 || brokers[0] == "" {
		log("KAFKA_BROKERS required")
		os.Exit(2)
	}
	sec := kafkaCommon.SecurityConfig{
		SecurityProtocol: os.Getenv("KAFKA_SECURITY_PROTOCOL"),
		SASLMechanism:    os.Getenv("KAFKA_SASL_MECHANISM"),
		SASLUsername:     os.Getenv("KAFKA_SASL_USERNAME"),
		SASLPassword:     os.Getenv("KAFKA_SASL_PASSWORD"),
		TLSCAFile:        os.Getenv("KAFKA_TLS_CA_FILE"),
		TLSServerName:    os.Getenv("KAFKA_TLS_SERVER_NAME"),
	}
	dialer, err := sec.Dialer("report-worker")
	if err != nil {
		log("kafka dialer: %v", err)
		os.Exit(2)
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers, Topic: commoncontracts.TopicAnalysisReportRequests,
		GroupID: "report-worker-v1", Dialer: dialer, MinBytes: 1, MaxBytes: 1e6,
		// 无历史 commit 时从最早消息开始(遗留请求经确定性隔离/幂等处理后前进)。
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	apiBase := strings.TrimSuffix(os.Getenv("ANALYSIS_SERVICE_URL"), "/")
	minioClient, err := minio.New(os.Getenv("MINIO_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""),
		Secure: false,
	})
	if err != nil {
		log("minio client: %v", err)
		os.Exit(2)
	}
	bucket := os.Getenv("REPORT_BUCKET")
	if bucket == "" {
		bucket = "analysis-reports"
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log("read: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		var req reportRequest
		if err := json.Unmarshal(msg.Value, &req); err != nil || req.ReportID == "" || req.RunID == "" {
			log("malformed report request (quarantined): %s", string(msg.Value))
			_ = reader.CommitMessages(ctx, msg)
			continue
		}
		if req.TenantID == "" {
			// 遗留请求(无 tenant)确定性隔离:无法回源,不重试。
			log("legacy report request without tenant (quarantined): %s", req.ReportID)
			_ = reader.CommitMessages(ctx, msg)
			continue
		}
		if err := process(ctx, httpClient, minioClient, bucket, apiBase, &req); err != nil {
			log("process report %s: %v", req.ReportID, err)
			// 临时失败重试(对象上传幂等:同 report_id 同 sha)。
			time.Sleep(5 * time.Second)
			continue
		}
		_ = reader.CommitMessages(ctx, msg)
		log("report %s AVAILABLE (run=%s)", req.ReportID, req.RunID)
	}
}

func process(ctx context.Context, httpClient *http.Client, minioClient *minio.Client, bucket, apiBase string, req *reportRequest) error {
	// 1. 读取冻结机器摘要(服务端权威;worker 零 PG 凭证)
	sumURL := fmt.Sprintf("%s/api/v1/analysis/runs/%s/summary", apiBase, req.RunID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, sumURL, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("X-Tenant-ID", req.TenantID)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("fetch summary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch summary status %d", resp.StatusCode)
	}
	var sum summaryPayload
	if err := json.NewDecoder(resp.Body).Decode(&sum); err != nil {
		return fmt.Errorf("decode summary: %w", err)
	}
	if sum.Data.SummarySHA256 == "" || (req.SummarySHA256 != "" && sum.Data.SummarySHA256 != req.SummarySHA256) {
		return fmt.Errorf("summary sha mismatch: request=%s actual=%s", req.SummarySHA256, sum.Data.SummarySHA256)
	}

	// 2. 确定性渲染
	var buf bytes.Buffer
	pretty := func(raw json.RawMessage) string {
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			return string(raw)
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		return string(b)
	}
	if err := reportTemplate.Execute(&buf, map[string]interface{}{
		"Data":              sum.Data,
		"KeyFindingsPretty": pretty(sum.Data.KeyFindings),
		"LimitationsPretty": pretty(sum.Data.Limitations),
		"EvidencePretty":    pretty(sum.Data.EvidenceEntries),
		"GeneratedAt":       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	objectKey := fmt.Sprintf("reports/%s/%s/%s.html", req.TenantID, req.RunID, req.ReportID)
	digest := sha256.Sum256(buf.Bytes())
	objectSHA := hex.EncodeToString(digest[:])

	// 3. 上传(幂等:同 key 同内容覆盖无副作用)
	if _, err := minioClient.PutObject(ctx, bucket, objectKey, bytes.NewReader(buf.Bytes()), int64(buf.Len()), minio.PutObjectOptions{ContentType: "text/html; charset=utf-8"}); err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	// 4. worker 回执(VERIFYING)→ 对象权威复核(AVAILABLE)
	post := func(path string, body map[string]interface{}) error {
		raw, _ := json.Marshal(body)
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+path, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Tenant-ID", req.TenantID)
		resp, err := httpClient.Do(r)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			var body bytes.Buffer
			_, _ = body.ReadFrom(resp.Body)
			return fmt.Errorf("status %d: %s", resp.StatusCode, body.String())
		}
		return nil
	}
	if err := post(fmt.Sprintf("/api/v1/analysis/reports/%s/worker-receipt", req.ReportID), map[string]interface{}{
		"object_key": objectKey, "object_sha256": objectSHA, "object_size": buf.Len(),
		"source_summary_sha256": sum.Data.SummarySHA256,
	}); err != nil {
		return fmt.Errorf("worker receipt: %w", err)
	}
	if err := post(fmt.Sprintf("/api/v1/analysis/reports/%s/verify", req.ReportID), map[string]interface{}{
		"object_sha256": objectSHA, "object_size": buf.Len(),
	}); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	return nil
}
