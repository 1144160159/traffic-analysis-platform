package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// AlertSnapshotIssue exposes a cross-store difference without silently
// repairing or deleting either source. Kind is one of missing, stale or extra.
type AlertSnapshotIssue struct {
	AlertID string `json:"alert_id"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Reason  string `json:"reason"`
}

type AlertSnapshotReconciliation struct {
	Missing []AlertSnapshotIssue `json:"missing"`
	Stale   []AlertSnapshotIssue `json:"stale"`
	Extra   []AlertSnapshotIssue `json:"extra"`
}

// AlertSnapshotSearchResult preserves OpenSearch pagination metadata while
// replacing every returned document with a ClickHouse source fact and, when
// newer, PostgreSQL analyst state.
type AlertSnapshotSearchResult struct {
	Alerts              []*persistence.Alert
	Total               int64
	TotalRelation       string
	Aggregations        map[string]interface{}
	AggregationsOmitted bool
	Took                int
	NextCursor          string
	HasMore             bool
	CursorMode          string
	SnapshotID          string
	AsOf                string
	Partial             bool
	MissingSections     []string
	SourceWatermarks    map[string]string
	Reconciliation      AlertSnapshotReconciliation
	StateSources        map[string]string
	ProjectionStatuses  map[string]string
}

type AlertManualState struct {
	AlertID          string
	StateVersion     int64
	Assignee         string
	Status           string
	ProjectionStatus string
	UpdatedAt        time.Time
}

type AlertProjectionReceipt struct {
	AlertID       string
	SourceVersion int64
	SourceSHA256  string
	AppliedAt     time.Time
}

type alertSnapshotFactSource interface {
	GetByIDs(context.Context, string, []string) ([]*persistence.Alert, error)
}

type alertSnapshotSearchSource interface {
	Search(context.Context, *SearchQuery) (*SearchResult, error)
}

type alertSnapshotMetadataSource interface {
	Load(context.Context, string, []string, string) (map[string]AlertManualState, map[string]AlertProjectionReceipt, error)
}

type AlertSnapshotRepository struct {
	facts              alertSnapshotFactSource
	search             alertSnapshotSearchSource
	metadata           alertSnapshotMetadataSource
	targetIndexVersion string
	now                func() time.Time
	logger             *zap.Logger
}

// NewAlertSnapshotRepository composes the three existing storage roles. It
// does not create a new authority: ClickHouse remains the alert fact source,
// PostgreSQL owns analyst state and receipts, and OpenSearch supplies search
// candidates only.
func NewAlertSnapshotRepository(
	clickhouse *AlertRepository,
	opensearch *OpenSearchRepository,
	postgres *sql.DB,
	targetIndexVersion string,
	logger *zap.Logger,
) *AlertSnapshotRepository {
	var facts alertSnapshotFactSource
	if clickhouse != nil {
		facts = clickhouse
	}
	var search alertSnapshotSearchSource
	if opensearch != nil {
		search = opensearch
	}
	var metadata alertSnapshotMetadataSource
	if postgres != nil {
		metadata = &postgresAlertSnapshotMetadataSource{db: postgres}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AlertSnapshotRepository{
		facts:              facts,
		search:             search,
		metadata:           metadata,
		targetIndexVersion: strings.TrimSpace(targetIndexVersion),
		now:                time.Now,
		logger:             logger,
	}
}

// Search uses OpenSearch only to select and order candidate IDs. A candidate
// absent from ClickHouse is reported as extra and is never returned as an
// alert fact. PostgreSQL metadata failure is a partial response; ClickHouse or
// OpenSearch failure is fail-closed because the requested search cannot be
// represented truthfully without either role.
func (r *AlertSnapshotRepository) Search(ctx context.Context, query *SearchQuery) (*AlertSnapshotSearchResult, error) {
	if r == nil || r.search == nil || r.facts == nil {
		return nil, commonerrors.New(commonerrors.ErrCodeServiceUnavailable, "alert snapshot repository is not configured")
	}
	if query == nil || strings.TrimSpace(query.TenantID) == "" {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "tenant-bound alert snapshot query is required")
	}

	searchResult, err := r.search.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	candidateIDs := make([]string, 0, len(searchResult.Alerts))
	searchDocuments := make(map[string]*persistence.Alert, len(searchResult.Alerts))
	for _, candidate := range searchResult.Alerts {
		if candidate == nil || strings.TrimSpace(candidate.AlertID) == "" {
			continue
		}
		candidateIDs = append(candidateIDs, candidate.AlertID)
		if _, exists := searchDocuments[candidate.AlertID]; !exists {
			searchDocuments[candidate.AlertID] = candidate
		}
	}

	authoritative, err := r.facts.GetByIDs(ctx, query.TenantID, candidateIDs)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "hydrate OpenSearch candidates from ClickHouse authority")
	}
	authoritativeByID := make(map[string]*persistence.Alert, len(authoritative))
	for _, alert := range authoritative {
		if alert != nil && alert.TenantID == query.TenantID {
			authoritativeByID[alert.AlertID] = alert
		}
	}

	states := map[string]AlertManualState{}
	receipts := map[string]AlertProjectionReceipt{}
	missingSections := make([]string, 0)
	metadataAvailable := r.metadata != nil
	if metadataAvailable && len(candidateIDs) > 0 {
		states, receipts, err = r.metadata.Load(ctx, query.TenantID, candidateIDs, r.targetIndexVersion)
		if err != nil {
			metadataAvailable = false
			states = map[string]AlertManualState{}
			receipts = map[string]AlertProjectionReceipt{}
			missingSections = append(missingSections, "postgresql.alert_snapshot_metadata")
			r.logger.Warn("PostgreSQL alert snapshot metadata is unavailable", zap.Error(err))
		}
	} else if !metadataAvailable && len(candidateIDs) > 0 {
		missingSections = append(missingSections, "postgresql.alert_snapshot_metadata")
	}

	result := &AlertSnapshotSearchResult{
		Alerts:              make([]*persistence.Alert, 0, len(candidateIDs)),
		Total:               searchResult.Total,
		TotalRelation:       searchResult.TotalRelation,
		Aggregations:        searchResult.Aggregations,
		AggregationsOmitted: searchResult.AggregationsOmitted,
		Took:                searchResult.Took,
		NextCursor:          searchResult.NextCursor,
		HasMore:             searchResult.HasMore,
		CursorMode:          searchResult.CursorMode,
		SnapshotID:          searchResult.SnapshotID,
		AsOf:                searchResult.AsOf,
		Partial:             searchResult.Partial || (len(candidateIDs) > 0 && !metadataAvailable),
		MissingSections:     missingSections,
		SourceWatermarks:    cloneStringMap(searchResult.SourceWatermarks),
		Reconciliation: AlertSnapshotReconciliation{
			Missing: []AlertSnapshotIssue{},
			Stale:   []AlertSnapshotIssue{},
			Extra:   []AlertSnapshotIssue{},
		},
		StateSources:       map[string]string{},
		ProjectionStatuses: map[string]string{},
	}
	if result.AsOf == "" {
		result.AsOf = r.now().UTC().Format(time.RFC3339Nano)
	}
	result.SourceWatermarks["opensearch.alerts.search"] = result.AsOf
	if r.targetIndexVersion != "" {
		result.SourceWatermarks["opensearch.alerts.target_index_version"] = r.targetIndexVersion
	}

	seenCandidates := make(map[string]struct{}, len(candidateIDs))
	var maxClickHouseVersion, maxManualStateVersion, maxReceiptVersion int64
	missingReceipt := false
	for _, alertID := range candidateIDs {
		if _, duplicate := seenCandidates[alertID]; duplicate {
			result.Reconciliation.Stale = append(result.Reconciliation.Stale, AlertSnapshotIssue{
				AlertID: alertID, Kind: "stale", Source: "opensearch.alerts.search",
				Reason: "duplicate document identity returned by the search projection",
			})
			continue
		}
		seenCandidates[alertID] = struct{}{}

		sourceFact := authoritativeByID[alertID]
		if sourceFact == nil {
			result.Reconciliation.Extra = append(result.Reconciliation.Extra, AlertSnapshotIssue{
				AlertID: alertID, Kind: "extra", Source: "opensearch.alerts.search",
				Reason: "search document has no ClickHouse authoritative fact",
			})
			continue
		}
		fact := sourceFact.Clone()
		expectedVersion := persistence.AlertSourceVersion(sourceFact)
		if expectedVersion > maxClickHouseVersion {
			maxClickHouseVersion = expectedVersion
		}

		if searchDocument := searchDocuments[alertID]; searchDocument != nil {
			expectedHash, expectedHashErr := persistence.AlertProjectionSHA256(sourceFact)
			searchHash, searchHashErr := persistence.AlertProjectionSHA256(searchDocument)
			if expectedHashErr != nil || searchHashErr != nil || expectedHash != searchHash {
				result.Reconciliation.Stale = append(result.Reconciliation.Stale, AlertSnapshotIssue{
					AlertID: alertID, Kind: "stale", Source: "opensearch.alerts.search",
					Reason: "search projection differs from the ClickHouse authoritative fact",
				})
			}

			if metadataAvailable {
				receipt, found := receipts[alertID]
				if !found {
					missingReceipt = true
					result.Reconciliation.Missing = append(result.Reconciliation.Missing, AlertSnapshotIssue{
						AlertID: alertID, Kind: "missing", Source: "postgresql.alert_opensearch_projection_watermarks",
						Reason: "projection receipt is missing",
					})
				} else {
					if receipt.SourceVersion > maxReceiptVersion {
						maxReceiptVersion = receipt.SourceVersion
					}
					if expectedHashErr != nil || receipt.SourceVersion != expectedVersion || receipt.SourceSHA256 != expectedHash {
						result.Reconciliation.Stale = append(result.Reconciliation.Stale, AlertSnapshotIssue{
							AlertID: alertID, Kind: "stale", Source: "postgresql.alert_opensearch_projection_watermarks",
							Reason: "projection receipt version or digest differs from the ClickHouse authoritative fact",
						})
					}
				}
			}
		}

		result.StateSources[alertID] = "clickhouse"
		if state, found := states[alertID]; found {
			result.ProjectionStatuses[alertID] = state.ProjectionStatus
			if state.StateVersion > maxManualStateVersion {
				maxManualStateVersion = state.StateVersion
			}
			if state.StateVersion >= expectedVersion {
				result.StateSources[alertID] = "postgresql"
				fact.Assignee = state.Assignee
				fact.Status = state.Status
				fact.StateVersion = uint64(state.StateVersion)
				fact.UpdatedTs = time.UnixMilli(state.StateVersion).UTC()
				if state.StateVersion != expectedVersion || sourceFact.Assignee != state.Assignee || sourceFact.Status != state.Status || state.ProjectionStatus != "applied" {
					result.Reconciliation.Stale = append(result.Reconciliation.Stale, AlertSnapshotIssue{
						AlertID: alertID, Kind: "stale", Source: "clickhouse.alerts.manual_state_projection",
						Reason: "ClickHouse analyst-state projection differs from PostgreSQL authority",
					})
				}
			} else {
				result.Reconciliation.Stale = append(result.Reconciliation.Stale, AlertSnapshotIssue{
					AlertID: alertID, Kind: "stale", Source: "postgresql.alert_assignment_states",
					Reason: "PostgreSQL analyst-state revision is older than the ClickHouse fact",
				})
			}
		}
		result.Alerts = append(result.Alerts, fact)
	}

	if missingReceipt {
		result.MissingSections = append(result.MissingSections, "postgresql.alerts.projection_receipts")
	}
	if maxClickHouseVersion > 0 {
		result.SourceWatermarks["clickhouse.alerts.source_version"] = strconv.FormatInt(maxClickHouseVersion, 10)
	}
	if maxManualStateVersion > 0 {
		result.SourceWatermarks["postgresql.alert_assignment_states.state_version"] = strconv.FormatInt(maxManualStateVersion, 10)
	}
	if maxReceiptVersion > 0 {
		result.SourceWatermarks["postgresql.alert_opensearch_projection_watermarks.source_version"] = strconv.FormatInt(maxReceiptVersion, 10)
	}
	result.MissingSections = uniqueSortedStrings(result.MissingSections)
	if len(result.Reconciliation.Missing)+len(result.Reconciliation.Stale)+len(result.Reconciliation.Extra) > 0 {
		result.Partial = true
	}
	return result, nil
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

type postgresAlertSnapshotMetadataSource struct {
	db *sql.DB
}

func (s *postgresAlertSnapshotMetadataSource) Load(
	ctx context.Context,
	tenantID string,
	alertIDs []string,
	targetIndexVersion string,
) (map[string]AlertManualState, map[string]AlertProjectionReceipt, error) {
	if s == nil || s.db == nil {
		return nil, nil, fmt.Errorf("PostgreSQL alert snapshot metadata source is unavailable")
	}
	if len(alertIDs) == 0 {
		return map[string]AlertManualState{}, map[string]AlertProjectionReceipt{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH requested AS (
			SELECT DISTINCT unnest($2::text[]) AS alert_id
		)
		SELECT requested.alert_id,
		       state.state_version,state.assignee,state.status,state.projection_status,state.updated_at,
		       receipt.source_version,receipt.source_sha256,receipt.applied_at
		FROM requested
		LEFT JOIN alert_assignment_states state
		  ON state.tenant_id=$1 AND state.alert_id=requested.alert_id
		LEFT JOIN alert_opensearch_projection_watermarks receipt
		  ON receipt.tenant_id=$1 AND receipt.alert_id=requested.alert_id
		 AND receipt.target_index_version=$3
		ORDER BY requested.alert_id`, tenantID, pq.Array(alertIDs), targetIndexVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("query PostgreSQL alert snapshot metadata: %w", err)
	}
	defer rows.Close()

	states := make(map[string]AlertManualState, len(alertIDs))
	receipts := make(map[string]AlertProjectionReceipt, len(alertIDs))
	for rows.Next() {
		var alertID string
		var stateVersion, receiptVersion sql.NullInt64
		var assignee, status, projectionStatus, receiptSHA sql.NullString
		var stateUpdatedAt, receiptAppliedAt sql.NullTime
		if err := rows.Scan(&alertID, &stateVersion, &assignee, &status, &projectionStatus, &stateUpdatedAt,
			&receiptVersion, &receiptSHA, &receiptAppliedAt); err != nil {
			return nil, nil, fmt.Errorf("scan PostgreSQL alert snapshot metadata: %w", err)
		}
		if stateVersion.Valid {
			states[alertID] = AlertManualState{
				AlertID: alertID, StateVersion: stateVersion.Int64, Assignee: assignee.String,
				Status: status.String, ProjectionStatus: projectionStatus.String, UpdatedAt: stateUpdatedAt.Time,
			}
		}
		if receiptVersion.Valid {
			receipts[alertID] = AlertProjectionReceipt{
				AlertID: alertID, SourceVersion: receiptVersion.Int64,
				SourceSHA256: receiptSHA.String, AppliedAt: receiptAppliedAt.Time,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate PostgreSQL alert snapshot metadata: %w", err)
	}
	return states, receipts, nil
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
