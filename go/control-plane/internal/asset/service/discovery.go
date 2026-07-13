package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func (s *AssetService) RegisterDiscoveryCredential(ctx context.Context, credential *config.DiscoveryCredential) (*config.DiscoveryCredential, error) {
	if credential == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "discovery credential is required")
	}
	credential.TenantID = strings.TrimSpace(credential.TenantID)
	credential.Name = strings.TrimSpace(credential.Name)
	credential.Protocol = normalizeDiscoveryMode(credential.Protocol)
	credential.SecretRef = strings.TrimSpace(credential.SecretRef)
	if credential.TenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if credential.Name == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "name is required")
	}
	if !isDiscoveryModeAllowed(credential.Protocol) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "protocol must be snmp, lldp, or snmp_lldp")
	}
	if credential.SecretRef == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "secret_ref is required; plaintext credentials are not accepted")
	}
	if credential.CredentialID == "" {
		credential.CredentialID = uuid.New().String()
	}
	now := time.Now()
	credential.CreatedAt = now
	credential.UpdatedAt = now
	if err := s.repo.RegisterDiscoveryCredential(ctx, credential); err != nil {
		return nil, fmt.Errorf("register discovery credential: %w", err)
	}
	return credential, nil
}

func (s *AssetService) ListDiscoveryCredentials(ctx context.Context, tenantID string, limit int) ([]*config.DiscoveryCredential, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListDiscoveryCredentials(ctx, tenantID, limit)
}

func (s *AssetService) RunActiveDiscovery(ctx context.Context, req *config.ActiveDiscoveryRequest) (*config.DiscoveryResult, error) {
	if req == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "discovery request is required")
	}
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.Mode = normalizeDiscoveryMode(req.Mode)
	if req.TenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if !isDiscoveryModeAllowed(req.Mode) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "mode must be snmp, lldp, or snmp_lldp")
	}

	now := time.Now()
	run := &config.DiscoveryRun{
		RunID:        uuid.New().String(),
		TenantID:     req.TenantID,
		Mode:         req.Mode,
		TargetCIDR:   strings.TrimSpace(req.TargetCIDR),
		CredentialID: strings.TrimSpace(req.CredentialID),
		Status:       config.DiscoveryStatusQueued,
		RequestedBy:  strings.TrimSpace(req.RequestedBy),
		StartedAt:    now,
	}
	if err := s.repo.CreateDiscoveryRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create discovery run: %w", err)
	}

	result := &config.DiscoveryResult{Run: run}
	observations := req.Observations
	scanAttempted := false
	if len(observations) == 0 && (run.TargetCIDR != "" || run.CredentialID != "") {
		scanAttempted = true
		credential, err := s.discoveryCredentialForRun(ctx, run)
		if err == nil && s.scanner != nil {
			observations, err = s.scanner.Scan(ctx, req, credential)
		} else if err == nil {
			err = fmt.Errorf("asset discovery scanner is not configured")
		}
		if err != nil {
			return s.failDiscoveryRun(ctx, run, result, err)
		}
	}

	for _, observation := range observations {
		assetID, ok, err := s.recordDiscoveryObservation(ctx, req.TenantID, req.Mode, run.RunID, observation)
		if err != nil {
			s.logger.Warn("active discovery observation rejected", zap.Error(err), zap.String("run_id", run.RunID))
			result.RejectedRecords++
			continue
		}
		if ok {
			result.AcceptedAssets++
		}
		links, rejected := s.recordDiscoveryNeighbors(ctx, req.TenantID, req.Mode, run.RunID, assetID, observation)
		result.AcceptedLinks += links
		result.RejectedRecords += rejected
	}

	completedAt := time.Now()
	status := config.DiscoveryStatusCompleted
	if len(observations) == 0 {
		status = config.DiscoveryStatusQueued
	}
	if scanAttempted && len(observations) == 0 {
		return s.failDiscoveryRun(ctx, run, result, fmt.Errorf("active discovery scan returned no observations"))
	}
	run.Status = status
	run.DiscoveredAssets = result.AcceptedAssets
	run.DiscoveredLinks = result.AcceptedLinks
	run.CompletedAt = completedAt
	if err := s.repo.CompleteDiscoveryRun(ctx, run.RunID, status, "", result.AcceptedAssets, result.AcceptedLinks, completedAt); err != nil {
		return nil, fmt.Errorf("complete discovery run: %w", err)
	}
	return result, nil
}

func (s *AssetService) discoveryCredentialForRun(ctx context.Context, run *config.DiscoveryRun) (*config.DiscoveryCredential, error) {
	if run.CredentialID == "" {
		return nil, nil
	}
	credential, err := s.repo.GetDiscoveryCredential(ctx, run.TenantID, run.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("load discovery credential: %w", err)
	}
	return credential, nil
}

func (s *AssetService) failDiscoveryRun(ctx context.Context, run *config.DiscoveryRun, result *config.DiscoveryResult, cause error) (*config.DiscoveryResult, error) {
	completedAt := time.Now()
	run.Status = config.DiscoveryStatusFailed
	run.ErrorMessage = cause.Error()
	run.CompletedAt = completedAt
	if err := s.repo.CompleteDiscoveryRun(ctx, run.RunID, config.DiscoveryStatusFailed, cause.Error(), result.AcceptedAssets, result.AcceptedLinks, completedAt); err != nil {
		return nil, fmt.Errorf("complete failed discovery run: %w", err)
	}
	s.logger.Warn("active discovery scan failed", zap.String("run_id", run.RunID), zap.Error(cause))
	return result, nil
}

func (s *AssetService) ListDiscoveryRuns(ctx context.Context, tenantID string, limit int) ([]*config.DiscoveryRun, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListDiscoveryRuns(ctx, tenantID, limit)
}

func (s *AssetService) ListTopologyLinks(ctx context.Context, tenantID, assetID string, limit int) ([]*config.TopologyLink, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListTopologyLinks(ctx, tenantID, assetID, limit)
}

func (s *AssetService) RecordAuditLog(ctx context.Context, tenantID, userID, action, objectType, objectID string, detail map[string]interface{}, ipAddr, userAgent string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.InsertAuditLog(ctx, tenantID, userID, action, objectType, objectID, detail, ipAddr, userAgent)
}

func (s *AssetService) recordDiscoveryObservation(ctx context.Context, tenantID, mode, runID string, observation config.DiscoveryObservation) (string, bool, error) {
	if strings.TrimSpace(observation.MACAddress) == "" {
		return "", false, errors.New(errors.ErrCodeInvalidParameter, "observation mac_address is required")
	}
	rec := &config.AssetRecord{
		AssetID:    uuid.New().String(),
		TenantID:   tenantID,
		IPAddress:  strings.TrimSpace(observation.IPAddress),
		MACAddress: normalizeMAC(observation.MACAddress),
		Hostname:   strings.TrimSpace(observation.Hostname),
		Vendor:     strings.TrimSpace(observation.Vendor),
		OSType:     strings.TrimSpace(observation.OSType),
		Source:     "active:" + mode,
		VlanID:     strings.TrimSpace(observation.VlanID),
		SwitchPort: strings.TrimSpace(observation.SwitchPort),
	}
	id, _, err := s.UpsertAsset(ctx, rec)
	if err != nil {
		return "", false, err
	}
	if runID != "" {
		s.repo.InsertEvent(ctx, id, tenantID, "active_discovered", "", fmt.Sprintf(`{"run_id":"%s","mode":"%s"}`, runID, mode))
	}
	return id, true, nil
}

func (s *AssetService) recordDiscoveryNeighbors(ctx context.Context, tenantID, mode, runID, sourceAssetID string, observation config.DiscoveryObservation) (int, int) {
	accepted, rejected := 0, 0
	sourceMAC := normalizeMAC(observation.MACAddress)
	for _, neighbor := range observation.Neighbors {
		if strings.TrimSpace(neighbor.MACAddress) == "" && strings.TrimSpace(neighbor.IPAddress) == "" {
			rejected++
			continue
		}
		neighborMAC := ""
		neighborAssetID := ""
		if strings.TrimSpace(neighbor.MACAddress) != "" {
			neighborMAC = normalizeMAC(neighbor.MACAddress)
			rec := &config.AssetRecord{
				AssetID:    uuid.New().String(),
				TenantID:   tenantID,
				IPAddress:  strings.TrimSpace(neighbor.IPAddress),
				MACAddress: neighborMAC,
				Hostname:   strings.TrimSpace(neighbor.Hostname),
				Source:     "active:" + mode,
				VlanID:     strings.TrimSpace(neighbor.VlanID),
				SwitchPort: strings.TrimSpace(neighbor.Interface),
			}
			if id, _, err := s.UpsertAsset(ctx, rec); err == nil {
				neighborAssetID = id
			}
		}
		protocol := normalizeDiscoveryMode(neighbor.Protocol)
		if protocol == "" {
			protocol = mode
		}
		link := &config.TopologyLink{
			LinkID:            uuid.New().String(),
			TenantID:          tenantID,
			RunID:             runID,
			SourceAssetID:     sourceAssetID,
			SourceMAC:         sourceMAC,
			SourceIP:          strings.TrimSpace(observation.IPAddress),
			SourceInterface:   strings.TrimSpace(observation.SwitchPort),
			NeighborAssetID:   neighborAssetID,
			NeighborMAC:       neighborMAC,
			NeighborIP:        strings.TrimSpace(neighbor.IPAddress),
			NeighborInterface: strings.TrimSpace(neighbor.Interface),
			Protocol:          protocol,
			Confidence:        90,
			ObservedAt:        time.Now(),
		}
		if err := s.repo.UpsertTopologyLink(ctx, link); err != nil {
			s.logger.Warn("active discovery topology link rejected", zap.Error(err), zap.String("run_id", runID))
			rejected++
			continue
		}
		accepted++
	}
	return accepted, rejected
}

func normalizeDiscoveryMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	mode = strings.ReplaceAll(mode, "-", "_")
	if mode == "snmp+lldp" || mode == "lldp_snmp" {
		return config.DiscoveryModeSNMPLLDP
	}
	return mode
}

func isDiscoveryModeAllowed(mode string) bool {
	switch mode {
	case config.DiscoveryModeSNMP, config.DiscoveryModeLLDP, config.DiscoveryModeSNMPLLDP:
		return true
	default:
		return false
	}
}
