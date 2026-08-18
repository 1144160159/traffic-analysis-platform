package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// ReportSummaryRef 报告输入引用(冻结机器摘要)。
type ReportSummaryRef struct {
	SummarySHA256 string
	SummaryExists bool
}

// GetRunSummaryHash 读取 Run 终态后的机器摘要哈希(报告请求前置)。
func (r *Repo) GetRunSummaryHash(ctx context.Context, tenantID, runID string) (*ReportSummaryRef, error) {
	var state, sha string
	err := r.db.QueryRowContext(ctx, `
		SELECT ru.state, COALESCE(s.canonical_sha256,'')
		FROM analysis_runs ru
		LEFT JOIN analysis_machine_summaries s ON s.run_id = ru.id
		WHERE ru.id=$1 AND ru.tenant_id=$2`, runID, tenantID).Scan(&state, &sha)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	return &ReportSummaryRef{SummarySHA256: sha, SummaryExists: sha != ""}, nil
}

// IsTerminalRunState 终态判定(报告只消费终态)。
func IsTerminalRunState(state string) bool {
	switch state {
	case "SUCCEEDED", "PARTIALLY_SUCCEEDED", "FAILED", "CANCELLED":
		return true
	}
	return false
}

// RequestHumanReportAtomic 人读报告请求事务(HR01-HR10):
// 验 Run 终态+摘要 hash→身份(tenant+run+summary+template+locale)→幂等重放→
// INSERT HumanReadableReport(QUEUED)+history/audit/outbox→COMMIT。失败不得改变 Run。
func (r *Repo) RequestHumanReportAtomic(ctx context.Context, tenantID, runID, summarySHA256, templateRevision, locale, requestHash, idempotencyKey string) (reportID string, replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin report request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, idempotencyKey); err != nil {
		return "", false, fmt.Errorf("report lock: %w", err)
	}
	var existingHash string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, idempotencyKey).Scan(&existingHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, idempotencyKey, requestHash); err != nil {
			return "", false, fmt.Errorf("report ledger: %w", err)
		}
	case err != nil:
		return "", false, fmt.Errorf("report ledger query: %w", err)
	case existingHash == requestHash:
		_ = tx.Commit()
		return "", true, nil
	default:
		return "", false, ErrPayloadMismatch
	}

	// Run 终态 + 摘要存在性(报告只消费冻结摘要)
	var runState, canonicalSHA string
	if err := tx.QueryRowContext(ctx, `
		SELECT ru.state, COALESCE(s.canonical_sha256,'')
		FROM analysis_runs ru
		LEFT JOIN analysis_machine_summaries s ON s.run_id = ru.id
		WHERE ru.id=$1 AND ru.tenant_id=$2 FOR UPDATE OF ru`,
		runID, tenantID).Scan(&runState, &canonicalSHA); err != nil {
		return "", false, fmt.Errorf("run lookup: %w", err)
	}
	if !IsTerminalRunState(runState) {
		return "", false, fmt.Errorf("%s: report requires terminal run", contract.ErrCodeInvalidTransition)
	}
	if canonicalSHA == "" || canonicalSHA != summarySHA256 {
		return "", false, fmt.Errorf("summary hash mismatch (report must consume frozen summary)")
	}

	reportID = uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_human_reports(id, tenant_id, run_id, summary_sha256, template_revision, locale, state)
		VALUES($1,$2,$3,$4,$5,$6,'QUEUED')
		ON CONFLICT (run_id, summary_sha256, template_revision, locale) DO NOTHING`,
		reportID, tenantID, runID, summarySHA256, templateRevision, locale); err != nil {
		return "", false, fmt.Errorf("insert report: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES($1,$2,$3,jsonb_build_object(
			'report_id',$1::text,'run_id',$4::text,'tenant_id',$5::text,
			'summary_sha256',$6::text,'template_revision',$7::text,'locale',$8::text))`,
		reportID, contract.TopicReportRequests, reportID, runID, tenantID, summarySHA256, templateRevision, locale); err != nil {
		return "", false, fmt.Errorf("report outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'human_report',$2,'REQUESTED',$1,$3)`,
		tenantID, reportID, []byte(`{}`)); err != nil {
		return "", false, fmt.Errorf("report history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit report request: %w", err)
	}
	return reportID, false, nil
}

// ApplyHumanReportReceiptAtomic 报告 worker 回执(对象 ACK):
// inbox 去重→身份/hash/size 验证→PG metadata+ReportState 推进;事务内不访问 MinIO;不改 RunState。
func (r *Repo) ApplyHumanReportReceiptAtomic(ctx context.Context, tenantID, reportID, objectKey, objectSHA256 string, objectSize int64, generatorVersion, sourceSummarySHA256 string) (state string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin report receipt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// inbox 去重(report_id 即事件身份)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_inbox(event_id, tuple_hash, outcome) VALUES($1,$2,'RECEIVED') ON CONFLICT (event_id) DO NOTHING`,
		reportID, objectSHA256); err != nil {
		return "", fmt.Errorf("report inbox: %w", err)
	}

	var summarySHA string
	if err := tx.QueryRowContext(ctx, `
		SELECT summary_sha256 FROM analysis_human_reports WHERE id=$1 AND tenant_id=$2 FOR UPDATE`,
		reportID, tenantID).Scan(&summarySHA); err != nil {
		return "", fmt.Errorf("report lookup: %w", err)
	}
	if summarySHA != sourceSummarySHA256 {
		return "", fmt.Errorf("report summary hash mismatch")
	}
	if objectKey == "" || objectSHA256 == "" || objectSize <= 0 {
		return "", fmt.Errorf("object identity incomplete (key/sha256/size required)")
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_human_reports SET state='VERIFYING', object_key=$3, object_sha256=$4, object_size=$5, updated_at=now()
		WHERE id=$1 AND tenant_id=$2 AND state IN ('QUEUED','GENERATING')`,
		reportID, tenantID, objectKey, objectSHA256, objectSize)
	if err != nil {
		return "", fmt.Errorf("cas report state: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", fmt.Errorf("report state transition conflict")
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit report receipt: %w", err)
	}
	return "VERIFYING", nil
}

// ConfirmHumanReportObjectAtomic 对象权威复核(verifier)后推进 AVAILABLE。
func (r *Repo) ConfirmHumanReportObjectAtomic(ctx context.Context, tenantID, reportID, objectSHA256 string, objectSize int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_human_reports SET state='AVAILABLE', updated_at=now()
		WHERE id=$1 AND tenant_id=$2 AND state='VERIFYING' AND object_sha256=$3 AND object_size=$4`,
		reportID, tenantID, objectSHA256, objectSize)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("report verification conflict")
	}
	return nil
}

// GetReport 读报告状态(下载前置)。
type ReportView struct {
	ReportID     string
	RunID        string
	State        string
	ObjectKey    string
	ObjectSHA256 string
	ObjectSize   int64
}

func (r *Repo) GetReport(ctx context.Context, tenantID, reportID string) (*ReportView, error) {
	var v ReportView
	var key, sha sql.NullString
	var size sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, run_id, state, object_key, object_sha256, object_size
		FROM analysis_human_reports WHERE id=$1 AND tenant_id=$2`,
		reportID, tenantID).Scan(&v.ReportID, &v.RunID, &v.State, &key, &sha, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("report not found")
	}
	if err != nil {
		return nil, err
	}
	v.ObjectKey = key.String
	v.ObjectSHA256 = sha.String
	v.ObjectSize = size.Int64
	return &v, nil
}

// ReportListView 人读报告列表视图(UI)。
type ReportListView struct {
	ReportID         string
	RunID            string
	SummarySHA256    string
	TemplateRevision string
	Locale           string
	State            string
	ObjectKey        string
	ObjectSHA256     string
	ObjectSize       int64
	CreatedAt        time.Time
}

// ListReports 人读报告列表(tenant 范围)。
func (r *Repo) ListReports(ctx context.Context, tenantID string) ([]ReportListView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, summary_sha256, template_revision, locale, state,
			COALESCE(object_key,''), COALESCE(object_sha256,''), COALESCE(object_size,0), created_at
		FROM analysis_human_reports WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 200`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportListView
	for rows.Next() {
		var v ReportListView
		if err := rows.Scan(&v.ReportID, &v.RunID, &v.SummarySHA256, &v.TemplateRevision, &v.Locale,
			&v.State, &v.ObjectKey, &v.ObjectSHA256, &v.ObjectSize, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RunSummaryContent 机器摘要内容(报告渲染输入)。
type RunSummaryContent struct {
	RunID               string
	RunState            string
	FindingConclusion   string
	RiskSeverity        string
	Completeness        string
	IntegrityState      string
	ExecutionSpecSHA256 string
	WindowStartMs       int64
	WindowEndMs         int64
	KeyFindings         json.RawMessage
	Limitations         json.RawMessage
	EvidenceEntries     json.RawMessage
	SummarySHA256       string
}

// GetRunSummaryContent 读取冻结机器摘要(报告只消费冻结摘要)。
func (r *Repo) GetRunSummaryContent(ctx context.Context, tenantID, runID string) (*RunSummaryContent, error) {
	var v RunSummaryContent
	var ws, we sql.NullInt64
	var kf, lim, ev string
	err := r.db.QueryRowContext(ctx, `
		SELECT ru.id, ru.state, ru.finding_conclusion, ru.risk_severity, ru.completeness, ru.integrity_state,
			ru.execution_spec_sha256, COALESCE((EXTRACT(EPOCH FROM ru.window_start)*1000)::bigint,0), COALESCE((EXTRACT(EPOCH FROM ru.window_end)*1000)::bigint,0),
			COALESCE(s.key_findings::text,'{}'), COALESCE(s.limitations::text,'{}'),
			COALESCE(jsonb_build_array(s.evidence_manifest_hash)::text,'[]'),
			COALESCE(s.canonical_sha256,'')
		FROM analysis_runs ru
		LEFT JOIN analysis_machine_summaries s ON s.run_id = ru.id
		WHERE ru.id=$1 AND ru.tenant_id=$2`, runID, tenantID).Scan(
		&v.RunID, &v.RunState, &v.FindingConclusion, &v.RiskSeverity, &v.Completeness, &v.IntegrityState,
		&v.ExecutionSpecSHA256, &ws, &we, &kf, &lim, &ev, &v.SummarySHA256)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run summary not found")
	}
	if err != nil {
		return nil, err
	}
	v.WindowStartMs, v.WindowEndMs = ws.Int64, we.Int64
	v.KeyFindings = json.RawMessage(kf)
	v.Limitations = json.RawMessage(lim)
	v.EvidenceEntries = json.RawMessage(ev)
	return &v, nil
}
