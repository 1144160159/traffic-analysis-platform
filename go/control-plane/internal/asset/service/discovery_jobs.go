package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

const discoveryActionID = "asset-active-discovery-run"

func (s *AssetService) DiscoveryJobsV2Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Discovery.JobsV2Enabled
}

func (s *AssetService) SubmitActiveDiscovery(
	ctx context.Context,
	req *config.ActiveDiscoveryRequest,
	command config.DiscoveryJobCommand,
) (*config.DiscoveryRun, error) {
	if req == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "discovery request is required")
	}
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.Mode = normalizeDiscoveryMode(req.Mode)
	req.ActionID = strings.TrimSpace(req.ActionID)
	if req.ActionID == "" {
		req.ActionID = discoveryActionID
	}
	req.TargetCIDR = strings.TrimSpace(req.TargetCIDR)
	req.CredentialID = strings.TrimSpace(req.CredentialID)
	req.RequestedBy = strings.TrimSpace(req.RequestedBy)
	req.Reason = strings.TrimSpace(req.Reason)
	req.ApprovedBy = strings.TrimSpace(req.ApprovedBy)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if req.TenantID == "" || command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant, authenticated actor and trace_id are required")
	}
	if req.ActionID != discoveryActionID {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported discovery action_id")
	}
	if !isDiscoveryModeAllowed(req.Mode) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "mode must be snmp, lldp, or snmp_lldp")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if len(req.Reason) < 4 || len(req.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}
	if req.ApprovedBy == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "approved_by is required for active discovery")
	}
	if req.ApprovedBy == command.Actor {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "requester cannot self-approve active discovery")
	}
	if len(req.Observations) != 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "observations are worker output and cannot be supplied by an API client")
	}
	if req.TargetCIDR == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "target_cidr is required")
	}
	_, network, err := net.ParseCIDR(req.TargetCIDR)
	if err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "target_cidr must be a valid CIDR")
	}
	req.TargetCIDR = network.String()
	if req.RateLimit == 0 {
		req.RateLimit = 10
	}
	if req.RateLimit < 1 || req.RateLimit > 10000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "rate_limit_per_second must be between 1 and 10000")
	}
	if req.SecurityFrom.IsZero() != req.SecurityTo.IsZero() {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "security window start and end must be provided together")
	}
	if !req.SecurityFrom.IsZero() && !req.SecurityTo.After(req.SecurityFrom) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "security_window_end must be after security_window_start")
	}
	if req.SecurityTo.Before(time.Now().UTC()) && !req.SecurityTo.IsZero() {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "security window has already expired")
	}
	if req.CredentialID != "" {
		if _, err := s.repo.GetDiscoveryCredential(ctx, req.TenantID, req.CredentialID); err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				return nil, errors.New(errors.ErrCodeInvalidParameter, "credential_id is not available in the authenticated tenant")
			}
			return nil, fmt.Errorf("verify discovery credential: %w", err)
		}
	}
	req.RequestedBy = command.Actor
	normalized, _ := json.Marshal(struct {
		ActionID     string    `json:"action_id"`
		Mode         string    `json:"mode"`
		TargetCIDR   string    `json:"target_cidr"`
		CredentialID string    `json:"credential_id"`
		Reason       string    `json:"reason"`
		RateLimit    int       `json:"rate_limit_per_second"`
		SecurityFrom time.Time `json:"security_window_start"`
		SecurityTo   time.Time `json:"security_window_end"`
		ApprovedBy   string    `json:"approved_by"`
		Actor        string    `json:"actor"`
	}{
		req.ActionID, req.Mode, req.TargetCIDR, req.CredentialID, req.Reason,
		req.RateLimit, req.SecurityFrom.UTC(), req.SecurityTo.UTC(), req.ApprovedBy,
		command.Actor,
	})
	requestHashBytes := sha256.Sum256(normalized)
	now := time.Now().UTC()
	run := &config.DiscoveryRun{
		RunID:        uuid.NewString(),
		TenantID:     req.TenantID,
		Mode:         req.Mode,
		TargetCIDR:   req.TargetCIDR,
		CredentialID: req.CredentialID,
		ActionID:     req.ActionID,
		Status:       config.DiscoveryStatusQueued,
		Revision:     1,
		RequestedBy:  command.Actor,
		Reason:       req.Reason,
		RateLimit:    req.RateLimit,
		SecurityFrom: req.SecurityFrom,
		SecurityTo:   req.SecurityTo,
		ApprovedBy:   req.ApprovedBy,
		TraceID:      command.TraceID,
		QueuedAt:     now,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	return s.repo.CreateDiscoveryJobAtomic(ctx, run, command, fmt.Sprintf("%x", requestHashBytes[:]))
}

func (s *AssetService) GetDiscoveryJob(ctx context.Context, tenantID, runID string) (*config.DiscoveryRun, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(runID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id and run_id are required")
	}
	return s.repo.GetDiscoveryRun(ctx, tenantID, runID)
}

func (s *AssetService) CancelDiscoveryJob(
	ctx context.Context,
	tenantID, runID, reason string,
	command config.DiscoveryJobCommand,
	expectedRevision int64,
) (*config.DiscoveryRun, error) {
	if len(strings.TrimSpace(reason)) < 4 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must contain at least 4 characters")
	}
	if expectedRevision < 1 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_revision must be positive")
	}
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "authenticated actor and trace_id are required")
	}
	return s.repo.CancelDiscoveryJob(
		ctx, tenantID, runID, strings.TrimSpace(reason), command, expectedRevision,
	)
}

func (s *AssetService) ListDiscoveryCandidates(
	ctx context.Context,
	tenantID, runID string,
	limit int,
) ([]*config.DiscoveryCandidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListDiscoveryCandidates(ctx, tenantID, runID, limit)
}

func (s *AssetService) MergeDiscoveryCandidate(
	ctx context.Context,
	tenantID, runID, candidateID string,
	command config.DiscoveryCandidateMergeCommand,
) (*config.DiscoveryCandidateMergeResult, error) {
	tenantID = strings.TrimSpace(tenantID)
	runID = strings.TrimSpace(runID)
	candidateID = strings.TrimSpace(candidateID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
	command.RequestID = strings.TrimSpace(command.RequestID)
	command.ClientIP = strings.TrimSpace(command.ClientIP)
	command.UserAgent = strings.TrimSpace(command.UserAgent)
	command.MergeMode = strings.ToLower(strings.TrimSpace(command.MergeMode))
	command.Reason = strings.TrimSpace(command.Reason)
	if tenantID == "" || runID == "" || candidateID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id, run_id and candidate_id are required")
	}
	if _, err := uuid.Parse(runID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "run_id must be a UUID")
	}
	if _, err := uuid.Parse(candidateID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "candidate_id must be a UUID")
	}
	if command.ExpectedCandidateRevision < 1 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_candidate_revision must be positive")
	}
	if command.ExpectedAssetRevision < 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_asset_revision cannot be negative")
	}
	if command.MergeMode != "manual" && command.MergeMode != "policy" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "merge_mode must be manual or policy")
	}
	if len(command.Reason) < 4 || len(command.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "authenticated actor and trace_id are required")
	}
	return s.repo.MergeDiscoveryCandidateAtomic(
		ctx, tenantID, runID, candidateID, command,
	)
}

func (s *AssetService) ListDiscoveryJobHistory(
	ctx context.Context,
	tenantID, runID string,
) ([]*config.DiscoveryRunHistory, error) {
	return s.repo.ListDiscoveryRunHistory(ctx, tenantID, runID)
}

func (s *AssetService) StartDiscoveryWorker(ctx context.Context) {
	if s == nil || s.cfg == nil || !s.cfg.Discovery.JobsV2Enabled || !s.cfg.Discovery.WorkerEnabled {
		return
	}
	interval := s.cfg.Discovery.WorkerInterval
	if interval <= 0 {
		interval = time.Second
	}
	workerID := "asset-discovery-" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			found, err := s.ProcessNextDiscoveryJob(ctx, workerID)
			if err != nil {
				s.logger.Warn("active discovery worker iteration failed", zap.Error(err))
			}
			if found {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *AssetService) ProcessNextDiscoveryJob(ctx context.Context, workerID string) (bool, error) {
	lease := 2 * time.Minute
	if s.cfg != nil && s.cfg.Discovery.WorkerLease > 0 {
		lease = s.cfg.Discovery.WorkerLease
	}
	run, err := s.repo.ClaimDiscoveryJob(ctx, workerID, lease)
	if stderrors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.Status == config.DiscoveryStatusBlocked {
		s.logger.Info("active discovery job blocked before lease",
			zap.String("run_id", run.RunID),
			zap.String("reason", run.ErrorMessage))
		return true, nil
	}
	req := &config.ActiveDiscoveryRequest{
		TenantID:     run.TenantID,
		ActionID:     run.ActionID,
		Mode:         run.Mode,
		TargetCIDR:   run.TargetCIDR,
		CredentialID: run.CredentialID,
		RequestedBy:  run.RequestedBy,
		Reason:       run.Reason,
		RateLimit:    run.RateLimit,
		SecurityFrom: run.SecurityFrom,
		SecurityTo:   run.SecurityTo,
		ApprovedBy:   run.ApprovedBy,
	}
	credential, credentialErr := s.discoveryCredentialForRun(ctx, run)
	var observations []config.DiscoveryObservation
	executionErr := credentialErr
	if executionErr == nil {
		if s.scanner == nil {
			executionErr = fmt.Errorf("asset discovery scanner is not configured")
		} else {
			observations, executionErr = s.scanner.Scan(ctx, req, credential)
		}
	}
	completed, completeErr := s.repo.CompleteDiscoveryJob(
		ctx, run, observations, 0, executionErr, workerID,
	)
	if completeErr != nil {
		return true, completeErr
	}
	s.logger.Info("active discovery job reached durable terminal state",
		zap.String("run_id", completed.RunID),
		zap.String("status", completed.Status),
		zap.Int("candidates", completed.CandidateCount))
	return true, nil
}
