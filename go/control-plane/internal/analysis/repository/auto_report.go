// Package repository AUTO_ASYNC 报告协调器候选(§8 报告策略自动异步模式):
// 终态且冻结摘要存在、尚无报告行、且定义最高修订报告策略 mode=AUTO_ASYNC 的 run
// 为自动报告候选。协调器只发起请求,生成由 G05 worker 独立推进。
package repository

import (
	"context"
	"fmt"
)

// AutoReportCandidate 自动报告候选(run + 冻结策略参数)。
type AutoReportCandidate struct {
	TenantID         string
	RunID            string
	SummarySHA256    string
	TemplateRevision string
	Locale           string
}

// NextAutoReportCandidates 读取最多 limit 个自动报告候选(只读;幂等由
// RequestHumanReportAtomic 台账裁决,重复扫描无害)。
func (r *Repo) NextAutoReportCandidates(ctx context.Context, limit int) ([]AutoReportCandidate, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT rn.tenant_id, rn.id, s.canonical_sha256,
			COALESCE(p.template_revision,'default-v1'), COALESCE(p.locale,'zh-CN')
		FROM analysis_runs rn
		JOIN analysis_machine_summaries s ON s.run_id = rn.id
		JOIN analysis_tasks t ON t.id = rn.task_id
		JOIN analysis_human_report_policies p ON p.task_definition_id = t.task_definition_id
			AND p.revision = (SELECT MAX(p2.revision) FROM analysis_human_report_policies p2
				WHERE p2.task_definition_id = t.task_definition_id)
		WHERE rn.state IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED')
		  AND p.mode='AUTO_ASYNC'
		  AND NOT EXISTS (SELECT 1 FROM analysis_human_reports hr WHERE hr.run_id = rn.id)
		ORDER BY rn.created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list auto report candidates: %w", err)
	}
	defer rows.Close()
	var out []AutoReportCandidate
	for rows.Next() {
		var c AutoReportCandidate
		if err := rows.Scan(&c.TenantID, &c.RunID, &c.SummarySHA256, &c.TemplateRevision, &c.Locale); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
