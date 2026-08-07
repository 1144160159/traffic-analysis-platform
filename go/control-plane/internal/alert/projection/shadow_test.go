package projection

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type shadowReader struct {
	alerts    []*persistence.Alert
	truncated bool
	err       error
	calls     int
	scope     persistence.ProjectionScope
}

func (r *shadowReader) ListProjectionAlerts(_ context.Context, scope persistence.ProjectionScope) ([]*persistence.Alert, bool, error) {
	r.calls++
	r.scope = scope
	return r.alerts, r.truncated, r.err
}

func shadowTestRequest(now time.Time) ShadowRequest {
	return ShadowRequest{
		RequestedBy:   "operator-a",
		TraceID:       "trace-a",
		EnvironmentID: "candidate-a",
		Scope: persistence.ProjectionScope{
			TenantID:           "tenant-a",
			StartTime:          now.Add(-2 * time.Hour),
			EndTime:            now.Add(-time.Hour),
			TargetIndexVersion: "alerts-v2-write",
			MaxDocuments:       100,
		},
		Target: ShadowTargetMetadata{
			ClusterUUID:  "cluster-a",
			ReadTarget:   "alerts",
			WriteAlias:   "alerts-v2-write",
			WriteIndices: []ShadowWriteIndex{{Index: "alerts-v2-000001", IsWriteIndex: true}},
		},
	}
}

func shadowTestConfig(now time.Time) ShadowConfig {
	return ShadowConfig{MaxDocuments: 10_000, MaxWindow: time.Hour, MinimumWindowAge: 15 * time.Minute, Now: func() time.Time { return now }}
}

func TestBuildShadowManifestClassifiesAndBindsReadOnlyDiff(t *testing.T) {
	now := time.Unix(1_900_000_000, 123_000_000).UTC()
	source := &shadowReader{alerts: []*persistence.Alert{reconcileAlert("b", "closed"), reconcileAlert("a", "new")}}
	target := &shadowReader{alerts: []*persistence.Alert{reconcileAlert("c", "new"), reconcileAlert("b", "new")}}
	request := shadowTestRequest(now)
	request.Target.WriteIndices = []ShadowWriteIndex{
		{Index: "alerts-v2-000002", IsWriteIndex: false},
		{Index: " alerts-v2-000001 ", IsWriteIndex: true},
	}

	manifest, err := BuildShadowManifest(context.Background(), shadowTestConfig(now), source, target, request)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != ShadowStatusDiff || manifest.ApprovalReadiness != ShadowApprovalReady {
		t.Fatalf("unexpected shadow state: %+v", manifest)
	}
	if manifest.MissingCount != 1 || manifest.StaleCount != 1 || manifest.ExtraCount != 1 {
		t.Fatalf("unexpected classification counts: %+v", manifest)
	}
	if manifest.ProductionApplied || len(manifest.ProductionMutations) != 0 || source.calls != 1 || target.calls != 1 {
		t.Fatalf("shadow comparator was not strictly read-only: %+v source=%d target=%d", manifest, source.calls, target.calls)
	}
	if manifest.BindingSHA256 == "" || len(manifest.Binding.Differences) != 3 {
		t.Fatalf("shadow binding is incomplete: %+v", manifest.Binding)
	}
	want := []struct{ id, classification string }{{"a", ShadowClassificationMissing}, {"b", ShadowClassificationStale}, {"c", ShadowClassificationExtra}}
	for index, expected := range want {
		difference := manifest.Binding.Differences[index]
		if difference.AlertID != expected.id || difference.Classification != expected.classification {
			t.Fatalf("difference %d = %+v, want %s/%s", index, difference, expected.id, expected.classification)
		}
		if difference.SourceSHA256 == "" && difference.TargetSHA256 == "" {
			t.Fatalf("difference %d has no projection digest: %+v", index, difference)
		}
	}
	if len(manifest.Warnings) != 1 || !strings.Contains(manifest.Warnings[0], "never be auto-deleted") {
		t.Fatalf("extra projection warning missing: %v", manifest.Warnings)
	}
	if got := manifest.Binding.Target.WriteIndices[0].Index; got != "alerts-v2-000001" {
		t.Fatalf("write-index binding was not normalized: %q", got)
	}
	if !reflect.DeepEqual(source.scope, target.scope) || source.scope.TargetIndexVersion != "alerts-v2-write" {
		t.Fatalf("source and target were not read under the same scope: source=%+v target=%+v", source.scope, target.scope)
	}
}

func TestBuildShadowManifestBindingIsDeterministic(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	requestA := shadowTestRequest(now)
	requestA.Target.WriteIndices = []ShadowWriteIndex{{Index: "index-b"}, {Index: "index-a", IsWriteIndex: true}}
	requestB := requestA
	requestB.Target.WriteIndices = []ShadowWriteIndex{{Index: "index-a", IsWriteIndex: true}, {Index: "index-b"}}
	a, err := BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{alerts: []*persistence.Alert{reconcileAlert("b", "new"), reconcileAlert("a", "new")}},
		&shadowReader{alerts: []*persistence.Alert{reconcileAlert("c", "new")}}, requestA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{alerts: []*persistence.Alert{reconcileAlert("a", "new"), reconcileAlert("b", "new")}},
		&shadowReader{alerts: []*persistence.Alert{reconcileAlert("c", "new")}}, requestB)
	if err != nil {
		t.Fatal(err)
	}
	if a.BindingSHA256 != b.BindingSHA256 || !reflect.DeepEqual(a.Binding, b.Binding) {
		t.Fatalf("equivalent shadow reads produced different bindings: %s != %s", a.BindingSHA256, b.BindingSHA256)
	}
}

func TestBuildShadowManifestBlocksTruncatedAndAmbiguousAlias(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	request := shadowTestRequest(now)
	request.Target.WriteIndices = []ShadowWriteIndex{{Index: "index-a", IsWriteIndex: true}, {Index: "index-b", IsWriteIndex: true}}
	manifest, err := BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{alerts: []*persistence.Alert{reconcileAlert("a", "new")}, truncated: true},
		&shadowReader{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != ShadowStatusPartial || manifest.ApprovalReadiness != ShadowApprovalBlocked || len(manifest.Blockers) != 2 {
		t.Fatalf("truncated ambiguous scope was not blocked: %+v", manifest)
	}
}

func TestBuildShadowManifestNeedsNoRepairForMatchedAuthority(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	alert := reconcileAlert("a", "new")
	manifest, err := BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{alerts: []*persistence.Alert{alert}}, &shadowReader{alerts: []*persistence.Alert{alert}}, shadowTestRequest(now))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != ShadowStatusMatched || manifest.ApprovalReadiness != ShadowApprovalNone || len(manifest.Blockers) != 0 {
		t.Fatalf("matched projection asked for repair: %+v", manifest)
	}
}

func TestBuildShadowManifestRejectsUnsafeScopesBeforeReads(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	tests := map[string]func(*ShadowRequest, *ShadowConfig){
		"wildcard tenant": func(request *ShadowRequest, _ *ShadowConfig) { request.Scope.TenantID = "tenant-*" },
		"open start":      func(request *ShadowRequest, _ *ShadowConfig) { request.Scope.StartTime = time.Time{} },
		"recent end":      func(request *ShadowRequest, _ *ShadowConfig) { request.Scope.EndTime = now.Add(-time.Minute) },
		"oversize window": func(request *ShadowRequest, _ *ShadowConfig) {
			request.Scope.StartTime = request.Scope.EndTime.Add(-time.Hour - time.Second)
		},
		"document budget": func(request *ShadowRequest, _ *ShadowConfig) { request.Scope.MaxDocuments = 10_001 },
		"target wildcard": func(request *ShadowRequest, _ *ShadowConfig) { request.Target.ReadTarget = "alerts-*" },
		"target mismatch": func(request *ShadowRequest, _ *ShadowConfig) { request.Scope.TargetIndexVersion = "other" },
		"young minimum":   func(_ *ShadowRequest, config *ShadowConfig) { config.MinimumWindowAge = time.Minute },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := shadowTestRequest(now)
			config := shadowTestConfig(now)
			mutate(&request, &config)
			source, target := &shadowReader{}, &shadowReader{}
			if _, err := BuildShadowManifest(context.Background(), config, source, target, request); err == nil {
				t.Fatal("unsafe shadow scope was accepted")
			}
			if source.calls != 0 || target.calls != 0 {
				t.Fatalf("unsafe scope reached data readers: source=%d target=%d", source.calls, target.calls)
			}
		})
	}
}

func TestBuildShadowManifestPropagatesReaderFailures(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	_, err := BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{err: errors.New("ClickHouse unavailable")}, &shadowReader{}, shadowTestRequest(now))
	if err == nil || !strings.Contains(err.Error(), "ClickHouse authoritative shadow") {
		t.Fatalf("source failure was not surfaced: %v", err)
	}
	_, err = BuildShadowManifest(context.Background(), shadowTestConfig(now),
		&shadowReader{}, &shadowReader{err: errors.New("OpenSearch unavailable")}, shadowTestRequest(now))
	if err == nil || !strings.Contains(err.Error(), "OpenSearch projection shadow") {
		t.Fatalf("target failure was not surfaced: %v", err)
	}
}
