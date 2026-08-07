package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

const (
	discoveryCredentialActionID = "asset-discovery-credential-upsert"
	discoveryTopologyActionID   = "asset-discovery-topology-link-upsert"
)

func (s *AssetService) RegisterDiscoveryCredential(
	ctx context.Context,
	credential *config.DiscoveryCredential,
	command config.DiscoveryResourceCommand,
) (*config.DiscoveryCredential, error) {
	if credential == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "discovery credential is required")
	}
	credential.TenantID = strings.TrimSpace(credential.TenantID)
	credential.Name = strings.TrimSpace(credential.Name)
	credential.Protocol = normalizeDiscoveryMode(credential.Protocol)
	credential.SecretRef = strings.TrimSpace(credential.SecretRef)
	credential.ActionID = strings.TrimSpace(credential.ActionID)
	credential.Reason = strings.TrimSpace(credential.Reason)
	if credential.ActionID == "" {
		credential.ActionID = discoveryCredentialActionID
	}
	command.ActionID = credential.ActionID
	command.ExpectedRevision = credential.ExpectedRevision
	command.Reason = credential.Reason
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
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
	if command.ActionID != discoveryCredentialActionID {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported credential action_id")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "authenticated actor and trace_id are required")
	}
	if len(command.Reason) < 4 || len(command.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}
	if command.ExpectedRevision < 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_revision must be non-negative")
	}
	if credential.CredentialID == "" {
		credential.CredentialID = uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("traffic.asset.discovery.credential:"+credential.TenantID+":"+credential.Name),
		).String()
	}
	normalized, _ := json.Marshal(struct {
		CredentialID     string `json:"credential_id"`
		TenantID         string `json:"tenant_id"`
		Name             string `json:"name"`
		Protocol         string `json:"protocol"`
		Endpoint         string `json:"endpoint"`
		SecretRef        string `json:"secret_ref"`
		ExpectedRevision int64  `json:"expected_revision"`
		Actor            string `json:"actor"`
		Reason           string `json:"reason"`
	}{credential.CredentialID, credential.TenantID, credential.Name, credential.Protocol,
		credential.Endpoint, credential.SecretRef, command.ExpectedRevision, command.Actor, command.Reason})
	requestHash := sha256.Sum256(normalized)
	created, err := s.repo.UpsertDiscoveryCredentialAtomic(ctx, credential, command, fmt.Sprintf("%x", requestHash[:]))
	if err != nil {
		return nil, fmt.Errorf("register discovery credential: %w", err)
	}
	return created, nil
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

func (s *AssetService) RunActiveDiscovery(
	ctx context.Context,
	req *config.ActiveDiscoveryRequest,
	command config.DiscoveryJobCommand,
) (*config.DiscoveryResult, error) {
	if req == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "discovery request is required")
	}
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.Mode = normalizeDiscoveryMode(req.Mode)
	req.ActionID = strings.TrimSpace(req.ActionID)
	if req.ActionID == "" {
		req.ActionID = discoveryActionID
	}
	req.Reason = strings.TrimSpace(req.Reason)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if req.TenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if req.ActionID != discoveryActionID {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported discovery action_id")
	}
	if !isDiscoveryModeAllowed(req.Mode) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "mode must be snmp, lldp, or snmp_lldp")
	}
	if command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "authenticated actor and trace_id are required")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if len(req.Reason) < 4 || len(req.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}

	normalized, _ := json.Marshal(struct {
		Request *config.ActiveDiscoveryRequest `json:"request"`
		Actor   string                         `json:"actor"`
	}{Request: req, Actor: command.Actor})
	requestHash := sha256.Sum256(normalized)
	now := time.Now().UTC()
	run := &config.DiscoveryRun{
		RunID:        uuid.New().String(),
		TenantID:     req.TenantID,
		ActionID:     req.ActionID,
		Mode:         req.Mode,
		TargetCIDR:   strings.TrimSpace(req.TargetCIDR),
		CredentialID: strings.TrimSpace(req.CredentialID),
		Status:       config.DiscoveryStatusQueued,
		Revision:     1,
		RequestedBy:  command.Actor,
		Reason:       req.Reason,
		RateLimit:    req.RateLimit,
		SecurityFrom: req.SecurityFrom,
		SecurityTo:   req.SecurityTo,
		ApprovedBy:   strings.TrimSpace(req.ApprovedBy),
		TraceID:      command.TraceID,
		QueuedAt:     now,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	created, err := s.repo.CreateDiscoveryJobAtomic(ctx, run, command, fmt.Sprintf("%x", requestHash[:]))
	if err != nil {
		return nil, fmt.Errorf("create discovery run: %w", err)
	}
	if created.IdempotentReplay {
		switch created.Status {
		case config.DiscoveryStatusSucceeded, config.DiscoveryStatusPartial, config.DiscoveryStatusFailed:
			return &config.DiscoveryResult{
				Run: created, AcceptedAssets: created.DiscoveredAssets,
				AcceptedLinks: created.DiscoveredLinks, RejectedRecords: created.RejectedRecords,
			}, nil
		default:
			return nil, fmt.Errorf("%w: run %s is %s", repository.ErrDiscoveryStateConflict, created.RunID, created.Status)
		}
	}
	run, err = s.repo.StartLegacyDiscoveryRunAtomic(ctx, created, command)
	if err != nil {
		return nil, fmt.Errorf("start discovery run: %w", err)
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
			return s.failDiscoveryRun(ctx, run, result, command, err)
		}
	}

	actor := strings.TrimSpace(run.RequestedBy)
	if actor == "" {
		actor = "asset-discovery-worker"
	}
	for observationIndex, observation := range observations {
		assetID, ok, err := s.recordDiscoveryObservation(
			ctx, req.TenantID, req.Mode, run.RunID, actor, run.StartedAt, observationIndex, observation,
		)
		if err != nil {
			s.logger.Warn("active discovery observation rejected", zap.Error(err), zap.String("run_id", run.RunID))
			result.RejectedRecords++
			continue
		}
		if ok {
			result.AcceptedAssets++
		}
		links, rejected := s.recordDiscoveryNeighbors(
			ctx, req.TenantID, req.Mode, run.RunID, actor, run.StartedAt,
			observationIndex, assetID, observation,
		)
		result.AcceptedLinks += links
		result.RejectedRecords += rejected
	}

	status := config.DiscoveryStatusSucceeded
	if result.RejectedRecords > 0 {
		status = config.DiscoveryStatusPartial
	}
	if scanAttempted && len(observations) == 0 {
		return s.failDiscoveryRun(ctx, run, result, command, fmt.Errorf("active discovery scan returned no observations"))
	}
	completed, err := s.repo.CompleteLegacyDiscoveryRunAtomic(
		ctx, run, status, "", result.AcceptedAssets, result.AcceptedLinks,
		result.RejectedRecords, command,
	)
	if err != nil {
		return nil, fmt.Errorf("complete discovery run: %w", err)
	}
	result.Run = completed
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

func (s *AssetService) failDiscoveryRun(
	ctx context.Context,
	run *config.DiscoveryRun,
	result *config.DiscoveryResult,
	command config.DiscoveryJobCommand,
	cause error,
) (*config.DiscoveryResult, error) {
	completed, err := s.repo.CompleteLegacyDiscoveryRunAtomic(
		ctx, run, config.DiscoveryStatusFailed, cause.Error(), result.AcceptedAssets,
		result.AcceptedLinks, result.RejectedRecords, command,
	)
	if err != nil {
		return nil, fmt.Errorf("complete failed discovery run: %w", err)
	}
	result.Run = completed
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
	if s.DiscoveryJobsV2Enabled() {
		return s.repo.ListDiscoveryJobs(ctx, tenantID, limit)
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

func (s *AssetService) recordDiscoveryObservation(
	ctx context.Context,
	tenantID, mode, runID, actor string,
	observedAt time.Time,
	observationIndex int,
	observation config.DiscoveryObservation,
) (string, bool, error) {
	if strings.TrimSpace(observation.MACAddress) == "" {
		return "", false, errors.New(errors.ErrCodeInvalidParameter, "observation mac_address is required")
	}
	rec := &config.AssetRecord{
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
	result, err := s.UpsertAssetAtomic(ctx, rec, config.AssetUpsertCommand{
		ActionID:               config.AssetObservationUpsertAction,
		ResolveCurrentRevision: true,
		IdempotencyKey: stableAssetCommandKey(
			"asset-discovery-observation",
			fmt.Sprintf("%s:primary:%d:%s", runID, observationIndex, rec.MACAddress),
		),
		Actor:            actor,
		Reason:           fmt.Sprintf("active discovery run %s (%s)", runID, mode),
		HistoryEventType: "active_discovered",
		ObservedAt:       observedAt,
		TraceID:          "asset-discovery:" + runID,
		RequestID:        runID,
	})
	if err != nil {
		return "", false, err
	}
	return result.AssetID, true, nil
}

func (s *AssetService) recordDiscoveryNeighbors(
	ctx context.Context,
	tenantID, mode, runID, actor string,
	observedAt time.Time,
	observationIndex int,
	sourceAssetID string,
	observation config.DiscoveryObservation,
) (int, int) {
	accepted, rejected := 0, 0
	sourceMAC := normalizeMAC(observation.MACAddress)
	for neighborIndex, neighbor := range observation.Neighbors {
		if strings.TrimSpace(neighbor.MACAddress) == "" && strings.TrimSpace(neighbor.IPAddress) == "" {
			rejected++
			continue
		}
		neighborMAC := ""
		neighborAssetID := ""
		if strings.TrimSpace(neighbor.MACAddress) != "" {
			neighborMAC = normalizeMAC(neighbor.MACAddress)
			rec := &config.AssetRecord{
				TenantID:   tenantID,
				IPAddress:  strings.TrimSpace(neighbor.IPAddress),
				MACAddress: neighborMAC,
				Hostname:   strings.TrimSpace(neighbor.Hostname),
				Source:     "active:" + mode,
				VlanID:     strings.TrimSpace(neighbor.VlanID),
				SwitchPort: strings.TrimSpace(neighbor.Interface),
			}
			result, upsertErr := s.UpsertAssetAtomic(ctx, rec, config.AssetUpsertCommand{
				ActionID:               config.AssetObservationUpsertAction,
				ResolveCurrentRevision: true,
				IdempotencyKey: stableAssetCommandKey(
					"asset-discovery-neighbor",
					fmt.Sprintf("%s:%d:%d:%s", runID, observationIndex, neighborIndex, neighborMAC),
				),
				Actor:            actor,
				Reason:           fmt.Sprintf("active discovery neighbor in run %s (%s)", runID, mode),
				HistoryEventType: "active_discovered",
				ObservedAt:       observedAt,
				TraceID:          "asset-discovery:" + runID,
				RequestID:        runID,
			})
			if upsertErr == nil {
				neighborAssetID = result.AssetID
			} else {
				s.logger.Warn("active discovery neighbor asset rejected", zap.Error(upsertErr), zap.String("run_id", runID))
				rejected++
				continue
			}
		}
		protocol := normalizeDiscoveryMode(neighbor.Protocol)
		if protocol == "" {
			protocol = mode
		}
		linkIdentity := fmt.Sprintf("%s:%s:%s:%s:%s:%s",
			tenantID, sourceMAC, neighborMAC, protocol,
			strings.TrimSpace(observation.SwitchPort), strings.TrimSpace(neighbor.Interface))
		link := &config.TopologyLink{
			LinkID: uuid.NewSHA1(
				uuid.NameSpaceURL,
				[]byte("traffic.asset.discovery.topology:"+linkIdentity),
			).String(),
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
			ObservedAt:        observedAt,
		}
		command := config.DiscoveryResourceCommand{
			ActionID:               discoveryTopologyActionID,
			ResolveCurrentRevision: true,
			IdempotencyKey: stableAssetCommandKey(
				"asset-discovery-topology",
				fmt.Sprintf("%s:%d:%d:%s", runID, observationIndex, neighborIndex, linkIdentity),
			),
			Actor:     actor,
			Reason:    fmt.Sprintf("active discovery topology link in run %s (%s)", runID, mode),
			TraceID:   "asset-discovery:" + runID,
			RequestID: runID,
		}
		normalized, _ := json.Marshal(struct {
			Link   *config.TopologyLink `json:"link"`
			Actor  string               `json:"actor"`
			Reason string               `json:"reason"`
		}{link, command.Actor, command.Reason})
		requestHash := sha256.Sum256(normalized)
		if _, err := s.repo.UpsertTopologyLinkAtomic(ctx, link, command, fmt.Sprintf("%x", requestHash[:])); err != nil {
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
