package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

type fakeAssetObservationReader struct {
	result *config.AssetObservationSummary
	marks  map[string]string
	err    error
}

func (f fakeAssetObservationReader) ReadAssetObservations(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetObservationSummary, map[string]string, error) {
	return f.result, f.marks, f.err
}

type fakeAssetAlertReader struct {
	result *config.AssetAlertContext
	marks  map[string]string
	err    error
}

type fakeAssetGraphReader struct {
	result   *config.AssetGraphProjection
	marks    map[string]string
	complete bool
	err      error
}

type fakeAssetEvidenceReader struct {
	result   *config.AssetEvidenceObjectSet
	marks    map[string]string
	complete bool
	err      error
}

func (f fakeAssetEvidenceReader) ReadAssetEvidenceObjects(context.Context, string, *config.AssetRecord, time.Time, *config.AssetAlertContext) (*config.AssetEvidenceObjectSet, map[string]string, bool, error) {
	return f.result, f.marks, f.complete, f.err
}

func (f fakeAssetGraphReader) ReadAssetGraphProjection(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetGraphProjection, map[string]string, bool, error) {
	return f.result, f.marks, f.complete, f.err
}

func (f fakeAssetAlertReader) ReadAssetAlertContext(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetAlertContext, map[string]string, error) {
	return f.result, f.marks, f.err
}

func baseCrossStoreSnapshot() *config.AssetDetailSnapshot {
	return &config.AssetDetailSnapshot{
		Asset:             &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", IPAddress: "192.0.2.9", Revision: 3},
		AvailableSections: []string{"asset", "details", "history", "postgresql_topology"},
		MissingSections:   []string{"clickhouse_observations", "alert_context", "evidence_objects", "nebulagraph_projection"},
		Partial:           true,
		SourceWatermarks:  map[string]string{"postgresql.assets.revision": "3"},
		AsOf:              time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestEnrichAssetDetailSnapshotMergesIndependentSuccessfulSections(t *testing.T) {
	snapshot := baseCrossStoreSnapshot()
	svc := &AssetService{
		logger: zap.NewNop(),
		observationReader: fakeAssetObservationReader{
			result: &config.AssetObservationSummary{AssetID: "asset-1", Source: "clickhouse.sessions"},
			marks:  map[string]string{"clickhouse.sessions.query_as_of": snapshot.AsOf.Format(time.RFC3339Nano)},
		},
		alertContextReader: fakeAssetAlertReader{
			result: &config.AssetAlertContext{AssetID: "asset-1", Source: "clickhouse.alerts.argmax_state_v1", Alerts: []config.AssetAlertSummary{}},
			marks:  map[string]string{"clickhouse.alerts.query_as_of": snapshot.AsOf.Format(time.RFC3339Nano)},
		},
	}
	svc.enrichAssetDetailSnapshot(context.Background(), "tenant-a", snapshot)
	if snapshot.Observations == nil || snapshot.AlertContext == nil {
		t.Fatalf("cross-store sections=%+v", snapshot)
	}
	if slices.Contains(snapshot.MissingSections, "clickhouse_observations") || slices.Contains(snapshot.MissingSections, "alert_context") {
		t.Fatalf("missing=%v", snapshot.MissingSections)
	}
	if !slices.Contains(snapshot.AvailableSections, "clickhouse_observations") || !slices.Contains(snapshot.AvailableSections, "alert_context") {
		t.Fatalf("available=%v", snapshot.AvailableSections)
	}
	if !snapshot.Partial || snapshot.SourceWatermarks["clickhouse.alerts.query_as_of"] == "" {
		t.Fatalf("partial=%v watermarks=%v", snapshot.Partial, snapshot.SourceWatermarks)
	}
}

func TestEnrichAssetDetailSnapshotKeepsOnlyFailedReaderMissing(t *testing.T) {
	snapshot := baseCrossStoreSnapshot()
	svc := &AssetService{
		logger:            zap.NewNop(),
		observationReader: fakeAssetObservationReader{err: errors.New("ClickHouse sessions timeout")},
		alertContextReader: fakeAssetAlertReader{
			result: &config.AssetAlertContext{AssetID: "asset-1", Alerts: []config.AssetAlertSummary{}},
			marks:  map[string]string{"clickhouse.alerts.query_as_of": snapshot.AsOf.Format(time.RFC3339Nano)},
		},
	}
	svc.enrichAssetDetailSnapshot(context.Background(), "tenant-a", snapshot)
	if snapshot.Observations != nil || snapshot.AlertContext == nil {
		t.Fatalf("cross-store sections=%+v", snapshot)
	}
	if !slices.Contains(snapshot.MissingSections, "clickhouse_observations") || slices.Contains(snapshot.MissingSections, "alert_context") {
		t.Fatalf("missing=%v", snapshot.MissingSections)
	}
}

func TestEnrichAssetDetailSnapshotOnlyClosesCurrentCompleteGraphProjection(t *testing.T) {
	for _, complete := range []bool{false, true} {
		snapshot := baseCrossStoreSnapshot()
		svc := &AssetService{logger: zap.NewNop(), graphProjectionReader: fakeAssetGraphReader{
			result: &config.AssetGraphProjection{AssetID: "asset-1", ProjectedRevision: 3, PostgresRevision: 3, Stale: !complete},
			marks:  map[string]string{"nebulagraph.entity.asset_revision": "3"}, complete: complete,
		}}
		svc.enrichAssetDetailSnapshot(context.Background(), "tenant-a", snapshot)
		if snapshot.GraphProjection == nil || snapshot.SourceWatermarks["nebulagraph.entity.asset_revision"] != "3" {
			t.Fatalf("projection=%+v marks=%v", snapshot.GraphProjection, snapshot.SourceWatermarks)
		}
		if slices.Contains(snapshot.MissingSections, "nebulagraph_projection") != !complete {
			t.Fatalf("complete=%v missing=%v", complete, snapshot.MissingSections)
		}
	}
}

func TestEnrichAssetDetailSnapshotOnlyClosesFullyReconciledEvidenceObjects(t *testing.T) {
	for _, complete := range []bool{false, true} {
		snapshot := baseCrossStoreSnapshot()
		snapshot.AlertContext = &config.AssetAlertContext{AssetID: "asset-1"}
		svc := &AssetService{logger: zap.NewNop(), evidenceObjectReader: fakeAssetEvidenceReader{
			result: &config.AssetEvidenceObjectSet{AssetID: "asset-1", Partial: !complete},
			marks:  map[string]string{"clickhouse.evidence.query_as_of": snapshot.AsOf.Format(time.RFC3339Nano)}, complete: complete,
		}}
		svc.enrichAssetDetailSnapshot(context.Background(), "tenant-a", snapshot)
		if snapshot.EvidenceObjects == nil || snapshot.SourceWatermarks["clickhouse.evidence.query_as_of"] == "" {
			t.Fatalf("evidence=%+v marks=%v", snapshot.EvidenceObjects, snapshot.SourceWatermarks)
		}
		if slices.Contains(snapshot.MissingSections, "evidence_objects") != !complete {
			t.Fatalf("complete=%v missing=%v", complete, snapshot.MissingSections)
		}
	}
}
