package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/protobuf/proto"
)

type recordingTarget struct {
	applied bool
	err     error
}

func (target *recordingTarget) Ready(context.Context) error { return nil }
func (target *recordingTarget) Apply(context.Context, *trafficv1.GraphProjectionEvent) error {
	target.applied = true
	return target.err
}

func TestWorkerClaimsOnlyPartitionHead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	worker, _ := NewWorker(db, &recordingTarget{}, WorkerConfig{WorkerID: "worker-1"})
	mock.ExpectQuery(`(?s)NOT EXISTS .*prior.source_offset<candidate.source_offset.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows(workerClaimColumns()))
	if err := worker.ProcessOne(context.Background()); !errors.Is(err, ErrNoProjectionWork) {
		t.Fatalf("expected no work, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerAdvancesWatermarkOnlyAfterTargetAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	target := &recordingTarget{}
	worker, _ := NewWorker(db, target, WorkerConfig{WorkerID: "worker-1"})
	event := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "event", "")
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	payloadSum := sha256.Sum256(payload)
	metadata, _ := metadataOf(event)
	claimToken := "00000000-0000-4000-8000-000000000111"
	mock.ExpectQuery(`(?s)WITH candidate AS .*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows(workerClaimColumns()).AddRow(
			event.GetHeader().GetEventId(), payload, hex.EncodeToString(payloadSum[:]),
			Topic, 2, int64(10), int64(1700000000000), 1, claimToken,
			metadata.tenantID, metadata.kind, metadata.projectionID, metadata.projectionSHA256,
			metadata.sourceEventID, metadata.sourceSystem, metadata.sourceSHA256,
			int64(metadata.aggregateVersion), metadata.revoked,
		))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT projection_state,claim_token::text").
		WithArgs(event.GetHeader().GetEventId()).
		WillReturnRows(sqlmock.NewRows([]string{"projection_state", "claim_token", "claimed_by"}).
			AddRow("PENDING", claimToken, "worker-1"))
	mock.ExpectQuery("SELECT aggregate_version,projection_sha256").
		WillReturnRows(sqlmock.NewRows([]string{"aggregate_version", "projection_sha256"}))
	mock.ExpectExec("INSERT INTO graph_projection_current_v1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE graph_projection_inbox_v1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO graph_projection_watermarks_v1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatalf("process graph projection: %v", err)
	}
	if !target.applied {
		t.Fatal("target was not acknowledged before PostgreSQL completion")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func workerClaimColumns() []string {
	return []string{
		"event_id", "raw_payload", "payload_sha256", "source_topic", "source_partition",
		"source_offset", "source_timestamp_ms", "attempts", "claim_token", "tenant_id",
		"projection_kind", "projection_id", "projection_sha256", "source_event_id",
		"source_system", "source_sha256", "aggregate_version", "revoked",
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	if retryBackoff(1) != time.Second || retryBackoff(100) > 5*time.Minute {
		t.Fatalf("unexpected retry backoff bounds")
	}
}
