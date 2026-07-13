package service

import (
	"context"
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
		Mode:         s.cfg.Discovery.Mode,
		TargetCIDR:   s.cfg.Discovery.TargetCIDR,
		CredentialID: s.cfg.Discovery.CredentialID,
		RequestedBy:  s.cfg.Discovery.RequestedBy,
	}
	result, err := s.RunActiveDiscovery(ctx, req)
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
