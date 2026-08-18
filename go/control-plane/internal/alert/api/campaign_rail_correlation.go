package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const campaignRailMinimumConfidence = 0.5

type CampaignRailScope struct {
	TenantID      string
	WindowFrom    time.Time
	WindowThrough time.Time
	// AsOf is the coordinated projection snapshot time. It is deliberately
	// separate from the CEP event-time window so an authority projection that
	// arrives after the window closes can still be correlated by a later,
	// explicit replay of that same closed window.
	AsOf         time.Time
	MaxCampaigns int
}

type CampaignRailSourcePosition struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type CampaignRailCorrelation struct {
	TenantID             string                       `json:"tenant_id"`
	CorrelationID        string                       `json:"correlation_id"`
	CEPCampaignID        string                       `json:"cep_campaign_id"`
	CEPEventID           string                       `json:"cep_event_id"`
	AggregateCampaignID  string                       `json:"aggregate_campaign_id,omitempty"`
	AggregateEventID     string                       `json:"aggregate_event_id,omitempty"`
	RelationIDs          []string                     `json:"relation_ids"`
	MembershipEventIDs   []string                     `json:"membership_event_ids"`
	RelationRevision     int64                        `json:"relation_revision"`
	State                string                       `json:"state"`
	CorrelationVersion   int64                        `json:"correlation_version"`
	CorrelationSHA256    string                       `json:"correlation_sha256"`
	CorrelationKeySHA256 string                       `json:"correlation_key_sha256"`
	AsOf                 time.Time                    `json:"as_of"`
	Confidence           float64                      `json:"confidence"`
	SourceWatermarks     campaignRailSourceWatermarks `json:"source_watermarks"`
	PartialReasons       []string                     `json:"partial_reasons"`
}

type CampaignRailProjectionReport struct {
	TenantID  string                    `json:"tenant_id"`
	AsOf      time.Time                 `json:"as_of"`
	Processed int                       `json:"processed"`
	Inserted  int                       `json:"inserted"`
	Replayed  int                       `json:"replayed"`
	ByState   map[string]int            `json:"by_state"`
	Receipts  []CampaignRailCorrelation `json:"receipts"`
}

type CampaignRailReconcileReport struct {
	RunID           string    `json:"run_id"`
	TenantID        string    `json:"tenant_id"`
	WindowFrom      time.Time `json:"window_from"`
	WindowThrough   time.Time `json:"window_through"`
	AsOf            time.Time `json:"as_of"`
	CEPCount        int       `json:"cep_count"`
	AggregateCount  int       `json:"aggregate_count"`
	CorrelatedCount int       `json:"correlated_count"`
	MissingCount    int       `json:"missing_count"`
	ConflictCount   int       `json:"conflict_count"`
	ExtraCount      int       `json:"extra_count"`
	ManifestSHA256  string    `json:"manifest_sha256"`
	State           string    `json:"state"`
}

type campaignRailDerived struct {
	TenantID, CampaignID, EventID string
	AlertIDs                      []string
	EventTimeEnd                  time.Time
	Position                      CampaignRailSourcePosition
}

type campaignRailMember struct {
	RelationID, EventID, AlertID string
	Revision                     int64
	Position                     CampaignRailSourcePosition
}

type campaignRailAuthority struct {
	TenantID, CampaignID, AggregateEventID string
	AggregateRevision                      int64
	Position                               CampaignRailSourcePosition
	Members                                []campaignRailMember
}

type campaignRailSourceWatermarks struct {
	CEP        CampaignRailSourcePosition   `json:"cep"`
	Aggregate  *CampaignRailSourcePosition  `json:"aggregate,omitempty"`
	Membership []CampaignRailSourcePosition `json:"membership"`
}

type campaignRailCanonicalReceipt struct {
	TenantID             string                       `json:"tenant_id"`
	CEPCampaignID        string                       `json:"cep_campaign_id"`
	CEPEventID           string                       `json:"cep_event_id"`
	AggregateCampaignID  string                       `json:"aggregate_campaign_id,omitempty"`
	AggregateEventID     string                       `json:"aggregate_event_id,omitempty"`
	RelationIDs          []string                     `json:"relation_ids"`
	MembershipEventIDs   []string                     `json:"membership_event_ids"`
	RelationRevision     int64                        `json:"relation_revision"`
	State                string                       `json:"state"`
	CorrelationKeySHA256 string                       `json:"correlation_key_sha256"`
	Confidence           float64                      `json:"confidence"`
	SourceWatermarks     campaignRailSourceWatermarks `json:"source_watermarks"`
	PartialReasons       []string                     `json:"partial_reasons"`
}

type campaignRailManifestItem struct {
	CEPEventID        string `json:"cep_event_id"`
	CorrelationSHA256 string `json:"correlation_sha256"`
	State             string `json:"state"`
}

type campaignRailReconcileManifest struct {
	TenantID      string                     `json:"tenant_id"`
	WindowFrom    time.Time                  `json:"window_from"`
	WindowThrough time.Time                  `json:"window_through"`
	Expected      []campaignRailManifestItem `json:"expected"`
	Actual        []campaignRailManifestItem `json:"actual"`
}

func normalizeCampaignRailScope(scope CampaignRailScope) (CampaignRailScope, error) {
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	if scope.AsOf.IsZero() {
		// Preserve the original direct-call contract while allowing workers and
		// replay tools to use a later processing-time snapshot explicitly.
		scope.AsOf = scope.WindowThrough
	}
	if scope.TenantID == "" || strings.EqualFold(scope.TenantID, "unknown") || scope.WindowFrom.IsZero() ||
		scope.WindowThrough.IsZero() || !scope.WindowFrom.Before(scope.WindowThrough) ||
		scope.AsOf.IsZero() || scope.AsOf.Before(scope.WindowThrough) ||
		scope.MaxCampaigns < 1 || scope.MaxCampaigns > 10000 {
		return CampaignRailScope{}, fmt.Errorf("invalid campaign rail closed-window scope")
	}
	scope.WindowFrom = scope.WindowFrom.UTC()
	scope.WindowThrough = scope.WindowThrough.UTC()
	scope.AsOf = scope.AsOf.UTC()
	return scope, nil
}

// ProjectCampaignRailCorrelations deterministically correlates the Protobuf
// detection rail with already-consumed JSON authority events. It never treats
// matching campaign IDs as proof and never writes back into either authority.
func (h *SystemHandler) ProjectCampaignRailCorrelations(ctx context.Context, scope CampaignRailScope) (CampaignRailProjectionReport, error) {
	if h.pgDB == nil {
		return CampaignRailProjectionReport{}, fmt.Errorf("campaign rail correlation database is unavailable")
	}
	var err error
	scope, err = normalizeCampaignRailScope(scope)
	if err != nil {
		return CampaignRailProjectionReport{}, err
	}
	derived, authorities, err := h.loadCampaignRailInputs(ctx, scope)
	if err != nil {
		return CampaignRailProjectionReport{}, err
	}
	receipts, err := buildCampaignRailCorrelations(derived, authorities, scope.AsOf, campaignRailMinimumConfidence)
	if err != nil {
		return CampaignRailProjectionReport{}, err
	}
	report := CampaignRailProjectionReport{TenantID: scope.TenantID, AsOf: scope.AsOf,
		Processed: len(receipts), ByState: map[string]int{}, Receipts: make([]CampaignRailCorrelation, 0, len(receipts))}
	for _, receipt := range receipts {
		persisted, inserted, err := h.persistCampaignRailCorrelation(ctx, receipt)
		if err != nil {
			return report, err
		}
		if inserted {
			report.Inserted++
		} else {
			report.Replayed++
		}
		report.ByState[persisted.State]++
		report.Receipts = append(report.Receipts, persisted)
	}
	return report, nil
}

func (h *SystemHandler) StartCampaignRailCorrelationWorker(
	ctx context.Context,
	interval, closedWindow, closeLag time.Duration,
	maxCampaigns int,
) error {
	if h.pgDB == nil || h.campaignCorrelateAdmit == nil {
		return fmt.Errorf("campaign rail correlation requires PostgreSQL and three consumer readiness receipts")
	}
	if interval <= 0 || closedWindow < time.Minute || closeLag < 0 || maxCampaigns < 1 || maxCampaigns > 10000 {
		return fmt.Errorf("invalid campaign rail correlation worker configuration")
	}
	go func() {
		run := func() {
			if err := h.runCampaignRailCorrelationWindow(ctx, closedWindow, closeLag, maxCampaigns); err != nil &&
				ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Campaign rail correlation window failed", zap.Error(err))
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return nil
}

func (h *SystemHandler) runCampaignRailCorrelationWindow(ctx context.Context, closedWindow, closeLag time.Duration, maxCampaigns int) error {
	if h.campaignCorrelateAdmit == nil {
		return fmt.Errorf("campaign rail correlation readiness gate is unavailable")
	}
	if err := h.campaignCorrelateAdmit(ctx); err != nil {
		return fmt.Errorf("campaign rail correlation consumer readiness withdrawn: %w", err)
	}
	asOf := time.Now().UTC()
	through := asOf.Add(-closeLag).Truncate(closedWindow)
	from := through.Add(-closedWindow)
	rows, err := h.pgDB.QueryContext(ctx, `SELECT DISTINCT tenant_id FROM campaign_proto_projection_inbox_v1
		WHERE state='applied' AND event_time_end_ms>=$1 AND event_time_end_ms<$2
		ORDER BY tenant_id LIMIT 1001`, from.UnixMilli(), through.UnixMilli())
	if err != nil {
		return fmt.Errorf("load campaign rail tenants: %w", err)
	}
	tenants := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			rows.Close()
			return err
		}
		tenants = append(tenants, tenantID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(tenants) > 1000 {
		return fmt.Errorf("campaign rail tenant budget exceeded")
	}
	for _, tenantID := range tenants {
		scope := CampaignRailScope{TenantID: tenantID, WindowFrom: from, WindowThrough: through,
			AsOf: asOf, MaxCampaigns: maxCampaigns}
		if _, err := h.ProjectCampaignRailCorrelations(ctx, scope); err != nil {
			return fmt.Errorf("project campaign rails for tenant %s: %w", tenantID, err)
		}
		if _, err := h.ReconcileCampaignRails(ctx, scope); err != nil {
			return fmt.Errorf("reconcile campaign rails for tenant %s: %w", tenantID, err)
		}
	}
	return nil
}

// ReconcileCampaignRails performs an exact closed-window set comparison. It
// records discrepancies but never deletes extra correlation evidence.
func (h *SystemHandler) ReconcileCampaignRails(ctx context.Context, scope CampaignRailScope) (CampaignRailReconcileReport, error) {
	if h.pgDB == nil {
		return CampaignRailReconcileReport{}, fmt.Errorf("campaign rail reconcile database is unavailable")
	}
	var err error
	scope, err = normalizeCampaignRailScope(scope)
	if err != nil {
		return CampaignRailReconcileReport{}, err
	}
	derived, authorities, err := h.loadCampaignRailInputs(ctx, scope)
	if err != nil {
		return CampaignRailReconcileReport{}, err
	}
	expectedReceipts, err := buildCampaignRailCorrelations(derived, authorities, scope.AsOf, campaignRailMinimumConfidence)
	if err != nil {
		return CampaignRailReconcileReport{}, err
	}
	expected := make([]campaignRailManifestItem, 0, len(expectedReceipts))
	conflicts, correlated := 0, 0
	for _, receipt := range expectedReceipts {
		expected = append(expected, campaignRailManifestItem{CEPEventID: receipt.CEPEventID,
			CorrelationSHA256: receipt.CorrelationSHA256, State: receipt.State})
		if receipt.State == "conflict" {
			conflicts++
		}
		if receipt.State == "correlated" {
			correlated++
		}
	}
	rows, err := h.pgDB.QueryContext(ctx, `SELECT current.cep_event_id,current.correlation_sha256,current.state FROM (
		SELECT DISTINCT ON (correlation.cep_event_id) correlation.cep_event_id::text AS cep_event_id,
		  correlation.correlation_sha256,correlation.state,correlation.correlation_version
		FROM campaign_rail_correlation_v1 correlation
		JOIN campaign_proto_projection_inbox_v1 inbox ON inbox.event_id=correlation.cep_event_id
		WHERE correlation.tenant_id=$1 AND correlation.as_of<=$2
		  AND inbox.event_time_end_ms>=$3 AND inbox.event_time_end_ms<$4
		ORDER BY correlation.cep_event_id,correlation.correlation_version DESC
	) current WHERE current.state<>'revoked'
	ORDER BY current.cep_event_id,current.correlation_sha256`, scope.TenantID, scope.AsOf,
		scope.WindowFrom.UTC().UnixMilli(), scope.WindowThrough.UTC().UnixMilli())
	if err != nil {
		return CampaignRailReconcileReport{}, fmt.Errorf("load persisted campaign correlations: %w", err)
	}
	actual := make([]campaignRailManifestItem, 0)
	for rows.Next() {
		var item campaignRailManifestItem
		if err := rows.Scan(&item.CEPEventID, &item.CorrelationSHA256, &item.State); err != nil {
			rows.Close()
			return CampaignRailReconcileReport{}, err
		}
		actual = append(actual, item)
	}
	if err := rows.Close(); err != nil {
		return CampaignRailReconcileReport{}, err
	}
	missing, extra := campaignRailExactSetDiff(expected, actual)
	manifest := campaignRailReconcileManifest{TenantID: scope.TenantID, WindowFrom: scope.WindowFrom,
		WindowThrough: scope.WindowThrough, Expected: expected, Actual: actual}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return CampaignRailReconcileReport{}, err
	}
	digest := sha256.Sum256(manifestJSON)
	report := CampaignRailReconcileReport{RunID: uuid.NewString(), TenantID: scope.TenantID,
		WindowFrom: scope.WindowFrom, WindowThrough: scope.WindowThrough, AsOf: scope.AsOf, CEPCount: len(derived),
		AggregateCount: len(authorities), CorrelatedCount: correlated, MissingCount: missing,
		ConflictCount: conflicts, ExtraCount: extra, ManifestSHA256: hex.EncodeToString(digest[:]), State: "exact"}
	if missing > 0 || conflicts > 0 || extra > 0 {
		report.State = "diff"
	}
	_, err = h.pgDB.ExecContext(ctx, `INSERT INTO campaign_rail_reconcile_runs_v1 (
		run_id,tenant_id,window_from,window_through,as_of,max_items,cep_count,aggregate_count,correlated_count,
		missing_count,conflict_count,extra_count,manifest_sha256,state)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, report.RunID, report.TenantID,
		report.WindowFrom, report.WindowThrough, report.AsOf, scope.MaxCampaigns, report.CEPCount, report.AggregateCount,
		report.CorrelatedCount, report.MissingCount, report.ConflictCount, report.ExtraCount,
		report.ManifestSHA256, report.State)
	if err != nil {
		return CampaignRailReconcileReport{}, fmt.Errorf("persist campaign rail reconcile manifest: %w", err)
	}
	return report, nil
}

func (h *SystemHandler) loadCampaignRailInputs(ctx context.Context, scope CampaignRailScope) ([]campaignRailDerived, []campaignRailAuthority, error) {
	fromMillis, throughMillis := scope.WindowFrom.UTC().UnixMilli(), scope.WindowThrough.UTC().UnixMilli()
	rows, err := h.pgDB.QueryContext(ctx, `SELECT current.tenant_id,current.campaign_id,current.event_id::text,
		inbox.payload_protobuf,inbox.event_time_end_ms,inbox.source_topic,inbox.source_partition,inbox.source_offset
		FROM campaign_proto_projection_current_v1 current
		JOIN campaign_proto_projection_inbox_v1 inbox ON inbox.event_id=current.event_id
		WHERE current.tenant_id=$1 AND inbox.state='applied'
		  AND inbox.event_time_end_ms>=$2 AND inbox.event_time_end_ms<$3
		ORDER BY inbox.event_time_end_ms,current.campaign_id,current.event_id LIMIT $4`,
		scope.TenantID, fromMillis, throughMillis, scope.MaxCampaigns+1)
	if err != nil {
		return nil, nil, fmt.Errorf("load campaign Protobuf rail: %w", err)
	}
	defer rows.Close()
	derived := make([]campaignRailDerived, 0)
	for rows.Next() {
		var item campaignRailDerived
		var payload []byte
		var eventEndMillis int64
		if err := rows.Scan(&item.TenantID, &item.CampaignID, &item.EventID, &payload, &eventEndMillis,
			&item.Position.Topic, &item.Position.Partition, &item.Position.Offset); err != nil {
			return nil, nil, err
		}
		var campaign trafficv1.Campaign
		if err := proto.Unmarshal(payload, &campaign); err != nil {
			return nil, nil, fmt.Errorf("decode persisted campaigns.v1 event %s: %w", item.EventID, err)
		}
		if campaign.GetTenantId() != item.TenantID || campaign.GetCampaignId() != item.CampaignID || campaign.GetEventId() != item.EventID {
			return nil, nil, fmt.Errorf("campaign Protobuf projection identity drift for %s", item.EventID)
		}
		item.AlertIDs = canonicalCampaignRailSet(campaign.GetAlerts())
		if len(item.AlertIDs) == 0 {
			return nil, nil, fmt.Errorf("campaign Protobuf projection %s has no alert identity", item.EventID)
		}
		item.EventTimeEnd = time.UnixMilli(eventEndMillis).UTC()
		derived = append(derived, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(derived) > scope.MaxCampaigns {
		return nil, nil, fmt.Errorf("campaign rail projection budget exceeded")
	}

	authorities := map[string]*campaignRailAuthority{}
	aggregateRows, err := h.pgDB.QueryContext(ctx, `SELECT DISTINCT ON (campaign_id)
		tenant_id,campaign_id,event_id::text,aggregate_revision,first_kafka_topic,first_kafka_partition,first_kafka_offset
		FROM campaign_event_projection_inbox
		WHERE tenant_id=$1 AND stream='aggregate' AND received_at<=$2
		ORDER BY campaign_id,aggregate_revision DESC,event_id DESC`, scope.TenantID, scope.AsOf)
	if err != nil {
		return nil, nil, fmt.Errorf("load campaign JSON aggregate rail: %w", err)
	}
	for aggregateRows.Next() {
		item := &campaignRailAuthority{}
		if err := aggregateRows.Scan(&item.TenantID, &item.CampaignID, &item.AggregateEventID, &item.AggregateRevision,
			&item.Position.Topic, &item.Position.Partition, &item.Position.Offset); err != nil {
			aggregateRows.Close()
			return nil, nil, err
		}
		authorities[item.CampaignID] = item
	}
	if err := aggregateRows.Close(); err != nil {
		return nil, nil, err
	}
	membershipRows, err := h.pgDB.QueryContext(ctx, `SELECT campaign_id,relation_id::text,event_id::text,alert_id,relation_revision,
		first_kafka_topic,first_kafka_partition,first_kafka_offset,event_type FROM (
		  SELECT DISTINCT ON (relation_id) campaign_id,relation_id,event_id,alert_id,relation_revision,
		    first_kafka_topic,first_kafka_partition,first_kafka_offset,event_type
		  FROM campaign_event_projection_inbox
		  WHERE tenant_id=$1 AND stream='membership' AND received_at<=$2
		  ORDER BY relation_id,relation_revision DESC,event_id DESC
		) latest WHERE event_type='traffic.campaign.v2.AlertLinked'
		ORDER BY campaign_id,relation_id`, scope.TenantID, scope.AsOf)
	if err != nil {
		return nil, nil, fmt.Errorf("load campaign JSON membership rail: %w", err)
	}
	defer membershipRows.Close()
	for membershipRows.Next() {
		var campaignID, eventType string
		var member campaignRailMember
		if err := membershipRows.Scan(&campaignID, &member.RelationID, &member.EventID, &member.AlertID, &member.Revision,
			&member.Position.Topic, &member.Position.Partition, &member.Position.Offset, &eventType); err != nil {
			return nil, nil, err
		}
		if authority := authorities[campaignID]; authority != nil {
			authority.Members = append(authority.Members, member)
		}
	}
	if err := membershipRows.Err(); err != nil {
		return nil, nil, err
	}
	ordered := make([]campaignRailAuthority, 0, len(authorities))
	for _, authority := range authorities {
		if len(authority.Members) == 0 {
			continue
		}
		sort.Slice(authority.Members, func(i, j int) bool { return authority.Members[i].RelationID < authority.Members[j].RelationID })
		ordered = append(ordered, *authority)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CampaignID < ordered[j].CampaignID })
	return derived, ordered, nil
}

func buildCampaignRailCorrelations(derived []campaignRailDerived, authorities []campaignRailAuthority, asOf time.Time, minimum float64) ([]CampaignRailCorrelation, error) {
	if minimum <= 0 || minimum > 1 || asOf.IsZero() {
		return nil, fmt.Errorf("invalid campaign rail correlation policy")
	}
	receipts := make([]CampaignRailCorrelation, 0, len(derived))
	for _, source := range derived {
		if source.TenantID == "" || source.CampaignID == "" || source.EventID == "" || len(source.AlertIDs) == 0 {
			return nil, fmt.Errorf("incomplete campaign rail derived input")
		}
		type scored struct {
			authority campaignRailAuthority
			score     float64
		}
		candidates := make([]scored, 0)
		for _, authority := range authorities {
			if authority.TenantID != source.TenantID {
				continue
			}
			authorityAlerts := make([]string, 0, len(authority.Members))
			for _, member := range authority.Members {
				authorityAlerts = append(authorityAlerts, member.AlertID)
			}
			score := campaignRailJaccard(source.AlertIDs, canonicalCampaignRailSet(authorityAlerts))
			if score > 0 {
				candidates = append(candidates, scored{authority: authority, score: score})
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].score == candidates[j].score {
				return candidates[i].authority.CampaignID < candidates[j].authority.CampaignID
			}
			return candidates[i].score > candidates[j].score
		})
		receipt := CampaignRailCorrelation{TenantID: source.TenantID, CEPCampaignID: source.CampaignID,
			CEPEventID: source.EventID, State: "cep_only", AsOf: asOf.UTC(), RelationIDs: []string{},
			MembershipEventIDs: []string{}, SourceWatermarks: campaignRailSourceWatermarks{CEP: source.Position, Membership: []CampaignRailSourcePosition{}},
			PartialReasons: []string{"no_authority_alert_intersection"}}
		if len(candidates) > 0 {
			receipt.Confidence = candidates[0].score
			tied := len(candidates) > 1 && math.Abs(candidates[0].score-candidates[1].score) < 1e-12
			if tied || candidates[0].score < minimum {
				receipt.State = "conflict"
				receipt.PartialReasons = []string{"below_minimum_confidence"}
				if tied {
					receipt.PartialReasons = []string{"tied_highest_authority_candidates"}
				}
			} else {
				winner := candidates[0].authority
				receipt.State, receipt.PartialReasons = "correlated", []string{}
				receipt.AggregateCampaignID, receipt.AggregateEventID = winner.CampaignID, winner.AggregateEventID
				receipt.SourceWatermarks.Aggregate = &winner.Position
				for _, member := range winner.Members {
					receipt.RelationIDs = append(receipt.RelationIDs, member.RelationID)
					receipt.MembershipEventIDs = append(receipt.MembershipEventIDs, member.EventID)
					receipt.SourceWatermarks.Membership = append(receipt.SourceWatermarks.Membership, member.Position)
					if member.Revision > receipt.RelationRevision {
						receipt.RelationRevision = member.Revision
					}
				}
				receipt.RelationIDs = canonicalCampaignRailSet(receipt.RelationIDs)
				receipt.MembershipEventIDs = canonicalCampaignRailSet(receipt.MembershipEventIDs)
			}
		}
		// This is the stable logical identity of the correlation subject. The
		// coordinated as_of controls which source facts are visible, but it must
		// not manufacture a new meaning/version when those facts are unchanged.
		receipt.CorrelationKeySHA256 = campaignRailDigest(source.TenantID, source.CampaignID, source.EventID)
		canonical := campaignRailCanonicalReceipt{TenantID: receipt.TenantID, CEPCampaignID: receipt.CEPCampaignID,
			CEPEventID: receipt.CEPEventID, AggregateCampaignID: receipt.AggregateCampaignID,
			AggregateEventID: receipt.AggregateEventID, RelationIDs: receipt.RelationIDs,
			MembershipEventIDs: receipt.MembershipEventIDs, RelationRevision: receipt.RelationRevision,
			State: receipt.State, CorrelationKeySHA256: receipt.CorrelationKeySHA256,
			Confidence: receipt.Confidence, SourceWatermarks: receipt.SourceWatermarks, PartialReasons: receipt.PartialReasons}
		payload, err := json.Marshal(canonical)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(payload)
		receipt.CorrelationSHA256 = hex.EncodeToString(digest[:])
		receipt.CorrelationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(receipt.TenantID+":"+receipt.CEPEventID+":"+receipt.CorrelationSHA256)).String()
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func (h *SystemHandler) persistCampaignRailCorrelation(ctx context.Context, receipt CampaignRailCorrelation) (CampaignRailCorrelation, bool, error) {
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return receipt, false, err
	}
	defer tx.Rollback()
	lockKey := receipt.TenantID + ":campaign-rail:" + receipt.CEPCampaignID
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return receipt, false, err
	}
	var existingID string
	var existingVersion int64
	var existingAsOf time.Time
	err = tx.QueryRowContext(ctx, `SELECT correlation_id::text,correlation_version,as_of FROM campaign_rail_correlation_v1
		WHERE tenant_id=$1 AND cep_event_id=$2::uuid AND correlation_sha256=$3 ORDER BY correlation_version LIMIT 1`,
		receipt.TenantID, receipt.CEPEventID, receipt.CorrelationSHA256).Scan(&existingID, &existingVersion, &existingAsOf)
	if err == nil {
		receipt.CorrelationID, receipt.CorrelationVersion, receipt.AsOf = existingID, existingVersion, existingAsOf.UTC()
		if err := tx.Commit(); err != nil {
			return receipt, false, err
		}
		return receipt, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return receipt, false, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(correlation_version),0)+1 FROM campaign_rail_correlation_v1
		WHERE tenant_id=$1 AND cep_campaign_id=$2`, receipt.TenantID, receipt.CEPCampaignID).Scan(&receipt.CorrelationVersion); err != nil {
		return receipt, false, err
	}
	watermarks, err := json.Marshal(receipt.SourceWatermarks)
	if err != nil {
		return receipt, false, err
	}
	var aggregateCampaignID, aggregateEventID interface{}
	if receipt.AggregateCampaignID != "" {
		aggregateCampaignID = receipt.AggregateCampaignID
		aggregateEventID = receipt.AggregateEventID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_rail_correlation_v1 (
		tenant_id,correlation_id,cep_campaign_id,cep_event_id,aggregate_campaign_id,aggregate_event_id,
		relation_ids,membership_event_ids,relation_revision,state,correlation_version,correlation_sha256,
		correlation_key_sha256,as_of,confidence,source_watermarks,partial_reasons)
		VALUES ($1,$2::uuid,$3,$4::uuid,$5,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,$17)`,
		receipt.TenantID, receipt.CorrelationID, receipt.CEPCampaignID, receipt.CEPEventID,
		aggregateCampaignID, aggregateEventID, pq.Array(receipt.RelationIDs), pq.Array(receipt.MembershipEventIDs),
		receipt.RelationRevision, receipt.State, receipt.CorrelationVersion, receipt.CorrelationSHA256,
		receipt.CorrelationKeySHA256, receipt.AsOf, receipt.Confidence, string(watermarks), pq.Array(receipt.PartialReasons))
	if err != nil {
		return receipt, false, fmt.Errorf("insert campaign rail correlation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return receipt, false, err
	}
	return receipt, true, nil
}

func canonicalCampaignRailSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func campaignRailJaccard(left, right []string) float64 {
	left, right = canonicalCampaignRailSet(left), canonicalCampaignRailSet(right)
	i, j, intersection := 0, 0, 0
	for i < len(left) && j < len(right) {
		switch {
		case left[i] == right[j]:
			intersection++
			i++
			j++
		case left[i] < right[j]:
			i++
		default:
			j++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func campaignRailExactSetDiff(expected, actual []campaignRailManifestItem) (missing, extra int) {
	key := func(item campaignRailManifestItem) string {
		return item.CEPEventID + ":" + item.CorrelationSHA256 + ":" + item.State
	}
	expectedSet, actualSet := map[string]struct{}{}, map[string]struct{}{}
	for _, item := range expected {
		expectedSet[key(item)] = struct{}{}
	}
	for _, item := range actual {
		actualSet[key(item)] = struct{}{}
	}
	for item := range expectedSet {
		if _, ok := actualSet[item]; !ok {
			missing++
		}
	}
	for item := range actualSet {
		if _, ok := expectedSet[item]; !ok {
			extra++
		}
	}
	return missing, extra
}

func campaignRailDigest(parts ...string) string {
	hash := sha256.New()
	var length [4]byte
	for _, part := range parts {
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
