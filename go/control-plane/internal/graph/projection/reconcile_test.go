package projection

import (
	"context"
	"strings"
	"testing"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type memorySnapshotReader struct {
	facts   []ProjectionFact
	profile string
}

func (reader *memorySnapshotReader) LoadReconcileSnapshot(context.Context, ReconcileScope) ([]ProjectionFact, string, error) {
	return append([]ProjectionFact(nil), reader.facts...), reader.profile, nil
}

type memoryRepairTarget struct{ memorySnapshotReader }

func (target *memoryRepairTarget) Ready(context.Context) error { return nil }
func (target *memoryRepairTarget) Apply(_ context.Context, event *trafficv1.GraphProjectionEvent) error {
	metadata, err := metadataOf(event)
	if err != nil {
		return err
	}
	fact := ProjectionFact{
		Kind: metadata.kind, ProjectionID: metadata.projectionID,
		AggregateVersion: metadata.aggregateVersion, ProjectionSHA256: metadata.projectionSHA256,
		Revoked: metadata.revoked,
	}
	for index := range target.facts {
		if target.facts[index].key() == fact.key() {
			target.facts[index] = fact
			return nil
		}
	}
	target.facts = append(target.facts, fact)
	return nil
}

type memoryRepairSource struct {
	events map[string]*trafficv1.GraphProjectionEvent
}

func (source memoryRepairSource) LoadProjectionEvents(_ context.Context, _ ReconcileScope, facts []ProjectionFact) ([]*trafficv1.GraphProjectionEvent, error) {
	events := make([]*trafficv1.GraphProjectionEvent, 0, len(facts))
	for _, fact := range facts {
		events = append(events, source.events[fact.key()])
	}
	return events, nil
}

type memoryManifestRecorder struct {
	manifests     []ReconcileManifest
	authorization *RepairAuthorization
}

func (recorder *memoryManifestRecorder) RecordReconcileManifest(_ context.Context, manifest ReconcileManifest) error {
	recorder.manifests = append(recorder.manifests, manifest)
	return nil
}
func (recorder *memoryManifestRecorder) RecordRepairAuthorization(_ context.Context, authorization RepairAuthorization) error {
	recorder.authorization = &authorization
	return nil
}

func TestGraphReconcileClassifiesClosedWindowWithoutDeletingExtra(t *testing.T) {
	shaA, shaB, shaC := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	authority := &memorySnapshotReader{facts: []ProjectionFact{
		{Kind: "entity", ProjectionID: "entity-missing", AggregateVersion: 1, ProjectionSHA256: shaA},
		{Kind: "relation", ProjectionID: "relation-stale", AggregateVersion: 2, ProjectionSHA256: shaB},
	}}
	target := &memorySnapshotReader{facts: []ProjectionFact{
		{Kind: "relation", ProjectionID: "relation-stale", AggregateVersion: 1, ProjectionSHA256: shaA},
		{Kind: "entity", ProjectionID: "entity-extra", AggregateVersion: 4, ProjectionSHA256: shaC},
	}}
	service, err := NewReconcileService(authority, target, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := service.Compare(context.Background(), "11111111-1111-4111-8111-111111111111", "before", reconcileTestScope())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.MissingCount != 1 || manifest.StaleCount != 1 || manifest.ExtraCount != 1 ||
		manifest.Converged || !manifest.ExtraPreserved || len(manifest.ManifestSHA256) != 64 {
		t.Fatalf("unexpected graph reconcile manifest: %+v", manifest)
	}
	var extraDifference *ReconcileDifference
	for index := range manifest.Differences {
		if manifest.Differences[index].Class == "extra" {
			extraDifference = &manifest.Differences[index]
		}
	}
	if extraDifference == nil || extraDifference.RepairEligible {
		t.Fatalf("target-only graph fact became a delete instruction: %+v", manifest.Differences)
	}
}

func TestGraphRepairRequiresIndependentApprovalAndConvergesMissingStale(t *testing.T) {
	missingEvent := reconcileEntityEvent(t, "projection-event-missing", "asset-missing", 1, "a")
	staleEvent := reconcileEntityEvent(t, "projection-event-stale", "asset-stale", 2, "b")
	missingFact, staleFact := factOfEvent(t, missingEvent), factOfEvent(t, staleEvent)
	extra := ProjectionFact{Kind: "entity", ProjectionID: "entity-extra", AggregateVersion: 4, ProjectionSHA256: strings.Repeat("c", 64)}
	authority := &memorySnapshotReader{facts: []ProjectionFact{missingFact, staleFact}, profile: "authority-profile"}
	target := &memoryRepairTarget{memorySnapshotReader{facts: []ProjectionFact{
		{Kind: staleFact.Kind, ProjectionID: staleFact.ProjectionID, AggregateVersion: 1, ProjectionSHA256: strings.Repeat("d", 64)}, extra,
	}, profile: "target-profile"}}
	recorder := &memoryManifestRecorder{}
	service, err := NewReconcileService(authority, target, memoryRepairSource{events: map[string]*trafficv1.GraphProjectionEvent{
		missingFact.key(): missingEvent, staleFact.key(): staleEvent,
	}}, target, recorder)
	if err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) }
	before, err := service.Compare(context.Background(), "22222222-2222-4222-8222-222222222222", "before", reconcileTestScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Repair(context.Background(), before, RepairAuthorization{
		RunID: before.RunID, RequestedBy: "operator-a", ApprovedBy: "operator-a",
		ApprovedAt: service.clock(), MaxItems: 2,
	}); err != ErrRepairNotAuthorized {
		t.Fatalf("self-approved graph repair was not rejected: %v", err)
	}
	after, err := service.Repair(context.Background(), before, RepairAuthorization{
		RunID: before.RunID, RequestedBy: "operator-a", ApprovedBy: "operator-b",
		ApprovedAt: service.clock(), MaxItems: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.MissingCount != 0 || after.StaleCount != 0 || after.ExtraCount != 1 || after.Converged {
		t.Fatalf("bounded graph repair did not preserve the target-only extra: %+v", after)
	}
	if recorder.authorization == nil || len(recorder.manifests) != 2 {
		t.Fatalf("repair evidence was not recorded: %+v", recorder)
	}
}

func TestGraphReconcileBudgetFailsBeforeUnboundedComparison(t *testing.T) {
	reader := &memorySnapshotReader{facts: []ProjectionFact{
		{Kind: "entity", ProjectionID: "one", AggregateVersion: 1, ProjectionSHA256: strings.Repeat("a", 64)},
		{Kind: "entity", ProjectionID: "two", AggregateVersion: 1, ProjectionSHA256: strings.Repeat("b", 64)},
	}}
	service, _ := NewReconcileService(reader, &memorySnapshotReader{}, nil, nil, nil)
	scope := reconcileTestScope()
	scope.MaxFacts = 1
	if _, err := service.Compare(context.Background(), "33333333-3333-4333-8333-333333333333", "before", scope); err == nil || !strings.Contains(err.Error(), ErrReconcileBudget.Error()) {
		t.Fatalf("oversized graph reconcile did not fail closed: %v", err)
	}
}

func reconcileTestScope() ReconcileScope {
	return ReconcileScope{
		TenantID: "tenant-a", WindowFrom: time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC),
		WindowThrough: time.Date(2023, 11, 15, 0, 0, 0, 0, time.UTC),
		MaxFacts:      100, MaxDuration: 5 * time.Second,
	}
}

func reconcileEntityEvent(t *testing.T, eventID, canonicalID string, version uint64, hashCharacter string) *trafficv1.GraphProjectionEvent {
	t.Helper()
	event, err := BuildEntityEvent(EntityInput{
		Source: SourceInput{
			EventID: eventID, TenantID: "tenant-a", TraceID: "0123456789abcdef0123456789abcdef",
			Producer: "test", SourceSystem: "test", SourceEventID: "source-" + eventID,
			AggregateType: "asset", AggregateID: canonicalID, AggregateVersion: version,
			SourceSHA256: strings.Repeat(hashCharacter, 64), OccurredAt: 1700000000000, ProducedAt: 1700000000001,
		},
		EntityType: "asset", CanonicalID: canonicalID, Attributes: map[string]string{"name": canonicalID},
		ValidFrom: 1700000000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func factOfEvent(t *testing.T, event *trafficv1.GraphProjectionEvent) ProjectionFact {
	t.Helper()
	metadata, err := metadataOf(event)
	if err != nil {
		t.Fatal(err)
	}
	return ProjectionFact{Kind: metadata.kind, ProjectionID: metadata.projectionID, AggregateVersion: metadata.aggregateVersion, ProjectionSHA256: metadata.projectionSHA256, Revoked: metadata.revoked}
}
