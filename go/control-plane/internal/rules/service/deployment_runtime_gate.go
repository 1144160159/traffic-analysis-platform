package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

const deploymentRuntimeGateConfigurationKey = "runtime_gate"

type deploymentRuntimeGateQueryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

// discoverDeploymentRuntimeGate resolves the latest exact receipts for the
// immutable rule/model versions referenced by a deployment.  It does not
// trust caller-supplied event IDs.  Approval persists the discovered IDs and
// execution compares them again, so a later event cannot silently replace the
// reviewed runtime fact.
func (s *DeploymentService) discoverDeploymentRuntimeGate(
	ctx context.Context,
	queryer deploymentRuntimeGateQueryer,
	deployment *model.Deployment,
	includeDeploymentProjection bool,
) (*model.DeploymentRuntimeGate, error) {
	gate := &model.DeploymentRuntimeGate{
		Enabled:         s.config.EnableRuntimeAckGate,
		CheckedAt:       time.Now().UTC(),
		BlockingReasons: make([]string, 0),
	}
	if !s.config.EnableRuntimeAckGate {
		gate.Status = "disabled"
		gate.Ready = true
		gate.ExpansionAllowed = true
		return gate, nil
	}
	if queryer == nil || deployment == nil {
		return nil, fmt.Errorf("deployment runtime gate database and deployment are required")
	}

	if strings.TrimSpace(deployment.RuleVersion) != "" {
		receipt, err := s.discoverRuleRuntimeReceipt(ctx, queryer, deployment)
		if err != nil {
			return nil, err
		}
		gate.Rule = receipt
		if receipt.Status != "applied" {
			gate.BlockingReasons = append(gate.BlockingReasons,
				fmt.Sprintf("rule %s runtime ACK is %s", receipt.ComponentID, receipt.Status))
		}
	}
	if strings.TrimSpace(deployment.ModelVersion) != "" {
		receipt, err := s.discoverModelRuntimeReceipt(ctx, queryer, deployment)
		if err != nil {
			return nil, err
		}
		gate.Model = receipt
		if receipt.Status != "applied" {
			gate.BlockingReasons = append(gate.BlockingReasons,
				fmt.Sprintf("model %s runtime ACK is %s", receipt.ComponentID, receipt.Status))
		}
	}
	if gate.Rule == nil && gate.Model == nil {
		gate.BlockingReasons = append(gate.BlockingReasons, "deployment has no rule or model version")
	}
	if includeDeploymentProjection {
		receipt, err := s.discoverGrayProjectionReceipt(ctx, queryer, deployment)
		if err != nil {
			return nil, err
		}
		gate.DeploymentProjection = receipt
		if receipt.Status != "applied" {
			gate.BlockingReasons = append(gate.BlockingReasons,
				fmt.Sprintf("gray deployment projection ACK is %s", receipt.Status))
		}
	}

	gate.Ready = len(gate.BlockingReasons) == 0
	gate.ExpansionAllowed = gate.Ready
	if gate.Ready {
		gate.Status = "ready"
	} else {
		gate.Status = "blocked"
	}
	return gate, nil
}

func (s *DeploymentService) discoverRuleRuntimeReceipt(
	ctx context.Context,
	queryer deploymentRuntimeGateQueryer,
	deployment *model.Deployment,
) (*model.DeploymentRuntimeReceipt, error) {
	receipt := &model.DeploymentRuntimeReceipt{
		Component:    "rule",
		ComponentID:  deployment.RuleVersion,
		Status:       "missing",
		ExpectedAcks: s.config.RuleAppliedExpectedParallelism,
	}
	if receipt.ExpectedAcks <= 0 {
		return nil, fmt.Errorf("deployment rule ACK parallelism contract is not configured")
	}
	var appliedAt sql.NullTime
	var minSuccess, maxSuccess int
	err := queryer.QueryRowContext(ctx, `
		SELECT COALESCE(o.payload->>'event_id',''), o.published, o.runtime_status,
		       o.runtime_applied_at, COALESCE(o.runtime_last_error,''),
		       COUNT(DISTINCT a.subtask_index),
		       COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status IN ('applied','duplicate')),
		       COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status IN ('stale','conflict')),
		       COALESCE(MAX(a.parallelism),0),
		       COALESCE(MIN(a.subtask_index) FILTER (WHERE a.status IN ('applied','duplicate')),-1),
		       COALESCE(MAX(a.subtask_index) FILTER (WHERE a.status IN ('applied','duplicate')),-1)
		FROM rule_versions rv
		JOIN rule_outbox o ON o.rule_id::text=rv.rule_id::text
		LEFT JOIN rule_update_applied_acks a ON a.event_id=o.payload->>'event_id'
		WHERE rv.rule_version=$1 AND rv.tenant_id=$2
		  AND COALESCE((o.payload->>'version')::bigint,0)=rv.version
		  AND COALESCE(o.payload->'rule'->>'tenant_id','')=rv.tenant_id
		GROUP BY o.id,o.payload,o.published,o.runtime_status,o.runtime_applied_at,
		         o.runtime_last_error,rv.rule_id
		ORDER BY o.id DESC LIMIT 1
	`, deployment.RuleVersion, deployment.TenantID).Scan(
		&receipt.EventID, &receipt.BrokerPublished, &receipt.Status,
		&appliedAt, &receipt.LastError, &receipt.ReceivedAcks,
		&receipt.SuccessfulAcks, &receipt.FailedAcks, &receipt.ConsumerParallel,
		&minSuccess, &maxSuccess,
	)
	if err == sql.ErrNoRows {
		return receipt, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve rule deployment runtime ACK: %w", err)
	}
	if appliedAt.Valid {
		value := appliedAt.Time.UTC()
		receipt.AppliedAt = &value
	}
	if !receipt.BrokerPublished {
		receipt.Status = "pending"
	} else if receipt.FailedAcks > 0 || receipt.Status == "failed" {
		receipt.Status = "failed"
	} else if receipt.Status == "applied" &&
		receipt.SuccessfulAcks == receipt.ExpectedAcks &&
		receipt.ConsumerParallel == receipt.ExpectedAcks &&
		minSuccess == 0 && maxSuccess == receipt.ExpectedAcks-1 {
		receipt.Status = "applied"
	} else {
		receipt.Status = "partial"
	}
	return receipt, nil
}

func (s *DeploymentService) discoverModelRuntimeReceipt(
	ctx context.Context,
	queryer deploymentRuntimeGateQueryer,
	deployment *model.Deployment,
) (*model.DeploymentRuntimeReceipt, error) {
	receipt := &model.DeploymentRuntimeReceipt{
		Component:    "model",
		ComponentID:  deployment.ModelVersion,
		Status:       "missing",
		ExpectedAcks: s.config.ModelAppliedExpectedParallelism,
	}
	if receipt.ExpectedAcks <= 0 {
		return nil, fmt.Errorf("deployment model ACK parallelism contract is not configured")
	}
	var appliedAt sql.NullTime
	var minSuccess, maxSuccess int
	err := queryer.QueryRowContext(ctx, `
		SELECT o.event_id, (o.status='published'), o.published_at,
		       COALESCE(MAX(NULLIF(o.last_error,'')),''),
		       COUNT(DISTINCT a.subtask_index),
		       COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status='applied'),
		       COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status='failed'),
		       COALESCE(MAX(a.parallelism),0),
		       COALESCE(MIN(a.subtask_index) FILTER (WHERE a.status='applied'),-1),
		       COALESCE(MAX(a.subtask_index) FILTER (WHERE a.status='applied'),-1)
		FROM model_versions mv
		JOIN model_update_outbox o ON o.model_id=mv.model_id::text AND o.model_version=mv.model_version
		LEFT JOIN model_update_applied_acks a ON a.event_id=o.event_id
		WHERE mv.model_version=$1 AND mv.tenant_id=$2 AND o.tenant_id=mv.tenant_id
		  AND o.action <> 'shadow-load'
		GROUP BY o.id,o.event_id,o.status,o.published_at,mv.model_id
		ORDER BY o.id DESC LIMIT 1
	`, deployment.ModelVersion, deployment.TenantID).Scan(
		&receipt.EventID, &receipt.BrokerPublished, &appliedAt, &receipt.LastError,
		&receipt.ReceivedAcks, &receipt.SuccessfulAcks, &receipt.FailedAcks,
		&receipt.ConsumerParallel, &minSuccess, &maxSuccess,
	)
	if err == sql.ErrNoRows {
		return receipt, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve model deployment runtime ACK: %w", err)
	}
	if appliedAt.Valid {
		value := appliedAt.Time.UTC()
		receipt.AppliedAt = &value
	}
	if !receipt.BrokerPublished {
		receipt.Status = "pending"
	} else if receipt.FailedAcks > 0 {
		receipt.Status = "failed"
		if receipt.LastError == "" {
			receipt.LastError = "one or more model consumers rejected the version"
		}
	} else if receipt.SuccessfulAcks == receipt.ExpectedAcks &&
		receipt.ConsumerParallel == receipt.ExpectedAcks &&
		minSuccess == 0 && maxSuccess == receipt.ExpectedAcks-1 {
		receipt.Status = "applied"
	} else {
		receipt.Status = "partial"
	}
	return receipt, nil
}

func (s *DeploymentService) discoverGrayProjectionReceipt(
	ctx context.Context,
	queryer deploymentRuntimeGateQueryer,
	deployment *model.Deployment,
) (*model.DeploymentRuntimeReceipt, error) {
	receipt := &model.DeploymentRuntimeReceipt{
		Component:    "deployment_projection",
		ComponentID:  deployment.DeploymentID,
		Status:       "missing",
		ExpectedAcks: 1,
	}
	var projectedAt sql.NullTime
	var partition sql.NullInt64
	var offset sql.NullInt64
	err := queryer.QueryRowContext(ctx, `
		SELECT o.event_id, (o.status='published'), p.projected_at,
		       p.kafka_partition, p.kafka_offset, COALESCE(o.last_error,'')
		FROM deployment_outbox o
		LEFT JOIN deployment_event_projection p
		  ON p.event_id::text=o.event_id AND p.tenant_id=o.tenant_id
		 AND p.deployment_id=o.deployment_id AND p.action='gray_started' AND p.status='gray'
		WHERE o.tenant_id=$1 AND o.deployment_id=$2 AND o.event_type='gray_started'
		ORDER BY o.id DESC LIMIT 1
	`, deployment.TenantID, deployment.DeploymentID).Scan(
		&receipt.EventID, &receipt.BrokerPublished, &projectedAt,
		&partition, &offset, &receipt.LastError,
	)
	if err == sql.ErrNoRows {
		return receipt, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve gray deployment projection ACK: %w", err)
	}
	if projectedAt.Valid {
		value := projectedAt.Time.UTC()
		receipt.AppliedAt = &value
	}
	if partition.Valid {
		value := int(partition.Int64)
		receipt.KafkaPartition = &value
	}
	if offset.Valid {
		value := offset.Int64
		receipt.KafkaOffset = &value
	}
	if !receipt.BrokerPublished {
		receipt.Status = "pending"
	} else if !projectedAt.Valid {
		receipt.Status = "partial"
	} else {
		receipt.Status = "applied"
		receipt.ReceivedAcks = 1
		receipt.SuccessfulAcks = 1
		receipt.ConsumerParallel = 1
	}
	return receipt, nil
}

func deploymentRuntimeGateFromConfiguration(configuration map[string]interface{}) (*model.DeploymentRuntimeGate, error) {
	raw, ok := configuration[deploymentRuntimeGateConfigurationKey]
	if !ok || raw == nil {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidStateTransition,
			"approved deployment is missing its runtime ACK snapshot; rerun precheck")
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError,
			"failed to encode approved runtime ACK snapshot")
	}
	var gate model.DeploymentRuntimeGate
	if err := json.Unmarshal(payload, &gate); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError,
			"failed to decode approved runtime ACK snapshot")
	}
	if !gate.Enabled || gate.Status != "ready" || !gate.Ready || !gate.ExpansionAllowed {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidStateTransition,
			"approved runtime ACK snapshot is not ready")
	}
	return &gate, nil
}

func (s *DeploymentService) verifyApprovedDeploymentRuntimeGate(
	ctx context.Context,
	queryer deploymentRuntimeGateQueryer,
	deployment *model.Deployment,
	configuration map[string]interface{},
	includeDeploymentProjection bool,
) (*model.DeploymentRuntimeGate, error) {
	if !s.config.EnableRuntimeAckGate {
		return s.discoverDeploymentRuntimeGate(ctx, queryer, deployment, includeDeploymentProjection)
	}
	approved, err := deploymentRuntimeGateFromConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	live, err := s.discoverDeploymentRuntimeGate(ctx, queryer, deployment, includeDeploymentProjection)
	if err != nil {
		return nil, err
	}
	if !live.Ready {
		return nil, commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition,
			"deployment runtime ACK gate blocked expansion: %s", strings.Join(live.BlockingReasons, "; "))
	}
	if !sameDeploymentRuntimeReceipt(approved.Rule, live.Rule) ||
		!sameDeploymentRuntimeReceipt(approved.Model, live.Model) {
		return nil, commonerrors.New(commonerrors.ErrCodeVersionConflict,
			"runtime ACK event changed after approval; rerun precheck and independent approval")
	}
	return live, nil
}

func sameDeploymentRuntimeReceipt(approved, live *model.DeploymentRuntimeReceipt) bool {
	if approved == nil || live == nil {
		return approved == nil && live == nil
	}
	return approved.Component == live.Component &&
		approved.ComponentID == live.ComponentID &&
		approved.EventID != "" && approved.EventID == live.EventID &&
		approved.ExpectedAcks == live.ExpectedAcks
}

func deploymentRuntimeGatePrecheckResult(gate *model.DeploymentRuntimeGate) interface{} {
	status := "failed"
	evidence := "runtime ACK gate is unavailable"
	if gate != nil {
		if gate.Ready {
			status = "passed"
			evidence = "rule/model broker receipts and exact Flink subtask ACK sets are complete"
		} else if len(gate.BlockingReasons) > 0 {
			evidence = strings.Join(gate.BlockingReasons, "; ")
		}
	}
	now := time.Now().UTC()
	return map[string]interface{}{
		"label":          "规则/模型运行时 ACK",
		"status":         status,
		"evidence":       evidence,
		"recommendation": "partial/failed/missing ACK 时停止灰度扩展",
		"observed_at":    now,
		"fresh_until":    now.Add(30 * time.Minute),
	}
}
