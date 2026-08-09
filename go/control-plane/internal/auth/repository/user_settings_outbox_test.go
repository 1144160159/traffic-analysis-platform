package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type userSettingsProducerStub struct {
	event *pb.UserEvent
	err   error
}

func (s *userSettingsProducerStub) SendProto(_ context.Context, _ string, message proto.Message, _ ...commonkafka.MessageHeader) error {
	s.event = message.(*pb.UserEvent)
	return s.err
}

func TestUserSettingsOutboxMarksPublishedOnlyAfterKafkaAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	producer := &userSettingsProducerStub{}
	worker := NewUserSettingsOutboxWorker(db, producer, zap.NewNop())
	worker.workerID = "auth-worker-1"
	payload := `{"event_id":"00000000-0000-0000-0000-000000000999","tenant_id":"tenant-a","user_id":"00000000-0000-0000-0000-000000000123","username":"analyst-a","event_type":"settings_update","resource":"user_settings/display","action":"update","result":"success","timestamp":` + fmt.Sprint(time.Now().UnixMilli()) + `}`
	mock.ExpectQuery("WITH candidates AS").WithArgs(10, "auth-worker-1", userSettingsOutboxMaxAttempts).
		WillReturnRows(sqlmock.NewRows([]string{"outbox_id", "event_id", "tenant_id", "user_id", "category", "aggregate_version", "event_type", "schema_version", "partition_key", "payload"}).
			AddRow(int64(7), "00000000-0000-0000-0000-000000000999", "tenant-a", "00000000-0000-0000-0000-000000000123", "display", int64(1), "traffic.user.settings.v1.SettingsUpdated", 1, "tenant-a:00000000-0000-0000-0000-000000000123", payload))
	mock.ExpectExec("UPDATE user_settings_outbox").WithArgs(int64(7), "auth-worker-1").WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := worker.Drain(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || producer.event == nil || producer.event.EventId != "00000000-0000-0000-0000-000000000999" {
		t.Fatalf("unexpected publish result: processed=%d event=%#v", processed, producer.event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserCommandOutboxMarksPublishedOnlyAfterKafkaAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	producer := &userSettingsProducerStub{}
	worker := NewUserSettingsOutboxWorker(db, producer, zap.NewNop())
	worker.workerID = "auth-worker-command"
	eventID := "00000000-0000-0000-0000-000000000998"
	userID := "00000000-0000-0000-0000-000000000123"
	payload := `{"event_id":"` + eventID + `","tenant_id":"tenant-a","user_id":"` + userID + `","username":"analyst-a","event_type":"profile_update","resource":"user","action":"auth-user-profile-update","result":"success","timestamp":` + fmt.Sprint(time.Now().UnixMilli()) + `}`
	mock.ExpectQuery("WITH candidates AS").WithArgs(10, "auth-worker-command", userSettingsOutboxMaxAttempts).
		WillReturnRows(sqlmock.NewRows([]string{"outbox_id", "event_id", "tenant_id", "user_id", "aggregate_version", "event_type", "schema_version", "partition_key", "payload"}).
			AddRow(int64(8), eventID, "tenant-a", userID, int64(2), "traffic.user.command.v1.profile_update", 1, "tenant-a:"+userID, payload))
	mock.ExpectExec("UPDATE user_command_outbox").WithArgs(int64(8), "auth-worker-command").WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := worker.DrainUserCommands(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || producer.event == nil || producer.event.EventId != eventID {
		t.Fatalf("unexpected publish result: processed=%d event=%#v", processed, producer.event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserCommandOutboxBrokerFailureNeverMarksPublished(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	producer := &userSettingsProducerStub{err: fmt.Errorf("broker unavailable")}
	worker := NewUserSettingsOutboxWorker(db, producer, zap.NewNop())
	worker.workerID = "auth-worker-failure"
	eventID := "00000000-0000-0000-0000-000000000997"
	userID := "00000000-0000-0000-0000-000000000123"
	payload := `{"event_id":"` + eventID + `","tenant_id":"tenant-a","user_id":"` + userID + `","event_type":"password_change","resource":"user","action":"auth-user-password-change","result":"success","timestamp":` + fmt.Sprint(time.Now().UnixMilli()) + `}`
	mock.ExpectQuery("WITH candidates AS").WithArgs(10, "auth-worker-failure", userSettingsOutboxMaxAttempts).
		WillReturnRows(sqlmock.NewRows([]string{"outbox_id", "event_id", "tenant_id", "user_id", "aggregate_version", "event_type", "schema_version", "partition_key", "payload"}).
			AddRow(int64(9), eventID, "tenant-a", userID, int64(3), "traffic.user.command.v1.password_change", 1, "tenant-a:"+userID, payload))
	mock.ExpectExec("UPDATE user_command_outbox").WithArgs(int64(9), "broker unavailable", "auth-worker-failure", userSettingsOutboxMaxAttempts).
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := worker.DrainUserCommands(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("expected no successful publish, got %d", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
