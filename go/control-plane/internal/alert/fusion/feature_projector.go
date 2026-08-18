package fusion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

func BuildFeatureProjection(
	selected []sourceSnapshotSummary,
	boundEntities []BoundSourceEntityFact,
	boundRelations []BoundSourceRelationFact,
	fullEntities []CanonicalEntity,
	fullRelations []CanonicalRelation,
) ([]FeatureMetric, []AblationResult, error) {
	selectedSources := make(map[string]struct{}, len(selected))
	maxSingleSourceEntities := 0
	for _, snapshot := range selected {
		selectedSources[snapshot.SourceID] = struct{}{}
		count := 0
		for _, entity := range boundEntities {
			if entity.SourceID == snapshot.SourceID {
				count++
			}
		}
		if count > maxSingleSourceEntities {
			maxSingleSourceEntities = count
		}
	}
	metrics := []FeatureMetric{
		{Name: "input_source_entity_count", Value: float64(len(boundEntities)), Unit: "entities", Semantics: "sum of selected source-snapshot entities before identity merge"},
		{Name: "multi_source_entity_count", Value: float64(len(fullEntities)), Unit: "entities", Semantics: "canonical entities after strong-identifier union"},
		{Name: "multi_source_relation_count", Value: float64(len(fullRelations)), Unit: "relations", Semantics: "observed relations retained after endpoint merge"},
		{Name: "entity_merge_reduction_count", Value: float64(len(boundEntities) - len(fullEntities)), Unit: "entities", Semantics: "identity coalescing count; not an accuracy metric"},
		{Name: "best_single_source_entity_count", Value: float64(maxSingleSourceEntities), Unit: "entities", Semantics: "largest same-window single-source entity count; not an accuracy baseline"},
		{Name: "source_coverage_ratio", Value: float64(len(selectedSources)) / float64(len(requiredSourceIDs)), Unit: "ratio", Semantics: "available required sources divided by four"},
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].Name < metrics[j].Name })
	ablations := make([]AblationResult, 0, len(requiredSourceIDs))
	for _, omitted := range requiredSourceIDs {
		if _, exists := selectedSources[omitted]; !exists {
			canonicalSHA, err := canonicalSHA256(map[string]interface{}{
				"algorithm": "per-source-ablation-v1", "omitted_source_id": omitted,
				"status": "not_applicable", "reason": "source_absent_from_full_snapshot",
			})
			if err != nil {
				return nil, nil, err
			}
			ablations = append(ablations, AblationResult{
				OmittedSourceID: omitted, Status: "not_applicable", IncludedSourceCount: len(selectedSources),
				EntityCount: len(fullEntities), RelationCount: len(fullRelations), CanonicalSHA256: canonicalSHA,
			})
			continue
		}
		entities := make([]BoundSourceEntityFact, 0, len(boundEntities))
		for _, entity := range boundEntities {
			if entity.SourceID != omitted {
				entities = append(entities, entity)
			}
		}
		relations := make([]BoundSourceRelationFact, 0, len(boundRelations))
		for _, relation := range boundRelations {
			if relation.SourceID != omitted {
				relations = append(relations, relation)
			}
		}
		mergedEntities, mergedRelations, err := MergeSourceEntities(entities, relations)
		if err != nil {
			return nil, nil, fmt.Errorf("ablate source %s: %w", omitted, err)
		}
		status := "partial"
		if len(selectedSources) == len(requiredSourceIDs) {
			status = "complete"
		}
		canonicalSHA, err := canonicalSHA256(map[string]interface{}{
			"algorithm": "per-source-ablation-v1", "omitted_source_id": omitted, "status": status,
			"included_source_count": len(selectedSources) - 1, "entities": mergedEntities, "relations": mergedRelations,
		})
		if err != nil {
			return nil, nil, err
		}
		ablations = append(ablations, AblationResult{
			OmittedSourceID: omitted, Status: status, IncludedSourceCount: len(selectedSources) - 1,
			EntityCount: len(mergedEntities), RelationCount: len(mergedRelations),
			EntityDelta: len(mergedEntities) - len(fullEntities), RelationDelta: len(mergedRelations) - len(fullRelations),
			CanonicalSHA256: canonicalSHA,
		})
	}
	sort.Slice(ablations, func(i, j int) bool { return ablations[i].OmittedSourceID < ablations[j].OmittedSourceID })
	return metrics, ablations, nil
}

func insertFeatureSnapshotTx(
	ctx context.Context,
	tx *sql.Tx,
	command SourceSyncCommand,
	snapshotID string,
	version int64,
	status string,
	partialSources []string,
	canonicalSHA string,
	dataSnapshotID string,
	dataVersion int64,
	dataSHA string,
	selected []sourceSnapshotSummary,
	metrics []FeatureMetric,
	ablations []AblationResult,
	entityCount int,
	relationCount int,
) error {
	quality := map[string]interface{}{
		"status": status, "partial_sources": partialSources, "required_sources": requiredSourceIDs,
		"ablation_count": len(ablations), "value_claim": "coverage_and_merge_metrics_only",
	}
	provenance := map[string]interface{}{
		"algorithm": "fusion-feature-ablation-v1", "data_snapshot_id": dataSnapshotID,
		"data_snapshot_version": dataVersion, "data_snapshot_sha256": dataSHA,
		"trigger_job_id": command.JobID, "source_snapshots": selected,
	}
	qualityJSON, _ := json.Marshal(quality)
	provenanceJSON, _ := json.Marshal(provenance)
	_, err := tx.ExecContext(ctx, `INSERT INTO fusion_snapshots (
		snapshot_id,tenant_id,fusion_level,snapshot_version,as_of,window_start,window_end,status,
		partial_sources,entity_count,relation_count,canonical_sha256,quality_summary,provenance,trace_id,created_by
	) VALUES ($1,$2,'feature',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14,$15)`,
		snapshotID, command.TenantID, version, command.WindowEnd, command.WindowStart, command.WindowEnd,
		status, pq.Array(partialSources), entityCount, relationCount, canonicalSHA, string(qualityJSON),
		string(provenanceJSON), command.TraceID, command.RequestedBy)
	if err != nil {
		return fmt.Errorf("insert fusion feature snapshot: %w", err)
	}
	for order, source := range selected {
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_snapshot_sources (
			tenant_id,fusion_snapshot_id,source_snapshot_id,source_role,included,exclusion_reason,source_order
		) VALUES ($1,$2,$3,'primary',true,'',$4)`, command.TenantID, snapshotID, source.SnapshotID, order)
		if err != nil {
			return fmt.Errorf("link feature source snapshot %s: %w", source.SnapshotID, err)
		}
	}
	for _, metric := range metrics {
		metricSHA, hashErr := canonicalSHA256(metric)
		if hashErr != nil {
			return hashErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_feature_metrics (
			tenant_id,feature_snapshot_id,metric_name,metric_value,metric_unit,metric_semantics,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`, command.TenantID, snapshotID, metric.Name,
			metric.Value, metric.Unit, metric.Semantics, metricSHA)
		if err != nil {
			return fmt.Errorf("insert fusion feature metric %s: %w", metric.Name, err)
		}
	}
	for _, ablation := range ablations {
		resultJSON, _ := json.Marshal(ablation)
		_, err = tx.ExecContext(ctx, `INSERT INTO fusion_feature_ablation_results (
			tenant_id,feature_snapshot_id,omitted_source_id,status,included_source_count,entity_count,
			relation_count,entity_delta,relation_delta,result,canonical_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`, command.TenantID, snapshotID,
			ablation.OmittedSourceID, ablation.Status, ablation.IncludedSourceCount, ablation.EntityCount,
			ablation.RelationCount, ablation.EntityDelta, ablation.RelationDelta, string(resultJSON), ablation.CanonicalSHA256)
		if err != nil {
			return fmt.Errorf("insert fusion feature ablation %s: %w", ablation.OmittedSourceID, err)
		}
	}
	return nil
}

func featureSnapshotCanonical(
	command SourceSyncCommand,
	version int64,
	status string,
	partialSources []string,
	dataSnapshotID string,
	dataVersion int64,
	dataSHA string,
	metrics []FeatureMetric,
	ablations []AblationResult,
) map[string]interface{} {
	return map[string]interface{}{
		"algorithm": "fusion-feature-ablation-v1", "tenant_id": command.TenantID,
		"fusion_level": "feature", "snapshot_version": version, "status": status,
		"window_start":    command.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":      command.WindowEnd.UTC().Format(time.RFC3339Nano),
		"partial_sources": partialSources, "data_snapshot_id": dataSnapshotID,
		"data_snapshot_version": dataVersion, "data_snapshot_sha256": dataSHA,
		"metrics": metrics, "ablations": ablations,
	}
}
