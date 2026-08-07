package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func (s *AssetService) StartDiscoveryScheduler(ctx context.Context) {
	if s == nil || s.cfg == nil || !s.cfg.Discovery.SchedulerEnabled {
		return
	}
	interval := s.cfg.Discovery.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	initialDelay := s.cfg.Discovery.InitialDelay
	if initialDelay < 0 {
		initialDelay = 0
	}
	go s.runDiscoveryScheduler(ctx, initialDelay, interval)
}

func (s *AssetService) runDiscoveryScheduler(ctx context.Context, initialDelay, interval time.Duration) {
	if initialDelay > 0 {
		timer := time.NewTimer(initialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}

	s.executeScheduledDiscovery(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.executeScheduledDiscovery(ctx)
		}
	}
}

func (s *AssetService) executeScheduledDiscovery(ctx context.Context) {
	req := &config.ActiveDiscoveryRequest{
		TenantID:     s.cfg.Discovery.TenantID,
		ActionID:     discoveryActionID,
		Mode:         s.cfg.Discovery.Mode,
		TargetCIDR:   s.cfg.Discovery.TargetCIDR,
		CredentialID: s.cfg.Discovery.CredentialID,
		RequestedBy:  s.cfg.Discovery.RequestedBy,
		Reason:       s.cfg.Discovery.SchedulerReason,
		ApprovedBy:   s.cfg.Discovery.SchedulerApprover,
		RateLimit:    s.cfg.Discovery.SchedulerRate,
	}
	bucketInterval := s.cfg.Discovery.Interval
	if bucketInterval <= 0 {
		bucketInterval = 30 * time.Minute
	}
	bucket := time.Now().UTC().Truncate(bucketInterval)
	keyMaterial := fmt.Sprintf(
		"%s:%s:%s:%s",
		req.TenantID, req.TargetCIDR, req.Mode, bucket.Format(time.RFC3339),
	)
	keyHash := sha256.Sum256([]byte(keyMaterial))
	traceID := fmt.Sprintf("asset-discovery-scheduler-%x", keyHash[:12])
	command := config.DiscoveryJobCommand{
		IdempotencyKey: traceID,
		Actor:          s.cfg.Discovery.RequestedBy,
		TraceID:        traceID,
		RequestID:      traceID,
	}
	if s.cfg.Discovery.JobsV2Enabled {
		run, err := s.SubmitActiveDiscovery(ctx, req, command)
		if err != nil {
			s.logger.Warn("scheduled active discovery job was not accepted", zap.Error(err))
			return
		}
		s.logger.Info("scheduled active discovery job accepted",
			zap.String("run_id", run.RunID),
			zap.String("status", run.Status),
			zap.Bool("idempotent_replay", run.IdempotentReplay))
		return
	}
	result, err := s.RunActiveDiscovery(ctx, req, command)
	if err != nil {
		s.logger.Warn("scheduled active discovery failed before run persisted", zap.Error(err))
		return
	}
	if result != nil && result.Run != nil {
		s.logger.Info("scheduled active discovery finished",
			zap.String("run_id", result.Run.RunID),
			zap.String("status", result.Run.Status),
			zap.Int("assets", result.AcceptedAssets),
			zap.Int("links", result.AcceptedLinks),
			zap.Int("rejected", result.RejectedRecords))
	}
}
