package api

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var modelFeedbackSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ModelFeedbackProducerReadiness struct {
	Topic           string
	ConsumerGroup   string
	CandidateSHA256 string
	ContractSHA256  string
}

// VerifyModelFeedbackProducerReadiness requires a consumer receipt tied to an
// accepted broker record. A synthetic READY row without the matching durable
// projection receipt cannot authorize producer startup.
func VerifyModelFeedbackProducerReadiness(
	ctx context.Context,
	db *sql.DB,
	options ModelFeedbackProducerReadiness,
) error {
	if db == nil {
		return fmt.Errorf("model feedback readiness database is unavailable")
	}
	options.Topic = strings.TrimSpace(options.Topic)
	options.ConsumerGroup = strings.TrimSpace(options.ConsumerGroup)
	options.CandidateSHA256 = strings.ToLower(strings.TrimSpace(options.CandidateSHA256))
	options.ContractSHA256 = strings.ToLower(strings.TrimSpace(options.ContractSHA256))
	if options.Topic != modelFeedbackRevisionEventType || options.ConsumerGroup == "" ||
		!modelFeedbackSHA256Pattern.MatchString(options.CandidateSHA256) ||
		options.CandidateSHA256 == strings.Repeat("0", 64) ||
		!modelFeedbackSHA256Pattern.MatchString(options.ContractSHA256) {
		return fmt.Errorf("approved non-zero candidate, contract, group and model.feedback.v1 topic are required")
	}
	var state string
	err := db.QueryRowContext(ctx, `
		SELECT r.state
		FROM model_feedback_consumer_readiness_receipt r
		JOIN model_feedback_revision_receipt e
		  ON e.event_id=r.event_id AND e.kafka_topic=r.kafka_topic
		 AND e.kafka_partition=r.kafka_partition AND e.kafka_offset=r.kafka_offset
		WHERE r.kafka_topic=$1 AND r.consumer_group=$2
		  AND r.candidate_sha256=$3 AND r.contract_sha256=$4
		  AND r.state='READY' AND e.outcome='ACCEPTED'`,
		options.Topic, options.ConsumerGroup, options.CandidateSHA256, options.ContractSHA256,
	).Scan(&state)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no matching consumer broker receipt authorizes model feedback producer startup")
		}
		return fmt.Errorf("verify model feedback consumer readiness: %w", err)
	}
	if state != "READY" {
		return fmt.Errorf("model feedback consumer is not READY")
	}
	return nil
}
