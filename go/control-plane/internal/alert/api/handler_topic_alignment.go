package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	topicSnapshotContractVersion = 1
	topicActionContractVersion   = 1
	topicActionLease             = 2 * time.Minute
	topicActionMaxAttempts       = 5
)

type topicActionCatalogEntry struct {
	ActionID         string `json:"action_id"`
	Risk             string `json:"risk"`
	Approval         string `json:"approval"`
	Executor         string `json:"executor"`
	Compensation     string `json:"compensation"`
	Enabled          bool   `json:"enabled"`
	UnavailableCause string `json:"unavailable_cause,omitempty"`
}

var topicActionCatalog = map[string]topicActionCatalogEntry{
	"inspect_detail":      {ActionID: "inspect_detail", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"inspect_alerts":      {ActionID: "inspect_alerts", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"trace":               {ActionID: "trace", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"inspect_session":     {ActionID: "inspect_session", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"inspect_certificate": {ActionID: "inspect_certificate", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"trace_path":          {ActionID: "trace_path", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"write_audit":         {ActionID: "write_audit", Risk: "low", Approval: "none", Executor: "internal_receipt", Compensation: "append_correction", Enabled: true},
	"change_layout":       {ActionID: "change_layout", Risk: "low", Approval: "none", Executor: "internal_receipt", Compensation: "restore_previous_layout", Enabled: true},
	"focus_view":          {ActionID: "focus_view", Risk: "read", Approval: "none", Executor: "internal_receipt", Compensation: "none", Enabled: true},
	"extract_pcap":        {ActionID: "extract_pcap", Risk: "medium", Approval: "policy", Executor: "forensics_job", Compensation: "cancel_forensics_job", Enabled: false, UnavailableCause: "forensics executor binding is not configured"},
	"contain":             {ActionID: "contain", Risk: "high", Approval: "two_person", Executor: "soar", Compensation: "remove_containment", Enabled: false, UnavailableCause: "SOAR executor binding is not configured"},
	"monitor":             {ActionID: "monitor", Risk: "medium", Approval: "policy", Executor: "watchlist", Compensation: "remove_watch", Enabled: false, UnavailableCause: "watchlist executor binding is not configured"},
	"review":              {ActionID: "review", Risk: "low", Approval: "none", Executor: "case_management", Compensation: "append_correction", Enabled: false, UnavailableCause: "case-management executor binding is not configured"},
	"review_exception":    {ActionID: "review_exception", Risk: "high", Approval: "two_person", Executor: "whitelist", Compensation: "revoke_exception", Enabled: false, UnavailableCause: "whitelist approval executor binding is not configured"},
	"mute":                {ActionID: "mute", Risk: "medium", Approval: "policy", Executor: "notification", Compensation: "remove_mute", Enabled: false, UnavailableCause: "notification executor binding is not configured"},
	"link_rule":           {ActionID: "link_rule", Risk: "medium", Approval: "policy", Executor: "rule_manager", Compensation: "unlink_rule", Enabled: false, UnavailableCause: "rule-manager executor binding is not configured"},
}

type topicSnapshotBuild struct {
	Data             map[string]interface{}
	Partial          bool
	MissingSections  []string
	SourceWatermarks map[string]string
}

type topicActionV2Request struct {
	ActionID         string                 `json:"action_id"`
	Action           string                 `json:"action,omitempty"`
	Label            string                 `json:"label"`
	Target           string                 `json:"target"`
	SnapshotID       string                 `json:"snapshot_id"`
	ExpectedRevision int64                  `json:"expected_revision"`
	Reason           string                 `json:"reason"`
	Detail           map[string]interface{} `json:"detail"`
}

type topicActionJobDTO struct {
	JobID            string                 `json:"job_id"`
	ActionID         string                 `json:"action_id"`
	TenantID         string                 `json:"tenant_id"`
	Topic            string                 `json:"topic"`
	Target           string                 `json:"target"`
	SnapshotID       string                 `json:"snapshot_id"`
	ExpectedRevision int64                  `json:"expected_revision"`
	Revision         int64                  `json:"revision"`
	Status           string                 `json:"status"`
	Executor         string                 `json:"executor"`
	Reason           string                 `json:"reason"`
	TraceID          string                 `json:"trace_id"`
	Receipt          map[string]interface{} `json:"receipt"`
	Error            map[string]interface{} `json:"error"`
	Attempts         int                    `json:"attempts"`
	RequestedBy      string                 `json:"requested_by"`
	CreatedAt        time.Time              `json:"-"`
	UpdatedAt        time.Time              `json:"-"`
}

func (j topicActionJobDTO) response() map[string]interface{} {
	return map[string]interface{}{
		"job_id": j.JobID, "action_id": j.ActionID, "tenant_id": j.TenantID,
		"topic": j.Topic, "target": j.Target, "snapshot_id": j.SnapshotID,
		"expected_revision": j.ExpectedRevision, "revision": j.Revision,
		"status": j.Status, "executor": j.Executor, "reason": j.Reason,
		"trace_id": j.TraceID, "receipt": j.Receipt, "error": j.Error,
		"attempts": j.Attempts, "requested_by": j.RequestedBy,
		"created_at": j.CreatedAt.UnixMilli(), "updated_at": j.UpdatedAt.UnixMilli(),
	}
}

func (h *SystemHandler) requireTopicReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, "topic:read") || hasSystemPermission(ctx, "topic:write") || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: topic:read required")
	return false
}

// GetTopicSnapshot freezes one immutable, tenant-scoped payload for the selected
// topic. The three workbench panels and right rail consume this same payload.
// Missing projections are explicit; this endpoint never reads simulation rows.
func (h *SystemHandler) GetTopicSnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.topicSnapshotV1 {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "FEATURE_DISABLED", "topic snapshot v1 is disabled")
		return
	}
	if !h.requireTopicReadPermission(w, r) {
		return
	}
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "topic snapshot manifest persistence is unavailable")
		return
	}
	topic := normalizeTopicKey(mux.Vars(r)["topic"])
	if !isValidTopicKey(topic) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
		return
	}
	tenantID := writeTenantID(r)
	scope, err := h.loadTopicScope(ctx, tenantID, topic)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SNAPSHOT_SOURCE_FAILED", "failed to load topic scope")
		return
	}
	if scope == nil {
		scope = defaultTopicScope(tenantID, topic)
	}
	fallback := 24 * time.Hour
	if topic == "apt" {
		fallback = 30 * 24 * time.Hour
	}
	start, end := queryTimeRange(r, topicScopeDuration(scope, fallback))
	asOf := time.Now().UTC()
	snapshotID := uuid.NewString()
	resourceRevision := scope.UpdatedAt
	if resourceRevision <= 0 {
		resourceRevision = 1
	}

	built := h.buildTopicSnapshotData(ctx, tenantID, topic, scope, start, end, asOf)
	built.Data["snapshot_id"] = snapshotID
	built.Data["revision"] = resourceRevision
	built.Data["action_catalog"] = topicCatalogList()
	payloadJSON, err := json.Marshal(built.Data)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SNAPSHOT_ENCODE_FAILED", "failed to encode topic snapshot")
		return
	}
	digest := sha256.Sum256(payloadJSON)
	watermarksJSON, _ := json.Marshal(built.SourceWatermarks)
	if _, err := h.pgDB.ExecContext(ctx, `
		INSERT INTO topic_snapshot_manifests
			(snapshot_id, tenant_id, topic, resource_revision, as_of, range_start, range_end,
			 payload, payload_sha256, partial, missing_sections, source_watermarks, trace_id)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12::jsonb,$13)`,
		snapshotID, tenantID, topic, resourceRevision, asOf, start, end, string(payloadJSON),
		hex.EncodeToString(digest[:]), built.Partial, pq.Array(built.MissingSections),
		string(watermarksJSON), httpx.GetTraceID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SNAPSHOT_PERSIST_FAILED", "failed to persist topic snapshot manifest")
		return
	}
	httpx.JSONContractSuccess(w, ctx, built.Data, httpx.ContractMeta{
		ContractVersion:  topicSnapshotContractVersion,
		SnapshotID:       snapshotID,
		AsOf:             asOf.Format(time.RFC3339Nano),
		TraceID:          httpx.GetTraceID(ctx),
		Partial:          built.Partial,
		MissingSections:  built.MissingSections,
		SourceWatermarks: built.SourceWatermarks,
	})
}

func defaultTopicScope(tenantID, topic string) *topicScopeDTO {
	return &topicScopeDTO{
		TenantID: tenantID, Topic: topic, ScopeName: "默认专题范围",
		IncludedAssets: []string{}, ExcludedAssets: []string{}, RiskLevels: []string{},
		TimeWindow: topicDefaultTimeWindow(topic), Detail: map[string]interface{}{},
	}
}

func topicCatalogList() []topicActionCatalogEntry {
	result := make([]topicActionCatalogEntry, 0, len(topicActionCatalog))
	for _, actionID := range []string{
		"inspect_detail", "inspect_alerts", "trace", "inspect_session", "inspect_certificate",
		"trace_path", "write_audit", "change_layout", "focus_view", "extract_pcap",
		"contain", "monitor", "review", "review_exception", "mute", "link_rule",
	} {
		result = append(result, topicActionCatalog[actionID])
	}
	return result
}

func (h *SystemHandler) buildTopicSnapshotData(
	ctx context.Context,
	tenantID, topic string,
	scope *topicScopeDTO,
	start, end int64,
	asOf time.Time,
) topicSnapshotBuild {
	missing := []string{"opensearch_evidence", "nebulagraph_projection", "minio_evidence_manifest"}
	watermarks := map[string]string{
		"postgresql.topic_scope.revision":              fmt.Sprint(maxInt64(scope.UpdatedAt, 1)),
		"opensearch.topic_evidence.projection_version": "unavailable",
		"nebulagraph.topic_projection.version":         "unavailable",
		"minio.topic_evidence.manifest":                "unavailable",
	}
	data := map[string]interface{}{
		"topic": topic, "updated_at": asOf.UnixMilli(),
		"time_range": map[string]int64{"start": start, "end": end},
		"scope":      scope,
	}
	// Keep the snapshot payload shape stable even when the authoritative
	// ClickHouse projection is unavailable. The UI can then render an explicit
	// empty/partial state instead of treating an omitted topology as a broken
	// contract.
	if topic == "exfil" || topic == "apt" {
		data["topology_nodes"] = []topicTopologyNodeDTO{}
		data["topology_links"] = []topicTopologyLinkDTO{}
	}
	if h.chClient == nil {
		missing = append(missing, topicClickHouseSections(topic)...)
		watermarks["clickhouse.topic_fact.watermark"] = "unavailable"
		data["summary"] = map[string]interface{}{}
		data["data_mode"] = "partial"
		return topicSnapshotBuild{Data: data, Partial: true, MissingSections: uniqueStrings(missing), SourceWatermarks: watermarks}
	}
	watermarks["clickhouse.topic_fact.watermark"] = fmt.Sprint(end)
	switch topic {
	case "tunnel":
		protocols, err := h.queryTunnelProtocols(ctx, tenantID, start, end)
		if err != nil {
			missing = append(missing, "tunnel.protocols")
		} else {
			data["protocols"] = protocols
		}
		users, err := h.queryTunnelUsers(ctx, tenantID, start, end, 200)
		if err != nil {
			missing = append(missing, "tunnel.users")
		} else {
			data["users"] = users
		}
		var sessionCount int64
		var totalBytes uint64
		for _, protocol := range protocols {
			sessionCount += protocol.Count
			totalBytes += protocol.TotalBytes
		}
		data["summary"] = map[string]interface{}{
			"protocol_count": len(protocols), "active_users": len(users),
			"session_count": sessionCount, "total_bytes": totalBytes,
			"high_risk_users": countTunnelUsersByRisk(users, "high"),
		}
	case "exfil":
		sources, err := h.queryExfiltrationSources(ctx, tenantID, start, end, 200)
		if err != nil {
			missing = append(missing, "exfil.top_sources")
		} else {
			data["top_sources"] = sources
		}
		risks, err := h.queryExfiltrationRisks(ctx, tenantID, start, end)
		if err != nil {
			missing = append(missing, "exfil.risk_types")
		} else {
			data["risk_types"] = risks
		}
		paths, err := h.queryExfiltrationPaths(ctx, tenantID, start, end, 200)
		if err != nil {
			missing = append(missing, "exfil.paths")
		} else {
			data["paths"] = paths
			topologyNodes, topologyLinks := buildExfiltrationTopicTopology(paths)
			data["topology_nodes"] = topologyNodes
			data["topology_links"] = topologyLinks
		}
		destinations, err := h.queryExfiltrationDestinations(ctx, tenantID, start, end, 200)
		if err != nil {
			missing = append(missing, "exfil.destinations")
		} else {
			data["destinations"] = destinations
		}
		trend, err := h.queryExfiltrationTrend(ctx, tenantID, start, end)
		if err != nil {
			missing = append(missing, "exfil.trend")
		} else {
			data["trend"] = trend
		}
		var uploadBytes uint64
		var sessionCount int64
		var maxSourceUploadBytes uint64
		for _, source := range sources {
			uploadBytes += source.UploadBytes
			sessionCount += source.SessionCount
			if source.UploadBytes > maxSourceUploadBytes {
				maxSourceUploadBytes = source.UploadBytes
			}
		}
		durationSeconds := math.Max(1, float64(end-start)/1000)
		data["summary"] = map[string]interface{}{
			"source_count": len(sources), "path_count": len(paths), "session_count": sessionCount,
			"upload_bytes": uploadBytes, "high_risk_sources": countExfiltrationSourcesByRisk(sources, "high"),
			"destination_count": len(destinations), "risk_type_count": len(risks),
			"peak_upload_gbps": float64(maxSourceUploadBytes) * 8 / durationSeconds / 1_000_000_000,
		}
	case "apt":
		campaigns, total, err := h.queryCampaigns(ctx, tenantID, campaignQueryFilters{}, start, end, 200, 0)
		if err != nil {
			missing = append(missing, "apt.campaigns")
			data["campaigns"] = []campaignDTO{}
			data["phase_distribution"] = map[string]int{}
			data["summary"] = map[string]interface{}{}
		} else {
			phaseDistribution, summary := summarizeAPTTopicCampaigns(campaigns, total)
			data["campaigns"] = campaigns
			data["phase_distribution"] = phaseDistribution
			data["summary"] = summary
			topologyNodes, topologyLinks := buildAPTTopicTopology(campaigns)
			data["topology_nodes"] = topologyNodes
			data["topology_links"] = topologyLinks
		}
	}
	missing = uniqueStrings(missing)
	data["data_mode"] = "partial"
	if len(missing) == 0 {
		data["data_mode"] = "live"
	}
	return topicSnapshotBuild{Data: data, Partial: len(missing) > 0, MissingSections: missing, SourceWatermarks: watermarks}
}

func topicClickHouseSections(topic string) []string {
	switch topic {
	case "tunnel":
		return []string{"tunnel.protocols", "tunnel.users"}
	case "exfil":
		return []string{"exfil.top_sources", "exfil.risk_types", "exfil.paths", "exfil.destinations", "exfil.trend"}
	default:
		return []string{"apt.campaigns"}
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// SubmitTopicAction is a compatibility adapter. New clients use the versioned
// contract; unchanged legacy bodies continue through the old additive route.
func (h *SystemHandler) SubmitTopicAction(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "failed to read topic action request")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var probe map[string]interface{}
	_ = json.Unmarshal(raw, &probe)
	_, hasActionID := probe["action_id"]
	_, hasSnapshotID := probe["snapshot_id"]
	_, hasRevision := probe["expected_revision"]
	if !hasActionID && !hasSnapshotID && !hasRevision && strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		h.SubmitLegacyTopicAction(w, r)
		return
	}
	h.submitTopicActionV2(w, r)
}

func (h *SystemHandler) submitTopicActionV2(w http.ResponseWriter, r *http.Request) {
	if !h.topicExecutorV2 {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "FEATURE_DISABLED", "topic executor v2 is disabled")
		return
	}
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "topic action persistence is unavailable")
		return
	}
	topic := normalizeTopicKey(mux.Vars(r)["topic"])
	if !isValidTopicKey(topic) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
		return
	}
	var req topicActionV2Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid versioned topic action request")
		return
	}
	req.ActionID = strings.ToLower(strings.TrimSpace(req.ActionID))
	if req.ActionID == "" {
		req.ActionID = strings.ToLower(strings.TrimSpace(req.Action))
	}
	req.Target = strings.TrimSpace(req.Target)
	req.SnapshotID = strings.TrimSpace(req.SnapshotID)
	req.Reason = strings.TrimSpace(req.Reason)
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if req.ActionID == "" || req.Target == "" || req.SnapshotID == "" || req.ExpectedRevision <= 0 || req.Reason == "" || idempotencyKey == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "action_id, target, snapshot_id, expected_revision, reason and Idempotency-Key are required")
		return
	}
	if len(idempotencyKey) > 200 || len(req.Target) > 500 || len(req.Reason) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "topic action field exceeds contract limit")
		return
	}
	spec, ok := topicActionCatalog[req.ActionID]
	if !ok {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "ACTION_NOT_CATALOGUED", "topic action is not present in the action catalog")
		return
	}
	if !spec.Enabled {
		httpx.JSONError(w, ctx, http.StatusConflict, "EXECUTOR_UNAVAILABLE", spec.UnavailableCause)
		return
	}
	tenantID := writeTenantID(r)
	var snapshotRevision int64
	if err := h.pgDB.QueryRowContext(ctx, `
		SELECT resource_revision FROM topic_snapshot_manifests
		WHERE snapshot_id=$1::uuid AND tenant_id=$2 AND topic=$3`,
		req.SnapshotID, tenantID, topic).Scan(&snapshotRevision); err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "topic snapshot does not exist for this tenant and topic")
		return
	} else if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to validate topic snapshot")
		return
	}
	if snapshotRevision != req.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "topic snapshot revision no longer matches the requested revision")
		return
	}
	if existing, err := h.loadTopicActionByIdempotency(ctx, tenantID, idempotencyKey); err == nil {
		if !topicActionMatchesRequest(existing, topic, req) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different topic action")
			return
		}
		httpx.JSONContractAccepted(w, ctx, existing.response(), topicActionMeta(ctx, existing))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve topic action idempotency")
		return
	}

	jobID := uuid.NewString()
	traceID := httpx.GetTraceID(ctx)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	detailJSON, _ := json.Marshal(req.Detail)
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin topic action transaction")
		return
	}
	defer tx.Rollback()
	var createdAt time.Time
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO topic_actions
			(action_id, tenant_id, topic, action, target, status, detail, requested_by,
			 idempotency_key, snapshot_id, expected_revision, revision, reason, trace_id, executor)
		VALUES ($1::uuid,$2,$3,$4,$5,'accepted',$6::jsonb,$7,$8,$9::uuid,$10,1,$11,$12,$13)
		RETURNING created_at`,
		jobID, tenantID, topic, req.ActionID, req.Target, string(detailJSON), httpx.GetUserID(ctx),
		idempotencyKey, req.SnapshotID, req.ExpectedRevision, req.Reason, traceID, spec.Executor).Scan(&createdAt); err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback()
			existing, loadErr := h.loadTopicActionByIdempotency(ctx, tenantID, idempotencyKey)
			if loadErr == nil {
				if !topicActionMatchesRequest(existing, topic, req) {
					httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different topic action")
					return
				}
				httpx.JSONContractAccepted(w, ctx, existing.response(), topicActionMeta(ctx, existing))
				return
			}
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist topic action")
		return
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO topic_action_history (job_id, tenant_id, revision, from_status, to_status, detail)
		VALUES ($1::uuid,$2,1,'','accepted',$3::jsonb)`,
		jobID, tenantID, `{"cause":"api_accept"}`); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist topic action history")
		return
	}
	eventID := uuid.NewString()
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "traffic.topic.v2.ActionRequested",
		"tenant_id": tenantID, "topic": topic, "job_id": jobID, "action_id": req.ActionID,
		"snapshot_id": req.SnapshotID, "revision": 1, "trace_id": traceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO topic_action_outbox
			(event_id, job_id, tenant_id, event_type, aggregate_version, partition_key, payload)
		VALUES ($1::uuid,$2::uuid,$3,'traffic.topic.v2.ActionRequested',1,$4,$5::jsonb)`,
		eventID, jobID, tenantID, tenantID+":"+topic, string(eventPayload)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue topic action event")
		return
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "TOPIC_ACTION_REQUESTED", ObjectType: "topic_action", ObjectID: jobID,
		TenantID: tenantID, UserID: httpx.GetUserID(ctx), Result: "accepted",
		Detail: map[string]interface{}{
			"topic": topic, "action_id": req.ActionID, "target": req.Target,
			"snapshot_id": req.SnapshotID, "expected_revision": req.ExpectedRevision,
			"idempotency_key": idempotencyKey, "reason": req.Reason, "trace_id": traceID,
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "failed to audit topic action")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit topic action")
		return
	}
	job := topicActionJobDTO{
		JobID: jobID, ActionID: req.ActionID, TenantID: tenantID, Topic: topic,
		Target: req.Target, SnapshotID: req.SnapshotID, ExpectedRevision: req.ExpectedRevision,
		Revision: 1, Status: "accepted", Executor: spec.Executor, Reason: req.Reason,
		TraceID: traceID, Receipt: map[string]interface{}{}, Error: map[string]interface{}{},
		Attempts: 0, RequestedBy: httpx.GetUserID(ctx), CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	httpx.JSONContractAccepted(w, ctx, job.response(), topicActionMeta(ctx, job))
}

func topicActionMatchesRequest(existing topicActionJobDTO, topic string, req topicActionV2Request) bool {
	return existing.Topic == topic &&
		existing.ActionID == req.ActionID &&
		existing.Target == req.Target &&
		existing.SnapshotID == req.SnapshotID &&
		existing.ExpectedRevision == req.ExpectedRevision &&
		existing.Reason == req.Reason
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func topicActionMeta(ctx context.Context, job topicActionJobDTO) httpx.ContractMeta {
	return httpx.ContractMeta{
		ContractVersion: topicActionContractVersion,
		SnapshotID:      job.SnapshotID,
		AsOf:            job.UpdatedAt.UTC().Format(time.RFC3339Nano),
		TraceID:         job.TraceID,
		Partial:         job.Status == "partial",
		MissingSections: []string{},
		SourceWatermarks: map[string]string{
			"postgresql.topic_actions.revision":    fmt.Sprint(job.Revision),
			"kafka.traffic.topic.action.v2.offset": "outbox_pending",
		},
	}
}

func (h *SystemHandler) GetTopicActionJob(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicReadPermission(w, r) {
		return
	}
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "topic action persistence is unavailable")
		return
	}
	topic := normalizeTopicKey(mux.Vars(r)["topic"])
	jobID := strings.TrimSpace(mux.Vars(r)["job_id"])
	if !isValidTopicKey(topic) || jobID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "valid topic and job_id are required")
		return
	}
	job, err := scanTopicActionJob(h.pgDB.QueryRowContext(ctx, topicActionSelectSQL+`
		WHERE tenant_id=$1 AND topic=$2 AND action_id=$3::uuid`,
		writeTenantID(r), topic, jobID))
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "JOB_NOT_FOUND", "topic action job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load topic action job")
		return
	}
	data := job.response()
	meta := topicActionMeta(ctx, job)
	delivery, watermark, err := h.topicActionDelivery(ctx, job.JobID)
	if err != nil {
		meta.Partial = true
		meta.MissingSections = append(meta.MissingSections, "kafka_delivery")
		meta.SourceWatermarks["kafka.traffic.topic.action.v2.offset"] = "unavailable"
	} else {
		data["outbox"] = delivery
		meta.SourceWatermarks["kafka.traffic.topic.action.v2.offset"] = watermark
	}
	httpx.JSONContractSuccess(w, ctx, data, meta)
}

func (h *SystemHandler) topicActionDelivery(
	ctx context.Context,
	jobID string,
) (map[string]interface{}, string, error) {
	var total, published, attempts int64
	var lastError string
	var publishedAt sql.NullTime
	err := h.pgDB.QueryRowContext(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE published),
		       COALESCE(sum(attempts),0),
		       COALESCE(max(NULLIF(last_error,'')),''),
		       max(published_at)
		FROM topic_action_outbox WHERE job_id=$1::uuid`, jobID).
		Scan(&total, &published, &attempts, &lastError, &publishedAt)
	if err != nil {
		return nil, "", err
	}
	pending := total - published
	delivery := map[string]interface{}{
		"topic": topicActionKafkaTopic, "total": total, "published": published,
		"pending": pending, "attempts": attempts, "last_error": lastError,
	}
	watermark := fmt.Sprintf("outbox_pending:%d/%d", published, total)
	if publishedAt.Valid {
		delivery["published_at"] = publishedAt.Time.UnixMilli()
	}
	if total > 0 && pending == 0 {
		watermark = fmt.Sprintf(
			"producer_acked_without_observed_offset:%d/%d@%s",
			published, total, publishedAt.Time.UTC().Format(time.RFC3339Nano),
		)
	}
	return delivery, watermark, nil
}

const topicActionSelectSQL = `
	SELECT action_id::text, action, tenant_id, topic, target, snapshot_id::text,
	       expected_revision, revision, status, executor, reason, trace_id,
	       receipt::text, error::text, attempts, requested_by, created_at, updated_at
	FROM topic_actions `

func (h *SystemHandler) loadTopicActionByIdempotency(ctx context.Context, tenantID, idempotencyKey string) (topicActionJobDTO, error) {
	return scanTopicActionJob(h.pgDB.QueryRowContext(ctx, topicActionSelectSQL+`
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey))
}

type topicActionScanner interface {
	Scan(...interface{}) error
}

func scanTopicActionJob(scanner topicActionScanner) (topicActionJobDTO, error) {
	var job topicActionJobDTO
	var receiptJSON, errorJSON []byte
	err := scanner.Scan(
		&job.JobID, &job.ActionID, &job.TenantID, &job.Topic, &job.Target, &job.SnapshotID,
		&job.ExpectedRevision, &job.Revision, &job.Status, &job.Executor, &job.Reason,
		&job.TraceID, &receiptJSON, &errorJSON, &job.Attempts, &job.RequestedBy,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return topicActionJobDTO{}, err
	}
	job.Receipt = map[string]interface{}{}
	job.Error = map[string]interface{}{}
	_ = json.Unmarshal(receiptJSON, &job.Receipt)
	_ = json.Unmarshal(errorJSON, &job.Error)
	return job, nil
}

// StartTopicActionWorker runs the durable internal executor. Only catalog
// entries bound to internal_receipt are claimed; unavailable external bindings
// are rejected at submission instead of being reported as fake success.
func (h *SystemHandler) StartTopicActionWorker(ctx context.Context, interval time.Duration) error {
	if h.pgDB == nil {
		return fmt.Errorf("topic action database is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := h.processOneTopicAction(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, context.Canceled) {
				if h.logger != nil {
					h.logger.Warn("Failed to process topic action job", zap.Error(err))
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *SystemHandler) processOneTopicAction(ctx context.Context) error {
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	job, err := scanTopicActionJob(tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT action_id FROM topic_actions
			WHERE executor='internal_receipt'
			  AND attempts < $1
			  AND (status='accepted' OR (status='running' AND lease_until < now()))
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE topic_actions a
		SET status='running', revision=a.revision+1, attempts=a.attempts+1,
		    lease_until=now()+$2::interval, updated_at=now()
		FROM candidate c WHERE a.action_id=c.action_id
		RETURNING a.action_id::text, a.action, a.tenant_id, a.topic, a.target,
		          a.snapshot_id::text, a.expected_revision, a.revision, a.status,
		          a.executor, a.reason, a.trace_id, a.receipt::text, a.error::text,
		          a.attempts, a.requested_by, a.created_at, a.updated_at`,
		topicActionMaxAttempts, fmt.Sprintf("%.0f seconds", topicActionLease.Seconds())))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO topic_action_history (job_id, tenant_id, revision, from_status, to_status, detail)
		VALUES ($1::uuid,$2,$3,'accepted','running',$4::jsonb)`,
		job.JobID, job.TenantID, job.Revision, `{"executor":"internal_receipt"}`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	receipt := map[string]interface{}{
		"receipt_id": uuid.NewString(), "executor": "internal_receipt",
		"operation": job.ActionID, "topic": job.Topic, "target": job.Target,
		"snapshot_id": job.SnapshotID, "verified_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	receiptJSON, _ := json.Marshal(receipt)
	resultEventID := uuid.NewString()
	completeTx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return h.releaseTopicActionLease(ctx, job.JobID, err)
	}
	defer completeTx.Rollback()
	failCompletion := func(cause error) error {
		_ = completeTx.Rollback()
		return h.releaseTopicActionLease(ctx, job.JobID, cause)
	}
	var completedRevision int64
	if err := completeTx.QueryRowContext(ctx, `
		UPDATE topic_actions SET status='completed', revision=revision+1,
		       receipt=$2::jsonb, error='{}'::jsonb, lease_until=NULL, updated_at=now()
		WHERE action_id=$1::uuid AND status='running'
		RETURNING revision`, job.JobID, string(receiptJSON)).Scan(&completedRevision); err != nil {
		return failCompletion(err)
	}
	if _, err := completeTx.ExecContext(ctx, `
		INSERT INTO topic_action_receipts (receipt_id, job_id, tenant_id, executor, effect_type, effect_ref, receipt)
		VALUES ($1::uuid,$2::uuid,$3,'internal_receipt',$4,$5,$6::jsonb)`,
		receipt["receipt_id"], job.JobID, job.TenantID, job.ActionID,
		fmt.Sprintf("topic/%s/%s/%s", job.Topic, job.ActionID, job.Target), string(receiptJSON)); err != nil {
		return failCompletion(err)
	}
	if _, err := completeTx.ExecContext(ctx, `
		INSERT INTO topic_action_history (job_id, tenant_id, revision, from_status, to_status, detail)
		VALUES ($1::uuid,$2,$3,'running','completed',$4::jsonb)`,
		job.JobID, job.TenantID, completedRevision, string(receiptJSON)); err != nil {
		return failCompletion(err)
	}
	resultPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": resultEventID, "event_type": "traffic.topic.v2.ActionResult",
		"tenant_id": job.TenantID, "topic": job.Topic, "job_id": job.JobID,
		"action_id": job.ActionID, "revision": completedRevision,
		"status": "completed", "receipt": receipt, "trace_id": job.TraceID,
	})
	if _, err := completeTx.ExecContext(ctx, `
		INSERT INTO topic_action_outbox
			(event_id, job_id, tenant_id, event_type, aggregate_version, partition_key, payload)
		VALUES ($1::uuid,$2::uuid,$3,'traffic.topic.v2.ActionResult',$4,$5,$6::jsonb)`,
		resultEventID, job.JobID, job.TenantID, completedRevision,
		job.TenantID+":"+job.Topic, string(resultPayload)); err != nil {
		return failCompletion(err)
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, completeTx, nil, AlertActionAuditRecord{
		Action: "TOPIC_ACTION_COMPLETED", ObjectType: "topic_action", ObjectID: job.JobID,
		TenantID: job.TenantID, UserID: job.RequestedBy, Result: "success",
		Detail: map[string]interface{}{"trace_id": job.TraceID, "receipt": receipt},
	}); err != nil {
		return failCompletion(err)
	}
	return completeTx.Commit()
}

func (h *SystemHandler) releaseTopicActionLease(ctx context.Context, jobID string, cause error) error {
	errorJSON, _ := json.Marshal(map[string]interface{}{"code": "EXECUTOR_FAILED", "message": cause.Error()})
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%v; begin lease release: %w", cause, err)
	}
	defer tx.Rollback()
	var tenantID, status string
	var revision int64
	releaseErr := tx.QueryRowContext(ctx, `
		UPDATE topic_actions
		SET status=CASE WHEN attempts < $2 THEN 'accepted' ELSE 'failed' END,
		    revision=revision+1, error=$3::jsonb, lease_until=NULL, updated_at=now()
		WHERE action_id=$1::uuid
		RETURNING tenant_id,status,revision`, jobID, topicActionMaxAttempts, string(errorJSON)).
		Scan(&tenantID, &status, &revision)
	if releaseErr != nil {
		return fmt.Errorf("%v; release lease: %w", cause, releaseErr)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO topic_action_history (job_id,tenant_id,revision,from_status,to_status,detail)
		VALUES ($1::uuid,$2,$3,'running',$4,$5::jsonb)`,
		jobID, tenantID, revision, status, string(errorJSON)); err != nil {
		return fmt.Errorf("%v; record lease release: %w", cause, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%v; commit lease release: %w", cause, err)
	}
	return cause
}
