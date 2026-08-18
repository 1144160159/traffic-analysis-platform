package fusion

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Projector struct {
	pg     *sql.DB
	reader SourceFactReader
	limit  int
}

type sourceSnapshotSummary struct {
	SourceID        string
	SnapshotID      string
	SourceVersion   int64
	QualityStatus   string
	PartialReasons  []string
	CanonicalSHA256 string
}

func NewProjector(pg *sql.DB, reader SourceFactReader, limit int) (*Projector, error) {
	if pg == nil || reader == nil {
		return nil, fmt.Errorf("fusion PostgreSQL and source-fact reader are required")
	}
	if limit <= 0 {
		limit = MaxSourceFacts
	}
	if limit > MaxSourceFacts {
		return nil, fmt.Errorf("fusion source fact limit exceeds %d", MaxSourceFacts)
	}
	return &Projector{pg: pg, reader: reader, limit: limit}, nil
}

func (projector *Projector) ApplySourceSync(
	ctx context.Context,
	command SourceSyncCommand,
	eventSHA256 string,
	position KafkaPosition,
) (ProjectionReceipt, error) {
	if projector == nil || projector.pg == nil || projector.reader == nil {
		return ProjectionReceipt{}, fmt.Errorf("fusion projector is not initialized")
	}
	if err := command.Validate(); err != nil {
		return ProjectionReceipt{}, err
	}
	if _, err := uuid.Parse(command.EventID); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("%w: event_id is not a UUID", ErrInvalidCommand)
	}
	if _, err := uuid.Parse(command.JobID); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("%w: job_id is not a UUID", ErrInvalidCommand)
	}
	if len(eventSHA256) != 64 || position.Topic != SourceSyncTopic || position.Partition < 0 || position.Offset < 0 {
		return ProjectionReceipt{}, fmt.Errorf("%w: event digest or Kafka position is invalid", ErrInvalidCommand)
	}
	if receipt, found, err := projector.lookupReceipt(ctx, command, eventSHA256); err != nil || found {
		return receipt, err
	}
	if err := projector.claimJob(ctx, command, eventSHA256); err != nil {
		return ProjectionReceipt{}, err
	}
	batch, err := projector.reader.ReadSourceFacts(
		ctx, command.TenantID, command.SourceID, command.WindowStart, command.WindowEnd, projector.limit,
	)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("read fusion source facts: %w", err)
	}
	entities, relations, err := DecodeSourceFacts(command.SourceID, command.TenantID, batch.Facts)
	if err != nil {
		if errors.Is(err, ErrInvalidSourceFact) {
			return projector.failJob(ctx, command, eventSHA256, position, "INVALID_SOURCE_FACT", err.Error())
		}
		return ProjectionReceipt{}, err
	}
	receipt, err := projector.applySnapshot(ctx, command, eventSHA256, position, batch, entities, relations)
	if errors.Is(err, ErrIdentityConflict) || errors.Is(err, ErrVersionConflict) {
		return projector.failJob(ctx, command, eventSHA256, position, domainFailureCode(err), err.Error())
	}
	return receipt, err
}

func (projector *Projector) lookupReceipt(
	ctx context.Context,
	command SourceSyncCommand,
	eventSHA256 string,
) (ProjectionReceipt, bool, error) {
	var tenantID, jobID, storedSHA, disposition, failureCode string
	var sourceSnapshotID, dataSnapshotID, featureSnapshotID sql.NullString
	err := projector.pg.QueryRowContext(ctx, `SELECT tenant_id,job_id::text,event_sha256,disposition,failure_code,
		source_snapshot_id::text,data_snapshot_id::text,feature_snapshot_id::text
		FROM fusion_projection_inbox WHERE event_id=$1`, command.EventID).Scan(
		&tenantID, &jobID, &storedSHA, &disposition, &failureCode, &sourceSnapshotID, &dataSnapshotID, &featureSnapshotID,
	)
	if err == sql.ErrNoRows {
		return ProjectionReceipt{}, false, nil
	}
	if err != nil {
		return ProjectionReceipt{}, false, fmt.Errorf("read fusion projection inbox: %w", err)
	}
	if tenantID != command.TenantID || jobID != command.JobID || storedSHA != eventSHA256 {
		return ProjectionReceipt{}, false, fmt.Errorf("%w: event replay bytes or tenant/job identity changed", ErrIdentityConflict)
	}
	receipt := ProjectionReceipt{
		EventID: command.EventID, JobID: command.JobID, SourceSnapshotID: sourceSnapshotID.String,
		DataSnapshotID: dataSnapshotID.String, FeatureSnapshotID: featureSnapshotID.String,
		Disposition: disposition, FailureCode: failureCode,
		QualityStatus: map[bool]string{true: "failed", false: "complete"}[disposition == "failed"], Replayed: true,
	}
	if disposition == "applied" {
		err = projector.pg.QueryRowContext(ctx, `SELECT s.source_version,d.snapshot_version,f.snapshot_version,f.status
			FROM fusion_source_snapshots s
			JOIN fusion_snapshots d ON d.tenant_id=s.tenant_id AND d.snapshot_id=$3 AND d.fusion_level='data'
			JOIN fusion_snapshots f ON f.tenant_id=s.tenant_id AND f.snapshot_id=$4 AND f.fusion_level='feature'
			WHERE s.tenant_id=$1 AND s.snapshot_id=$2`, command.TenantID, sourceSnapshotID.String,
			dataSnapshotID.String, featureSnapshotID.String).Scan(
			&receipt.SourceVersion, &receipt.DataVersion, &receipt.FeatureVersion, &receipt.QualityStatus,
		)
		if err != nil {
			return ProjectionReceipt{}, false, fmt.Errorf("read replayed fusion snapshot versions: %w", err)
		}
	}
	return receipt, true, nil
}

func (projector *Projector) claimJob(ctx context.Context, command SourceSyncCommand, eventSHA256 string) error {
	tx, err := projector.pg.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fusion job claim: %w", err)
	}
	defer tx.Rollback()
	var sourceID, sourceKind, requestSHA, status, traceID string
	var windowStart, windowEnd time.Time
	var expectedVersion sql.NullInt64
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT source_id,source_kind,request_sha256,requested_window_start,
		requested_window_end,expected_source_version,status,revision,trace_id
		FROM fusion_source_sync_jobs WHERE tenant_id=$1 AND job_id=$2 FOR UPDATE`,
		command.TenantID, command.JobID,
	).Scan(&sourceID, &sourceKind, &requestSHA, &windowStart, &windowEnd, &expectedVersion, &status, &revision, &traceID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: source-sync job does not exist", ErrInvalidCommand)
	}
	if err != nil {
		return fmt.Errorf("lock fusion source-sync job: %w", err)
	}
	if sourceID != command.SourceID || sourceKind != command.SourceKind || requestSHA != eventSHA256 ||
		!windowStart.Equal(command.WindowStart) || !windowEnd.Equal(command.WindowEnd) || traceID != command.TraceID ||
		revision != command.AggregateVersion || !sameExpectedVersion(expectedVersion, command.ExpectedSourceVersion) {
		return fmt.Errorf("%w: source-sync event does not match its durable job", ErrIdentityConflict)
	}
	switch status {
	case "queued":
		result, updateErr := tx.ExecContext(ctx, `UPDATE fusion_source_sync_jobs
			SET status='running',started_at=now() WHERE tenant_id=$1 AND job_id=$2 AND status='queued' AND revision=$3`,
			command.TenantID, command.JobID, revision)
		if updateErr != nil {
			return fmt.Errorf("claim fusion source-sync job: %w", updateErr)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: source-sync job claim lost", ErrVersionConflict)
		}
	case "running":
		// A Kafka replay resumes an interrupted claim using the same immutable event.
	case "succeeded", "failed":
		return fmt.Errorf("%w: terminal job is missing its inbox receipt", ErrIdentityConflict)
	default:
		return fmt.Errorf("%w: unsupported job status", ErrIdentityConflict)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fusion job claim: %w", err)
	}
	return nil
}

func (projector *Projector) applySnapshot(
	ctx context.Context,
	command SourceSyncCommand,
	eventSHA256 string,
	position KafkaPosition,
	batch SourceFactBatch,
	entities []SourceEntityFact,
	relations []SourceRelationFact,
) (ProjectionReceipt, error) {
	tx, err := projector.pg.BeginTx(ctx, nil)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("begin fusion snapshot transaction: %w", err)
	}
	defer tx.Rollback()
	if receipt, found, err := lookupReceiptTx(ctx, tx, command, eventSHA256); err != nil || found {
		if err == nil {
			receipt.Replayed = true
		}
		return receipt, err
	}
	if err := lockFusionJobTx(ctx, tx, command, eventSHA256, "running"); err != nil {
		return ProjectionReceipt{}, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":source:"+command.SourceID); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("lock fusion source snapshot stream: %w", err)
	}
	var previousVersion int64
	err = tx.QueryRowContext(ctx, `SELECT source_version FROM fusion_source_snapshots
		WHERE tenant_id=$1 AND source_id=$2 ORDER BY source_version DESC LIMIT 1 FOR UPDATE`,
		command.TenantID, command.SourceID,
	).Scan(&previousVersion)
	if err != nil && err != sql.ErrNoRows {
		return ProjectionReceipt{}, fmt.Errorf("read fusion source version: %w", err)
	}
	if command.ExpectedSourceVersion != nil && previousVersion != *command.ExpectedSourceVersion {
		return ProjectionReceipt{}, fmt.Errorf("%w: expected source version %d, current %d", ErrVersionConflict, *command.ExpectedSourceVersion, previousVersion)
	}
	sourceVersion := previousVersion + 1
	qualityStatus, partialReasons := sourceQuality(batch)
	sourceCursor := buildSourceCursor(command, batch)
	sourceCanonical := map[string]interface{}{
		"algorithm": "source-snapshot-v1", "tenant_id": command.TenantID, "source_id": command.SourceID,
		"source_kind": command.SourceKind, "source_version": sourceVersion,
		"window_start":   command.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":     command.WindowEnd.UTC().Format(time.RFC3339Nano),
		"quality_status": qualityStatus, "partial_reasons": partialReasons,
		"cursor": sourceCursor, "facts": canonicalFactSummaries(batch.Facts),
	}
	sourceSHA, err := canonicalSHA256(sourceCanonical)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	sourceSnapshotID := uuid.NewString()
	provenance := map[string]interface{}{
		"algorithm": "source-snapshot-v1", "job_id": command.JobID, "event_id": command.EventID,
		"window_start":       command.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":         command.WindowEnd.UTC().Format(time.RFC3339Nano),
		"observed_row_count": batch.Total, "materialized_row_count": len(batch.Facts),
		"truncated": batch.Truncated,
	}
	if err := insertSourceSnapshotTx(ctx, tx, command, sourceSnapshotID, sourceVersion, qualityStatus,
		partialReasons, sourceCursor, sourceSHA, len(batch.Facts), provenance); err != nil {
		return ProjectionReceipt{}, err
	}
	if err := insertSourceFactsTx(ctx, tx, command.TenantID, sourceSnapshotID, entities, relations); err != nil {
		return ProjectionReceipt{}, err
	}
	selected, missing, err := loadLatestSourceSnapshotsTx(ctx, tx, command.TenantID, command.WindowStart, command.WindowEnd)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	boundEntities, boundRelations, err := loadBoundSourceFactsTx(ctx, tx, command.TenantID, selected)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	canonicalEntities, canonicalRelations, err := MergeSourceEntities(boundEntities, boundRelations)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":fusion:data"); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("lock fusion data snapshot stream: %w", err)
	}
	var previousDataVersion int64
	err = tx.QueryRowContext(ctx, `SELECT snapshot_version FROM fusion_snapshots
		WHERE tenant_id=$1 AND fusion_level='data' ORDER BY snapshot_version DESC LIMIT 1 FOR UPDATE`, command.TenantID).Scan(&previousDataVersion)
	if err != nil && err != sql.ErrNoRows {
		return ProjectionReceipt{}, fmt.Errorf("read fusion data version: %w", err)
	}
	dataVersion := previousDataVersion + 1
	partialSources := append([]string(nil), missing...)
	for _, snapshot := range selected {
		if snapshot.QualityStatus != "complete" {
			partialSources = append(partialSources, snapshot.SourceID)
		}
	}
	partialSources = uniqueSorted(partialSources)
	dataStatus := "complete"
	if len(partialSources) > 0 {
		dataStatus = "partial"
	}
	dataCanonical := map[string]interface{}{
		"algorithm": "strong-identifier-union-v1", "tenant_id": command.TenantID,
		"fusion_level": "data", "snapshot_version": dataVersion,
		"window_start": command.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":   command.WindowEnd.UTC().Format(time.RFC3339Nano), "status": dataStatus,
		"partial_sources": partialSources, "sources": selected,
		"entities": canonicalEntities, "relations": canonicalRelations,
	}
	dataSHA, err := canonicalSHA256(dataCanonical)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	dataSnapshotID := uuid.NewString()
	if err := insertDataSnapshotTx(ctx, tx, command, dataSnapshotID, dataVersion, dataStatus,
		partialSources, dataSHA, selected, canonicalEntities, canonicalRelations); err != nil {
		return ProjectionReceipt{}, err
	}
	metrics, ablations, err := BuildFeatureProjection(
		selected, boundEntities, boundRelations, canonicalEntities, canonicalRelations,
	)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("build fusion feature projection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":fusion:feature"); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("lock fusion feature snapshot stream: %w", err)
	}
	var previousFeatureVersion int64
	err = tx.QueryRowContext(ctx, `SELECT snapshot_version FROM fusion_snapshots
		WHERE tenant_id=$1 AND fusion_level='feature' ORDER BY snapshot_version DESC LIMIT 1 FOR UPDATE`, command.TenantID).Scan(&previousFeatureVersion)
	if err != nil && err != sql.ErrNoRows {
		return ProjectionReceipt{}, fmt.Errorf("read fusion feature version: %w", err)
	}
	featureVersion := previousFeatureVersion + 1
	featureCanonical := featureSnapshotCanonical(command, featureVersion, dataStatus, partialSources,
		dataSnapshotID, dataVersion, dataSHA, metrics, ablations)
	featureSHA, err := canonicalSHA256(featureCanonical)
	if err != nil {
		return ProjectionReceipt{}, err
	}
	featureSnapshotID := uuid.NewString()
	if err := insertFeatureSnapshotTx(ctx, tx, command, featureSnapshotID, featureVersion, dataStatus,
		partialSources, featureSHA, dataSnapshotID, dataVersion, dataSHA, selected, metrics, ablations,
		len(metrics), len(ablations)); err != nil {
		return ProjectionReceipt{}, err
	}
	if err := appendSnapshotOutboxTx(ctx, tx, command, "source_snapshot", sourceSnapshotID, sourceVersion,
		"fusion.source-snapshot.closed.v1", sourceSHA, qualityStatus, len(entities), len(relations), partialReasons); err != nil {
		return ProjectionReceipt{}, err
	}
	if err := appendSnapshotOutboxTx(ctx, tx, command, "fusion_snapshot", dataSnapshotID, dataVersion,
		"fusion.data-snapshot.closed.v1", dataSHA, dataStatus, len(canonicalEntities), len(canonicalRelations), partialSources); err != nil {
		return ProjectionReceipt{}, err
	}
	if err := appendSnapshotOutboxTx(ctx, tx, command, "fusion_snapshot", featureSnapshotID, featureVersion,
		"fusion.feature-snapshot.closed.v1", featureSHA, dataStatus, len(metrics), len(ablations), partialSources); err != nil {
		return ProjectionReceipt{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE fusion_source_sync_jobs SET status='succeeded',revision=revision+1,
		result_source_snapshot_id=$1,result_data_snapshot_id=$2,result_feature_snapshot_id=$3,
		error_code='',error_detail='',completed_at=now()
		WHERE tenant_id=$4 AND job_id=$5 AND status='running'`,
		sourceSnapshotID, dataSnapshotID, featureSnapshotID, command.TenantID, command.JobID)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("complete fusion source-sync job: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ProjectionReceipt{}, fmt.Errorf("%w: source-sync completion lost", ErrVersionConflict)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_projection_inbox (
		event_id,tenant_id,job_id,source_id,event_sha256,source_topic,source_partition,source_offset,
		disposition,failure_code,source_snapshot_id,data_snapshot_id,feature_snapshot_id,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'applied','',$9,$10,$11,$12)`,
		command.EventID, command.TenantID, command.JobID, command.SourceID, eventSHA256,
		position.Topic, position.Partition, position.Offset, sourceSnapshotID, dataSnapshotID, featureSnapshotID, command.TraceID)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("commit fusion projection inbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("commit fusion snapshot transaction: %w", err)
	}
	return ProjectionReceipt{
		EventID: command.EventID, JobID: command.JobID, SourceSnapshotID: sourceSnapshotID,
		DataSnapshotID: dataSnapshotID, FeatureSnapshotID: featureSnapshotID,
		SourceVersion: sourceVersion, DataVersion: dataVersion, FeatureVersion: featureVersion,
		Disposition: "applied", QualityStatus: dataStatus,
	}, nil
}

func (projector *Projector) failJob(
	ctx context.Context,
	command SourceSyncCommand,
	eventSHA256 string,
	position KafkaPosition,
	failureCode string,
	detail string,
) (ProjectionReceipt, error) {
	tx, err := projector.pg.BeginTx(ctx, nil)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("begin fusion failure receipt: %w", err)
	}
	defer tx.Rollback()
	if receipt, found, lookupErr := lookupReceiptTx(ctx, tx, command, eventSHA256); lookupErr != nil || found {
		if lookupErr == nil {
			receipt.Replayed = true
		}
		return receipt, lookupErr
	}
	if err := lockFusionJobTx(ctx, tx, command, eventSHA256, "running"); err != nil {
		return ProjectionReceipt{}, err
	}
	if len(detail) > 4000 {
		detail = detail[:4000]
	}
	result, err := tx.ExecContext(ctx, `UPDATE fusion_source_sync_jobs SET status='failed',revision=revision+1,
		error_code=$1,error_detail=$2,completed_at=now() WHERE tenant_id=$3 AND job_id=$4 AND status='running'`,
		failureCode, detail, command.TenantID, command.JobID)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("fail fusion source-sync job: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ProjectionReceipt{}, fmt.Errorf("%w: source-sync failure receipt lost", ErrVersionConflict)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_projection_inbox (
		event_id,tenant_id,job_id,source_id,event_sha256,source_topic,source_partition,source_offset,
		disposition,failure_code,source_snapshot_id,data_snapshot_id,feature_snapshot_id,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'failed',$9,NULL,NULL,NULL,$10)`,
		command.EventID, command.TenantID, command.JobID, command.SourceID, eventSHA256,
		position.Topic, position.Partition, position.Offset, failureCode, command.TraceID)
	if err != nil {
		return ProjectionReceipt{}, fmt.Errorf("write fusion failure inbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectionReceipt{}, fmt.Errorf("commit fusion failure receipt: %w", err)
	}
	return ProjectionReceipt{
		EventID: command.EventID, JobID: command.JobID, Disposition: "failed",
		FailureCode: failureCode, QualityStatus: "failed",
	}, nil
}

func lookupReceiptTx(ctx context.Context, tx *sql.Tx, command SourceSyncCommand, eventSHA256 string) (ProjectionReceipt, bool, error) {
	var tenantID, jobID, storedSHA, disposition, failureCode string
	var sourceSnapshotID, dataSnapshotID, featureSnapshotID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT tenant_id,job_id::text,event_sha256,disposition,failure_code,
		source_snapshot_id::text,data_snapshot_id::text,feature_snapshot_id::text
		FROM fusion_projection_inbox WHERE event_id=$1 FOR UPDATE`, command.EventID).Scan(
		&tenantID, &jobID, &storedSHA, &disposition, &failureCode, &sourceSnapshotID, &dataSnapshotID, &featureSnapshotID,
	)
	if err == sql.ErrNoRows {
		return ProjectionReceipt{}, false, nil
	}
	if err != nil {
		return ProjectionReceipt{}, false, fmt.Errorf("lock fusion projection inbox: %w", err)
	}
	if tenantID != command.TenantID || jobID != command.JobID || storedSHA != eventSHA256 {
		return ProjectionReceipt{}, false, fmt.Errorf("%w: event replay identity changed", ErrIdentityConflict)
	}
	return ProjectionReceipt{
		EventID: command.EventID, JobID: command.JobID, SourceSnapshotID: sourceSnapshotID.String,
		DataSnapshotID: dataSnapshotID.String, FeatureSnapshotID: featureSnapshotID.String,
		Disposition: disposition, FailureCode: failureCode,
		QualityStatus: map[bool]string{true: "failed", false: "complete"}[disposition == "failed"],
	}, true, nil
}

func lockFusionJobTx(ctx context.Context, tx *sql.Tx, command SourceSyncCommand, eventSHA256, expectedStatus string) error {
	var sourceID, sourceKind, requestSHA, status, traceID string
	var windowStart, windowEnd time.Time
	var expectedVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT source_id,source_kind,request_sha256,requested_window_start,
		requested_window_end,expected_source_version,status,trace_id FROM fusion_source_sync_jobs
		WHERE tenant_id=$1 AND job_id=$2 FOR UPDATE`, command.TenantID, command.JobID).Scan(
		&sourceID, &sourceKind, &requestSHA, &windowStart, &windowEnd, &expectedVersion, &status, &traceID,
	)
	if err != nil {
		return fmt.Errorf("lock fusion source-sync job: %w", err)
	}
	if sourceID != command.SourceID || sourceKind != command.SourceKind || requestSHA != eventSHA256 ||
		!windowStart.Equal(command.WindowStart) || !windowEnd.Equal(command.WindowEnd) ||
		traceID != command.TraceID || !sameExpectedVersion(expectedVersion, command.ExpectedSourceVersion) {
		return fmt.Errorf("%w: durable job identity changed", ErrIdentityConflict)
	}
	if status != expectedStatus {
		return fmt.Errorf("%w: expected job status %s, current %s", ErrVersionConflict, expectedStatus, status)
	}
	return nil
}

func sameExpectedVersion(stored sql.NullInt64, expected *int64) bool {
	if expected == nil {
		return !stored.Valid
	}
	return stored.Valid && stored.Int64 == *expected
}

func sourceQuality(batch SourceFactBatch) (string, []string) {
	reasons := make([]string, 0, 2)
	if batch.Total == 0 {
		reasons = append(reasons, "not_arrived")
	}
	if batch.Truncated {
		reasons = append(reasons, "fact_limit_exceeded")
	}
	if len(reasons) > 0 {
		return "partial", reasons
	}
	return "complete", reasons
}

func buildSourceCursor(command SourceSyncCommand, batch SourceFactBatch) map[string]interface{} {
	offsets := make(map[string]int64)
	var maxSourceVersion uint64
	for _, fact := range batch.Facts {
		key := fact.SourceTopic + ":" + strconv.Itoa(fact.SourcePartition)
		if previous, exists := offsets[key]; !exists || fact.SourceOffset > previous {
			offsets[key] = fact.SourceOffset
		}
		if fact.SourceVersion > maxSourceVersion {
			maxSourceVersion = fact.SourceVersion
		}
	}
	return map[string]interface{}{
		"window_start":      command.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":        command.WindowEnd.UTC().Format(time.RFC3339Nano),
		"partition_offsets": offsets, "max_fact_source_version": maxSourceVersion,
		"observed_row_count": batch.Total, "materialized_row_count": len(batch.Facts),
	}
}

func canonicalFactSummaries(facts []SourceFact) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(facts))
	for _, fact := range facts {
		result = append(result, map[string]interface{}{
			"aggregate_id": fact.AggregateID, "event_id": fact.EventID,
			"event_time":   fact.EventTime.UTC().Format(time.RFC3339Nano),
			"source_topic": fact.SourceTopic, "source_partition": fact.SourcePartition,
			"source_offset": fact.SourceOffset, "source_payload_sha256": fact.SourcePayloadSHA256,
			"source_version": fact.SourceVersion, "projection_identity": fact.ProjectionIdentity,
			"projection_hash": fact.ProjectionHash,
		})
	}
	return result
}

func canonicalSHA256(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal canonical fusion value: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func insertSourceSnapshotTx(
	ctx context.Context, tx *sql.Tx, command SourceSyncCommand, snapshotID string, version int64,
	qualityStatus string, partialReasons []string, cursor map[string]interface{}, canonicalSHA string,
	rowCount int, provenance map[string]interface{},
) error {
	cursorJSON, _ := json.Marshal(cursor)
	provenanceJSON, _ := json.Marshal(provenance)
	_, err := tx.ExecContext(ctx, `INSERT INTO fusion_source_snapshots (
		snapshot_id,tenant_id,source_id,source_kind,source_cursor,as_of,window_start,window_end,
		source_version,quality_status,partial_reasons,row_count,canonical_sha256,provenance,trace_id
	) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15)`,
		snapshotID, command.TenantID, command.SourceID, command.SourceKind, string(cursorJSON), command.WindowEnd,
		command.WindowStart, command.WindowEnd, version, qualityStatus, pq.Array(partialReasons), rowCount,
		canonicalSHA, string(provenanceJSON), command.TraceID)
	if err != nil {
		return fmt.Errorf("insert fusion source snapshot: %w", err)
	}
	return nil
}

func insertSourceFactsTx(
	ctx context.Context, tx *sql.Tx, tenantID, snapshotID string,
	entities []SourceEntityFact, relations []SourceRelationFact,
) error {
	for _, entity := range entities {
		identifiersJSON, _ := json.Marshal(entity.Identifiers)
		evidenceJSON, _ := json.Marshal(entity.EvidenceEventIDs)
		provenanceJSON, _ := json.Marshal(entity.Provenance)
		canonicalSHA, err := canonicalSHA256(entity)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_source_entity_facts (
			tenant_id,source_snapshot_id,source_entity_id,entity_kind,identifiers,evidence_event_ids,provenance,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8)`, tenantID, snapshotID,
			entity.SourceEntityID, entity.EntityKind, string(identifiersJSON), string(evidenceJSON), string(provenanceJSON), canonicalSHA)
		if err != nil {
			return fmt.Errorf("insert fusion source entity %s: %w", entity.SourceEntityID, err)
		}
	}
	for _, relation := range relations {
		evidenceJSON, _ := json.Marshal(relation.EvidenceEventIDs)
		provenanceJSON, _ := json.Marshal(relation.Provenance)
		canonicalSHA, err := canonicalSHA256(relation)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_source_relation_facts (
			tenant_id,source_snapshot_id,source_relation_id,source_entity_id,target_entity_id,
			relation_kind,event_time,evidence_event_ids,provenance,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10)`, tenantID, snapshotID,
			relation.SourceRelationID, relation.SourceEntityID, relation.TargetEntityID, relation.RelationKind,
			relation.EventTime, string(evidenceJSON), string(provenanceJSON), canonicalSHA)
		if err != nil {
			return fmt.Errorf("insert fusion source relation %s: %w", relation.SourceRelationID, err)
		}
	}
	return nil
}

func loadLatestSourceSnapshotsTx(
	ctx context.Context, tx *sql.Tx, tenantID string, windowStart, windowEnd time.Time,
) ([]sourceSnapshotSummary, []string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON (source_id) source_id,snapshot_id::text,source_version,
		quality_status,partial_reasons,canonical_sha256 FROM fusion_source_snapshots
		WHERE tenant_id=$1 AND window_start=$2 AND window_end=$3 AND source_id=ANY($4::text[])
		ORDER BY source_id,source_version DESC`, tenantID, windowStart, windowEnd, pq.Array(requiredSourceIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("load fusion source snapshot set: %w", err)
	}
	defer rows.Close()
	selected := make([]sourceSnapshotSummary, 0, len(requiredSourceIDs))
	found := make(map[string]struct{})
	for rows.Next() {
		var summary sourceSnapshotSummary
		if err := rows.Scan(&summary.SourceID, &summary.SnapshotID, &summary.SourceVersion,
			&summary.QualityStatus, pq.Array(&summary.PartialReasons), &summary.CanonicalSHA256); err != nil {
			return nil, nil, fmt.Errorf("scan fusion source snapshot set: %w", err)
		}
		selected = append(selected, summary)
		found[summary.SourceID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate fusion source snapshot set: %w", err)
	}
	missing := make([]string, 0)
	for _, sourceID := range requiredSourceIDs {
		if _, exists := found[sourceID]; !exists {
			missing = append(missing, sourceID)
		}
	}
	return selected, missing, nil
}

func loadBoundSourceFactsTx(
	ctx context.Context, tx *sql.Tx, tenantID string, selected []sourceSnapshotSummary,
) ([]BoundSourceEntityFact, []BoundSourceRelationFact, error) {
	if len(selected) == 0 {
		return []BoundSourceEntityFact{}, []BoundSourceRelationFact{}, nil
	}
	snapshotIDs := make([]string, 0, len(selected))
	sourceBySnapshot := make(map[string]string, len(selected))
	for _, snapshot := range selected {
		snapshotIDs = append(snapshotIDs, snapshot.SnapshotID)
		sourceBySnapshot[snapshot.SnapshotID] = snapshot.SourceID
	}
	entityRows, err := tx.QueryContext(ctx, `SELECT source_snapshot_id::text,source_entity_id,entity_kind,
		identifiers::text,evidence_event_ids::text,provenance::text FROM fusion_source_entity_facts
		WHERE tenant_id=$1 AND source_snapshot_id=ANY($2::uuid[]) ORDER BY source_snapshot_id,source_entity_id`,
		tenantID, pq.Array(snapshotIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("load fusion source entities: %w", err)
	}
	boundEntities := make([]BoundSourceEntityFact, 0)
	for entityRows.Next() {
		var snapshotID, identifiersJSON, evidenceJSON, provenanceJSON string
		var fact SourceEntityFact
		if err := entityRows.Scan(&snapshotID, &fact.SourceEntityID, &fact.EntityKind,
			&identifiersJSON, &evidenceJSON, &provenanceJSON); err != nil {
			entityRows.Close()
			return nil, nil, fmt.Errorf("scan fusion source entity: %w", err)
		}
		if err := json.Unmarshal([]byte(identifiersJSON), &fact.Identifiers); err != nil {
			entityRows.Close()
			return nil, nil, fmt.Errorf("decode fusion source entity identifiers: %w", err)
		}
		if err := json.Unmarshal([]byte(evidenceJSON), &fact.EvidenceEventIDs); err != nil {
			entityRows.Close()
			return nil, nil, fmt.Errorf("decode fusion source entity evidence: %w", err)
		}
		if err := json.Unmarshal([]byte(provenanceJSON), &fact.Provenance); err != nil {
			entityRows.Close()
			return nil, nil, fmt.Errorf("decode fusion source entity provenance: %w", err)
		}
		boundEntities = append(boundEntities, BoundSourceEntityFact{SourceID: sourceBySnapshot[snapshotID], SourceSnapshotID: snapshotID, Fact: fact})
	}
	if err := entityRows.Close(); err != nil {
		return nil, nil, fmt.Errorf("close fusion source entity rows: %w", err)
	}
	relationRows, err := tx.QueryContext(ctx, `SELECT source_snapshot_id::text,source_relation_id,source_entity_id,
		target_entity_id,relation_kind,event_time,evidence_event_ids::text,provenance::text
		FROM fusion_source_relation_facts WHERE tenant_id=$1 AND source_snapshot_id=ANY($2::uuid[])
		ORDER BY source_snapshot_id,source_relation_id`, tenantID, pq.Array(snapshotIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("load fusion source relations: %w", err)
	}
	defer relationRows.Close()
	boundRelations := make([]BoundSourceRelationFact, 0)
	for relationRows.Next() {
		var snapshotID, evidenceJSON, provenanceJSON string
		var fact SourceRelationFact
		if err := relationRows.Scan(&snapshotID, &fact.SourceRelationID, &fact.SourceEntityID,
			&fact.TargetEntityID, &fact.RelationKind, &fact.EventTime, &evidenceJSON, &provenanceJSON); err != nil {
			return nil, nil, fmt.Errorf("scan fusion source relation: %w", err)
		}
		if err := json.Unmarshal([]byte(evidenceJSON), &fact.EvidenceEventIDs); err != nil {
			return nil, nil, fmt.Errorf("decode fusion source relation evidence: %w", err)
		}
		if err := json.Unmarshal([]byte(provenanceJSON), &fact.Provenance); err != nil {
			return nil, nil, fmt.Errorf("decode fusion source relation provenance: %w", err)
		}
		boundRelations = append(boundRelations, BoundSourceRelationFact{SourceID: sourceBySnapshot[snapshotID], SourceSnapshotID: snapshotID, Fact: fact})
	}
	if err := relationRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate fusion source relations: %w", err)
	}
	return boundEntities, boundRelations, nil
}

func insertDataSnapshotTx(
	ctx context.Context, tx *sql.Tx, command SourceSyncCommand, snapshotID string, version int64,
	status string, partialSources []string, canonicalSHA string, selected []sourceSnapshotSummary,
	entities []CanonicalEntity, relations []CanonicalRelation,
) error {
	quality := map[string]interface{}{
		"status": status, "partial_sources": partialSources, "required_sources": requiredSourceIDs,
		"confidence_semantics": "source_coverage_not_accuracy",
	}
	provenance := map[string]interface{}{
		"algorithm": "strong-identifier-union-v1", "trigger_job_id": command.JobID,
		"trigger_event_id": command.EventID, "source_snapshots": selected,
	}
	qualityJSON, _ := json.Marshal(quality)
	provenanceJSON, _ := json.Marshal(provenance)
	_, err := tx.ExecContext(ctx, `INSERT INTO fusion_snapshots (
		snapshot_id,tenant_id,fusion_level,snapshot_version,as_of,window_start,window_end,status,
		partial_sources,entity_count,relation_count,canonical_sha256,quality_summary,provenance,trace_id,created_by
	) VALUES ($1,$2,'data',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14,$15)`,
		snapshotID, command.TenantID, version, command.WindowEnd, command.WindowStart, command.WindowEnd,
		status, pq.Array(partialSources), len(entities), len(relations), canonicalSHA, string(qualityJSON),
		string(provenanceJSON), command.TraceID, command.RequestedBy)
	if err != nil {
		return fmt.Errorf("insert fusion data snapshot: %w", err)
	}
	for order, source := range selected {
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_snapshot_sources (
			tenant_id,fusion_snapshot_id,source_snapshot_id,source_role,included,exclusion_reason,source_order
		) VALUES ($1,$2,$3,'primary',true,'',$4)`, command.TenantID, snapshotID, source.SnapshotID, order)
		if err != nil {
			return fmt.Errorf("link fusion source snapshot %s: %w", source.SnapshotID, err)
		}
	}
	for _, entity := range entities {
		identifiersJSON, _ := json.Marshal(entity.Identifiers)
		refsJSON, _ := json.Marshal(entity.SourceEntityRefs)
		provenanceJSON, _ := json.Marshal(entity.Provenance)
		entitySHA, hashErr := canonicalSHA256(entity)
		if hashErr != nil {
			return hashErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_snapshot_entities (
			tenant_id,fusion_snapshot_id,entity_id,entity_kind,identifiers,source_entity_refs,
			source_count,confidence,provenance,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,$9::jsonb,$10)`, command.TenantID, snapshotID,
			entity.EntityID, entity.EntityKind, string(identifiersJSON), string(refsJSON), entity.SourceCount,
			entity.Confidence, string(provenanceJSON), entitySHA)
		if err != nil {
			return fmt.Errorf("insert fusion entity %s: %w", entity.EntityID, err)
		}
	}
	for _, relation := range relations {
		evidenceJSON, _ := json.Marshal(relation.EvidenceRefs)
		provenanceJSON, _ := json.Marshal(relation.Provenance)
		relationSHA, hashErr := canonicalSHA256(relation)
		if hashErr != nil {
			return hashErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_snapshot_relations (
			tenant_id,fusion_snapshot_id,relation_id,source_entity_id,target_entity_id,relation_kind,
			edge_origin,event_time,confidence,evidence_refs,provenance,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12)`, command.TenantID, snapshotID,
			relation.RelationID, relation.SourceEntityID, relation.TargetEntityID, relation.RelationKind,
			relation.EdgeOrigin, relation.EventTime, relation.Confidence, string(evidenceJSON), string(provenanceJSON), relationSHA)
		if err != nil {
			return fmt.Errorf("insert fusion relation %s: %w", relation.RelationID, err)
		}
	}
	return nil
}

func appendSnapshotOutboxTx(
	ctx context.Context, tx *sql.Tx, command SourceSyncCommand, aggregateType, aggregateID string,
	aggregateVersion int64, eventType, canonicalSHA, status string, entityCount, relationCount int,
	partial []string,
) error {
	payload := map[string]interface{}{
		"event_id": uuid.NewString(), "event_type": eventType, "schema_version": 1,
		"aggregate_type": aggregateType, "aggregate_id": aggregateID, "aggregate_version": aggregateVersion,
		"partition_key": command.TenantID + ":" + aggregateID, "tenant_id": command.TenantID,
		"canonical_sha256": canonicalSHA, "status": status, "partial": partial,
		"entity_count": entityCount, "relation_count": relationCount, "trace_id": command.TraceID,
		"occurred_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	eventID := payload["event_id"].(string)
	payloadJSON, _ := json.Marshal(payload)
	payloadSHA := sha256.Sum256(payloadJSON)
	_, err := tx.ExecContext(ctx, `INSERT INTO fusion_projection_outbox (
		event_id,tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type,partition_key,
		payload,payload_sha256,publish_state,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,'PENDING',$10)`, eventID, command.TenantID,
		aggregateType, aggregateID, aggregateVersion, eventType, command.TenantID+":"+aggregateID,
		string(payloadJSON), hex.EncodeToString(payloadSHA[:]), command.TraceID)
	if err != nil {
		return fmt.Errorf("append %s fusion outbox: %w", eventType, err)
	}
	return nil
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func domainFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrIdentityConflict):
		return "IDENTITY_CONFLICT"
	case errors.Is(err, ErrVersionConflict):
		return "VERSION_CONFLICT"
	default:
		return "PROJECTION_FAILED"
	}
}
