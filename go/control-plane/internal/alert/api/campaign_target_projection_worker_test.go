package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type recordingCampaignProjectionTarget struct {
	name       string
	projection []byte
	applyErr   error
	applyCalls int
}

func (target *recordingCampaignProjectionTarget) Name() string { return target.name }

func (target *recordingCampaignProjectionTarget) Projection(CampaignProjectionEvent) ([]byte, error) {
	return append([]byte(nil), target.projection...), nil
}

func (target *recordingCampaignProjectionTarget) Apply(
	_ context.Context,
	_ CampaignProjectionEvent,
	_ []byte,
) error {
	target.applyCalls++
	return target.applyErr
}

func campaignProjectionTestTargets() []*recordingCampaignProjectionTarget {
	return []*recordingCampaignProjectionTarget{
		{name: campaignProjectionClickHouse, projection: []byte(`{"target":"clickhouse"}`)},
		{name: campaignProjectionOpenSearch, projection: []byte(`{"target":"opensearch"}`)},
		{name: campaignProjectionNebula, projection: []byte(`{"target":"nebulagraph"}`)},
	}
}

func campaignProjectionTargetInterfaces(targets []*recordingCampaignProjectionTarget) []CampaignProjectionTarget {
	result := make([]CampaignProjectionTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, target)
	}
	return result
}

func newCampaignProjectionWorkerForTest(
	t *testing.T,
	targets []*recordingCampaignProjectionTarget,
	maxAttempts int,
) (*CampaignTargetProjectionWorker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	worker, err := NewCampaignTargetProjectionWorker(db, campaignProjectionTargetInterfaces(targets), CampaignTargetProjectionWorkerConfig{
		WorkerID:    "campaign-projector-test",
		Lease:       30 * time.Second,
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return worker, mock
}

func campaignProjectionLeaseColumns() []string {
	return []string{
		"stream", "event_id", "tenant_id", "aggregate_id", "campaign_id", "relation_id",
		"alert_id", "event_type", "schema_version", "aggregate_revision", "relation_revision",
		"partition_key", "trace_id", "payload", "target_status", "attempt_count",
		"received_at",
	}
}

func validCampaignAggregateProjectionPayload() string {
	return `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"traffic.campaign.v2.StatusChanged","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":3,"campaign_id":"campaign-a","schema_version":2,"partition_key":"tenant-a:campaign-a","trace_id":"trace-campaign-3"}`
}

func expectCampaignProjectionLease(
	mock sqlmock.Sqlmock,
	payload string,
	targetStatus string,
	attemptCount int,
) {
	mock.ExpectQuery(`(?s)WITH candidate AS .*UPDATE campaign_event_projection_inbox inbox`).
		WithArgs("campaign-projector-test", int64(30)).
		WillReturnRows(sqlmock.NewRows(campaignProjectionLeaseColumns()).AddRow(
			campaignAggregateStream,
			"11111111-1111-4111-8111-111111111111",
			"tenant-a",
			"campaign-a",
			"campaign-a",
			"",
			"",
			"traffic.campaign.v2.StatusChanged",
			2,
			int64(3),
			int64(0),
			"tenant-a:campaign-a",
			"trace-campaign-3",
			payload,
			targetStatus,
			attemptCount,
			time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC),
		))
}

func TestNewCampaignTargetProjectionWorkerRequiresAllTargets(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = NewCampaignTargetProjectionWorker(db, []CampaignProjectionTarget{
		&recordingCampaignProjectionTarget{name: campaignProjectionClickHouse},
		&recordingCampaignProjectionTarget{name: campaignProjectionOpenSearch},
	}, CampaignTargetProjectionWorkerConfig{WorkerID: "worker-a"})
	if err == nil {
		t.Fatal("worker must fail closed when one target is missing")
	}
}

func TestCampaignTargetProjectionWorkerMalformedPayloadIsRetriedWithoutStrandingLease(t *testing.T) {
	targets := campaignProjectionTestTargets()
	worker, mock := newCampaignProjectionWorkerForTest(t, targets, 8)
	expectCampaignProjectionLease(mock, `{}`, `{"clickhouse":"pending","opensearch":"pending","nebulagraph":"pending"}`, 1)
	for range campaignProjectionTargetOrder {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE campaign_event_projection_inbox")).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(`(?s)UPDATE campaign_event_projection_inbox.*SET projection_status=CASE`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := worker.ProjectNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v, want a recorded validation failure", found, err)
	}
	for _, target := range targets {
		if target.applyCalls != 0 {
			t.Fatalf("invalid durable payload reached %s target", target.name)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignTargetProjectionWorkerRetriesOnlyFailedTarget(t *testing.T) {
	targets := campaignProjectionTestTargets()
	targets[2].applyErr = errors.New("nebula timeout")
	worker, mock := newCampaignProjectionWorkerForTest(t, targets, 8)
	expectCampaignProjectionLease(
		mock,
		validCampaignAggregateProjectionPayload(),
		`{"clickhouse":"applied","opensearch":"applied","nebulagraph":"pending"}`,
		2,
	)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT projection_version,event_id::text,projection_sha256")).
		WithArgs("tenant-a", "campaign:campaign-a", campaignProjectionNebula).
		WillReturnRows(sqlmock.NewRows([]string{"projection_version", "event_id", "projection_sha256"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE campaign_event_projection_inbox")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE campaign_event_projection_inbox.*SET projection_status=CASE`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := worker.ProjectNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v, want retryable target failure", found, err)
	}
	if targets[0].applyCalls != 0 || targets[1].applyCalls != 0 || targets[2].applyCalls != 1 {
		t.Fatalf("unexpected apply calls: ch=%d os=%d nebula=%d", targets[0].applyCalls, targets[1].applyCalls, targets[2].applyCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignTargetProjectionWorkerSameWatermarkSkipsDuplicateExternalApply(t *testing.T) {
	targets := campaignProjectionTestTargets()
	worker, mock := newCampaignProjectionWorkerForTest(t, targets, 8)
	expectCampaignProjectionLease(
		mock,
		validCampaignAggregateProjectionPayload(),
		`{"clickhouse":"pending","opensearch":"applied","nebulagraph":"applied"}`,
		2,
	)
	hash := sha256.Sum256(targets[0].projection)
	projectionSHA := hex.EncodeToString(hash[:])
	watermarkRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"projection_version", "event_id", "projection_sha256"}).
			AddRow(int64(3), "11111111-1111-4111-8111-111111111111", projectionSHA)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT projection_version,event_id::text,projection_sha256")).
		WithArgs("tenant-a", "campaign:campaign-a", campaignProjectionClickHouse).
		WillReturnRows(watermarkRows())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT projection_version,event_id::text,projection_sha256")).
		WithArgs("tenant-a", "campaign:campaign-a", campaignProjectionClickHouse).
		WillReturnRows(watermarkRows())
	mock.ExpectExec(regexp.QuoteMeta("UPDATE campaign_event_projection_inbox")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(`(?s)UPDATE campaign_event_projection_inbox.*SET projection_status=CASE`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := worker.ProjectNext(context.Background())
	if !found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if targets[0].applyCalls != 0 {
		t.Fatal("duplicate watermark must suppress the external target call")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignTargetProjectionWorkerMalformedStatusReachesDeadState(t *testing.T) {
	targets := campaignProjectionTestTargets()
	worker, mock := newCampaignProjectionWorkerForTest(t, targets, 1)
	expectCampaignProjectionLease(mock, validCampaignAggregateProjectionPayload(), `[]`, 1)
	for range campaignProjectionTargetOrder {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE campaign_event_projection_inbox")).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(`(?s)UPDATE campaign_event_projection_inbox.*SET projection_status=CASE`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := worker.ProjectNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v, want terminal invalid-status failure", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
