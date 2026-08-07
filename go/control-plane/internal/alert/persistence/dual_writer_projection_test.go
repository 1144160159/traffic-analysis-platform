package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/opensearchbulk"
)

type projectionTestWriter struct {
	mu      sync.Mutex
	err     error
	writes  int
	version string
}

func (w *projectionTestWriter) WriteAlert(context.Context, *Alert) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	return w.err
}
func (w *projectionTestWriter) WriteBatch(context.Context, []*Alert) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	return w.err
}
func (w *projectionTestWriter) Ping(context.Context) error { return nil }
func (w *projectionTestWriter) Close() error               { return nil }
func (w *projectionTestWriter) TargetVersion() string      { return w.version }

type projectionTestDebtRecorder struct {
	err     error
	alerts  []*Alert
	version string
}

func (r *projectionTestDebtRecorder) RecordProjectionDebt(_ context.Context, alerts []*Alert, version string, _ error) error {
	r.alerts = append([]*Alert(nil), alerts...)
	r.version = version
	return r.err
}

func projectionTestAlerts() []*Alert {
	now := time.Unix(1_800_000_000, 123_000_000).UTC()
	return []*Alert{
		{TenantID: "tenant-a", AlertID: "alert-a", EventID: "event-a", FirstSeen: now, LastSeen: now, UpdatedTs: now},
		{TenantID: "tenant-a", AlertID: "alert-b", EventID: "event-b", FirstSeen: now, LastSeen: now, UpdatedTs: now},
	}
}

func TestWriteBatchRequiresDurableProjectionDebtBeforeCommit(t *testing.T) {
	ch := &projectionTestWriter{}
	os := &projectionTestWriter{err: errors.New("OpenSearch unavailable"), version: "alerts-v2-write"}
	dual := NewDualWriter(ch, os, 100, zap.NewNop())
	dual.maxRetries = 1
	recorder := &projectionTestDebtRecorder{}
	dual.SetProjectionDebtRecorder(recorder)

	outcome, err := dual.WriteBatchWithOutcome(context.Background(), projectionTestAlerts())
	var pending *ProjectionPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("expected ProjectionPendingError, got %v", err)
	}
	if !outcome.ClickHouseCommitted || outcome.OpenSearchCommitted || !outcome.DebtRecorded || outcome.DebtCount != 2 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if len(recorder.alerts) != 2 || recorder.version != "alerts-v2-write" {
		t.Fatalf("unexpected debt record: alerts=%d version=%q", len(recorder.alerts), recorder.version)
	}
	if err := dual.WriteBatch(context.Background(), projectionTestAlerts()); err == nil {
		t.Fatal("ordinary caller must not observe projection-pending as final success")
	}
}

func TestWriteBatchDebtPersistenceFailureBlocksCommit(t *testing.T) {
	ch := &projectionTestWriter{}
	os := &projectionTestWriter{err: errors.New("OpenSearch unavailable"), version: "alerts-v2-write"}
	dual := NewDualWriter(ch, os, 100, zap.NewNop())
	dual.maxRetries = 1
	dual.SetProjectionDebtRecorder(&projectionTestDebtRecorder{err: errors.New("PostgreSQL unavailable")})

	outcome, err := dual.WriteBatchWithOutcome(context.Background(), projectionTestAlerts())
	if err == nil || outcome.DebtRecorded {
		t.Fatalf("debt persistence failure must block commit: outcome=%+v err=%v", outcome, err)
	}
	var pending *ProjectionPendingError
	if errors.As(err, &pending) {
		t.Fatalf("undurable debt must not be classified as safely pending: %v", err)
	}
}

func TestWriteBatchRecordsOnlyAcknowledgedBulkFailures(t *testing.T) {
	ch := &projectionTestWriter{}
	os := &projectionTestWriter{version: "alerts-v2-write", err: &opensearchbulk.PartialFailureError{
		Expected: 2, Received: 2, Failures: []opensearchbulk.ItemFailure{{ID: "alert-b", Status: 429, Retryable: true}},
	}}
	dual := NewDualWriter(ch, os, 100, zap.NewNop())
	dual.maxRetries = 1
	recorder := &projectionTestDebtRecorder{}
	dual.SetProjectionDebtRecorder(recorder)

	outcome, err := dual.WriteBatchWithOutcome(context.Background(), projectionTestAlerts())
	if err == nil || outcome.DebtCount != 1 || len(recorder.alerts) != 1 || recorder.alerts[0].AlertID != "alert-b" {
		t.Fatalf("expected only failed item debt, outcome=%+v alerts=%v err=%v", outcome, recorder.alerts, err)
	}
}

func TestAlertProjectionHashExcludesRuntimeEnrichment(t *testing.T) {
	alert := projectionTestAlerts()[0]
	first, err := AlertProjectionSHA256(alert)
	if err != nil {
		t.Fatal(err)
	}
	alert.AttackPhase = "execution"
	alert.ArkimeLink = "https://example.invalid/session"
	alert.EvidenceCount = 99
	second, err := AlertProjectionSHA256(alert)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("runtime-only enrichment changed authoritative hash: %s != %s", first, second)
	}
}
