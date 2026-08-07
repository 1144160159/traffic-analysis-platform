package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

const (
	ShadowStatusMatched = "MATCHED"
	ShadowStatusDiff    = "DIFF"
	ShadowStatusPartial = "PARTIAL"

	ShadowApprovalReady         = "READY_FOR_BOUNDED_REPAIR_REVIEW"
	ShadowApprovalNone          = "NO_AUTOMATIC_REPAIR_REQUIRED"
	ShadowApprovalBlocked       = "BLOCKED"
	ShadowClassificationMissing = "missing"
	ShadowClassificationExtra   = "extra"
	ShadowClassificationStale   = "stale"
)

type ShadowConfig struct {
	MaxDocuments     int
	MaxWindow        time.Duration
	MinimumWindowAge time.Duration
	Now              func() time.Time
}

type ShadowWriteIndex struct {
	Index        string `json:"index"`
	IsWriteIndex bool   `json:"is_write_index"`
}

type ShadowTargetMetadata struct {
	ClusterUUID  string             `json:"cluster_uuid"`
	ReadTarget   string             `json:"read_target"`
	WriteAlias   string             `json:"write_alias"`
	WriteIndices []ShadowWriteIndex `json:"write_indices"`
}

type ShadowRequest struct {
	RequestedBy   string
	TraceID       string
	EnvironmentID string
	Scope         persistence.ProjectionScope
	Target        ShadowTargetMetadata
}

type ShadowDifference struct {
	AlertID        string `json:"alert_id"`
	Classification string `json:"classification"`
	SourceSHA256   string `json:"source_sha256,omitempty"`
	TargetSHA256   string `json:"target_sha256,omitempty"`
}

type ShadowBinding struct {
	EnvironmentID string               `json:"environment_id"`
	TenantID      string               `json:"tenant_id"`
	StartTime     string               `json:"start_time"`
	EndTime       string               `json:"end_time"`
	MaxDocuments  int                  `json:"max_documents"`
	Target        ShadowTargetMetadata `json:"target"`
	SourceCount   int                  `json:"source_count"`
	TargetCount   int                  `json:"target_count"`
	Differences   []ShadowDifference   `json:"differences"`
}

type ShadowManifest struct {
	SchemaVersion       int           `json:"schema_version"`
	RemediationID       string        `json:"remediation_id"`
	Mode                string        `json:"mode"`
	Status              string        `json:"status"`
	ApprovalReadiness   string        `json:"approval_readiness"`
	CapturedAt          string        `json:"captured_at"`
	RequestedBy         string        `json:"requested_by"`
	TraceID             string        `json:"trace_id"`
	BindingSHA256       string        `json:"binding_sha256"`
	Binding             ShadowBinding `json:"binding"`
	MissingCount        int           `json:"missing_count"`
	StaleCount          int           `json:"stale_count"`
	ExtraCount          int           `json:"extra_count"`
	SourceTruncated     bool          `json:"source_truncated"`
	TargetTruncated     bool          `json:"target_truncated"`
	Blockers            []string      `json:"blockers"`
	Warnings            []string      `json:"warnings"`
	ReadOnlyOperations  []string      `json:"read_only_operations"`
	ProductionApplied   bool          `json:"production_applied"`
	ProductionMutations []string      `json:"production_mutations"`
}

func BuildShadowManifest(
	ctx context.Context,
	config ShadowConfig,
	source ProjectionReader,
	target ProjectionReader,
	request ShadowRequest,
) (ShadowManifest, error) {
	manifest := ShadowManifest{
		SchemaVersion:       1,
		RemediationID:       "T-OS-004",
		Mode:                "READ_ONLY_SHADOW",
		ApprovalReadiness:   ShadowApprovalBlocked,
		ReadOnlyOperations:  []string{"clickhouse_projection_select", "opensearch_projection_search", "opensearch_info", "opensearch_get_alias"},
		ProductionApplied:   false,
		ProductionMutations: []string{},
		Blockers:            []string{},
		Warnings:            []string{},
	}
	if source == nil || target == nil {
		return manifest, fmt.Errorf("shadow source and target readers are required")
	}
	if config.MaxDocuments < 1 || config.MaxDocuments > 100000 {
		return manifest, fmt.Errorf("invalid shadow max document budget")
	}
	if config.MaxWindow <= 0 || config.MaxWindow > time.Hour {
		return manifest, fmt.Errorf("shadow max window must be positive and no more than one hour")
	}
	if config.MinimumWindowAge < 15*time.Minute {
		return manifest, fmt.Errorf("shadow minimum window age cannot be less than 15 minutes")
	}
	now := time.Now().UTC()
	if config.Now != nil {
		now = config.Now().UTC()
	}
	request.Scope.TenantID = strings.TrimSpace(request.Scope.TenantID)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	request.TraceID = strings.TrimSpace(request.TraceID)
	request.EnvironmentID = strings.TrimSpace(request.EnvironmentID)
	request.Target.ClusterUUID = strings.TrimSpace(request.Target.ClusterUUID)
	request.Target.ReadTarget = strings.TrimSpace(request.Target.ReadTarget)
	request.Target.WriteAlias = strings.TrimSpace(request.Target.WriteAlias)
	request.Target.WriteIndices = append([]ShadowWriteIndex(nil), request.Target.WriteIndices...)
	for index := range request.Target.WriteIndices {
		request.Target.WriteIndices[index].Index = strings.TrimSpace(request.Target.WriteIndices[index].Index)
	}
	sort.Slice(request.Target.WriteIndices, func(i, j int) bool {
		if request.Target.WriteIndices[i].Index == request.Target.WriteIndices[j].Index {
			return !request.Target.WriteIndices[i].IsWriteIndex && request.Target.WriteIndices[j].IsWriteIndex
		}
		return request.Target.WriteIndices[i].Index < request.Target.WriteIndices[j].Index
	})
	if request.Scope.TenantID == "" || strings.ContainsAny(request.Scope.TenantID, "*?[]") {
		return manifest, fmt.Errorf("shadow tenant must be one explicit non-wildcard tenant")
	}
	if request.RequestedBy == "" || request.TraceID == "" || request.EnvironmentID == "" {
		return manifest, fmt.Errorf("requested_by, trace_id and environment_id are required")
	}
	if request.Scope.StartTime.IsZero() || request.Scope.EndTime.IsZero() {
		return manifest, fmt.Errorf("shadow requires an explicit closed start and end time")
	}
	if !request.Scope.EndTime.After(request.Scope.StartTime) {
		return manifest, fmt.Errorf("shadow end_time must be after start_time")
	}
	if request.Scope.EndTime.Sub(request.Scope.StartTime) > config.MaxWindow {
		return manifest, fmt.Errorf("shadow window exceeds %s", config.MaxWindow)
	}
	if request.Scope.EndTime.After(now.Add(-config.MinimumWindowAge)) {
		return manifest, fmt.Errorf("shadow window has not reached the minimum stability age")
	}
	if request.Scope.MaxDocuments < 1 || request.Scope.MaxDocuments > config.MaxDocuments {
		return manifest, fmt.Errorf("shadow max_documents exceeds approved budget %d", config.MaxDocuments)
	}
	if request.Target.ClusterUUID == "" || request.Target.ReadTarget == "" || request.Target.WriteAlias == "" ||
		strings.ContainsAny(request.Target.ReadTarget, "*?[]") || strings.ContainsAny(request.Target.WriteAlias, "*?[]") {
		return manifest, fmt.Errorf("shadow target metadata requires exact cluster, read target and write alias identities")
	}
	if request.Scope.TargetIndexVersion != request.Target.WriteAlias {
		return manifest, fmt.Errorf("shadow target version differs from the approved write alias")
	}

	sourceAlerts, sourceTruncated, err := source.ListProjectionAlerts(ctx, request.Scope)
	if err != nil {
		return manifest, fmt.Errorf("read ClickHouse authoritative shadow: %w", err)
	}
	targetAlerts, targetTruncated, err := target.ListProjectionAlerts(ctx, request.Scope)
	if err != nil {
		return manifest, fmt.Errorf("read OpenSearch projection shadow: %w", err)
	}
	sourceByID, err := projectionMap(sourceAlerts)
	if err != nil {
		return manifest, fmt.Errorf("invalid ClickHouse shadow identity: %w", err)
	}
	targetByID, err := projectionMap(targetAlerts)
	if err != nil {
		return manifest, fmt.Errorf("invalid OpenSearch shadow identity: %w", err)
	}
	missing, extra, stale := projectionDiff(sourceByID, targetByID)
	differences := make([]ShadowDifference, 0, len(missing)+len(extra)+len(stale))
	for _, alertID := range missing {
		sourceSHA, hashErr := projectionSHA256(sourceByID[alertID])
		if hashErr != nil {
			return manifest, fmt.Errorf("hash missing ClickHouse projection %s: %w", alertID, hashErr)
		}
		differences = append(differences, ShadowDifference{AlertID: alertID, Classification: ShadowClassificationMissing, SourceSHA256: sourceSHA})
	}
	for _, alertID := range stale {
		sourceSHA, hashErr := projectionSHA256(sourceByID[alertID])
		if hashErr != nil {
			return manifest, fmt.Errorf("hash stale ClickHouse projection %s: %w", alertID, hashErr)
		}
		targetSHA, hashErr := projectionSHA256(targetByID[alertID])
		if hashErr != nil {
			return manifest, fmt.Errorf("hash stale OpenSearch projection %s: %w", alertID, hashErr)
		}
		differences = append(differences, ShadowDifference{AlertID: alertID, Classification: ShadowClassificationStale, SourceSHA256: sourceSHA, TargetSHA256: targetSHA})
	}
	for _, alertID := range extra {
		targetSHA, hashErr := projectionSHA256(targetByID[alertID])
		if hashErr != nil {
			return manifest, fmt.Errorf("hash extra OpenSearch projection %s: %w", alertID, hashErr)
		}
		differences = append(differences, ShadowDifference{AlertID: alertID, Classification: ShadowClassificationExtra, TargetSHA256: targetSHA})
	}
	sort.Slice(differences, func(i, j int) bool {
		if differences[i].AlertID == differences[j].AlertID {
			return differences[i].Classification < differences[j].Classification
		}
		return differences[i].AlertID < differences[j].AlertID
	})
	manifest.CapturedAt = now.Format(time.RFC3339Nano)
	manifest.RequestedBy = request.RequestedBy
	manifest.TraceID = request.TraceID
	manifest.MissingCount = len(missing)
	manifest.StaleCount = len(stale)
	manifest.ExtraCount = len(extra)
	manifest.SourceTruncated = sourceTruncated
	manifest.TargetTruncated = targetTruncated
	manifest.Binding = ShadowBinding{
		EnvironmentID: request.EnvironmentID,
		TenantID:      request.Scope.TenantID,
		StartTime:     request.Scope.StartTime.UTC().Format(time.RFC3339Nano),
		EndTime:       request.Scope.EndTime.UTC().Format(time.RFC3339Nano),
		MaxDocuments:  request.Scope.MaxDocuments,
		Target:        request.Target,
		SourceCount:   len(sourceAlerts),
		TargetCount:   len(targetAlerts),
		Differences:   differences,
	}
	bindingPayload, err := json.Marshal(manifest.Binding)
	if err != nil {
		return manifest, fmt.Errorf("encode shadow binding: %w", err)
	}
	digest := sha256.Sum256(bindingPayload)
	manifest.BindingSHA256 = hex.EncodeToString(digest[:])
	writeIndices := 0
	for _, index := range request.Target.WriteIndices {
		if index.IsWriteIndex && strings.TrimSpace(index.Index) != "" {
			writeIndices++
		}
	}
	if sourceTruncated || targetTruncated {
		manifest.Status = ShadowStatusPartial
		manifest.Blockers = append(manifest.Blockers, "bounded scope was truncated; split into smaller closed windows")
	} else if len(differences) == 0 {
		manifest.Status = ShadowStatusMatched
	} else {
		manifest.Status = ShadowStatusDiff
	}
	if writeIndices != 1 {
		manifest.Blockers = append(manifest.Blockers, "target write alias does not resolve to exactly one write index")
	}
	if manifest.ExtraCount > 0 {
		manifest.Warnings = append(manifest.Warnings, "extra OpenSearch documents require manual adjudication and must never be auto-deleted")
	}
	if len(manifest.Blockers) == 0 {
		if manifest.MissingCount+manifest.StaleCount > 0 {
			manifest.ApprovalReadiness = ShadowApprovalReady
		} else {
			manifest.ApprovalReadiness = ShadowApprovalNone
		}
	}
	return manifest, nil
}

func projectionSHA256(alert *persistence.Alert) (string, error) {
	return persistence.AlertProjectionSHA256(alert)
}
