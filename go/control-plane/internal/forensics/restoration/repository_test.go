package restoration

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/extractor"
	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

func completeCommand() CommitCommand {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	tenantID := "tenant-a"
	idempotencyKey := "restore-request-0001"
	restorationID := deterministicRestorationID(tenantID, idempotencyKey)
	content := []byte("OK")
	digest := "565339bc4d33d72817b583024112eb7f5cdf3e5eef0252d6ec1b9c9a94e12bb3"
	object := s3client.ObjectAuthority{
		Bucket: "forensics-quarantine", Key: "tenants/" + tenantID + "/restorations/" + restorationID.String() + "/1/content.bin",
		VersionID: "object-version-1", ETag: "object-etag-1", SizeBytes: int64(len(content)), SHA256: digest,
		ObservedAt: now, RetentionUntil: now.Add(24 * time.Hour),
	}
	source := s3client.ObjectAuthority{
		Bucket: "pcap", Key: "tenant-a/probe-a/source.pcap", VersionID: "source-version-1",
		ETag: "source-etag-1", SizeBytes: 4096, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ObservedAt: now,
	}
	return CommitCommand{
		ActorID: "restoration-worker", Reason: "approved protocol extraction", TraceID: "trace-restore-1",
		ClaimToken:     uuid.MustParse("3fdd1cc3-e999-4d67-a674-3015d9de107a"),
		IdempotencyKey: idempotencyKey, RequestSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Manifest: Manifest{
			RestorationID: restorationID, TenantID: tenantID, Revision: 1, IdempotencyKey: idempotencyKey,
			SessionID: "session-1", CommunityID: "1:community", PrimaryFlowID: "flow-1", FlowIDs: []string{"flow-1"},
			FlowSelections: []FlowSelection{{
				Role: "primary", CommunityID: "1:community", FlowID: "flow-1",
				FiveTuple: FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
				Direction: "server_to_client",
			}},
			SessionAuthority: forensicsindex.RestorationSessionAuthority{
				TenantID: tenantID, SessionID: "session-1", CommunityID: "1:community", EventID: "session-event-1", ProbeID: "probe-a",
				FlowIDs: []string{"flow-1"}, TsStart: now.Add(-2 * time.Minute), TsEnd: now.Add(time.Minute),
				EventSchemaVersion: "v1", AggregateVersion: 1, IdentityVersion: "session-id-sha256-v1",
				SessionVersion: 1, Completeness: "SESSION_COMPLETENESS_COMPLETE",
			},
			PcapIndexIDs: []string{"index-1"}, SourceObjectReceipts: []s3client.ObjectAuthority{source},
			FiveTuple: FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
			Direction: "server_to_client", CaptureTimeStart: now.Add(-time.Minute), CaptureTimeEnd: now,
			PacketRanges: []PacketRange{{ObjectBucket: source.Bucket, ObjectKey: source.Key, ObjectVersion: source.VersionID, Start: 100, End: 102}},
			TCPSequenceRanges: []reassembly.SourceRange{{SequenceStart: 1000, SequenceEnd: 1002, PacketIndex: 7,
				ObjectBucket: source.Bucket, ObjectKey: source.Key, ObjectVersion: source.VersionID,
				ObjectRangeStart: 100, ObjectRangeEnd: 102, ObjectRangeExact: true, Length: 2}},
			Extraction: extractor.Result{
				ProfileID: extractor.ProfileHTTP1Response, ParserName: "http1-response", ParserVersion: "1.0.0",
				AlgorithmVersion: "m03-restoration-v1", Status: extractor.StatusComplete, StatusReason: "content-length body complete",
				WireFilename: "result.bin", SanitizedFilename: "result.bin", DeclaredMIMEType: "application/octet-stream",
				DetectedMIMEType: "application/octet-stream", VisibleSize: 2, RestoredSize: 2, WireSHA256: digest,
				ContentSHA256: digest, Content: content, Quarantined: true, MalwareScanStatus: "not_scanned",
			},
			Object: &object, TraceID: "trace-restore-1", CreatedAt: now.Add(-time.Second), CompletedAt: now,
		},
	}
}

func newMockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return repository, mock, func() { db.Close() }
}

func TestVerifySchemaRequiresMigrationAndAllAuthorityTables(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(true))
	if err := repository.VerifySchema(context.Background()); err != nil {
		t.Fatalf("VerifySchema() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMarshalJSONArrayNormalizesNilToEmptyArray(t *testing.T) {
	encoded, err := marshalJSONArray[string](nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("nil array encoded as %s", encoded)
	}
}

func TestCommitWritesManifestOutboxAuditAndReceiptAtomically(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "response_payload"}).AddRow(command.RequestSHA256, "processing", []byte(`{}`)))
	mock.ExpectExec("INSERT INTO file_restoration_manifests").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO file_restoration_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO file_restoration_audit").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE file_restoration_requests").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE file_restoration_orphans").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	receipt, err := repository.Commit(context.Background(), command)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if receipt.Replayed || receipt.RestorationID != command.Manifest.RestorationID.String() || receipt.ObjectSHA256 != command.Manifest.Object.SHA256 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitReplaysTheStoredReceiptWithoutRewritingAuthority(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	requestSHA := command.RequestSHA256
	stored := `{"tenant_id":"tenant-a","restoration_id":"` + command.Manifest.RestorationID.String() + `","revision":1,"status":"complete","object_sha256":"` + command.Manifest.Object.SHA256 + `","event_id":"event-1","outbox_status":"pending","created_at":"2026-08-14T12:00:00Z","replayed":false}`

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "response_payload"}).AddRow(requestSHA, "committed", []byte(stored)))
	mock.ExpectCommit()

	receipt, err := repository.Commit(context.Background(), command)
	if err != nil {
		t.Fatalf("Commit() replay error = %v", err)
	}
	if !receipt.Replayed || receipt.EventID != "event-1" {
		t.Fatalf("unexpected replay receipt: %+v", receipt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitRejectsIdempotencyConflictAndRollsBack(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "response_payload"}).AddRow("different-request-sha", "processing", []byte(`{}`)))
	mock.ExpectRollback()

	_, err := repository.Commit(context.Background(), completeCommand())
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Commit() error = %v, want ErrIdempotencyConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordOrphanRejectsAuthorityConflict(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	object := *command.Manifest.Object

	mock.ExpectQuery("INSERT INTO file_restoration_orphans").
		WillReturnRows(sqlmock.NewRows([]string{"reconciliation_status"}).AddRow("quarantined_conflict"))
	err := repository.RecordOrphan(context.Background(), command.Manifest.TenantID, command.Manifest.RestorationID, object)
	if err == nil || !regexp.MustCompile("authority conflicts").MatchString(err.Error()) {
		t.Fatalf("RecordOrphan() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLookupRequestReturnsStableReplayReceipt(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	payload := `{"tenant_id":"tenant-a","restoration_id":"` + command.Manifest.RestorationID.String() + `","revision":1,"status":"complete","event_id":"event-1"}`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "response_payload"}).AddRow(command.RequestSHA256, "committed", []byte(payload)))
	receipt, found, err := repository.LookupRequest(context.Background(), command.Manifest.TenantID, command.IdempotencyKey, command.RequestSHA256)
	if err != nil || !found || !receipt.Replayed || receipt.EventID != "event-1" {
		t.Fatalf("LookupRequest() = %+v, %v, %v", receipt, found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRequestCreatesOneProcessingLease(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	now := command.Manifest.CompletedAt
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,claim_token,lease_until,response_payload")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM file_restoration_requests").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO file_restoration_requests").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	admission, err := repository.ClaimRequest(context.Background(), command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now, now.Add(time.Minute), 1)
	if err != nil || admission.Result != AdmissionClaimed || admission.ClaimToken == uuid.Nil {
		t.Fatalf("ClaimRequest() = %+v, %v", admission, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRequestObservesUnexpiredLease(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	now := command.Manifest.CompletedAt
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,claim_token,lease_until,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "claim_token", "lease_until", "response_payload"}).
			AddRow(command.RequestSHA256, "processing", command.ClaimToken, now.Add(time.Minute), []byte(`{}`)))
	mock.ExpectCommit()
	admission, err := repository.ClaimRequest(context.Background(), command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now, now.Add(time.Minute), 1)
	if err != nil || admission.Result != AdmissionInProgress {
		t.Fatalf("ClaimRequest() = %+v, %v", admission, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRequestReclaimsExpiredLease(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	now := command.Manifest.CompletedAt
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,claim_token,lease_until,response_payload")).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "state", "claim_token", "lease_until", "response_payload"}).
			AddRow(command.RequestSHA256, "processing", command.ClaimToken, now.Add(-time.Second), []byte(`{}`)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM file_restoration_requests").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE file_restoration_requests").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	admission, err := repository.ClaimRequest(context.Background(), command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now, now.Add(time.Minute), 1)
	if err != nil || admission.Result != AdmissionClaimed || admission.ClaimToken == uuid.Nil || admission.ClaimToken == command.ClaimToken {
		t.Fatalf("ClaimRequest() = %+v, %v", admission, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimRequestRejectsCrossPodTenantConcurrencyOverflow(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	now := command.Manifest.CompletedAt
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_sha256,state,claim_token,lease_until,response_payload")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM file_restoration_requests").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	admission, err := repository.ClaimRequest(context.Background(), command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now, now.Add(time.Minute), 1)
	if !errors.Is(err, ErrTenantConcurrencyExceeded) || admission.Result != "" {
		t.Fatalf("ClaimRequest() overflow = %+v, %v", admission, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseRequestClaimIsFencedByClaimToken(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	mock.ExpectExec("DELETE FROM file_restoration_requests").WithArgs(
		command.Manifest.TenantID, command.IdempotencyKey, command.RequestSHA256, command.ClaimToken,
	).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := repository.ReleaseRequestClaim(context.Background(), command.Manifest.TenantID, command.IdempotencyKey, command.RequestSHA256, command.ClaimToken); err != nil {
		t.Fatalf("ReleaseRequestClaim() stale fence error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitRejectsStaleClaimTokenBeforeTransaction(t *testing.T) {
	command := completeCommand()
	command.ClaimToken = uuid.Nil
	if err := command.Validate(); err == nil {
		t.Fatal("CommitCommand.Validate() accepted an empty fencing token")
	}
}

func TestManifestRejectsSourceRangeOnDifferentImmutableVersion(t *testing.T) {
	command := completeCommand()
	command.Manifest.TCPSequenceRanges[0].ObjectVersion = "different-version"
	if err := command.Validate(); err == nil {
		t.Fatal("CommitCommand.Validate() accepted source proof from a different immutable version")
	}
}

func TestReconcileOrphansReschedulesMissingManifestWithoutDeletingAuthority(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	object := *command.Manifest.Object
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT tenant_id,restoration_id,object_bucket,object_key,object_version,object_sha256,reconciliation_attempts").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "restoration_id", "object_bucket", "object_key", "object_version", "object_sha256", "reconciliation_attempts"}).
			AddRow(command.Manifest.TenantID, command.Manifest.RestorationID, object.Bucket, object.Key, object.VersionID, object.SHA256, 0))
	mock.ExpectQuery("SELECT object_bucket,object_key,object_version,object_sha256").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE file_restoration_orphans").WithArgs(30, command.Manifest.TenantID, object.Bucket, object.Key,
		object.VersionID, command.Manifest.RestorationID, object.SHA256).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	report, err := repository.ReconcileOrphans(context.Background(), command.Manifest.CompletedAt.Add(time.Hour), 10)
	if err != nil || report.Pending != 1 || report.Reconciled != 0 || report.Conflicts != 0 {
		t.Fatalf("ReconcileOrphans() = %+v, %v", report, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileOrphansMarksExactManifestReceiptReconciled(t *testing.T) {
	repository, mock, closeDB := newMockRepository(t)
	defer closeDB()
	command := completeCommand()
	object := *command.Manifest.Object
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT tenant_id,restoration_id,object_bucket,object_key,object_version,object_sha256,reconciliation_attempts").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "restoration_id", "object_bucket", "object_key", "object_version", "object_sha256", "reconciliation_attempts"}).
			AddRow(command.Manifest.TenantID, command.Manifest.RestorationID, object.Bucket, object.Key, object.VersionID, object.SHA256, 1))
	mock.ExpectQuery("SELECT object_bucket,object_key,object_version,object_sha256").
		WillReturnRows(sqlmock.NewRows([]string{"object_bucket", "object_key", "object_version", "object_sha256"}).
			AddRow(object.Bucket, object.Key, object.VersionID, object.SHA256))
	mock.ExpectExec("UPDATE file_restoration_orphans").WithArgs("reconciled", command.Manifest.TenantID, object.Bucket,
		object.Key, object.VersionID, command.Manifest.RestorationID, object.SHA256).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	report, err := repository.ReconcileOrphans(context.Background(), command.Manifest.CompletedAt.Add(time.Hour), 10)
	if err != nil || report.Reconciled != 1 || report.Pending != 0 || report.Conflicts != 0 {
		t.Fatalf("ReconcileOrphans() = %+v, %v", report, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
