package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

var lifecycleStates = map[string]bool{"candidate": true, "confirmed": true, "managed": true, "isolated": true, "retired": true, "merged": true}
var governanceActions = map[string]bool{"asset-governance-approve": true, "asset-governance-reject": true,
	"asset-governance-start": true, "asset-governance-complete": true, "asset-governance-fail": true,
	"asset-governance-cancel": true, "asset-governance-compensate": true}

func (s *AssetService) CreateAssetGovernanceWorkOrder(ctx context.Context, assetID string,
	command config.AssetGovernanceCreateCommand) (*config.AssetGovernanceWorkOrder, error) {
	if _, err := uuid.Parse(assetID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id must be a UUID")
	}
	if strings.TrimSpace(command.TenantID) == "" || strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.TraceID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "verified tenant, actor and trace_id are required")
	}
	if command.ActionID != config.AssetGovernanceCreateAction {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid governance create action_id")
	}
	if !lifecycleStates[command.TargetLifecycleState] {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid target_lifecycle_state")
	}
	if command.TargetLifecycleState == "merged" {
		if _, err := uuid.Parse(command.TargetAssetID); err != nil || command.TargetAssetID == assetID {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "merged lifecycle requires another target_asset_id")
		}
	} else if command.TargetAssetID != "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "target_asset_id is only valid for merged lifecycle")
	}
	if strings.TrimSpace(command.Owner) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "owner is required")
	}
	if command.DueAt.IsZero() || !command.DueAt.After(time.Now().UTC()) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "due_at must be in the future")
	}
	if command.ExpectedAssetRevision < 1 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_asset_revision must be positive")
	}
	if len(strings.TrimSpace(command.Reason)) < 8 || len(command.Reason) > 2000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 8-2000 characters")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	return s.repo.CreateAssetGovernanceWorkOrder(ctx, assetID, command)
}

func (s *AssetService) ApplyAssetGovernanceAction(ctx context.Context, orderID string,
	command config.AssetGovernanceActionCommand) (*config.AssetGovernanceWorkOrder, error) {
	if _, err := uuid.Parse(orderID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "work_order_id must be a UUID")
	}
	if !governanceActions[command.ActionID] {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid governance action_id")
	}
	if command.ExpectedRevision < 1 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_revision must be positive")
	}
	if len(strings.TrimSpace(command.Reason)) < 8 || len(command.Reason) > 2000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 8-2000 characters")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if command.TenantID == "" || command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "verified tenant, actor and trace_id are required")
	}
	for i, ref := range command.EvidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || len(ref) > 1000 {
			return nil, errors.New(errors.ErrCodeInvalidParameter, fmt.Sprintf("evidence_refs[%d] is invalid", i))
		}
		command.EvidenceRefs[i] = ref
	}
	return s.repo.ApplyAssetGovernanceAction(ctx, orderID, command)
}

func (s *AssetService) GetAssetGovernanceWorkOrder(ctx context.Context, tenantID, orderID string) (*config.AssetGovernanceWorkOrder, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	return s.repo.GetAssetGovernanceWorkOrder(ctx, tenantID, orderID)
}

func (s *AssetService) ListAssetGovernanceWorkOrders(ctx context.Context, tenantID, assetID string) ([]config.AssetGovernanceWorkOrder, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	return s.repo.ListAssetGovernanceWorkOrders(ctx, tenantID, assetID)
}
