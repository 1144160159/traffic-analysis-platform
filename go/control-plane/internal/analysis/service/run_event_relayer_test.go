package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

type fakeEventPublisher struct {
	published map[string][]byte
	err       error
}

func (f *fakeEventPublisher) Publish(_ context.Context, topic, key string, payload []byte) error {
	if f.err != nil {
		return f.err
	}
	if f.published == nil {
		f.published = map[string][]byte{}
	}
	f.published[topic+"/"+key] = payload
	return nil
}

func TestRunEventRelayerPublishesSubscription(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepo(db)
	pub := &fakeEventPublisher{}
	relayer := NewRunEventRelayer(repo, pub, nil)

	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"})) // plan:空
	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"}).
			AddRow(1, "ev-1", "analysis.run.events.v1", "run-1", []byte(`{"tenant_id":"default","execution_spec_sha256":"spec-1"}`)))
	mock.ExpectQuery(`SELECT`). // GetRun 行
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "run_id", "task_id", "execution_spec_sha256", "state", "completeness", "integrity_state", "finding_conclusion", "risk_severity", "window_start_ms", "window_end_ms", "revision", "created_at", "finalized_at", "report_state"}).
			AddRow("default", "run-1", "task-1", "spec-1", "ACCEPTED", "UNKNOWN", "UNVERIFIED", "NOT_EVALUATED", "UNKNOWN", 1700000000000, 1700000600000, 1, time.Now(), nil, "NOT_REQUESTED"))
	mock.ExpectExec(`UPDATE analysis_outbox SET state='PUBLISHED'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"})) // report:空

	published, failed, err := relayer.RelayOnce(context.Background(), 20)
	if err != nil || published != 1 || failed != 0 {
		t.Fatalf("relay: pub=%d failed=%d err=%v", published, failed, err)
	}
	payload, ok := pub.published["analysis.run.events.v1/run-1"]
	if !ok {
		t.Fatalf("subscription not published")
	}
	var sub struct {
		TenantID           string `json:"tenant_id"`
		RunID              string `json:"run_id"`
		State              string `json:"state"`
		Revision           int64  `json:"revision"`
		ExecutionSpecSHA   string `json:"execution_spec_sha256"`
		WindowStartMs      int64  `json:"window_start_ms"`
	}
	if err := json.Unmarshal(payload, &sub); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if sub.RunID != "run-1" || sub.State != "ACTIVE" || sub.Revision != 1 || sub.ExecutionSpecSHA != "spec-1" || sub.WindowStartMs != 1700000000000 {
		t.Fatalf("subscription mismatch: %+v", sub)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunEventRelayerPublishFailureMarksFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepo(db)
	pub := &fakeEventPublisher{err: errors.New("broker down")}
	relayer := NewRunEventRelayer(repo, pub, nil)

	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"})) // plan:空
	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"}).
			AddRow(1, "ev-1", "analysis.run.events.v1", "run-1", []byte(`{"tenant_id":"default"}`)))
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "run_id", "task_id", "execution_spec_sha256", "state", "completeness", "integrity_state", "finding_conclusion", "risk_severity", "window_start_ms", "window_end_ms", "revision", "created_at", "finalized_at", "report_state"}).
			AddRow("default", "run-1", "task-1", "spec-1", "ACCEPTED", "UNKNOWN", "UNVERIFIED", "NOT_EVALUATED", "UNKNOWN", 1700000000000, 1700000600000, 1, time.Now(), nil, "NOT_REQUESTED"))
	mock.ExpectExec(`UPDATE analysis_outbox\s+SET attempts=attempts\+1`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, event_id, topic, key, payload FROM analysis_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "topic", "key", "payload"})) // report:空

	published, failed, err := relayer.RelayOnce(context.Background(), 20)
	if err != nil || published != 0 || failed != 1 {
		t.Fatalf("relay failure path: pub=%d failed=%d err=%v", published, failed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunEventRelayerNilPublisherFailsClosed(t *testing.T) {
	relayer := NewRunEventRelayer(repository.NewRepo(nil), nil, nil)
	if _, _, err := relayer.RelayOnce(context.Background(), 1); err == nil {
		t.Fatalf("nil publisher must fail closed")
	}
}
