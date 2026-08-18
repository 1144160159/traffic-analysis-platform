package whitelist

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestVerifyProducerReadinessRejectsUnapprovedBindings(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := ProducerReadiness{
		Topic: WhitelistEventTopicV2, ConsumerGroup: "rule-manager-whitelist-rule-effect-v2",
		CandidateSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64),
	}
	for _, mutate := range []func(*ProducerReadiness){
		func(value *ProducerReadiness) { value.Topic = "wrong.v1" },
		func(value *ProducerReadiness) { value.ConsumerGroup = "" },
		func(value *ProducerReadiness) { value.CandidateSHA256 = strings.Repeat("0", 64) },
		func(value *ProducerReadiness) { value.ContractSHA256 = "not-a-sha" },
	} {
		options := base
		mutate(&options)
		if err := VerifyProducerReadiness(context.Background(), db, options); err == nil {
			t.Fatalf("expected invalid readiness binding to fail: %+v", options)
		}
	}
}

func TestVerifyProducerReadinessRequiresJoinedBrokerProjectionReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	options := ProducerReadiness{
		Topic: WhitelistEventTopicV2, ConsumerGroup: "rule-manager-whitelist-rule-effect-v2",
		CandidateSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64),
	}
	query := regexp.QuoteMeta("SELECT receipt.state") + ".*" + regexp.QuoteMeta("FROM whitelist_consumer_readiness_receipt receipt")
	mock.ExpectQuery(query).WithArgs(options.Topic, options.ConsumerGroup, options.CandidateSHA256, options.ContractSHA256).
		WillReturnError(sql.ErrNoRows)
	if err := VerifyProducerReadiness(context.Background(), db, options); err == nil {
		t.Fatal("synthetic or missing readiness without a joined projection must fail")
	}
	mock.ExpectQuery(query).WithArgs(options.Topic, options.ConsumerGroup, options.CandidateSHA256, options.ContractSHA256).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("READY"))
	if err := VerifyProducerReadiness(context.Background(), db, options); err != nil {
		t.Fatalf("joined broker projection receipt should authorize producer: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
