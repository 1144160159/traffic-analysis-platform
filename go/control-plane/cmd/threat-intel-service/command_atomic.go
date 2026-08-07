package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
)

var (
	errThreatIntelIdempotencyConflict = errors.New("threat intel idempotency key payload conflict")
	errThreatIntelRevisionConflict    = errors.New("threat intel resource revision conflict")
)

type threatIntelCommandMeta struct {
	ActionID         string
	IdempotencyKey   string
	RequestSHA256    string
	CommandType      string
	ExpectedRevision int64
	Reason           string
	TraceID          string
	Compatibility    bool
}

type threatIntelCommandReceipt struct {
	EventID          string                   `json:"event_id"`
	ActionID         string                   `json:"action_id"`
	CommandType      string                   `json:"command_type"`
	AggregateVersion int64                    `json:"aggregate_version"`
	Entries          []threatintel.IntelEntry `json:"entries,omitempty"`
	Feed             *threatintel.FeedSource  `json:"feed,omitempty"`
	Replayed         bool                     `json:"replayed"`
	Compatibility    bool                     `json:"compatibility_mode"`
}

type threatIntelCommand struct {
	Entries  []threatintel.IntelEntry
	Feed     *threatintel.FeedSource
	Event    threatIntelEvent
	Action   string
	ObjectID string
	Meta     threatIntelCommandMeta
}

func newThreatIntelCommandMeta(
	r *http.Request,
	tenantID string,
	commandType string,
	payload interface{},
	expectedRevision int64,
) (threatIntelCommandMeta, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return threatIntelCommandMeta{}, fmt.Errorf("marshal threat intel command: %w", err)
	}
	digest := sha256.Sum256(encoded)
	requestHash := hex.EncodeToString(digest[:])
	idempotencyKey := ""
	actionID := ""
	reason := ""
	traceID := ""
	if r != nil {
		idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		actionID = strings.TrimSpace(r.Header.Get("X-Action-Id"))
		reason = strings.TrimSpace(r.Header.Get("X-Action-Reason"))
		traceID = strings.TrimSpace(httpx.GetTraceID(r.Context()))
	}
	compatibility := false
	if idempotencyKey == "" {
		idempotencyKey = "compat:" + requestHash
		compatibility = true
	}
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		return threatIntelCommandMeta{}, fmt.Errorf("idempotency key length must be between 16 and 200")
	}
	if actionID == "" {
		actionID = idempotencyKey
		compatibility = true
	}
	if reason == "" {
		reason = "legacy_compatibility"
		compatibility = true
	}
	if traceID == "" {
		traceID = actionID
	}
	return threatIntelCommandMeta{
		ActionID: actionID, IdempotencyKey: idempotencyKey,
		RequestSHA256: requestHash, CommandType: commandType,
		ExpectedRevision: max(expectedRevision, 0), Reason: reason,
		TraceID: traceID, Compatibility: compatibility,
	}, nil
}

func deterministicThreatIntelEventID(tenantID, idempotencyKey string) string {
	value := tenantID + "\x00" + idempotencyKey
	return "ti-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(value)).String()
}

func (s *server) commitThreatIntelCommand(
	ctx context.Context,
	r *http.Request,
	command threatIntelCommand,
) (*threatIntelCommandReceipt, error) {
	if s.auditDB == nil {
		return nil, fmt.Errorf("threat intel database is not configured")
	}
	if command.Event.TenantID == "" || command.Meta.IdempotencyKey == "" || command.Meta.RequestSHA256 == "" {
		return nil, fmt.Errorf("incomplete threat intel command metadata")
	}
	for index := range command.Entries {
		if command.Entries[index].TenantID != command.Event.TenantID {
			return nil, fmt.Errorf("threat intel entry tenant mismatch")
		}
	}
	if command.Feed != nil && command.Feed.TenantID != command.Event.TenantID {
		return nil, fmt.Errorf("threat intel feed tenant mismatch")
	}
	command.Event.EventID = deterministicThreatIntelEventID(command.Event.TenantID, command.Meta.IdempotencyKey)
	command.Event.ActionID = command.Meta.ActionID
	command.Event.Reason = command.Meta.Reason
	command.Event.CompatibilityMode = command.Meta.Compatibility
	command.Event.TraceID = command.Meta.TraceID

	tx, err := s.auditDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin threat intel command: %w", err)
	}
	defer tx.Rollback()

	if receipt, found, err := loadThreatIntelCommandReplay(ctx, tx, command.Event.TenantID, command.Meta); err != nil {
		return nil, err
	} else if found {
		receipt.Replayed = true
		return receipt, nil
	}

	resultingRevision := int64(0)
	for index := range command.Entries {
		entry := &command.Entries[index]
		currentRevision, err := lockThreatIntelEntryRevision(ctx, tx, entry.TenantID, entry.Type, entry.Value)
		if err != nil {
			return nil, err
		}
		if entry.Revision > 0 && entry.Revision != currentRevision {
			return nil, fmt.Errorf("%w: entry %s:%s expected=%d actual=%d",
				errThreatIntelRevisionConflict, entry.Type, entry.Value, entry.Revision, currentRevision)
		}
		if !command.Meta.Compatibility && currentRevision > 0 && entry.Revision == 0 {
			return nil, fmt.Errorf("%w: strict entry update requires revision", errThreatIntelRevisionConflict)
		}
		entry.Revision = currentRevision + 1
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO threat_intel (
				tenant_id,type,value,reputation,category,source,description,last_seen,revision
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (tenant_id,type,value) DO UPDATE SET
				reputation=EXCLUDED.reputation,category=EXCLUDED.category,source=EXCLUDED.source,
				description=EXCLUDED.description,last_seen=EXCLUDED.last_seen,
				revision=EXCLUDED.revision,updated_at=now()`,
			entry.TenantID, entry.Type, entry.Value, entry.Reputation,
			entry.Category, entry.Source, entry.Description, entry.LastSeen, entry.Revision,
		); err != nil {
			return nil, fmt.Errorf("upsert authoritative threat intel entry %d: %w", index, err)
		}
		resultingRevision = max(resultingRevision, entry.Revision)
	}

	if command.Feed != nil {
		feed := command.Feed
		currentRevision, err := lockThreatIntelFeedRevision(ctx, tx, feed.TenantID, feed.Name)
		if err != nil {
			return nil, err
		}
		if feed.Revision > 0 && feed.Revision != currentRevision {
			return nil, fmt.Errorf("%w: feed %s expected=%d actual=%d",
				errThreatIntelRevisionConflict, feed.Name, feed.Revision, currentRevision)
		}
		if !command.Meta.Compatibility && currentRevision > 0 && feed.Revision == 0 {
			return nil, fmt.Errorf("%w: strict feed update requires revision", errThreatIntelRevisionConflict)
		}
		feed.Revision = currentRevision + 1
		entriesJSON, err := json.Marshal(feed.Entries)
		if err != nil {
			return nil, fmt.Errorf("marshal threat intel feed entries: %w", err)
		}
		row := tx.QueryRowContext(ctx, `
			INSERT INTO threat_intel_feeds (
				tenant_id,name,enabled,interval_seconds,entries,last_run_at,next_run_at,
				last_status,last_error,run_count,revision
			) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (tenant_id,name) DO UPDATE SET
				enabled=EXCLUDED.enabled,interval_seconds=EXCLUDED.interval_seconds,
				entries=EXCLUDED.entries,last_run_at=EXCLUDED.last_run_at,
				next_run_at=EXCLUDED.next_run_at,last_status=EXCLUDED.last_status,
				last_error=EXCLUDED.last_error,run_count=EXCLUDED.run_count,
				revision=EXCLUDED.revision,updated_at=now()
			RETURNING id::text,created_at,updated_at`,
			feed.TenantID, feed.Name, feed.Enabled, feed.IntervalSeconds, string(entriesJSON),
			feed.LastRunAt, feed.NextRunAt, feed.LastStatus, feed.LastError, feed.RunCount, feed.Revision,
		)
		if err := row.Scan(&feed.ID, &feed.CreatedAt, &feed.UpdatedAt); err != nil {
			return nil, fmt.Errorf("upsert authoritative threat intel feed: %w", err)
		}
		resultingRevision = max(resultingRevision, feed.Revision)
	}
	if resultingRevision == 0 {
		return nil, fmt.Errorf("threat intel command has no aggregate mutation")
	}

	command.Event.AggregateVersion = resultingRevision
	command.Event.Entry = nil
	command.Event.Entries = command.Entries
	if len(command.Entries) == 1 {
		command.Event.Entry = &command.Entries[0]
		command.Event.Entries = nil
	}
	command.Event.Feed = command.Feed
	command.Event.Count = len(command.Entries)
	payload, err := json.Marshal(command.Event)
	if err != nil {
		return nil, fmt.Errorf("marshal threat intel outbox event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO threat_intel_event_outbox (event_id,tenant_id,partition_key,payload)
		VALUES ($1,$2,$3,$4::jsonb)`,
		command.Event.EventID, command.Event.TenantID, command.Event.TenantID, string(payload),
	); err != nil {
		return nil, fmt.Errorf("insert threat intel event outbox: %w", err)
	}
	if err := s.insertThreatIntelAudit(ctx, tx, r, command.Event, command.Action, command.ObjectID); err != nil {
		return nil, err
	}
	for index := range command.Entries {
		entry := command.Entries[index]
		if err := insertThreatIntelHistory(ctx, tx, command, "entry", entry.Type+":"+entry.Value, entry.Revision, entry); err != nil {
			return nil, err
		}
	}
	if command.Feed != nil {
		if err := insertThreatIntelHistory(ctx, tx, command, "feed", command.Feed.Name, command.Feed.Revision, command.Feed); err != nil {
			return nil, err
		}
	}
	receipt := &threatIntelCommandReceipt{
		EventID: command.Event.EventID, ActionID: command.Meta.ActionID,
		CommandType: command.Meta.CommandType, AggregateVersion: resultingRevision,
		Entries: command.Entries, Feed: command.Feed,
		Compatibility: command.Meta.Compatibility,
	}
	responseJSON, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO threat_intel_command_requests (
			tenant_id,idempotency_key,request_sha256,action_id,command_type,
			expected_revision,resulting_revision,event_id,reason,trace_id,
			compatibility_mode,response_payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)`,
		command.Event.TenantID, command.Meta.IdempotencyKey, command.Meta.RequestSHA256,
		command.Meta.ActionID, command.Meta.CommandType, command.Meta.ExpectedRevision,
		resultingRevision, command.Event.EventID, command.Meta.Reason, command.Meta.TraceID,
		command.Meta.Compatibility, string(responseJSON),
	); err != nil {
		return nil, fmt.Errorf("insert threat intel command request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit threat intel command: %w", err)
	}
	s.intel.CacheCommittedEntries(command.Entries)
	return receipt, nil
}

func loadThreatIntelCommandReplay(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	meta threatIntelCommandMeta,
) (*threatIntelCommandReceipt, bool, error) {
	var requestHash string
	var responseJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT request_sha256,response_payload::text
		FROM threat_intel_command_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, meta.IdempotencyKey,
	).Scan(&requestHash, &responseJSON)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load threat intel command replay: %w", err)
	}
	if requestHash != meta.RequestSHA256 {
		return nil, false, errThreatIntelIdempotencyConflict
	}
	var receipt threatIntelCommandReceipt
	if err := json.Unmarshal(responseJSON, &receipt); err != nil {
		return nil, false, fmt.Errorf("decode threat intel command replay: %w", err)
	}
	return &receipt, true, nil
}

func lockThreatIntelEntryRevision(ctx context.Context, tx *sql.Tx, tenantID, typ, value string) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `
		SELECT revision FROM threat_intel
		WHERE tenant_id=$1 AND type=$2 AND value=$3
		FOR UPDATE`, tenantID, typ, value).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock threat intel entry revision: %w", err)
	}
	return revision, nil
}

func lockThreatIntelFeedRevision(ctx context.Context, tx *sql.Tx, tenantID, name string) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `
		SELECT revision FROM threat_intel_feeds
		WHERE tenant_id=$1 AND name=$2
		FOR UPDATE`, tenantID, name).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock threat intel feed revision: %w", err)
	}
	return revision, nil
}

func insertThreatIntelHistory(
	ctx context.Context,
	tx *sql.Tx,
	command threatIntelCommand,
	aggregateType string,
	aggregateID string,
	revision int64,
	snapshot interface{},
) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal threat intel history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO threat_intel_command_history (
			event_id,tenant_id,aggregate_type,aggregate_id,revision,action_id,
			operation,reason,trace_id,compatibility_mode,snapshot
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)`,
		command.Event.EventID, command.Event.TenantID, aggregateType, aggregateID, revision,
		command.Meta.ActionID, command.Meta.CommandType, command.Meta.Reason,
		command.Meta.TraceID, command.Meta.Compatibility, string(encoded),
	); err != nil {
		return fmt.Errorf("insert threat intel command history: %w", err)
	}
	return nil
}

func writeThreatIntelCommandError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	code := "THREAT_INTEL_TRANSACTION_FAILED"
	if errors.Is(err, errThreatIntelIdempotencyConflict) || errors.Is(err, errThreatIntelRevisionConflict) {
		status = http.StatusConflict
		code = "THREAT_INTEL_COMMAND_CONFLICT"
	}
	httpx.JSONError(w, r.Context(), status, code, err.Error())
}
