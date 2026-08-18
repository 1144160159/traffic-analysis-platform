package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

var (
	ErrReconcileBudget       = errors.New("graph reconcile query budget exceeded")
	ErrRepairNotAuthorized   = errors.New("graph reconcile repair is not authorized")
	ErrReconcileNotConverged = errors.New("graph projection did not converge after repair")
)

type ReconcileScope struct {
	TenantID      string        `json:"tenant_id"`
	WindowFrom    time.Time     `json:"window_from"`
	WindowThrough time.Time     `json:"window_through"`
	MaxFacts      int           `json:"max_facts"`
	MaxDuration   time.Duration `json:"max_duration"`
}

type ProjectionFact struct {
	Kind             string `json:"kind"`
	ProjectionID     string `json:"projection_id"`
	AggregateVersion uint64 `json:"aggregate_version"`
	ProjectionSHA256 string `json:"projection_sha256"`
	Revoked          bool   `json:"revoked"`
}

func (fact ProjectionFact) key() string { return fact.Kind + ":" + fact.ProjectionID }

type ReconcileSnapshot struct {
	Facts  []ProjectionFact `json:"facts"`
	SHA256 string           `json:"sha256"`
}

type ReconcileDifference struct {
	Class          string          `json:"class"`
	Kind           string          `json:"kind"`
	ProjectionID   string          `json:"projection_id"`
	Authority      *ProjectionFact `json:"authority,omitempty"`
	Target         *ProjectionFact `json:"target,omitempty"`
	RepairEligible bool            `json:"repair_eligible"`
}

type QueryProfile struct {
	Authority string `json:"authority"`
	Target    string `json:"target"`
}

type ReconcileManifest struct {
	RunID          string                `json:"run_id"`
	Phase          string                `json:"phase"`
	Scope          ReconcileScope        `json:"scope"`
	Authority      ReconcileSnapshot     `json:"authority"`
	Target         ReconcileSnapshot     `json:"target"`
	Differences    []ReconcileDifference `json:"differences"`
	MissingCount   int                   `json:"missing_count"`
	StaleCount     int                   `json:"stale_count"`
	ExtraCount     int                   `json:"extra_count"`
	Converged      bool                  `json:"converged"`
	ExtraPreserved bool                  `json:"extra_preserved"`
	Profile        QueryProfile          `json:"profile"`
	CreatedAt      time.Time             `json:"created_at"`
	ManifestSHA256 string                `json:"manifest_sha256"`
}

type SnapshotReader interface {
	LoadReconcileSnapshot(context.Context, ReconcileScope) ([]ProjectionFact, string, error)
}

type RepairSource interface {
	LoadProjectionEvents(context.Context, ReconcileScope, []ProjectionFact) ([]*trafficv1.GraphProjectionEvent, error)
}

type ManifestRecorder interface {
	RecordReconcileManifest(context.Context, ReconcileManifest) error
}

type RepairAuthorizationRecorder interface {
	RecordRepairAuthorization(context.Context, RepairAuthorization) error
}

type ReconcileService struct {
	authority SnapshotReader
	target    SnapshotReader
	repair    RepairSource
	apply     Target
	recorder  ManifestRecorder
	clock     func() time.Time
}

func NewReconcileService(
	authority SnapshotReader,
	target SnapshotReader,
	repair RepairSource,
	apply Target,
	recorder ManifestRecorder,
) (*ReconcileService, error) {
	if authority == nil || target == nil {
		return nil, fmt.Errorf("graph reconcile authority and target readers are required")
	}
	return &ReconcileService{
		authority: authority, target: target, repair: repair, apply: apply,
		recorder: recorder, clock: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (service *ReconcileService) Compare(ctx context.Context, runID, phase string, scope ReconcileScope) (ReconcileManifest, error) {
	if err := validateReconcileScope(runID, phase, scope); err != nil {
		return ReconcileManifest{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, scope.MaxDuration)
	defer cancel()
	authorityFacts, authorityProfile, err := service.authority.LoadReconcileSnapshot(queryCtx, scope)
	if err != nil {
		return ReconcileManifest{}, fmt.Errorf("load authoritative graph projection set: %w", err)
	}
	targetFacts, targetProfile, err := service.target.LoadReconcileSnapshot(queryCtx, scope)
	if err != nil {
		return ReconcileManifest{}, fmt.Errorf("load NebulaGraph projection set: %w", err)
	}
	if len(authorityFacts) > scope.MaxFacts || len(targetFacts) > scope.MaxFacts {
		return ReconcileManifest{}, fmt.Errorf("%w: authority=%d target=%d max=%d", ErrReconcileBudget, len(authorityFacts), len(targetFacts), scope.MaxFacts)
	}
	authoritySnapshot, err := canonicalSnapshot(authorityFacts)
	if err != nil {
		return ReconcileManifest{}, fmt.Errorf("canonicalize authoritative graph projection set: %w", err)
	}
	targetSnapshot, err := canonicalSnapshot(targetFacts)
	if err != nil {
		return ReconcileManifest{}, fmt.Errorf("canonicalize NebulaGraph projection set: %w", err)
	}
	differences := compareSnapshots(authoritySnapshot.Facts, targetSnapshot.Facts)
	manifest := ReconcileManifest{
		RunID: runID, Phase: phase, Scope: normalizeScope(scope),
		Authority: authoritySnapshot, Target: targetSnapshot, Differences: differences,
		Converged: len(differences) == 0, ExtraPreserved: true,
		Profile:   QueryProfile{Authority: authorityProfile, Target: targetProfile},
		CreatedAt: service.clock(),
	}
	for _, difference := range differences {
		switch difference.Class {
		case "missing":
			manifest.MissingCount++
		case "stale":
			manifest.StaleCount++
		case "extra":
			manifest.ExtraCount++
		}
	}
	manifest.ManifestSHA256, err = manifestSHA256(manifest)
	if err != nil {
		return ReconcileManifest{}, err
	}
	if service.recorder != nil {
		if err := service.recorder.RecordReconcileManifest(ctx, manifest); err != nil {
			return ReconcileManifest{}, fmt.Errorf("record graph reconcile manifest: %w", err)
		}
	}
	return manifest, nil
}

type RepairAuthorization struct {
	RunID       string    `json:"run_id"`
	RequestedBy string    `json:"requested_by"`
	ApprovedBy  string    `json:"approved_by"`
	ApprovedAt  time.Time `json:"approved_at"`
	MaxItems    int       `json:"max_items"`
}

// Repair reapplies only authority-owned missing or stale facts. Target-only
// extras are deliberately preserved and must never be deleted by this path.
func (service *ReconcileService) Repair(
	ctx context.Context,
	before ReconcileManifest,
	authorization RepairAuthorization,
) (ReconcileManifest, error) {
	if service.repair == nil || service.apply == nil {
		return ReconcileManifest{}, fmt.Errorf("%w: repair source or target is unavailable", ErrRepairNotAuthorized)
	}
	if authorization.RunID != before.RunID || strings.TrimSpace(authorization.RequestedBy) == "" ||
		strings.TrimSpace(authorization.ApprovedBy) == "" ||
		authorization.RequestedBy == authorization.ApprovedBy || authorization.ApprovedAt.IsZero() ||
		authorization.ApprovedAt.After(service.clock()) || authorization.MaxItems <= 0 {
		return ReconcileManifest{}, ErrRepairNotAuthorized
	}
	eligible := make([]ProjectionFact, 0, before.MissingCount+before.StaleCount)
	for _, difference := range before.Differences {
		if !difference.RepairEligible || difference.Authority == nil {
			continue
		}
		eligible = append(eligible, *difference.Authority)
	}
	if len(eligible) > authorization.MaxItems || len(eligible) > before.Scope.MaxFacts {
		return ReconcileManifest{}, fmt.Errorf("%w: repair=%d approved=%d", ErrReconcileBudget, len(eligible), authorization.MaxItems)
	}
	if recorder, ok := service.recorder.(RepairAuthorizationRecorder); ok {
		if err := recorder.RecordRepairAuthorization(ctx, authorization); err != nil {
			return ReconcileManifest{}, fmt.Errorf("record graph repair authorization: %w", err)
		}
	}
	events, err := service.repair.LoadProjectionEvents(ctx, before.Scope, eligible)
	if err != nil {
		return ReconcileManifest{}, fmt.Errorf("load graph projection repair events: %w", err)
	}
	if len(events) != len(eligible) {
		return ReconcileManifest{}, fmt.Errorf("graph projection repair source returned %d events for %d facts", len(events), len(eligible))
	}
	for _, event := range events {
		if err := ValidateEvent(event); err != nil {
			return ReconcileManifest{}, fmt.Errorf("validate graph projection repair event: %w", err)
		}
		if err := service.apply.Apply(ctx, event); err != nil {
			return ReconcileManifest{}, fmt.Errorf("reapply graph projection: %w", err)
		}
	}
	after, err := service.Compare(ctx, before.RunID, "after", before.Scope)
	if err != nil {
		return ReconcileManifest{}, err
	}
	if after.MissingCount != 0 || after.StaleCount != 0 {
		return after, ErrReconcileNotConverged
	}
	return after, nil
}

func validateReconcileScope(runID, phase string, scope ReconcileScope) error {
	if strings.TrimSpace(runID) == "" || (phase != "before" && phase != "after") ||
		strings.TrimSpace(scope.TenantID) == "" || scope.WindowFrom.IsZero() || scope.WindowThrough.IsZero() ||
		!scope.WindowFrom.Before(scope.WindowThrough) || scope.WindowThrough.After(time.Now().UTC()) ||
		scope.MaxFacts <= 0 || scope.MaxFacts > 100000 || scope.MaxDuration < time.Second || scope.MaxDuration > 5*time.Minute {
		return fmt.Errorf("invalid graph reconcile closed-window scope")
	}
	return nil
}

func normalizeScope(scope ReconcileScope) ReconcileScope {
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.WindowFrom = scope.WindowFrom.UTC()
	scope.WindowThrough = scope.WindowThrough.UTC()
	return scope
}

func canonicalSnapshot(facts []ProjectionFact) (ReconcileSnapshot, error) {
	values := append([]ProjectionFact(nil), facts...)
	seen := make(map[string]struct{}, len(values))
	for _, fact := range values {
		if (fact.Kind != "entity" && fact.Kind != "relation") || strings.TrimSpace(fact.ProjectionID) == "" ||
			fact.AggregateVersion == 0 || !isSHA256(fact.ProjectionSHA256) {
			return ReconcileSnapshot{}, fmt.Errorf("invalid projection fact %q", fact.key())
		}
		if _, exists := seen[fact.key()]; exists {
			return ReconcileSnapshot{}, fmt.Errorf("duplicate projection fact %q", fact.key())
		}
		seen[fact.key()] = struct{}{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].key() < values[j].key() })
	payload, err := json.Marshal(values)
	if err != nil {
		return ReconcileSnapshot{}, err
	}
	sum := sha256.Sum256(payload)
	return ReconcileSnapshot{Facts: values, SHA256: hex.EncodeToString(sum[:])}, nil
}

func compareSnapshots(authority, target []ProjectionFact) []ReconcileDifference {
	authorityByKey := make(map[string]ProjectionFact, len(authority))
	targetByKey := make(map[string]ProjectionFact, len(target))
	for _, fact := range authority {
		authorityByKey[fact.key()] = fact
	}
	for _, fact := range target {
		targetByKey[fact.key()] = fact
	}
	keys := make([]string, 0, len(authorityByKey)+len(targetByKey))
	for key := range authorityByKey {
		keys = append(keys, key)
	}
	for key := range targetByKey {
		if _, exists := authorityByKey[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	differences := make([]ReconcileDifference, 0)
	for _, key := range keys {
		authorityFact, inAuthority := authorityByKey[key]
		targetFact, inTarget := targetByKey[key]
		kind, projectionID, _ := strings.Cut(key, ":")
		switch {
		case inAuthority && !inTarget:
			authorityCopy := authorityFact
			differences = append(differences, ReconcileDifference{Class: "missing", Kind: kind, ProjectionID: projectionID, Authority: &authorityCopy, RepairEligible: true})
		case !inAuthority && inTarget:
			targetCopy := targetFact
			differences = append(differences, ReconcileDifference{Class: "extra", Kind: kind, ProjectionID: projectionID, Target: &targetCopy, RepairEligible: false})
		case authorityFact != targetFact:
			authorityCopy, targetCopy := authorityFact, targetFact
			differences = append(differences, ReconcileDifference{Class: "stale", Kind: kind, ProjectionID: projectionID, Authority: &authorityCopy, Target: &targetCopy, RepairEligible: true})
		}
	}
	return differences
}

func manifestSHA256(manifest ReconcileManifest) (string, error) {
	manifest.ManifestSHA256 = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal graph reconcile manifest: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
