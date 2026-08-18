package restoration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/extractor"
	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

var (
	ErrIdempotencyConflict = errors.New("file restoration idempotency conflict")
	ErrRequestInProgress   = errors.New("file restoration request is already in progress")
)
var requestSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type DB interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type FiveTuple struct {
	SourceIP        string `json:"source_ip"`
	DestinationIP   string `json:"destination_ip"`
	SourcePort      uint16 `json:"source_port"`
	DestinationPort uint16 `json:"destination_port"`
	Protocol        uint8  `json:"protocol"`
}

type PacketRange struct {
	ObjectBucket  string `json:"object_bucket"`
	ObjectKey     string `json:"object_key"`
	ObjectVersion string `json:"object_version"`
	Start         uint64 `json:"start"`
	End           uint64 `json:"end"`
}

type FlowSelection struct {
	Role        string    `json:"role"`
	CommunityID string    `json:"community_id"`
	FlowID      string    `json:"flow_id"`
	FiveTuple   FiveTuple `json:"five_tuple"`
	Direction   string    `json:"direction"`
}

type Manifest struct {
	RestorationID        uuid.UUID
	TenantID             string
	Revision             int64
	IdempotencyKey       string
	SessionID            string
	CommunityID          string
	SessionAuthority     forensicsindex.RestorationSessionAuthority
	PrimaryFlowID        string
	FlowIDs              []string
	FlowSelections       []FlowSelection
	PcapIndexIDs         []string
	SourceObjectReceipts []s3client.ObjectAuthority
	FiveTuple            FiveTuple
	Direction            string
	CaptureTimeStart     time.Time
	CaptureTimeEnd       time.Time
	PacketRanges         []PacketRange
	TCPSequenceRanges    []reassembly.SourceRange
	Extraction           extractor.Result
	Object               *s3client.ObjectAuthority
	TraceID              string
	CreatedAt            time.Time
	CompletedAt          time.Time
}

type CommitCommand struct {
	ActorID        string
	Reason         string
	TraceID        string
	ClaimToken     uuid.UUID
	IdempotencyKey string
	RequestSHA256  string
	Manifest       Manifest
}

type CommitReceipt struct {
	TenantID      string    `json:"tenant_id"`
	RestorationID string    `json:"restoration_id"`
	Revision      int64     `json:"revision"`
	Status        string    `json:"status"`
	ObjectSHA256  string    `json:"object_sha256"`
	EventID       string    `json:"event_id"`
	OutboxStatus  string    `json:"outbox_status"`
	CreatedAt     time.Time `json:"created_at"`
	Replayed      bool      `json:"replayed"`
}

type AdmissionResult string

const (
	AdmissionClaimed    AdmissionResult = "claimed"
	AdmissionInProgress AdmissionResult = "in_progress"
	AdmissionReplay     AdmissionResult = "replay"
)

type AdmissionReceipt struct {
	Result     AdmissionResult
	ClaimToken uuid.UUID
	Receipt    *CommitReceipt
}

type OrphanReconciliationReport struct {
	Scanned    int
	Reconciled int
	Conflicts  int
	Pending    int
}

type Repository struct{ db DB }

func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("file restoration database is required")
	}
	return &Repository{db: db}, nil
}

// VerifySchema makes activation fail closed unless the separately approved
// expand migration is both registered and complete. Default-off deployments
// do not need the tables and therefore do not run this check.
func (repository *Repository) VerifySchema(ctx context.Context) error {
	var ready bool
	err := repository.db.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM alignment_schema_migrations WHERE version='202608141300')
		AND to_regclass('public.file_restoration_manifests') IS NOT NULL
		AND to_regclass('public.file_restoration_outbox') IS NOT NULL
		AND to_regclass('public.file_restoration_requests') IS NOT NULL
		AND to_regclass('public.file_restoration_audit') IS NOT NULL
		AND to_regclass('public.file_restoration_orphans') IS NOT NULL
		AND EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='file_restoration_manifests' AND column_name='session_authority'
		)
		AND EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='file_restoration_manifests' AND column_name='primary_flow_id'
		)
		AND EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='file_restoration_manifests' AND column_name='flow_selections'
		)`).Scan(&ready)
	if err != nil {
		return fmt.Errorf("verify restoration authority schema: %w", err)
	}
	if !ready {
		return errors.New("restoration authority expand migration 202608141300 is not complete")
	}
	return nil
}

func deterministicRestorationID(tenantID, idempotencyKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-restoration\x00"+tenantID+"\x00"+idempotencyKey))
}

func deterministicEventID(tenantID, idempotencyKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-restoration-event\x00"+tenantID+"\x00"+idempotencyKey))
}

func canonicalSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateSortedUnique(values []string, label string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty identity", label)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("%s must be sorted and unique", label)
		}
	}
	return nil
}

func (manifest Manifest) Validate() error {
	if strings.TrimSpace(manifest.TenantID) == "" || strings.TrimSpace(manifest.IdempotencyKey) == "" {
		return errors.New("restoration tenant and idempotency key are required")
	}
	if len(manifest.IdempotencyKey) < 16 || len(manifest.IdempotencyKey) > 200 {
		return errors.New("restoration idempotency key length must be between 16 and 200")
	}
	if manifest.RestorationID != deterministicRestorationID(manifest.TenantID, manifest.IdempotencyKey) {
		return errors.New("restoration identity is not deterministically bound to tenant/idempotency key")
	}
	if manifest.Revision != 1 {
		return errors.New("new restoration manifest revision must be one")
	}
	if strings.TrimSpace(manifest.SessionID) == "" || strings.TrimSpace(manifest.CommunityID) == "" {
		return errors.New("restoration session and community identities are required")
	}
	if manifest.Direction != "client_to_server" && manifest.Direction != "server_to_client" {
		return errors.New("restoration direction is invalid")
	}
	if manifest.CaptureTimeStart.IsZero() || manifest.CaptureTimeEnd.Before(manifest.CaptureTimeStart) {
		return errors.New("restoration capture window is invalid")
	}
	if manifest.CreatedAt.IsZero() || manifest.CompletedAt.Before(manifest.CreatedAt) {
		return errors.New("restoration processing timestamps are invalid")
	}
	if strings.TrimSpace(manifest.TraceID) == "" {
		return errors.New("restoration trace identity is required")
	}
	if err := validateSortedUnique(manifest.FlowIDs, "flow_ids"); err != nil {
		return err
	}
	flowPosition := sort.SearchStrings(manifest.FlowIDs, manifest.PrimaryFlowID)
	if strings.TrimSpace(manifest.PrimaryFlowID) == "" || flowPosition == len(manifest.FlowIDs) || manifest.FlowIDs[flowPosition] != manifest.PrimaryFlowID {
		return errors.New("restoration primary flow is not present in flow_ids")
	}
	if err := manifest.SessionAuthority.Validate(forensicsindex.RestorationSessionQuery{
		TenantID: manifest.TenantID, SessionID: manifest.SessionID, CommunityID: manifest.CommunityID,
		PrimaryFlowID: manifest.PrimaryFlowID, StartTime: manifest.CaptureTimeStart, EndTime: manifest.CaptureTimeEnd,
	}); err != nil {
		return fmt.Errorf("invalid restoration session authority: %w", err)
	}
	if len(manifest.FlowSelections) < 1 || len(manifest.FlowSelections) > 2 {
		return errors.New("restoration flow selections must contain one primary and at most one FTP data flow")
	}
	primary := manifest.FlowSelections[0]
	if primary.Role != "primary" || primary.CommunityID != manifest.CommunityID || primary.FlowID != manifest.PrimaryFlowID ||
		primary.FiveTuple != manifest.FiveTuple || primary.Direction != manifest.Direction {
		return errors.New("restoration primary flow selection differs from manifest authority")
	}
	selectedFlowIDs := []string{primary.FlowID}
	if len(manifest.FlowSelections) == 2 {
		data := manifest.FlowSelections[1]
		if manifest.Extraction.ProfileID != extractor.ProfileFTPPassive || data.Role != "ftp_data" ||
			data.CommunityID == "" || data.CommunityID == primary.CommunityID || data.FlowID == "" || data.FlowID == primary.FlowID ||
			data.Direction != "server_to_client" || data.FiveTuple.Protocol != 6 {
			return errors.New("restoration FTP data flow selection is invalid")
		}
		selectedFlowIDs = append(selectedFlowIDs, data.FlowID)
	}
	sort.Strings(selectedFlowIDs)
	if len(selectedFlowIDs) != len(manifest.FlowIDs) {
		return errors.New("restoration flow selections differ from flow_ids")
	}
	for index := range selectedFlowIDs {
		if selectedFlowIDs[index] != manifest.FlowIDs[index] {
			return errors.New("restoration flow selections differ from flow_ids")
		}
	}
	if err := validateSortedUnique(manifest.PcapIndexIDs, "pcap_index_ids"); err != nil {
		return err
	}
	if len(manifest.FlowIDs) == 0 || len(manifest.PcapIndexIDs) == 0 || len(manifest.SourceObjectReceipts) == 0 {
		return errors.New("restoration source proof is incomplete")
	}
	sourceAuthorities := make(map[string]s3client.ObjectAuthority, len(manifest.SourceObjectReceipts))
	for index, source := range manifest.SourceObjectReceipts {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("invalid source object receipt: %w", err)
		}
		sourceKey := source.Bucket + "\x00" + source.Key + "\x00" + source.VersionID
		if index > 0 {
			prior := manifest.SourceObjectReceipts[index-1]
			priorKey := prior.Bucket + "\x00" + prior.Key + "\x00" + prior.VersionID
			if priorKey >= sourceKey {
				return errors.New("restoration source object receipts are not canonically sorted")
			}
		}
		if _, duplicate := sourceAuthorities[sourceKey]; duplicate {
			return errors.New("restoration source object receipts are duplicated")
		}
		sourceAuthorities[sourceKey] = source
	}
	for index, packet := range manifest.PacketRanges {
		if packet.ObjectBucket == "" || packet.ObjectKey == "" || packet.ObjectVersion == "" || packet.End <= packet.Start {
			return fmt.Errorf("packet range %d is invalid", index)
		}
		source, authorized := sourceAuthorities[packet.ObjectBucket+"\x00"+packet.ObjectKey+"\x00"+packet.ObjectVersion]
		if !authorized || packet.End > uint64(source.SizeBytes) {
			return fmt.Errorf("packet range %d escapes source object authority", index)
		}
		if index > 0 {
			prior := manifest.PacketRanges[index-1]
			priorKey := prior.ObjectBucket + "\x00" + prior.ObjectKey + "\x00" + prior.ObjectVersion
			packetKey := packet.ObjectBucket + "\x00" + packet.ObjectKey + "\x00" + packet.ObjectVersion
			if priorKey > packetKey || (priorKey == packetKey && prior.End > packet.Start) {
				return errors.New("packet ranges overlap or are unsorted")
			}
		}
	}
	for index, sourceRange := range manifest.TCPSequenceRanges {
		if sourceRange.ObjectBucket == "" || sourceRange.ObjectKey == "" || sourceRange.ObjectVersion == "" ||
			sourceRange.SequenceStart == sourceRange.SequenceEnd ||
			sourceRange.ObjectRangeEnd <= sourceRange.ObjectRangeStart || sourceRange.Length <= 0 ||
			uint32(sourceRange.Length) != sourceRange.SequenceEnd-sourceRange.SequenceStart {
			return fmt.Errorf("TCP sequence source range %d is invalid", index)
		}
		authorized := false
		for _, packetRange := range manifest.PacketRanges {
			if packetRange.ObjectBucket == sourceRange.ObjectBucket && packetRange.ObjectKey == sourceRange.ObjectKey &&
				packetRange.ObjectVersion == sourceRange.ObjectVersion && sourceRange.ObjectRangeStart >= packetRange.Start &&
				sourceRange.ObjectRangeEnd <= packetRange.End {
				authorized = true
				break
			}
		}
		if !authorized {
			return fmt.Errorf("TCP sequence source range %d escapes packet authority", index)
		}
		if index > 0 {
			prior := manifest.TCPSequenceRanges[index-1]
			if !sequenceBefore(prior.SequenceStart, sourceRange.SequenceStart) || sequenceBefore(sourceRange.SequenceStart, prior.SequenceEnd) {
				return errors.New("TCP sequence source ranges overlap or are unsorted")
			}
		}
	}
	extraction := manifest.Extraction
	if extraction.Executable || extraction.AutomaticOpen || extraction.AutomaticDecompress || !extraction.Quarantined {
		return errors.New("restoration extraction is not inert and quarantined")
	}
	allowedStatus := map[extractor.Status]bool{
		extractor.StatusComplete: true, extractor.StatusPartial: true,
		extractor.StatusTruncated: true, extractor.StatusCorrupt: true,
		extractor.StatusOversize: true, extractor.StatusUnsupported: true,
	}
	if !allowedStatus[extraction.Status] || extraction.ProfileID == "" || extraction.ParserName == "" || extraction.ParserVersion == "" || extraction.AlgorithmVersion == "" {
		return errors.New("restoration extraction identity or terminal status is invalid")
	}
	if extraction.Status == extractor.StatusComplete && len(extraction.MissingRanges) > 0 {
		return errors.New("complete restoration has missing sequence ranges")
	}
	if extraction.Status == extractor.StatusUnsupported && manifest.Object != nil {
		return errors.New("unsupported restoration must not have an object")
	}
	if extraction.Status == extractor.StatusComplete && manifest.Object == nil {
		return errors.New("complete restoration requires a trusted object receipt")
	}
	if manifest.Object != nil {
		if err := manifest.Object.Validate(); err != nil {
			return fmt.Errorf("invalid restored object receipt: %w", err)
		}
		expectedPrefix := "tenants/" + manifest.TenantID + "/restorations/" + manifest.RestorationID.String() + "/"
		if !strings.HasPrefix(manifest.Object.Key, expectedPrefix) {
			return errors.New("restored object key is not tenant/restoration scoped")
		}
		if manifest.Object.SHA256 != extraction.ContentSHA256 || manifest.Object.SizeBytes != extraction.RestoredSize {
			return errors.New("restored object receipt differs from extracted content")
		}
		if !manifest.Object.RetentionUntil.After(manifest.CompletedAt) {
			return errors.New("restored object retention expired before manifest commit")
		}
	}
	return nil
}

func (command CommitCommand) Validate() error {
	if strings.TrimSpace(command.ActorID) == "" || strings.TrimSpace(command.Reason) == "" || strings.TrimSpace(command.TraceID) == "" {
		return errors.New("restoration actor, reason and trace are required")
	}
	if command.ClaimToken == uuid.Nil {
		return errors.New("restoration admission claim token is required")
	}
	if command.IdempotencyKey != command.Manifest.IdempotencyKey || command.TraceID != command.Manifest.TraceID {
		return errors.New("restoration command and manifest identity drift")
	}
	if !requestSHA256Pattern.MatchString(command.RequestSHA256) {
		return errors.New("restoration command request SHA-256 is invalid")
	}
	return command.Manifest.Validate()
}

func marshalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal restoration JSON: %w", err)
	}
	return encoded, nil
}

// PostgreSQL authority columns require JSON arrays, while encoding/json emits
// null for a nil Go slice. Normalize nil to [] so an empty but valid proof does
// not fail the database contract or acquire a second wire representation.
func marshalJSONArray[T any](values []T) ([]byte, error) {
	if values == nil {
		values = []T{}
	}
	return marshalJSON(values)
}

func objectValues(object *s3client.ObjectAuthority) (string, string, string, string, int64, string, any, any, bool) {
	if object == nil {
		return "", "", "", "", 0, "", nil, nil, false
	}
	var retention any
	if !object.RetentionUntil.IsZero() {
		retention = object.RetentionUntil
	}
	return object.Bucket, object.Key, object.VersionID, object.ETag, object.SizeBytes,
		object.SHA256, object.ObservedAt, retention, object.LegalHold
}

func extractionValues(extraction extractor.Result) (any, any) {
	var declared any
	if extraction.DeclaredSize != nil {
		declared = *extraction.DeclaredSize
	}
	var truncation any
	if extraction.TruncationOffset != nil {
		truncation = int64(*extraction.TruncationOffset)
	}
	return declared, truncation
}

func (repository *Repository) Commit(ctx context.Context, command CommitCommand) (*CommitReceipt, error) {
	if err := command.Validate(); err != nil {
		return nil, err
	}
	requestSHA := command.RequestSHA256
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin restoration authority transaction: %w", err)
	}
	defer tx.Rollback()
	if receipt, found, replayErr := loadReplay(ctx, tx, command.Manifest.TenantID, command.IdempotencyKey, requestSHA); replayErr != nil {
		return nil, replayErr
	} else if found {
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit restoration replay: %w", err)
		}
		return receipt, nil
	}

	manifest := command.Manifest
	flowIDs, err := marshalJSONArray(manifest.FlowIDs)
	if err != nil {
		return nil, err
	}
	pcapIDs, err := marshalJSONArray(manifest.PcapIndexIDs)
	if err != nil {
		return nil, err
	}
	sourceReceipts, err := marshalJSONArray(manifest.SourceObjectReceipts)
	if err != nil {
		return nil, err
	}
	sessionAuthority, err := marshalJSON(manifest.SessionAuthority)
	if err != nil {
		return nil, err
	}
	flowSelections, err := marshalJSONArray(manifest.FlowSelections)
	if err != nil {
		return nil, err
	}
	fiveTuple, err := marshalJSON(manifest.FiveTuple)
	if err != nil {
		return nil, err
	}
	packetRanges, err := marshalJSONArray(manifest.PacketRanges)
	if err != nil {
		return nil, err
	}
	sequenceRanges, err := marshalJSONArray(manifest.TCPSequenceRanges)
	if err != nil {
		return nil, err
	}
	missingRanges, err := marshalJSONArray(manifest.Extraction.MissingRanges)
	if err != nil {
		return nil, err
	}
	bucket, objectKey, version, etag, objectSize, objectSHA, observedAt, retention, legalHold := objectValues(manifest.Object)
	declaredSize, truncationOffset := extractionValues(manifest.Extraction)

	_, err = tx.ExecContext(ctx, `INSERT INTO file_restoration_manifests (
		tenant_id,restoration_id,revision,idempotency_key,request_sha256,session_id,community_id,
		session_authority,primary_flow_id,flow_ids,flow_selections,pcap_index_ids,source_object_receipts,five_tuple,direction,capture_time_start,capture_time_end,
		packet_ranges,tcp_sequence_ranges,protocol_profile_id,parser_name,parser_version,algorithm_version,
		status,status_reason,missing_ranges,truncation_offset,wire_filename,sanitized_filename,
		declared_mime_type,detected_mime_type,declared_size,visible_size,restored_size,wire_sha256,content_sha256,
		object_bucket,object_key,object_version,object_etag,object_size_bytes,object_sha256,object_observed_at,
		retention_until,legal_hold,quarantined,executable,automatic_open,automatic_decompress,
		malware_scan_status,download_eligible,trace_id,created_at,completed_at
	) VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15,$16,$17,$18::jsonb,$19::jsonb,
		$20,$21,$22,$23,$24,$25,$26::jsonb,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,
		$43,$44,$45,true,false,false,false,$46,false,$47,$48,$49)`,
		manifest.TenantID, manifest.RestorationID, manifest.Revision, manifest.IdempotencyKey, requestSHA,
		manifest.SessionID, manifest.CommunityID, sessionAuthority, manifest.PrimaryFlowID, flowIDs, flowSelections, pcapIDs, sourceReceipts, fiveTuple, manifest.Direction,
		manifest.CaptureTimeStart, manifest.CaptureTimeEnd, packetRanges, sequenceRanges,
		manifest.Extraction.ProfileID, manifest.Extraction.ParserName, manifest.Extraction.ParserVersion,
		manifest.Extraction.AlgorithmVersion, manifest.Extraction.Status, manifest.Extraction.StatusReason,
		missingRanges, truncationOffset, manifest.Extraction.WireFilename, manifest.Extraction.SanitizedFilename,
		manifest.Extraction.DeclaredMIMEType, manifest.Extraction.DetectedMIMEType, declaredSize,
		manifest.Extraction.VisibleSize, manifest.Extraction.RestoredSize, manifest.Extraction.WireSHA256,
		manifest.Extraction.ContentSHA256, bucket, objectKey, version, etag, objectSize, objectSHA, observedAt,
		retention, legalHold, manifest.Extraction.MalwareScanStatus, manifest.TraceID,
		manifest.CreatedAt, manifest.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert restoration manifest: %w", err)
	}

	eventID := deterministicEventID(manifest.TenantID, manifest.IdempotencyKey)
	receipt := &CommitReceipt{
		TenantID: manifest.TenantID, RestorationID: manifest.RestorationID.String(), Revision: manifest.Revision,
		Status: string(manifest.Extraction.Status), ObjectSHA256: objectSHA, EventID: eventID.String(),
		OutboxStatus: "pending", CreatedAt: manifest.CompletedAt,
	}
	payload, err := marshalJSON(receipt)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO file_restoration_outbox
		(event_id,tenant_id,restoration_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,occurred_at)
		VALUES($1,$2,$3,$4,'traffic.forensics.file-restoration.v1.committed',1,$2,$5::jsonb,$6,$7)`,
		eventID, manifest.TenantID, manifest.RestorationID, manifest.Revision, payload, manifest.TraceID, manifest.CompletedAt); err != nil {
		return nil, fmt.Errorf("insert restoration outbox: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO file_restoration_audit
		(event_id,tenant_id,restoration_id,action,actor_id,reason,trace_id,object_sha256,status,occurred_at)
		VALUES($1,$2,$3,'forensics.file-restoration.commit',$4,$5,$6,$7,$8,$9)`,
		eventID, manifest.TenantID, manifest.RestorationID, command.ActorID, command.Reason,
		manifest.TraceID, objectSHA, manifest.Extraction.Status, manifest.CompletedAt); err != nil {
		return nil, fmt.Errorf("insert restoration audit: %w", err)
	}
	response, err := marshalJSON(receipt)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE file_restoration_requests
		SET state='committed',restoration_id=$1,resulting_revision=$2,event_id=$3,response_payload=$4::jsonb,
		    lease_until=$5,updated_at=$5
		WHERE tenant_id=$6 AND idempotency_key=$7 AND request_sha256=$8 AND claim_token=$9 AND state='processing'`,
		manifest.RestorationID, manifest.Revision, eventID, response, manifest.CompletedAt,
		manifest.TenantID, manifest.IdempotencyKey, requestSHA, command.ClaimToken)
	if err != nil {
		return nil, fmt.Errorf("finalize restoration idempotency receipt: %w", err)
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil || rows != 1 {
		return nil, errors.New("restoration commit lacks one exact admission claim")
	}
	if manifest.Object != nil {
		result, updateErr := tx.ExecContext(ctx, `UPDATE file_restoration_orphans
			SET reconciliation_status='reconciled',reconciled_at=$1
			WHERE tenant_id=$2 AND restoration_id=$3 AND object_bucket=$4 AND object_key=$5
			  AND object_version=$6 AND object_sha256=$7 AND reconciliation_status='candidate'`,
			manifest.CompletedAt, manifest.TenantID, manifest.RestorationID, bucket, objectKey, version, objectSHA)
		if updateErr != nil {
			return nil, fmt.Errorf("reconcile restoration orphan in authority transaction: %w", updateErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil || rows != 1 {
			return nil, errors.New("restoration object lacks one exact orphan candidate")
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit restoration authority: %w", err)
	}
	return receipt, nil
}

func loadReplay(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestSHA string) (*CommitReceipt, bool, error) {
	var priorSHA string
	var state string
	var response []byte
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,state,response_payload
		FROM file_restoration_requests WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, idempotencyKey).Scan(&priorSHA, &state, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load restoration idempotency receipt: %w", err)
	}
	if priorSHA != requestSHA {
		return nil, false, ErrIdempotencyConflict
	}
	if state == "processing" {
		return nil, false, nil
	}
	if state != "committed" {
		return nil, false, fmt.Errorf("unexpected restoration request state %q", state)
	}
	var receipt CommitReceipt
	if err := json.Unmarshal(response, &receipt); err != nil {
		return nil, false, fmt.Errorf("decode restoration replay receipt: %w", err)
	}
	return &receipt, true, nil
}

// LookupRequest is the pre-object-write idempotency barrier. A stable request
// fingerprint can replay the exact committed receipt without reading PCAP or
// creating another versioned quarantine object.
func (repository *Repository) LookupRequest(ctx context.Context, tenantID, idempotencyKey, requestSHA string) (*CommitReceipt, bool, error) {
	if strings.TrimSpace(tenantID) == "" || len(idempotencyKey) < 16 || len(idempotencyKey) > 200 || !requestSHA256Pattern.MatchString(requestSHA) {
		return nil, false, errors.New("invalid restoration idempotency lookup")
	}
	var priorSHA, state string
	var response []byte
	err := repository.db.QueryRowContext(ctx, `SELECT request_sha256,state,response_payload
		FROM file_restoration_requests WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&priorSHA, &state, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lookup restoration idempotency receipt: %w", err)
	}
	if priorSHA != requestSHA {
		return nil, false, ErrIdempotencyConflict
	}
	if state == "processing" {
		return nil, false, ErrRequestInProgress
	}
	if state != "committed" {
		return nil, false, fmt.Errorf("unexpected restoration request state %q", state)
	}
	var receipt CommitReceipt
	if err := json.Unmarshal(response, &receipt); err != nil {
		return nil, false, fmt.Errorf("decode restoration idempotency receipt: %w", err)
	}
	receipt.Replayed = true
	return &receipt, true, nil
}

// ClaimRequest serializes object creation across service replicas. It inserts
// an in-progress receipt before any PCAP read or object write. A committed row
// replays, a different hash conflicts, and an unexpired in-progress row never
// permits a second object writer.
func (repository *Repository) ClaimRequest(
	ctx context.Context,
	tenantID, idempotencyKey, requestSHA, traceID string,
	claimedAt, leaseUntil time.Time,
	tenantConcurrency int,
) (AdmissionReceipt, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(traceID) == "" || claimedAt.IsZero() ||
		!leaseUntil.After(claimedAt) || len(idempotencyKey) < 16 || len(idempotencyKey) > 200 || !requestSHA256Pattern.MatchString(requestSHA) {
		return AdmissionReceipt{}, errors.New("invalid restoration admission claim")
	}
	if tenantConcurrency <= 0 {
		return AdmissionReceipt{}, errors.New("restoration admission tenant concurrency must be positive")
	}
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return AdmissionReceipt{}, fmt.Errorf("begin restoration admission claim: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("restoration-tenant:%d:%s", len(tenantID), tenantID)); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("lock restoration tenant admission: %w", err)
	}
	// A row lock cannot serialize the first insert because no row exists yet.
	// The transaction-scoped advisory lock closes that cross-pod empty-row race.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("restoration-request:%d:%s:%d:%s", len(tenantID), tenantID, len(idempotencyKey), idempotencyKey)); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("lock restoration admission identity: %w", err)
	}
	var priorSHA, state string
	var priorClaimToken uuid.UUID
	var priorLease time.Time
	var response []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256,state,claim_token,lease_until,response_payload
		FROM file_restoration_requests WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, tenantID, idempotencyKey).
		Scan(&priorSHA, &state, &priorClaimToken, &priorLease, &response)
	if err == nil {
		if priorSHA != requestSHA {
			return AdmissionReceipt{}, ErrIdempotencyConflict
		}
		switch state {
		case "committed":
			var receipt CommitReceipt
			if decodeErr := json.Unmarshal(response, &receipt); decodeErr != nil {
				return AdmissionReceipt{}, fmt.Errorf("decode restoration admission replay: %w", decodeErr)
			}
			receipt.Replayed = true
			if commitErr := tx.Commit(); commitErr != nil {
				return AdmissionReceipt{}, fmt.Errorf("commit restoration admission replay: %w", commitErr)
			}
			return AdmissionReceipt{Result: AdmissionReplay, Receipt: &receipt}, nil
		case "processing":
			if !priorLease.After(claimedAt) {
				if err := enforceTenantConcurrency(ctx, tx, tenantID, claimedAt, tenantConcurrency); err != nil {
					return AdmissionReceipt{}, err
				}
				claimToken := uuid.New()
				result, updateErr := tx.ExecContext(ctx, `UPDATE file_restoration_requests
					SET trace_id=$1,claim_token=$2,lease_until=$3,updated_at=$4
					WHERE tenant_id=$5 AND idempotency_key=$6 AND request_sha256=$7 AND claim_token=$8 AND state='processing'`,
					traceID, claimToken, leaseUntil, claimedAt, tenantID, idempotencyKey, requestSHA, priorClaimToken)
				if updateErr != nil {
					return AdmissionReceipt{}, fmt.Errorf("reclaim expired restoration admission: %w", updateErr)
				}
				rows, rowsErr := result.RowsAffected()
				if rowsErr != nil || rows != 1 {
					return AdmissionReceipt{}, errors.New("expired restoration admission lacks one exact fenced claim")
				}
				if commitErr := tx.Commit(); commitErr != nil {
					return AdmissionReceipt{}, fmt.Errorf("commit reclaimed restoration admission: %w", commitErr)
				}
				return AdmissionReceipt{Result: AdmissionClaimed, ClaimToken: claimToken}, nil
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return AdmissionReceipt{}, fmt.Errorf("commit restoration in-progress observation: %w", commitErr)
			}
			return AdmissionReceipt{Result: AdmissionInProgress}, nil
		default:
			return AdmissionReceipt{}, fmt.Errorf("unexpected restoration request state %q", state)
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AdmissionReceipt{}, fmt.Errorf("load restoration admission claim: %w", err)
	}
	if err := enforceTenantConcurrency(ctx, tx, tenantID, claimedAt, tenantConcurrency); err != nil {
		return AdmissionReceipt{}, err
	}
	claimToken := uuid.New()
	if _, err = tx.ExecContext(ctx, `INSERT INTO file_restoration_requests
		(tenant_id,idempotency_key,request_sha256,state,trace_id,claim_token,lease_until,response_payload,created_at,updated_at)
		VALUES($1,$2,$3,'processing',$4,$5,$6,'{}'::jsonb,$7,$7)`, tenantID, idempotencyKey, requestSHA, traceID, claimToken, leaseUntil, claimedAt); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("insert restoration admission claim: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("commit restoration admission claim: %w", err)
	}
	return AdmissionReceipt{Result: AdmissionClaimed, ClaimToken: claimToken}, nil
}

func enforceTenantConcurrency(ctx context.Context, tx *sql.Tx, tenantID string, at time.Time, limit int) error {
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM file_restoration_requests
		WHERE tenant_id=$1 AND state='processing' AND lease_until>$2`, tenantID, at).Scan(&active); err != nil {
		return fmt.Errorf("count active restoration tenant leases: %w", err)
	}
	if active >= limit {
		return ErrTenantConcurrencyExceeded
	}
	return nil
}

func (repository *Repository) ReleaseRequestClaim(ctx context.Context, tenantID, idempotencyKey, requestSHA string, claimToken uuid.UUID) error {
	if claimToken == uuid.Nil {
		return errors.New("restoration admission release claim token is required")
	}
	result, err := repository.db.ExecContext(ctx, `DELETE FROM file_restoration_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 AND request_sha256=$3 AND claim_token=$4 AND state='processing'`,
		tenantID, idempotencyKey, requestSHA, claimToken)
	if err != nil {
		return fmt.Errorf("release restoration admission claim: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect restoration admission release: %w", err)
	}
	if rows > 1 {
		return errors.New("restoration admission release affected multiple rows")
	}
	return nil
}

func (repository *Repository) RecordOrphan(ctx context.Context, tenantID string, restorationID uuid.UUID, object s3client.ObjectAuthority) error {
	if strings.TrimSpace(tenantID) == "" || restorationID == uuid.Nil {
		return errors.New("orphan tenant and restoration identity are required")
	}
	if err := object.Validate(); err != nil {
		return err
	}
	expectedPrefix := "tenants/" + tenantID + "/restorations/" + restorationID.String() + "/"
	if !strings.HasPrefix(object.Key, expectedPrefix) {
		return errors.New("orphan object key is not tenant/restoration scoped")
	}
	var status string
	err := repository.db.QueryRowContext(ctx, `INSERT INTO file_restoration_orphans
		(tenant_id,restoration_id,object_bucket,object_key,object_version,object_etag,object_size_bytes,
		 object_sha256,observed_at,retention_until,reconciliation_status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'candidate')
		ON CONFLICT (tenant_id,object_bucket,object_key,object_version) DO UPDATE
		SET reconciliation_status=CASE
		  WHEN file_restoration_orphans.restoration_id=EXCLUDED.restoration_id
		   AND file_restoration_orphans.object_etag=EXCLUDED.object_etag
		   AND file_restoration_orphans.object_sha256=EXCLUDED.object_sha256
		   AND file_restoration_orphans.object_size_bytes=EXCLUDED.object_size_bytes THEN file_restoration_orphans.reconciliation_status
		  ELSE 'quarantined_conflict' END
		RETURNING reconciliation_status`, tenantID, restorationID, object.Bucket, object.Key,
		object.VersionID, object.ETag, object.SizeBytes, object.SHA256, object.ObservedAt,
		nullTime(object.RetentionUntil)).Scan(&status)
	if err != nil {
		return fmt.Errorf("record restoration orphan candidate: %w", err)
	}
	if status == "quarantined_conflict" {
		return errors.New("restoration orphan authority conflicts with an existing object receipt")
	}
	if status != "candidate" && status != "reconciled" {
		return fmt.Errorf("unexpected restoration orphan status %q", status)
	}
	return nil
}

// ReconcileOrphans classifies old committed object receipts without deleting
// bytes. A missing manifest remains a retryable candidate; only an existing,
// incompatible authority row becomes a quarantined conflict.
func (repository *Repository) ReconcileOrphans(ctx context.Context, observedBefore time.Time, limit int) (OrphanReconciliationReport, error) {
	if observedBefore.IsZero() || limit <= 0 || limit > 1000 {
		return OrphanReconciliationReport{}, errors.New("invalid restoration orphan reconciliation window")
	}
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("begin restoration orphan reconciliation: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT tenant_id,restoration_id,object_bucket,object_key,object_version,object_sha256,reconciliation_attempts
		FROM file_restoration_orphans
		WHERE reconciliation_status='candidate' AND observed_at<=$1 AND next_reconcile_at<=now()
		ORDER BY next_reconcile_at,observed_at,tenant_id,object_bucket,object_key,object_version
		FOR UPDATE SKIP LOCKED LIMIT $2`, observedBefore, limit)
	if err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("lease restoration orphan candidates: %w", err)
	}
	type orphanCandidate struct {
		tenantID, bucket, key, version, sha string
		restorationID                       uuid.UUID
		attempts                            int
	}
	candidates := make([]orphanCandidate, 0, limit)
	for rows.Next() {
		var candidate orphanCandidate
		if err := rows.Scan(&candidate.tenantID, &candidate.restorationID, &candidate.bucket,
			&candidate.key, &candidate.version, &candidate.sha, &candidate.attempts); err != nil {
			rows.Close()
			return OrphanReconciliationReport{}, fmt.Errorf("scan restoration orphan candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("close restoration orphan candidates: %w", err)
	}
	if err := rows.Err(); err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("iterate restoration orphan candidates: %w", err)
	}
	report := OrphanReconciliationReport{Scanned: len(candidates)}
	for _, candidate := range candidates {
		var bucket, key, version, sha string
		err := tx.QueryRowContext(ctx, `SELECT object_bucket,object_key,object_version,object_sha256
			FROM file_restoration_manifests WHERE tenant_id=$1 AND restoration_id=$2`,
			candidate.tenantID, candidate.restorationID).Scan(&bucket, &key, &version, &sha)
		if errors.Is(err, sql.ErrNoRows) {
			delaySeconds := 30 * (1 << min(candidate.attempts, 7))
			result, updateErr := tx.ExecContext(ctx, `UPDATE file_restoration_orphans
				SET reconciliation_attempts=reconciliation_attempts+1,
				    next_reconcile_at=now()+($1*interval '1 second')
				WHERE tenant_id=$2 AND object_bucket=$3 AND object_key=$4 AND object_version=$5
				  AND restoration_id=$6 AND object_sha256=$7 AND reconciliation_status='candidate'`,
				delaySeconds, candidate.tenantID, candidate.bucket, candidate.key, candidate.version,
				candidate.restorationID, candidate.sha)
			if updateErr != nil {
				return OrphanReconciliationReport{}, fmt.Errorf("reschedule restoration orphan candidate: %w", updateErr)
			}
			updated, rowsErr := result.RowsAffected()
			if rowsErr != nil || updated != 1 {
				return OrphanReconciliationReport{}, errors.New("restoration orphan lease lost during reschedule")
			}
			report.Pending++
			continue
		}
		if err != nil {
			return OrphanReconciliationReport{}, fmt.Errorf("load restoration manifest for orphan: %w", err)
		}
		status := "reconciled"
		if bucket != candidate.bucket || key != candidate.key || version != candidate.version || sha != candidate.sha {
			status = "quarantined_conflict"
		}
		result, err := tx.ExecContext(ctx, `UPDATE file_restoration_orphans
			SET reconciliation_status=$1,reconciliation_attempts=reconciliation_attempts+1,
			    reconciled_at=CASE WHEN $1='reconciled' THEN now() ELSE NULL END
			WHERE tenant_id=$2 AND object_bucket=$3 AND object_key=$4 AND object_version=$5
			  AND restoration_id=$6 AND object_sha256=$7 AND reconciliation_status='candidate'`,
			status, candidate.tenantID, candidate.bucket, candidate.key, candidate.version,
			candidate.restorationID, candidate.sha)
		if err != nil {
			return OrphanReconciliationReport{}, fmt.Errorf("classify restoration orphan: %w", err)
		}
		updated, rowsErr := result.RowsAffected()
		if rowsErr != nil || updated != 1 {
			return OrphanReconciliationReport{}, errors.New("restoration orphan lease lost during classification")
		}
		if status == "reconciled" {
			report.Reconciled++
		} else {
			report.Conflicts++
		}
	}
	if err := tx.Commit(); err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("commit restoration orphan reconciliation: %w", err)
	}
	return report, nil
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
