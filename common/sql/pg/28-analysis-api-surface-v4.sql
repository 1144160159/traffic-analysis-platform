-- M10 统一分析任务调度中心 v4:API 面补齐(§20 页面到 API 唯一映射)——加法式迁移。
-- 变更:
--   analysis_report_download_tickets:人读报告下载票(短期有效+使用审计);
--   analysis_human_report_policies:独立报告策略修订(不进执行 plan hash)。
BEGIN;

CREATE TABLE IF NOT EXISTS analysis_report_download_tickets (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    report_id UUID NOT NULL REFERENCES analysis_human_reports(id),
    ticket_sha256 TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_report_tickets_report ON analysis_report_download_tickets(report_id);

CREATE TABLE IF NOT EXISTS analysis_human_report_policies (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    task_definition_id UUID NOT NULL REFERENCES analysis_task_definitions(id),
    revision BIGINT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'ON_DEMAND',   -- DISABLED|ON_DEMAND|AUTO_ASYNC
    template_revision TEXT NOT NULL DEFAULT 'default-v1',
    locale TEXT NOT NULL DEFAULT 'zh-CN',
    retention_days BIGINT NOT NULL DEFAULT 30,
    policy_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_analysis_human_report_policies UNIQUE (tenant_id, task_definition_id, revision)
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608170400', 'M10 analysis api surface: download tickets + report policies')
ON CONFLICT (version) DO NOTHING;

COMMIT;
