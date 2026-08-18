package kafka

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	segmentkafka "github.com/segmentio/kafka-go"
)

var ErrDLQAcknowledgementConflict = errors.New("Kafka DLQ acknowledgement receipt conflicts with prior source identity")

type PostgresDLQAcknowledgementStore struct {
	db            *sql.DB
	consumerGroup string
}

func NewPostgresDLQAcknowledgementBarrier(
	db *sql.DB,
	consumerGroup string,
) (DLQAcknowledgementBarrier, error) {
	consumerGroup = strings.TrimSpace(consumerGroup)
	if db == nil || consumerGroup == "" {
		return nil, fmt.Errorf("PostgreSQL and consumer group are required for the DLQ acknowledgement barrier")
	}
	store := &PostgresDLQAcknowledgementStore{db: db, consumerGroup: consumerGroup}
	return store.Acknowledge, nil
}

func (store *PostgresDLQAcknowledgementStore) Acknowledge(
	ctx context.Context,
	message *ReceivedMessage,
	processingErr error,
) error {
	if store == nil || store.db == nil || strings.TrimSpace(store.consumerGroup) == "" ||
		message == nil || strings.TrimSpace(message.Topic) == "" ||
		message.Partition < 0 || message.Offset < 0 || processingErr == nil {
		return fmt.Errorf("Kafka DLQ acknowledgement identity is incomplete")
	}
	keyDigest := sha256.Sum256(message.Key)
	payloadDigest := sha256.Sum256(message.Value)
	headerDigest := canonicalKafkaHeaderDigest(message.Headers)
	errorDigest := sha256.Sum256([]byte(processingErr.Error()))
	var exact bool
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO kafka_dlq_acknowledgement_receipts
			(consumer_group,source_topic,source_partition,source_offset,
			 source_key_sha256,payload_sha256,headers_sha256,error_sha256)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (consumer_group,source_topic,source_partition,source_offset)
		DO UPDATE SET acknowledged_at=kafka_dlq_acknowledgement_receipts.acknowledged_at
		WHERE kafka_dlq_acknowledgement_receipts.source_key_sha256=EXCLUDED.source_key_sha256
		  AND kafka_dlq_acknowledgement_receipts.payload_sha256=EXCLUDED.payload_sha256
		  AND kafka_dlq_acknowledgement_receipts.headers_sha256=EXCLUDED.headers_sha256
		  AND kafka_dlq_acknowledgement_receipts.error_sha256=EXCLUDED.error_sha256
		RETURNING true`,
		store.consumerGroup, message.Topic, message.Partition, message.Offset,
		fmt.Sprintf("%x", keyDigest[:]), fmt.Sprintf("%x", payloadDigest[:]),
		headerDigest, fmt.Sprintf("%x", errorDigest[:]),
	).Scan(&exact)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDLQAcknowledgementConflict
	}
	if err != nil {
		return fmt.Errorf("persist Kafka DLQ acknowledgement receipt: %w", err)
	}
	if !exact {
		return ErrDLQAcknowledgementConflict
	}
	return nil
}

func canonicalKafkaHeaderDigest(headers []segmentkafka.Header) string {
	canonical := make([]string, 0, len(headers))
	for _, header := range headers {
		canonical = append(canonical, strings.ToLower(strings.TrimSpace(header.Key))+"\x00"+string(header.Value))
	}
	sort.Strings(canonical)
	hasher := sha256.New()
	for _, item := range canonical {
		_, _ = hasher.Write([]byte(item))
		_, _ = hasher.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}
