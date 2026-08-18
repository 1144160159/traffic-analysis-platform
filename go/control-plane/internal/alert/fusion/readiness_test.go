package fusion

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestReadinessStoreRecordsAssignedGenerationAndGatesCandidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewReadinessStore(db, SourceSyncGroup)
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Unix(1000, 0).UTC()
	candidate := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT owner_id,owner_epoch,observed_at`).
		WithArgs(ProjectionPipelineID, SourceSyncGroup).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO fusion_projection_readiness_history`).
		WithArgs(sqlmock.AnyArg(), ProjectionPipelineID, SourceSyncGroup, SourceSyncTopic, candidate,
			"owner-a:member-a", int64(9), int32(3), "READY", sqlmock.AnyArg(), observedAt, observedAt.Add(time.Minute)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO fusion_projection_readiness_current`).
		WithArgs(ProjectionPipelineID, SourceSyncGroup, SourceSyncTopic, candidate, sqlmock.AnyArg(),
			"owner-a:member-a", int64(9), int32(3), "READY", sqlmock.AnyArg(), observedAt, observedAt.Add(time.Minute)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	err = store.RecordLifecycle(context.Background(), commonkafka.GroupLifecycleReceipt{
		Topic: SourceSyncTopic, GroupID: SourceSyncGroup, MemberID: "member-a", GenerationID: 3,
		OwnerID: "owner-a", OwnerEpoch: 9, State: commonkafka.GroupLifecycleReady,
		Assignments: []commonkafka.GroupPartitionAssignment{{Topic: SourceSyncTopic, Partition: 0, Offset: 4}},
		ObservedAt:  observedAt,
	}, candidate, time.Minute)
	if err != nil {
		t.Fatalf("record ready lifecycle: %v", err)
	}
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return observedAt.Add(30 * time.Second) }
	mock.ExpectQuery(`SELECT observed_topic,candidate_sha256,state,lease_expires_at`).
		WithArgs(ProjectionPipelineID, SourceSyncGroup).
		WillReturnRows(sqlmock.NewRows([]string{"observed_topic", "candidate_sha256", "state", "lease_expires_at"}).
			AddRow(SourceSyncTopic, candidate, "READY", observedAt.Add(time.Minute)))
	if err := store.AssertReadyTx(context.Background(), tx, candidate); err != nil {
		t.Fatalf("expected current candidate to pass gate: %v", err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessGateRejectsExpiredLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := NewReadinessStore(db, SourceSyncGroup)
	now := time.Unix(2000, 0).UTC()
	store.now = func() time.Time { return now }
	candidate := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mock.ExpectBegin()
	tx, _ := db.BeginTx(context.Background(), nil)
	mock.ExpectQuery(`SELECT observed_topic,candidate_sha256,state,lease_expires_at`).
		WithArgs(ProjectionPipelineID, SourceSyncGroup).
		WillReturnRows(sqlmock.NewRows([]string{"observed_topic", "candidate_sha256", "state", "lease_expires_at"}).
			AddRow(SourceSyncTopic, candidate, "READY", now.Add(-time.Second)))
	if err := store.AssertReadyTx(context.Background(), tx, candidate); !errors.Is(err, ErrProjectionNotReady) {
		t.Fatalf("expected expired readiness rejection, got %v", err)
	}
	mock.ExpectRollback()
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
