package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
)

const alertEvidenceManifestSchemaVersion = "202608091700"

var errAlertEvidenceManifestUnavailable = errors.New("alert evidence manifest is unavailable")

// AlertEvidenceManifest is the PostgreSQL authority for an immutable evidence
// reference. ClickHouse remains the analytical read model; it must never be
// used to invent an object location or checksum at download time.
type AlertEvidenceManifest struct {
	TenantID         string            `json:"tenant_id"`
	AlertID          string            `json:"alert_id"`
	EvidenceID       string            `json:"evidence_id"`
	EventID          string            `json:"event_id,omitempty"`
	EvidenceType     string            `json:"evidence_type"`
	SourceStore      string            `json:"source_store"`
	ObjectBucket     string            `json:"object_bucket,omitempty"`
	ObjectKey        string            `json:"object_key,omitempty"`
	ObjectVersion    string            `json:"object_version,omitempty"`
	ObjectSHA256     string            `json:"object_sha256,omitempty"`
	SizeBytes        int64             `json:"size_bytes"`
	ContentType      string            `json:"content_type,omitempty"`
	State            string            `json:"state"`
	Revision         int64             `json:"revision"`
	SourceWatermarks map[string]string `json:"source_watermarks"`
	ObservedAt       time.Time         `json:"observed_at"`
	ExpiresAt        *time.Time        `json:"expires_at,omitempty"`
}

type AlertEvidenceManifestStore interface {
	VerifySchema(context.Context) error
	List(context.Context, string, string) ([]AlertEvidenceManifest, error)
	Get(context.Context, string, string, string) (*AlertEvidenceManifest, error)
}

type postgresAlertEvidenceManifestStore struct {
	db *sql.DB
}

func NewPostgresAlertEvidenceManifestStore(db *sql.DB) AlertEvidenceManifestStore {
	if db == nil {
		return nil
	}
	return &postgresAlertEvidenceManifestStore{db: db}
}

func (s *postgresAlertEvidenceManifestStore) VerifySchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errAlertEvidenceManifestUnavailable
	}
	var applied bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM alignment_schema_migrations WHERE version=$1
	) AND to_regclass('public.alert_evidence_manifests') IS NOT NULL`, alertEvidenceManifestSchemaVersion).Scan(&applied)
	if err != nil {
		return fmt.Errorf("verify alert evidence manifest schema: %w", err)
	}
	if !applied {
		return fmt.Errorf("%w: migration %s is not applied", errAlertEvidenceManifestUnavailable, alertEvidenceManifestSchemaVersion)
	}
	return nil
}

func (s *postgresAlertEvidenceManifestStore) List(ctx context.Context, tenantID, alertID string) ([]AlertEvidenceManifest, error) {
	if s == nil || s.db == nil {
		return nil, errAlertEvidenceManifestUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,
		object_bucket,object_key,object_version,object_sha256,size_bytes,content_type,state,revision,
		source_watermarks::text,observed_at,expires_at
		FROM alert_evidence_manifests
		WHERE tenant_id=$1 AND alert_id=$2
		ORDER BY observed_at DESC,evidence_id`, strings.TrimSpace(tenantID), strings.TrimSpace(alertID))
	if err != nil {
		return nil, fmt.Errorf("list alert evidence manifests: %w", err)
	}
	defer rows.Close()
	items := make([]AlertEvidenceManifest, 0)
	for rows.Next() {
		manifest, scanErr := scanAlertEvidenceManifest(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *manifest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert evidence manifests: %w", err)
	}
	return items, nil
}

func (s *postgresAlertEvidenceManifestStore) Get(ctx context.Context, tenantID, alertID, evidenceID string) (*AlertEvidenceManifest, error) {
	if s == nil || s.db == nil {
		return nil, errAlertEvidenceManifestUnavailable
	}
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,
		object_bucket,object_key,object_version,object_sha256,size_bytes,content_type,state,revision,
		source_watermarks::text,observed_at,expires_at
		FROM alert_evidence_manifests
		WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3`, strings.TrimSpace(tenantID), strings.TrimSpace(alertID), strings.TrimSpace(evidenceID))
	manifest, err := scanAlertEvidenceManifest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get alert evidence manifest: %w", err)
	}
	return manifest, nil
}

type alertEvidenceManifestScanner interface {
	Scan(...interface{}) error
}

func scanAlertEvidenceManifest(scanner alertEvidenceManifestScanner) (*AlertEvidenceManifest, error) {
	var manifest AlertEvidenceManifest
	var watermarksJSON string
	var expiresAt sql.NullTime
	err := scanner.Scan(
		&manifest.TenantID, &manifest.AlertID, &manifest.EvidenceID, &manifest.EventID,
		&manifest.EvidenceType, &manifest.SourceStore, &manifest.ObjectBucket, &manifest.ObjectKey,
		&manifest.ObjectVersion, &manifest.ObjectSHA256, &manifest.SizeBytes, &manifest.ContentType,
		&manifest.State, &manifest.Revision, &watermarksJSON, &manifest.ObservedAt, &expiresAt,
	)
	if err != nil {
		return nil, err
	}
	manifest.ObjectSHA256 = strings.ToLower(strings.TrimSpace(manifest.ObjectSHA256))
	manifest.SourceWatermarks = map[string]string{}
	if err := json.Unmarshal([]byte(watermarksJSON), &manifest.SourceWatermarks); err != nil {
		return nil, fmt.Errorf("decode alert evidence source watermarks: %w", err)
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		manifest.ExpiresAt = &value
	}
	return &manifest, nil
}

func reconcileAlertEvidenceManifests(evidences []*alertservice.EvidenceDTO, manifests []AlertEvidenceManifest, now time.Time) ([]map[string]interface{}, bool, []string, map[string]string) {
	manifestByID := make(map[string]AlertEvidenceManifest, len(manifests))
	maxRevision := int64(0)
	for _, manifest := range manifests {
		manifestByID[manifest.EvidenceID] = manifest
		if manifest.Revision > maxRevision {
			maxRevision = manifest.Revision
		}
	}
	items := make([]map[string]interface{}, 0, len(evidences)+len(manifests))
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(evidences))
	for _, evidence := range evidences {
		if evidence == nil {
			continue
		}
		item := evidenceDTOMap(evidence)
		manifest, ok := manifestByID[evidence.EvidenceID]
		if !ok {
			item["availability"] = "manifest_missing"
			missing = append(missing, "postgresql.manifest:"+evidence.EvidenceID)
		} else {
			manifestCopy := manifest
			item["manifest"] = manifestCopy
			item["availability"] = manifest.State
			if err := validateAlertEvidenceManifest(&manifestCopy, evidence, now); err != nil {
				item["integrity_error"] = err.Error()
				missing = append(missing, "evidence.integrity:"+evidence.EvidenceID)
			}
		}
		items = append(items, item)
		seen[evidence.EvidenceID] = struct{}{}
	}
	orphanIDs := make([]string, 0)
	for evidenceID := range manifestByID {
		if _, ok := seen[evidenceID]; !ok {
			orphanIDs = append(orphanIDs, evidenceID)
		}
	}
	sort.Strings(orphanIDs)
	for _, evidenceID := range orphanIDs {
		manifest := manifestByID[evidenceID]
		items = append(items, map[string]interface{}{
			"tenant_id": manifest.TenantID, "alert_id": manifest.AlertID, "evidence_id": evidenceID,
			"type": manifest.EvidenceType, "event_id": manifest.EventID, "availability": "read_model_missing", "manifest": manifest,
		})
		missing = append(missing, "clickhouse.evidence:"+evidenceID)
	}
	sort.Strings(missing)
	return items, len(missing) > 0, missing, map[string]string{
		"postgresql.alert_evidence_manifests.max_revision": fmt.Sprintf("%d", maxRevision),
		"postgresql.alert_evidence_manifests.count":        fmt.Sprintf("%d", len(manifests)),
		"clickhouse.evidence.count":                        fmt.Sprintf("%d", len(evidences)),
	}
}

func evidenceDTOMap(evidence *alertservice.EvidenceDTO) map[string]interface{} {
	payload, _ := json.Marshal(evidence)
	item := map[string]interface{}{}
	_ = json.Unmarshal(payload, &item)
	return item
}
