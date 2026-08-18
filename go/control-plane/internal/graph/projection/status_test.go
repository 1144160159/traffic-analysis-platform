package projection

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProjectionStatusExposesConsumerFirstPartialState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewStatusRepository(db, true, false, 5*time.Minute)
	mockStatusQueries(mock, 0, 0, 0, nil, nil, 0, 0, 0)
	status, err := repository.Load(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "partial" || status.Complete || !status.ConsumerEnabled || status.WorkerEnabled {
		t.Fatalf("unexpected consumer-first status: %+v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionStatusFailsVisibleForDeadEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewStatusRepository(db, true, true, 5*time.Minute)
	now := time.Now().UTC()
	mockStatusQueries(mock, 0, 4, 1, nil, now, 2, 2, 1)
	status, err := repository.Load(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "failed" || status.Complete || status.DeadCount != 1 || status.RevokedCount != 1 {
		t.Fatalf("unexpected failed status: %+v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionStatusMarksOldPendingWorkStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewStatusRepository(db, true, true, 5*time.Minute)
	repository.clock = func() time.Time { return time.Unix(2000, 0).UTC() }
	oldest := time.Unix(1000, 0).UTC()
	mockStatusQueries(mock, 1, 0, 0, oldest, nil, 0, 0, 0)
	status, err := repository.Load(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stale" || status.Complete {
		t.Fatalf("unexpected stale status: %+v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func mockStatusQueries(
	mock sqlmock.Sqlmock,
	pending, applied, dead int64,
	oldestPending interface{},
	lastApplied interface{},
	entities, relations, revoked int64,
) {
	mock.ExpectQuery("SELECT count\\(\\*\\) FILTER .*projection_state='PENDING'").
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"pending", "applied", "dead", "oldest", "last"}).
			AddRow(pending, applied, dead, oldestPending, lastApplied))
	mock.ExpectQuery("SELECT count\\(\\*\\) FILTER .*projection_kind='entity'").
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"entities", "relations", "revoked"}).
			AddRow(entities, relations, revoked))
	mock.ExpectQuery("SELECT source_partition,source_offset,event_id,projection_sha256").
		WithArgs("tenant-a", Topic).
		WillReturnRows(sqlmock.NewRows([]string{
			"source_partition", "source_offset", "event_id", "projection_sha256", "source_timestamp_ms", "projected_at",
		}))
}
