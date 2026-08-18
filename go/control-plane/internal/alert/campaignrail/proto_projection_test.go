package campaignrail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/protobuf/proto"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestProtoProjectionStoreCommitsInboxAndCurrentAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := NewProtoProjectionStore(db)
	input := validProtoProjectionInput(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_proto_projection_inbox_v1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_proto_projection_current_v1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE campaign_proto_projection_inbox_v1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.ApplyProtoProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProtoProjectionStoreRejectsEventIdentityCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := NewProtoProjectionStore(db)
	input := validProtoProjectionInput(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_proto_projection_inbox_v1")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM campaign_proto_projection_inbox_v1")).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()
	if err := store.ApplyProtoProjection(context.Background(), input); err == nil || !strings.Contains(err.Error(), ErrProtoIdentityCollision.Error()) {
		t.Fatalf("identity collision was not rejected: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProtoProjectionInputRejectsUnknownTenantAndSHADrift(t *testing.T) {
	input := validProtoProjectionInput(t)
	input.Campaign.TenantId = "unknown"
	input.Campaign.Header.TenantId = "unknown"
	if err := ValidateProtoProjectionInput(input); err == nil {
		t.Fatal("unknown tenant was accepted")
	}
	input = validProtoProjectionInput(t)
	input.PayloadSHA256 = strings.Repeat("b", 64)
	if err := ValidateProtoProjectionInput(input); err == nil {
		t.Fatal("payload SHA drift was accepted")
	}
}

func validProtoProjectionInput(t *testing.T) ProtoProjectionInput {
	t.Helper()
	campaign := &trafficv1.Campaign{
		TenantId: "tenant-a", CampaignId: "campaign-1", TsStart: 1, TsEnd: 2,
		Entities: []string{"ip:a"}, Alerts: []string{"alert-1"}, Score: 0.8,
		EventId: "11111111-1111-4111-8111-111111111111", CampaignType: "scan_exploit",
		Header: &trafficv1.EventHeader{TenantId: "tenant-a", EventId: "11111111-1111-4111-8111-111111111111"},
	}
	payload, err := proto.Marshal(campaign)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return ProtoProjectionInput{
		Campaign: campaign, Payload: payload, PayloadSHA256: hex.EncodeToString(digest[:]),
		KafkaTopic: ProtoTopic, KafkaPartition: 0, KafkaOffset: 1,
		ReceivedAt: time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC),
	}
}
