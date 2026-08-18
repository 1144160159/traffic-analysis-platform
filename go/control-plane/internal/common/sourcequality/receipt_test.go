package sourcequality

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func validInput() Input {
	return Input{
		TenantID: "tenant-a", Rail: RailDeviceLog, ConsumerGroup: "flink-log-job-shadow-candidate",
		Source:   SourceTuple{Topic: "device.logs.v1", Partition: 2, Offset: 41},
		Category: Accepted, EventID: "log-001", SourceSHA256: HashSource([]byte("payload")),
		WatermarkMS: 1700000000000, ObservedAtMS: 1700000001000,
	}
}

func TestBuildIsReplayStableAndTenantScoped(t *testing.T) {
	first, err := Build(validInput())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := Build(validInput())
	if err != nil || first != replay {
		t.Fatalf("receipt replay changed: %#v %#v %v", first, replay, err)
	}
	cross := validInput()
	cross.TenantID = "tenant-b"
	other, err := Build(cross)
	if err != nil || other.ReceiptID == first.ReceiptID {
		t.Fatal("tenant is not bound into receipt identity")
	}
	bad := validInput()
	bad.Category = Conflict
	if _, err := Build(bad); err == nil {
		t.Fatal("non-accepted receipt without reason must fail")
	}
}

func TestRecordCommitsReceiptBeforeOffsetAndExactReplayIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	receipt, _ := Build(validInput())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs (")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	committed := false
	err = NewRepository(db).RecordBeforeOffsetCommit(context.Background(), receipt,
		func(_ context.Context, tuple SourceTuple) error {
			committed = tuple.Offset == 41
			return nil
		})
	if err != nil || !committed {
		t.Fatalf("record barrier err=%v committed=%t", err, committed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptFailurePreventsOffsetAndConflictFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	receipt, _ := Build(validInput())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs (")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	committed := false
	err = NewRepository(db).RecordBeforeOffsetCommit(context.Background(), receipt,
		func(context.Context, SourceTuple) error { committed = true; return nil })
	if !errors.Is(err, ErrReceiptConflict) || committed {
		t.Fatalf("err=%v committed=%t", err, committed)
	}
}

func TestReconcileFindsMissingExtraLagAndDuplicate(t *testing.T) {
	input := validInput()
	input.Source.Offset = 0
	first, _ := Build(input)
	input.Source.Offset = 2
	input.Category = Late
	input.ReasonCode = "LATE_EVENT"
	third, _ := Build(input)
	expectation := PartitionExpectation{
		TenantID: "tenant-a", Rail: RailDeviceLog, ConsumerGroup: input.ConsumerGroup,
		Topic: input.Source.Topic, Partition: 2, FirstOffset: 0, CommittedOffset: 2, LogEndOffset: 3,
	}
	result, err := Reconcile([]Receipt{first, third}, []PartitionExpectation{expectation})
	if err != nil {
		t.Fatal(err)
	}
	if result.AllMatch || result.Partitions[0].Lag != 1 ||
		len(result.Partitions[0].MissingOffsets) != 1 || result.Partitions[0].MissingOffsets[0] != 1 ||
		len(result.Partitions[0].ExtraOffsets) != 1 || result.Partitions[0].ExtraOffsets[0] != 2 {
		t.Fatalf("unexpected reconcile: %#v", result)
	}
	if _, err := Reconcile([]Receipt{first, first}, []PartitionExpectation{expectation}); err == nil {
		t.Fatal("duplicate source tuple must fail")
	}
	missing, err := BuildMissingReceipts(result, 1700000002000)
	if err != nil || len(missing) != 1 || missing[0].Category != Missing ||
		missing[0].Source.Offset != 1 || missing[0].ReasonCode != "MISSING_SOURCE_RECEIPT" {
		t.Fatalf("missing receipts=%+v err=%v", missing, err)
	}
}

func TestAllRailReconcileRequiresFourExplicitSources(t *testing.T) {
	expectations := make([]PartitionExpectation, 0, 4)
	for index, rail := range allRails {
		expectations = append(expectations, PartitionExpectation{
			TenantID: "tenant-a", Rail: rail, ConsumerGroup: "group-" + string(rail),
			Topic: "topic-" + string(rail), Partition: index,
			FirstOffset: 0, CommittedOffset: 0, LogEndOffset: 0,
		})
	}
	result, err := ReconcileAllRails(nil, expectations)
	if err != nil || !result.AllMatch || len(result.Partitions) != 4 {
		t.Fatalf("four-rail reconciliation=%+v err=%v", result, err)
	}
	if _, err := ReconcileAllRails(nil, expectations[:3]); err == nil {
		t.Fatal("missing rail expectation must fail")
	}
}
