package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

var consumerSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var consumerSemverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func validateModelConsumerReadyAck(ack ModelAppliedAck, config ModelServiceConfig) error {
	expectedParallelism := config.AppliedAckExpectedParallelism
	if ack.AckType != "consumer_ready" || ack.Status != "consumer_ready" {
		return fmt.Errorf("model consumer readiness requires ack_type and status consumer_ready")
	}
	if strings.TrimSpace(ack.EventID) == "" || strings.TrimSpace(ack.ConsumerDeploymentID) == "" {
		return fmt.Errorf("model consumer readiness requires event_id and consumer_deployment_id")
	}
	if strings.TrimSpace(config.ModelConsumerDeploymentID) == "" ||
		!consumerSHA256Pattern.MatchString(config.ModelConsumerProfileSHA256) {
		return fmt.Errorf("server model consumer compatibility profile is not configured")
	}
	if ack.ConsumerDeploymentID != config.ModelConsumerDeploymentID ||
		ack.ConsumerProfileSHA256 != config.ModelConsumerProfileSHA256 {
		return fmt.Errorf("model consumer readiness deployment or profile does not match server contract")
	}
	if !consumerSHA256Pattern.MatchString(ack.ConsumerProfileSHA256) || ack.ArtifactSHA256 != ack.ConsumerProfileSHA256 {
		return fmt.Errorf("model consumer readiness profile SHA-256 is invalid or inconsistent")
	}
	if ack.RuntimeContract != "traffic.behavior.inference.v1" || !consumerSemverPattern.MatchString(ack.RuntimeVersion) {
		return fmt.Errorf("model consumer readiness runtime contract or version is unsupported")
	}
	if ack.RuntimeContract != config.ModelConsumerRuntimeContract ||
		ack.RuntimeVersion != config.ModelConsumerRuntimeVersion ||
		ack.FeatureSchemaVersion != config.ModelConsumerFeatureSchema ||
		ack.GraphSchemaVersion != config.ModelConsumerGraphSchema ||
		ack.SupportedModelFormats != config.ModelConsumerFormats {
		return fmt.Errorf("model consumer readiness compatibility fields do not match server contract")
	}
	if ack.FeatureSchemaVersion <= 0 || ack.GraphSchemaVersion <= 0 {
		return fmt.Errorf("model consumer readiness feature and graph schema versions must be positive")
	}
	formats := strings.Split(ack.SupportedModelFormats, ",")
	formatSet := make(map[string]bool, len(formats))
	for _, format := range formats {
		formatSet[strings.TrimSpace(format)] = true
	}
	if !formatSet["onnx"] || !formatSet["numpy_npz_v1"] {
		return fmt.Errorf("model consumer readiness must support ONNX and numpy_npz_v1")
	}
	if expectedParallelism <= 0 || ack.Parallelism != expectedParallelism ||
		ack.SubtaskIndex < 0 || ack.SubtaskIndex >= expectedParallelism {
		return fmt.Errorf("model consumer readiness subtask %d/%d does not match server contract %d",
			ack.SubtaskIndex, ack.Parallelism, expectedParallelism)
	}
	return nil
}

func (s *ModelService) handleModelConsumerReadyAck(ctx context.Context, ack ModelAppliedAck, payload []byte) error {
	expected := s.config.AppliedAckExpectedParallelism
	if err := validateModelConsumerReadyAck(ack, s.config); err != nil {
		return err
	}
	ttl := s.config.ModelConsumerReadyTTL
	if ttl <= 0 {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model consumer readiness TTL must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model consumer readiness transaction")
	}
	defer tx.Rollback()

	var accepted int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO model_update_consumer_readiness (
			consumer_deployment_id, subtask_index, event_id, consumer_profile_sha256,
			runtime_contract, runtime_version, feature_schema_version, graph_schema_version,
			supported_model_formats, parallelism, status, payload, received_at, last_seen_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'ready',$11::jsonb,now(),now())
		ON CONFLICT (consumer_deployment_id, subtask_index) DO UPDATE SET
			event_id = EXCLUDED.event_id, payload = EXCLUDED.payload, last_seen_at = now()
		WHERE model_update_consumer_readiness.consumer_profile_sha256 = EXCLUDED.consumer_profile_sha256
		  AND model_update_consumer_readiness.runtime_contract = EXCLUDED.runtime_contract
		  AND model_update_consumer_readiness.runtime_version = EXCLUDED.runtime_version
		  AND model_update_consumer_readiness.feature_schema_version = EXCLUDED.feature_schema_version
		  AND model_update_consumer_readiness.graph_schema_version = EXCLUDED.graph_schema_version
		  AND model_update_consumer_readiness.supported_model_formats = EXCLUDED.supported_model_formats
		  AND model_update_consumer_readiness.parallelism = EXCLUDED.parallelism
		RETURNING 1
	`, ack.ConsumerDeploymentID, ack.SubtaskIndex, ack.EventID, ack.ConsumerProfileSHA256,
		ack.RuntimeContract, ack.RuntimeVersion, ack.FeatureSchemaVersion, ack.GraphSchemaVersion,
		ack.SupportedModelFormats, ack.Parallelism, string(payload)).Scan(&accepted)
	if err == sql.ErrNoRows {
		return errors.New(errors.ErrCodeVersionConflict, "model consumer deployment reused with a different compatibility profile")
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model consumer readiness subtask")
	}

	var readyCount, minSubtask, maxSubtask int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MIN(subtask_index),-1), COALESCE(MAX(subtask_index),-1)
		FROM model_update_consumer_readiness
		WHERE consumer_deployment_id=$1 AND consumer_profile_sha256=$2
		  AND runtime_contract=$3 AND runtime_version=$4
		  AND feature_schema_version=$5 AND graph_schema_version=$6
		  AND supported_model_formats=$7 AND parallelism=$8 AND status='ready'
	`, ack.ConsumerDeploymentID, ack.ConsumerProfileSHA256, ack.RuntimeContract,
		ack.RuntimeVersion, ack.FeatureSchemaVersion, ack.GraphSchemaVersion,
		ack.SupportedModelFormats, ack.Parallelism).Scan(&readyCount, &minSubtask, &maxSubtask); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to aggregate model consumer readiness")
	}
	if readyCount == expected && minSubtask == 0 && maxSubtask == expected-1 {
		var receiptAccepted int
		expiresAt := time.Now().UTC().Add(ttl)
		err := tx.QueryRowContext(ctx, `
			INSERT INTO model_update_consumer_ready_receipts (
				consumer_deployment_id, consumer_profile_sha256, runtime_contract, runtime_version,
				feature_schema_version, graph_schema_version, supported_model_formats,
				expected_parallelism, ready_subtasks, status, ready_at, last_seen_at, expires_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,'ready',now(),now(),$9)
			ON CONFLICT (consumer_deployment_id) DO UPDATE SET
				ready_subtasks=EXCLUDED.ready_subtasks, last_seen_at=now(), expires_at=EXCLUDED.expires_at
			WHERE model_update_consumer_ready_receipts.consumer_profile_sha256=EXCLUDED.consumer_profile_sha256
			  AND model_update_consumer_ready_receipts.runtime_contract=EXCLUDED.runtime_contract
			  AND model_update_consumer_ready_receipts.runtime_version=EXCLUDED.runtime_version
			  AND model_update_consumer_ready_receipts.feature_schema_version=EXCLUDED.feature_schema_version
			  AND model_update_consumer_ready_receipts.graph_schema_version=EXCLUDED.graph_schema_version
			  AND model_update_consumer_ready_receipts.supported_model_formats=EXCLUDED.supported_model_formats
			  AND model_update_consumer_ready_receipts.expected_parallelism=EXCLUDED.expected_parallelism
			RETURNING 1
		`, ack.ConsumerDeploymentID, ack.ConsumerProfileSHA256, ack.RuntimeContract,
			ack.RuntimeVersion, ack.FeatureSchemaVersion, ack.GraphSchemaVersion,
			ack.SupportedModelFormats, expected, expiresAt).Scan(&receiptAccepted)
		if err == sql.ErrNoRows {
			return errors.New(errors.ErrCodeVersionConflict, "model consumer ready receipt profile drifted")
		}
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to close model consumer readiness quorum")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model consumer readiness")
	}
	return nil
}
