package api

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
)

const requiredPostgresColumnsQuery = `
	SELECT table_name, column_name
	FROM information_schema.columns
	WHERE table_schema = current_schema()
	  AND table_name = ANY($1)`

var campaignWorkbenchRequiredColumns = map[string][]string{
	"campaign_workbench_state": {
		"tenant_id", "campaign_id", "assignee", "status", "state_version",
		"updated_by", "updated_at",
	},
	"campaign_reports": {
		"report_id", "tenant_id", "campaign_id", "format", "status", "sections",
		"evidence_count", "created_by", "created_at", "completed_at",
	},
}

var campaignAggregateV2RequiredColumns = map[string][]string{
	"campaign_action_jobs": {
		"job_id", "tenant_id", "campaign_id", "action_id", "status", "result",
		"idempotency_key", "request_sha256", "expected_revision", "resource_revision", "reason",
	},
	"campaign_workbench_state": {
		"tenant_id", "campaign_id", "assignee", "status", "state_version",
		"member_count", "last_event_id", "updated_by", "updated_at",
	},
	"campaign_aggregate_history": {
		"event_id", "tenant_id", "campaign_id", "aggregate_revision", "event_type",
		"status", "assignee", "member_count", "payload", "reason", "created_by", "created_at",
	},
	"campaign_aggregate_outbox": {
		"event_id", "tenant_id", "aggregate_id", "aggregate_revision", "event_type",
		"schema_version", "partition_key", "payload", "published", "attempts",
		"last_error", "next_attempt_at", "locked_until", "locked_by", "created_at", "published_at",
	},
	"campaign_reports": {
		"report_id", "tenant_id", "campaign_id", "format", "status", "sections",
		"campaign_revision", "snapshot_id", "snapshot", "snapshot_sha256",
		"object_manifest", "error_message", "idempotency_key", "created_by", "created_at",
		"job_id", "object_bucket", "object_key", "mime_type", "artifact_sha256", "size_bytes",
		"attempts", "next_attempt_at", "locked_until", "locked_by", "updated_at", "completed_at",
	},
	"campaign_soar_jobs": {
		"job_id", "tenant_id", "campaign_id", "playbook_id", "target", "source_snapshot_id",
		"campaign_revision", "status", "approval_status", "executor_status", "revision", "request",
		"execution_receipt", "compensation_receipt", "error_message", "attempts", "next_attempt_at",
		"locked_until", "locked_by", "requested_by", "approved_by", "approved_at", "created_at",
		"updated_at", "completed_at",
	},
	"campaign_soar_approvals": {
		"approval_id", "job_id", "tenant_id", "campaign_id", "decision", "expected_revision",
		"idempotency_key", "reason", "decided_by", "resulting_revision", "resulting_status",
		"approval_status", "created_at",
	},
	"campaign_soar_execution_receipts": {
		"receipt_id", "job_id", "tenant_id", "campaign_id", "phase", "attempt", "provider",
		"provider_receipt_id", "status", "external_effect", "payload", "payload_sha256", "created_at",
	},
	"campaign_soar_control_requests": {
		"request_id", "job_id", "tenant_id", "campaign_id", "operation", "expected_revision",
		"idempotency_key", "reason", "requested_by", "resulting_revision", "resulting_status", "created_at",
	},
	"campaign_alert_links": {
		"tenant_id", "campaign_id", "alert_id", "status", "revision", "campaign_revision",
	},
	"campaign_alert_link_history": {
		"event_id", "relation_id", "tenant_id", "campaign_id", "alert_id",
		"event_type", "revision", "campaign_revision", "payload", "created_by", "created_at",
	},
	"campaign_membership_commands": {
		"command_id", "tenant_id", "relation_id", "campaign_id", "alert_id", "operation",
		"idempotency_key", "request_sha256", "expected_relation_revision",
		"expected_campaign_revision", "relation_revision", "campaign_revision",
		"result", "created_by", "created_at",
	},
	"campaign_merge_receipts": {
		"merge_id", "job_id", "tenant_id", "source_campaign_id", "target_campaign_id",
		"idempotency_key", "request_sha256", "source_expected_revision", "target_expected_revision",
		"source_revision", "target_revision", "source_member_count", "target_member_count_before",
		"target_member_count_after", "moved_count", "relinked_count", "deduplicated_count",
		"manifest", "manifest_sha256", "reason", "trace_id", "created_by", "created_at",
	},
	"campaign_merge_items": {
		"merge_id", "tenant_id", "source_relation_id", "target_relation_id", "alert_id", "outcome",
		"source_relation_revision", "target_relation_revision", "source_event_id", "target_event_id", "created_at",
	},
	"campaign_membership_backfill_runs": {
		"run_id", "tenant_id", "source_kind", "source_uri", "source_sha256", "source_snapshot_id",
		"source_as_of", "manifest", "manifest_sha256", "reason", "status", "campaign_count",
		"source_member_count", "completed_campaign_count", "failed_campaign_count", "inserted_count",
		"bound_count", "unchanged_count", "skipped_unlinked_count", "created_by", "created_at",
		"updated_at", "completed_at",
	},
	"campaign_membership_backfill_campaigns": {
		"run_id", "tenant_id", "campaign_id", "manifest_sha256", "expected_campaign_revision",
		"starting_campaign_revision", "resulting_campaign_revision", "source_member_count",
		"resulting_member_count", "inserted_count", "bound_count", "unchanged_count",
		"skipped_unlinked_count", "status", "error_code", "error_message", "aggregate_event_id",
		"started_at", "completed_at",
	},
	"campaign_membership_backfill_items": {
		"run_id", "tenant_id", "campaign_id", "alert_id", "source_ordinal", "outcome",
		"relation_id", "relation_revision", "campaign_revision", "event_id", "created_at",
	},
}

var topicGovernanceRequiredColumns = map[string][]string{
	"topic_saved_views": {
		"view_id", "tenant_id", "topic", "name", "filters", "visibility",
		"favorite", "shared", "share_token", "created_by", "created_at", "updated_at",
	},
	"topic_scope_overrides": {
		"tenant_id", "topic", "scope_name", "included_assets", "excluded_assets",
		"risk_levels", "time_window", "detail", "updated_by", "updated_at",
	},
	"topic_subscriptions": {
		"subscription_id", "tenant_id", "topic", "channel", "threshold", "schedule",
		"recipients", "enabled", "created_by", "created_at", "updated_at", "detail",
	},
	"topic_exports": {
		"export_id", "tenant_id", "topic", "export_type", "status", "parameters",
		"result", "generated_by", "generated_at",
	},
	"topic_actions": {
		"action_id", "tenant_id", "topic", "action", "target", "status", "detail",
		"requested_by", "created_at", "updated_at", "idempotency_key", "snapshot_id",
		"expected_revision", "revision", "reason", "trace_id", "executor", "receipt",
		"error", "attempts", "lease_until",
	},
}

func verifyRequiredPostgresColumns(
	ctx context.Context,
	db *sql.DB,
	required map[string][]string,
) error {
	if db == nil {
		return fmt.Errorf("postgres is not configured")
	}

	tables := make([]string, 0, len(required))
	for table := range required {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	rows, err := db.QueryContext(ctx, requiredPostgresColumnsQuery, pq.Array(tables))
	if err != nil {
		return fmt.Errorf("query postgres schema capabilities: %w", err)
	}
	defer rows.Close()

	available := make(map[string]map[string]struct{}, len(required))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return fmt.Errorf("scan postgres schema capabilities: %w", err)
		}
		if available[table] == nil {
			available[table] = map[string]struct{}{}
		}
		available[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read postgres schema capabilities: %w", err)
	}

	var missing []string
	for _, table := range tables {
		for _, column := range required[table] {
			if _, ok := available[table][column]; !ok {
				missing = append(missing, table+"."+column)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("required postgres schema capabilities are missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func verifyCampaignWorkbenchSchema(ctx context.Context, db *sql.DB) error {
	return verifyRequiredPostgresColumns(ctx, db, campaignWorkbenchRequiredColumns)
}

func verifyCampaignAggregateV2Schema(ctx context.Context, db *sql.DB) error {
	return verifyRequiredPostgresColumns(ctx, db, campaignAggregateV2RequiredColumns)
}

func verifyTopicGovernanceSchema(ctx context.Context, db *sql.DB) error {
	return verifyRequiredPostgresColumns(ctx, db, topicGovernanceRequiredColumns)
}
