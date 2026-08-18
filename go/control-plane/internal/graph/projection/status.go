package projection

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type PartitionWatermark struct {
	SourcePartition  int       `json:"source_partition"`
	SourceOffset     int64     `json:"source_offset"`
	EventID          string    `json:"event_id"`
	ProjectionSHA256 string    `json:"projection_sha256"`
	SourceTimestamp  int64     `json:"source_timestamp_ms"`
	ProjectedAt      time.Time `json:"projected_at"`
}

type Status struct {
	State             string               `json:"state"`
	ConsumerEnabled   bool                 `json:"consumer_enabled"`
	WorkerEnabled     bool                 `json:"worker_enabled"`
	Topic             string               `json:"topic"`
	ConsumerGroup     string               `json:"consumer_group"`
	PendingCount      int64                `json:"pending_count"`
	AppliedCount      int64                `json:"applied_count"`
	DeadCount         int64                `json:"dead_count"`
	EntityCount       int64                `json:"entity_count"`
	RelationCount     int64                `json:"relation_count"`
	RevokedCount      int64                `json:"revoked_count"`
	OldestPendingAt   *time.Time           `json:"oldest_pending_at,omitempty"`
	LastAppliedAt     *time.Time           `json:"last_applied_at,omitempty"`
	StaleAfterSeconds int64                `json:"stale_after_seconds"`
	Watermarks        []PartitionWatermark `json:"watermarks"`
	Complete          bool                 `json:"complete"`
}

type StatusRepository struct {
	db              *sql.DB
	consumerEnabled bool
	workerEnabled   bool
	staleAfter      time.Duration
	clock           func() time.Time
}

func NewStatusRepository(
	db *sql.DB,
	consumerEnabled bool,
	workerEnabled bool,
	staleAfter time.Duration,
) (*StatusRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("graph projection status database is required")
	}
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	return &StatusRepository{
		db: db, consumerEnabled: consumerEnabled, workerEnabled: workerEnabled,
		staleAfter: staleAfter, clock: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (repository *StatusRepository) Load(ctx context.Context, tenantID string) (Status, error) {
	if tenantID == "" {
		return Status{}, fmt.Errorf("tenant ID is required for graph projection status")
	}
	status := Status{
		State: "empty", ConsumerEnabled: repository.consumerEnabled,
		WorkerEnabled: repository.workerEnabled, Topic: Topic, ConsumerGroup: ConsumerGroup,
		StaleAfterSeconds: int64(repository.staleAfter / time.Second),
		Watermarks:        make([]PartitionWatermark, 0),
	}
	var oldestPending, lastApplied sql.NullTime
	if err := repository.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE projection_state='PENDING'),
		       count(*) FILTER (WHERE projection_state='APPLIED'),
		       count(*) FILTER (WHERE projection_state='DEAD'),
		       min(received_at) FILTER (WHERE projection_state='PENDING'),
		       max(applied_at) FILTER (WHERE projection_state='APPLIED')
		FROM graph_projection_inbox_v1 WHERE tenant_id=$1`, tenantID,
	).Scan(&status.PendingCount, &status.AppliedCount, &status.DeadCount, &oldestPending, &lastApplied); err != nil {
		return Status{}, fmt.Errorf("load graph projection inbox status: %w", err)
	}
	if oldestPending.Valid {
		value := oldestPending.Time.UTC()
		status.OldestPendingAt = &value
	}
	if lastApplied.Valid {
		value := lastApplied.Time.UTC()
		status.LastAppliedAt = &value
	}
	if err := repository.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE projection_kind='entity'),
		       count(*) FILTER (WHERE projection_kind='relation'),
		       count(*) FILTER (WHERE revoked)
		FROM graph_projection_current_v1 WHERE tenant_id=$1`, tenantID,
	).Scan(&status.EntityCount, &status.RelationCount, &status.RevokedCount); err != nil {
		return Status{}, fmt.Errorf("load graph projection current status: %w", err)
	}
	rows, err := repository.db.QueryContext(ctx, `
		SELECT source_partition,source_offset,event_id,projection_sha256,
		       source_timestamp_ms,projected_at
		FROM graph_projection_watermarks_v1
		WHERE tenant_id=$1 AND source_topic=$2
		ORDER BY source_partition`, tenantID, Topic)
	if err != nil {
		return Status{}, fmt.Errorf("load graph projection watermarks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var watermark PartitionWatermark
		if err := rows.Scan(
			&watermark.SourcePartition, &watermark.SourceOffset, &watermark.EventID,
			&watermark.ProjectionSHA256, &watermark.SourceTimestamp, &watermark.ProjectedAt,
		); err != nil {
			return Status{}, fmt.Errorf("scan graph projection watermark: %w", err)
		}
		watermark.ProjectedAt = watermark.ProjectedAt.UTC()
		status.Watermarks = append(status.Watermarks, watermark)
	}
	if err := rows.Err(); err != nil {
		return Status{}, fmt.Errorf("iterate graph projection watermarks: %w", err)
	}

	switch {
	case !repository.consumerEnabled && !repository.workerEnabled:
		status.State = "disabled"
	case status.DeadCount > 0:
		status.State = "failed"
	case status.PendingCount > 0 && status.OldestPendingAt != nil &&
		repository.clock().Sub(*status.OldestPendingAt) > repository.staleAfter:
		status.State = "stale"
	case status.PendingCount > 0 || (repository.consumerEnabled && !repository.workerEnabled):
		status.State = "partial"
	case status.AppliedCount > 0:
		status.State = "ready"
	default:
		status.State = "empty"
	}
	status.Complete = status.State == "ready" || status.State == "empty" || status.State == "disabled"
	return status, nil
}
