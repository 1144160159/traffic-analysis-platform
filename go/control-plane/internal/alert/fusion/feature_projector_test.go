package fusion

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildFeatureProjectionProducesSameWindowBaselineAndFourAblations(t *testing.T) {
	selected := []sourceSnapshotSummary{
		{SourceID: "asset", SnapshotID: "00000000-0000-0000-0000-000000000001"},
		{SourceID: "behavior", SnapshotID: "00000000-0000-0000-0000-000000000002"},
		{SourceID: "log", SnapshotID: "00000000-0000-0000-0000-000000000003"},
		{SourceID: "traffic", SnapshotID: "00000000-0000-0000-0000-000000000004"},
	}
	boundEntities := []BoundSourceEntityFact{
		featureEntity("asset", selected[0].SnapshotID, "asset-a", "asset", map[string]string{"asset_id": "asset-a", "ip": "10.0.0.1"}),
		featureEntity("behavior", selected[1].SnapshotID, "user-a", "user", map[string]string{"user_id": "user-a"}),
		featureEntity("log", selected[2].SnapshotID, "device-a", "device", map[string]string{"ip": "10.0.0.1"}),
		featureEntity("traffic", selected[3].SnapshotID, "ip-a", "ip", map[string]string{"ip": "10.0.0.1"}),
		featureEntity("traffic", selected[3].SnapshotID, "ip-b", "ip", map[string]string{"ip": "10.0.0.2"}),
	}
	boundRelations := []BoundSourceRelationFact{{
		SourceID: "traffic", SourceSnapshotID: selected[3].SnapshotID,
		Fact: SourceRelationFact{SourceRelationID: "flow-a", SourceEntityID: "ip-a", TargetEntityID: "ip-b",
			RelationKind: "communicated_with", EventTime: time.Unix(100, 0).UTC(), EvidenceEventIDs: []string{"event-a"}},
	}}
	entities, relations, err := MergeSourceEntities(boundEntities, boundRelations)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ablations, err := BuildFeatureProjection(selected, boundEntities, boundRelations, entities, relations)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 6 || len(ablations) != 4 {
		t.Fatalf("unexpected feature projection dimensions: metrics=%d ablations=%d", len(metrics), len(ablations))
	}
	metricByName := make(map[string]FeatureMetric, len(metrics))
	for _, metric := range metrics {
		metricByName[metric.Name] = metric
		if strings.Contains(strings.ToLower(metric.Semantics), "accuracy improvement") {
			t.Fatalf("metric %s overclaims accuracy: %q", metric.Name, metric.Semantics)
		}
	}
	if got := metricByName["best_single_source_entity_count"].Value; got != 2 {
		t.Fatalf("same-window single-source baseline = %v, want 2", got)
	}
	if got := metricByName["source_coverage_ratio"].Value; got != 1 {
		t.Fatalf("source coverage = %v, want 1", got)
	}
	for _, ablation := range ablations {
		if ablation.Status != "complete" || ablation.IncludedSourceCount != 3 || len(ablation.CanonicalSHA256) != 64 {
			t.Fatalf("unexpected complete ablation: %#v", ablation)
		}
	}
	metricsAgain, ablationsAgain, err := BuildFeatureProjection(selected, boundEntities, boundRelations, entities, relations)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metrics, metricsAgain) || !reflect.DeepEqual(ablations, ablationsAgain) {
		t.Fatal("feature projection is not deterministic")
	}
}

func TestBuildFeatureProjectionMarksMissingSourceAblationNotApplicable(t *testing.T) {
	selected := []sourceSnapshotSummary{
		{SourceID: "asset", SnapshotID: "00000000-0000-0000-0000-000000000011"},
		{SourceID: "log", SnapshotID: "00000000-0000-0000-0000-000000000012"},
		{SourceID: "traffic", SnapshotID: "00000000-0000-0000-0000-000000000013"},
	}
	bound := []BoundSourceEntityFact{
		featureEntity("asset", selected[0].SnapshotID, "asset-a", "asset", map[string]string{"asset_id": "asset-a"}),
		featureEntity("log", selected[1].SnapshotID, "device-a", "device", map[string]string{"mac": "00:00:00:00:00:01"}),
		featureEntity("traffic", selected[2].SnapshotID, "ip-a", "ip", map[string]string{"ip": "10.0.0.1"}),
	}
	entities, relations, err := MergeSourceEntities(bound, nil)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ablations, err := BuildFeatureProjection(selected, bound, nil, entities, relations)
	if err != nil {
		t.Fatal(err)
	}
	if got := featureMetricValue(metrics, "source_coverage_ratio"); got != 0.75 {
		t.Fatalf("partial source coverage = %v, want 0.75", got)
	}
	for _, ablation := range ablations {
		if ablation.OmittedSourceID == "behavior" {
			if ablation.Status != "not_applicable" || ablation.IncludedSourceCount != 3 {
				t.Fatalf("missing-source ablation silently treated as data: %#v", ablation)
			}
			return
		}
	}
	t.Fatal("missing behavior source has no explicit ablation result")
}

func featureEntity(sourceID, snapshotID, entityID, kind string, identifiers map[string]string) BoundSourceEntityFact {
	return BoundSourceEntityFact{SourceID: sourceID, SourceSnapshotID: snapshotID, Fact: SourceEntityFact{
		SourceEntityID: entityID, EntityKind: kind, Identifiers: identifiers,
	}}
}

func featureMetricValue(metrics []FeatureMetric, name string) float64 {
	for _, metric := range metrics {
		if metric.Name == name {
			return metric.Value
		}
	}
	return -1
}
