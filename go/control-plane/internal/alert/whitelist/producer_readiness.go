package whitelist

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var whitelistSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProducerReadiness struct {
	Topic           string
	ConsumerGroup   string
	CandidateSHA256 string
	ContractSHA256  string
}

// VerifyProducerReadiness admits the whitelist outbox producer only after the
// exact rule-manager candidate has consumed a broker record, committed the
// corresponding rule projection and acknowledged that same entry version.
// A hand-written READY row cannot satisfy the joins below.
func VerifyProducerReadiness(ctx context.Context, db *sql.DB, options ProducerReadiness) error {
	if db == nil {
		return fmt.Errorf("whitelist producer readiness database is unavailable")
	}
	options.Topic = strings.TrimSpace(options.Topic)
	options.ConsumerGroup = strings.TrimSpace(options.ConsumerGroup)
	options.CandidateSHA256 = strings.ToLower(strings.TrimSpace(options.CandidateSHA256))
	options.ContractSHA256 = strings.ToLower(strings.TrimSpace(options.ContractSHA256))
	if options.Topic != WhitelistEventTopicV2 || options.ConsumerGroup == "" ||
		!validWhitelistSHA256(options.CandidateSHA256, true) ||
		!validWhitelistSHA256(options.ContractSHA256, false) {
		return fmt.Errorf("approved non-zero candidate, contract, group and whitelist.events.v2 topic are required")
	}

	var state string
	err := db.QueryRowContext(ctx, `
		SELECT receipt.state
		FROM whitelist_consumer_readiness_receipt receipt
		JOIN whitelist_rule_projection projection
		  ON projection.source_event_id=receipt.event_id
		 AND projection.kafka_partition=receipt.kafka_partition
		 AND projection.kafka_offset=receipt.kafka_offset
		JOIN whitelist_rule_effects effect
		  ON effect.event_id=projection.source_event_id
		 AND effect.tenant_id=projection.tenant_id
		 AND effect.entry_id=projection.entry_id
		 AND effect.entry_version=projection.entry_version
		 AND effect.status='applied'
		 AND effect.rule_revision=projection.rule_revision
		WHERE receipt.kafka_topic=$1 AND receipt.consumer_group=$2
		  AND receipt.candidate_sha256=$3 AND receipt.contract_sha256=$4
		  AND receipt.state='READY'`,
		options.Topic, options.ConsumerGroup, options.CandidateSHA256, options.ContractSHA256,
	).Scan(&state)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no matching consumer broker projection receipt authorizes whitelist producer startup")
		}
		return fmt.Errorf("verify whitelist consumer readiness: %w", err)
	}
	if state != "READY" {
		return fmt.Errorf("whitelist rule consumer is not READY")
	}
	return nil
}

func validWhitelistSHA256(value string, rejectZero bool) bool {
	if !whitelistSHA256Pattern.MatchString(value) {
		return false
	}
	return !rejectZero || strings.Trim(value, "0") != ""
}
