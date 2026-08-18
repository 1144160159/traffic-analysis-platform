package api

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestReadinessFenceClaimMatrix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT consumer_role").
		WithArgs(alertconfig.ProbeOperationPipelineID).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_role"}).
			AddRow(string(alertconfig.ProbeAckAuthorityConsumer)).
			AddRow(string(alertconfig.ProbeCommandDeliveryConsumer)).
			AddRow(string(alertconfig.ProbeLifecycleProjectionConsumer)))
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(10, "worker-ready").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "operation_id", "tenant_id", "event_type", "partition_key",
			"aggregate_version", "schema_version", "publish_attempt", "payload",
		}))
	mock.ExpectCommit()
	items, err := store.FenceClaim(context.Background(), "worker-ready", 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProbeDispatcherReadinessMatrix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := NewProbeDispatcherGate(store)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT consumer_role").
		WithArgs(alertconfig.ProbeOperationPipelineID).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_role"}).
			AddRow(string(alertconfig.ProbeAckAuthorityConsumer)).
			AddRow(string(alertconfig.ProbeCommandDeliveryConsumer)))
	mock.ExpectRollback()
	if _, err := gate.AllowClaim(context.Background(), "worker-blocked", 10); !errors.Is(err, ErrProbePipelineNotReady) {
		t.Fatalf("err=%v, want ErrProbePipelineNotReady", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessAuthorityLifecycleMatrix(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	ready := alertconfig.ProbePipelineReadinessReceipt{
		PipelineID:    alertconfig.ProbeOperationPipelineID,
		ConsumerRole:  alertconfig.ProbeAckAuthorityConsumer,
		ConsumerGroup: "alert-probe-acks", OwnerID: "member-a", OwnerEpoch: 4,
		State: alertconfig.ProbePipelineReady, ObservedAt: now, LeaseExpiresAt: now.Add(time.Minute),
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	mock.ExpectQuery("INSERT INTO probe_pipeline_readiness_epochs").
		WithArgs(ready.PipelineID, string(ready.ConsumerRole), ready.ConsumerGroup,
			ready.OwnerID, ready.OwnerEpoch, ready.ObservedAt, ready.LeaseExpiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"owner_epoch"}).AddRow(int64(4)))
	if err := store.IssueRenewRevoke(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	revoked := ready
	revoked.State = alertconfig.ProbePipelineRevoked
	revoked.LeaseExpiresAt = time.Time{}
	revoked.ObservedAt = now.Add(10 * time.Second)
	mock.ExpectExec("UPDATE probe_pipeline_readiness_epochs").
		WithArgs(revoked.PipelineID, string(revoked.ConsumerRole), revoked.ConsumerGroup,
			revoked.OwnerID, revoked.OwnerEpoch, revoked.ObservedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.IssueRenewRevoke(context.Background(), revoked); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE probe_pipeline_readiness_epochs").
		WithArgs(revoked.PipelineID, string(revoked.ConsumerRole), revoked.ConsumerGroup,
			revoked.OwnerID, revoked.OwnerEpoch, revoked.ObservedAt).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT consumer_group,owner_id,owner_epoch,ready").
		WithArgs(revoked.PipelineID, string(revoked.ConsumerRole)).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_group", "owner_id", "owner_epoch", "ready"}).
			AddRow("replacement-group", "replacement-owner", int64(5), true))
	if err := store.IssueRenewRevoke(context.Background(), revoked); !errors.Is(err, ErrProbeReadinessStaleOwner) {
		t.Fatalf("err=%v, want stale owner", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessAuthorityDuplicateRevokeIsIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	revoked := alertconfig.ProbePipelineReadinessReceipt{
		PipelineID:    alertconfig.ProbeOperationPipelineID,
		ConsumerRole:  alertconfig.ProbeCommandDeliveryConsumer,
		ConsumerGroup: "ingest-probe-control", OwnerID: "publisher-a:member-a", OwnerEpoch: 9,
		State: alertconfig.ProbePipelineRevoked, ObservedAt: now,
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	mock.ExpectExec("UPDATE probe_pipeline_readiness_epochs").
		WithArgs(revoked.PipelineID, string(revoked.ConsumerRole), revoked.ConsumerGroup,
			revoked.OwnerID, revoked.OwnerEpoch, revoked.ObservedAt).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT consumer_group,owner_id,owner_epoch,ready").
		WithArgs(revoked.PipelineID, string(revoked.ConsumerRole)).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_group", "owner_id", "owner_epoch", "ready"}).
			AddRow(revoked.ConsumerGroup, revoked.OwnerID, revoked.OwnerEpoch, false))
	if err := store.IssueRenewRevoke(context.Background(), revoked); err != nil {
		t.Fatalf("duplicate revoke err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
