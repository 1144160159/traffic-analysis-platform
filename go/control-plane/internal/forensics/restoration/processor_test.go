package restoration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/extractor"
	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type fakeSourceIndex struct {
	sources          []forensicsindex.RestorationSource
	err              error
	queries          []forensicsindex.RestorationSourceQuery
	sessionAuthority forensicsindex.RestorationSessionAuthority
	sessionError     error
	sessionQueries   []forensicsindex.RestorationSessionQuery
}

func (index *fakeSourceIndex) VerifyRestorationSession(_ context.Context, query forensicsindex.RestorationSessionQuery) (forensicsindex.RestorationSessionAuthority, error) {
	index.sessionQueries = append(index.sessionQueries, query)
	return index.sessionAuthority, index.sessionError
}

func (index *fakeSourceIndex) LookupRestorationSources(_ context.Context, query forensicsindex.RestorationSourceQuery) ([]forensicsindex.RestorationSource, error) {
	index.queries = append(index.queries, query)
	return append([]forensicsindex.RestorationSource(nil), index.sources...), index.err
}

type fakeObjectStore struct {
	findCalls          int
	findResult         s3client.ObjectAuthority
	findFound          bool
	findError          error
	putCalls           int
	putError           error
	putErrorAfterWrite bool
	lastKey            string
}

func (store *fakeObjectStore) ReadVerifiedObject(context.Context, string, string, string, string, string, int64, int64) ([]byte, s3client.ObjectAuthority, error) {
	return nil, s3client.ObjectAuthority{}, errors.New("unexpected real object read in processor unit test")
}

func (store *fakeObjectStore) FindQuarantineObject(
	_ context.Context,
	_, key, _, _, _ string,
	_ int64,
) (s3client.ObjectAuthority, bool, error) {
	store.findCalls++
	store.lastKey = key
	return store.findResult, store.findFound, store.findError
}

func (store *fakeObjectStore) PutQuarantineObject(
	_ context.Context,
	bucket, key, _, _ string,
	content []byte, _, expectedSHA string,
	retention time.Time,
) (s3client.ObjectAuthority, error) {
	store.putCalls++
	store.lastKey = key
	if store.putError != nil {
		if store.putErrorAfterWrite {
			store.findFound = true
			store.findResult = s3client.ObjectAuthority{
				Bucket: bucket, Key: key, VersionID: "unknown-outcome-version-1", ETag: "unknown-outcome-etag-1",
				SizeBytes: int64(len(content)), SHA256: expectedSHA, ObservedAt: time.Now().UTC(), RetentionUntil: retention,
			}
		}
		return s3client.ObjectAuthority{}, store.putError
	}
	return s3client.ObjectAuthority{
		Bucket: bucket, Key: key, VersionID: "quarantine-version-1", ETag: "quarantine-etag-1",
		SizeBytes: int64(len(content)), SHA256: expectedSHA, ObservedAt: time.Now().UTC(), RetentionUntil: retention,
	}, nil
}

type fakeManifestAuthority struct {
	orphanCalls  int
	commitCalls  int
	claimCalls   int
	releaseCalls int
	orphanError  error
	claimResult  AdmissionReceipt
	claimError   error
	command      CommitCommand
}

func (authority *fakeManifestAuthority) ClaimRequest(_ context.Context, _, _, _, _ string, _, _ time.Time, _ int) (AdmissionReceipt, error) {
	authority.claimCalls++
	if authority.claimResult.Result == "" {
		return AdmissionReceipt{Result: AdmissionClaimed, ClaimToken: uuid.MustParse("3fdd1cc3-e999-4d67-a674-3015d9de107a")}, authority.claimError
	}
	return authority.claimResult, authority.claimError
}

func (authority *fakeManifestAuthority) ReleaseRequestClaim(_ context.Context, _, _, _ string, _ uuid.UUID) error {
	authority.releaseCalls++
	return nil
}

func (authority *fakeManifestAuthority) RecordOrphan(_ context.Context, _ string, _ uuid.UUID, _ s3client.ObjectAuthority) error {
	authority.orphanCalls++
	return authority.orphanError
}

func (authority *fakeManifestAuthority) Commit(_ context.Context, command CommitCommand) (*CommitReceipt, error) {
	authority.commitCalls++
	authority.command = command
	if err := command.Validate(); err != nil {
		return nil, err
	}
	objectSHA := ""
	if command.Manifest.Object != nil {
		objectSHA = command.Manifest.Object.SHA256
	}
	return &CommitReceipt{
		TenantID: command.Manifest.TenantID, RestorationID: command.Manifest.RestorationID.String(),
		Revision: command.Manifest.Revision, Status: string(command.Manifest.Extraction.Status), ObjectSHA256: objectSHA,
	}, nil
}

func enabledProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		Enabled: true, QuarantineBucket: "forensics-quarantine", MaxSourceBytes: 1 << 20,
		MaxSourceObjects: 10, MaxPackets: 100, MaxStreamBytes: 1 << 16, MaxObjectBytes: 1 << 15,
		MaxPartCount: 10, MaxMIMEDepth: 3, MaxExpansionRatio: 10, TaskTimeout: time.Minute,
		TenantConcurrency: 1, RetentionDuration: 24 * time.Hour,
	}
}

func processRequest(profile string) ProcessRequest {
	now := time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC)
	return ProcessRequest{
		TenantID: "tenant-a", IdempotencyKey: "restore-request-0001", SessionID: "session-1",
		CommunityID: "1:community", FlowIDs: []string{"flow-1"}, FlowID: "flow-1",
		Tuple:     FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
		Direction: "server_to_client", StartTime: now.Add(-time.Second), EndTime: now.Add(time.Second),
		ProfileID: profile, ActorID: "restoration-worker", Reason: "approved extraction", TraceID: "trace-1",
	}
}

func sessionAuthorityForRequest(request ProcessRequest) forensicsindex.RestorationSessionAuthority {
	return forensicsindex.RestorationSessionAuthority{
		TenantID: request.TenantID, SessionID: request.SessionID, CommunityID: request.CommunityID,
		EventID: "session-event-1", ProbeID: "probe-a", FlowIDs: []string{request.FlowID},
		TsStart: request.StartTime.Add(-time.Second), TsEnd: request.EndTime.Add(time.Second),
		EventSchemaVersion: "v1", AggregateVersion: 1, IdentityVersion: "session-id-sha256-v1",
		SessionVersion: 1, Completeness: "SESSION_COMPLETENESS_COMPLETE", IsPartial: false,
	}
}

func loadedHTTPResponse() LoadedSegments {
	now := time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC)
	sourceKey := "tenant-a/probe-a/source.pcap"
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return LoadedSegments{
		ClientToServer: []reassembly.Segment{{Sequence: 100, Payload: []byte("GET / HTTP/1.1\r\n\r\n"), CapturedLength: 74, OriginalLength: 74, PacketIndex: 1, CapturedAt: now, ObjectBucket: "pcap", ObjectKey: sourceKey, ObjectVersion: "source-version-1", ObjectRangeStart: 24, ObjectRangeEnd: 114, ObjectRangeExact: true}},
		ServerToClient: []reassembly.Segment{{Sequence: 500, Payload: []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nContent-Type: text/plain\r\n\r\nOK"), CapturedLength: 126, OriginalLength: 126, PacketIndex: 2, CapturedAt: now.Add(time.Millisecond), ObjectBucket: "pcap", ObjectKey: sourceKey, ObjectVersion: "source-version-1", ObjectRangeStart: 114, ObjectRangeEnd: 256, ObjectRangeExact: true}},
		SourceReceipts: []s3client.ObjectAuthority{{Bucket: "pcap", Key: sourceKey, VersionID: "source-version-1", ETag: "source-etag-1", SizeBytes: 4096, SHA256: digest, ObservedAt: now}},
		PacketRanges:   []PacketRange{{ObjectBucket: "pcap", ObjectKey: sourceKey, ObjectVersion: "source-version-1", Start: 0, End: 4096}},
		PcapIndexIDs:   []string{digest},
	}
}

func processorWithLoadedHTTP(t *testing.T, config ProcessorConfig, authority *fakeManifestAuthority, objects *fakeObjectStore) *Processor {
	t.Helper()
	request := processRequest(extractor.ProfileHTTP1Response)
	index := &fakeSourceIndex{
		sources:          []forensicsindex.RestorationSource{{ProjectionIdentity: "source"}},
		sessionAuthority: sessionAuthorityForRequest(request),
	}
	processor, err := NewProcessor(config, index, objects, authority)
	if err != nil {
		t.Fatal(err)
	}
	processor.loadSegments = func(context.Context, VerifiedObjectReader, []forensicsindex.RestorationSource, SegmentLoadQuery, SegmentLoadLimits) (LoadedSegments, error) {
		return loadedHTTPResponse(), nil
	}
	return processor
}

func TestProcessorIsInertWhileAdmissionIsDisabled(t *testing.T) {
	processor, err := NewProcessor(ProcessorConfig{Enabled: false}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if !errors.Is(err, ErrAdmissionDisabled) {
		t.Fatalf("Process() error = %v, want ErrAdmissionDisabled", err)
	}
}

func TestProcessorCompleteWritesQuarantineBeforeAuthorityCommit(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)

	receipt, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if objects.findCalls != 1 || objects.putCalls != 1 || authority.orphanCalls != 1 || authority.commitCalls != 1 {
		t.Fatalf("find/write/orphan/commit calls = %d/%d/%d/%d", objects.findCalls, objects.putCalls, authority.orphanCalls, authority.commitCalls)
	}
	if receipt.Status != string(extractor.StatusComplete) || string(authority.command.Manifest.Extraction.Content) != "OK" {
		t.Fatalf("unexpected receipt/manifest: %+v / %+v", receipt, authority.command.Manifest.Extraction)
	}
	wantPrefix := "tenants/tenant-a/restorations/" + authority.command.Manifest.RestorationID.String() + "/"
	if authority.command.Manifest.Object == nil || authority.command.Manifest.Object.Key != objects.lastKey || len(objects.lastKey) <= len(wantPrefix) || objects.lastKey[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("object key is not restoration scoped: %q", objects.lastKey)
	}
}

func TestProcessorUnsupportedCommitsMetadataWithoutObject(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)
	request := processRequest("tls-encrypted-application-v1")

	receipt, err := processor.Process(context.Background(), request)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if receipt.Status != string(extractor.StatusUnsupported) || objects.putCalls != 0 || authority.orphanCalls != 0 || authority.commitCalls != 1 {
		t.Fatalf("unsupported result wrote object or skipped metadata: receipt=%+v calls=%d/%d/%d", receipt, objects.putCalls, authority.orphanCalls, authority.commitCalls)
	}
	if authority.command.Manifest.Object != nil {
		t.Fatal("unsupported restoration contains an object receipt")
	}
}

func TestFTPDataLookupUsesItsOwnCommunityAuthority(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{}
	request := processRequest(extractor.ProfileFTPPassive)
	request.Tuple = FiveTuple{SourceIP: "192.0.2.10", DestinationIP: "198.51.100.20", SourcePort: 40000, DestinationPort: 21, Protocol: 6}
	request.FlowIDs = []string{"flow-control", "flow-data"}
	request.FlowID = "flow-control"
	request.FTPData = &FTPDataRequest{
		CommunityID: "1:ftp-data", FlowID: "flow-data",
		Tuple: FiveTuple{SourceIP: "192.0.2.10", DestinationIP: "198.51.100.20", SourcePort: 40001, DestinationPort: 49152, Protocol: 6},
	}
	index := &fakeSourceIndex{
		sources:          []forensicsindex.RestorationSource{{ProjectionIdentity: "source"}},
		sessionAuthority: sessionAuthorityForRequest(request),
	}
	processor, err := NewProcessor(enabledProcessorConfig(), index, objects, authority)
	if err != nil {
		t.Fatal(err)
	}
	loadCalls := 0
	processor.loadSegments = func(_ context.Context, _ VerifiedObjectReader, _ []forensicsindex.RestorationSource, query SegmentLoadQuery, _ SegmentLoadLimits) (LoadedSegments, error) {
		loadCalls++
		if loadCalls == 1 {
			return LoadedSegments{
				ClientToServer: []reassembly.Segment{{Sequence: 100, Payload: []byte("PASV\r\nRETR file.bin\r\n"), CapturedLength: 24, OriginalLength: 24, PacketIndex: 1, CapturedAt: request.StartTime, ObjectBucket: "pcap", ObjectKey: "control.pcap", ObjectVersion: "v1", ObjectRangeStart: 0, ObjectRangeEnd: 24, ObjectRangeExact: true}},
				ServerToClient: []reassembly.Segment{{Sequence: 200, Payload: []byte("227 Entering Passive Mode (198,51,100,20,192,0)\r\n150 Opening\r\n226 Complete\r\n"), CapturedLength: 83, OriginalLength: 83, PacketIndex: 2, CapturedAt: request.StartTime, ObjectBucket: "pcap", ObjectKey: "control.pcap", ObjectVersion: "v1", ObjectRangeStart: 24, ObjectRangeEnd: 107, ObjectRangeExact: true}},
				SourceReceipts: []s3client.ObjectAuthority{{Bucket: "pcap", Key: "control.pcap", VersionID: "v1", ETag: "e1", SizeBytes: 107, SHA256: strings.Repeat("a", 64), ObservedAt: time.Now()}},
				PacketRanges:   []PacketRange{{ObjectBucket: "pcap", ObjectKey: "control.pcap", ObjectVersion: "v1", Start: 0, End: 107}}, PcapIndexIDs: []string{strings.Repeat("a", 64)},
			}, nil
		}
		start, end := uint32(500), uint32(504)
		return LoadedSegments{
			ServerToClient:      []reassembly.Segment{{Sequence: start, Payload: []byte("DATA"), CapturedLength: 4, OriginalLength: 4, PacketIndex: 3, CapturedAt: request.StartTime, ObjectBucket: "pcap", ObjectKey: "data.pcap", ObjectVersion: "v1", ObjectRangeStart: 0, ObjectRangeEnd: 4, ObjectRangeExact: true}},
			ServerToClientStart: &start, ServerToClientEnd: &end,
			SourceReceipts: []s3client.ObjectAuthority{{Bucket: "pcap", Key: "data.pcap", VersionID: "v1", ETag: "e2", SizeBytes: 4, SHA256: strings.Repeat("b", 64), ObservedAt: time.Now()}},
			PacketRanges:   []PacketRange{{ObjectBucket: "pcap", ObjectKey: "data.pcap", ObjectVersion: "v1", Start: 0, End: 4}}, PcapIndexIDs: []string{strings.Repeat("b", 64)},
		}, nil
	}
	if _, err := processor.Process(context.Background(), request); err != nil {
		t.Fatalf("Process() FTP error = %v", err)
	}
	if len(index.queries) != 2 || index.queries[0].CommunityID != request.CommunityID || index.queries[1].CommunityID != request.FTPData.CommunityID {
		t.Fatalf("FTP source authority queries = %+v", index.queries)
	}
	if authority.command.Manifest.PrimaryFlowID != request.FlowID || authority.command.Manifest.SessionAuthority.EventID != "session-event-1" {
		t.Fatalf("FTP manifest authority = %+v", authority.command.Manifest)
	}
}

func TestProcessorNeverCommitsManifestWhenOrphanRegistrationFails(t *testing.T) {
	authority := &fakeManifestAuthority{orphanError: errors.New("postgres unavailable")}
	objects := &fakeObjectStore{}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)

	_, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if err == nil || authority.orphanCalls != 1 || authority.commitCalls != 0 {
		t.Fatalf("Process() error/calls = %v / %d/%d", err, authority.orphanCalls, authority.commitCalls)
	}
}

func TestProcessorRecoversPutBeforeOrphanCrashWithoutCreatingAnotherVersion(t *testing.T) {
	authority := &fakeManifestAuthority{}
	request := processRequest(extractor.ProfileHTTP1Response)
	restorationID := deterministicRestorationID(request.TenantID, request.IdempotencyKey)
	objects := &fakeObjectStore{
		findFound: true,
		findResult: s3client.ObjectAuthority{
			Bucket:    "forensics-quarantine",
			Key:       "tenants/" + request.TenantID + "/restorations/" + restorationID.String() + "/1/content.bin",
			VersionID: "crash-version-1", ETag: "crash-etag-1", SizeBytes: 2,
			SHA256:     "565339bc4d33d72817b583024112eb7f5cdf3e5eef0252d6ec1b9c9a94e12bb3",
			ObservedAt: time.Now().UTC(), RetentionUntil: time.Now().UTC().Add(time.Hour),
		},
	}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)
	receipt, err := processor.Process(context.Background(), request)
	if err != nil {
		t.Fatalf("Process() recovery error = %v", err)
	}
	if receipt.ObjectSHA256 != objects.findResult.SHA256 || objects.findCalls != 1 || objects.putCalls != 0 ||
		authority.orphanCalls != 1 || authority.commitCalls != 1 {
		t.Fatalf("recovery find/put/orphan/commit = %d/%d/%d/%d receipt=%+v", objects.findCalls, objects.putCalls, authority.orphanCalls, authority.commitCalls, receipt)
	}
}

func TestProcessorRejectsExistingQuarantineAuthorityConflictWithoutPut(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{findError: s3client.ErrQuarantineObjectConflict}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)
	_, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if !errors.Is(err, s3client.ErrQuarantineObjectConflict) || objects.putCalls != 0 || authority.orphanCalls != 0 || authority.commitCalls != 0 {
		t.Fatalf("conflict error/calls = %v / %d/%d/%d", err, objects.putCalls, authority.orphanCalls, authority.commitCalls)
	}
}

func TestProcessorRecoversUnknownPutOutcomeByExactAuthority(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{putError: errors.New("connection reset after request body"), putErrorAfterWrite: true}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)
	receipt, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if err != nil {
		t.Fatalf("Process() unknown PUT outcome error = %v", err)
	}
	if receipt.ObjectSHA256 != objects.findResult.SHA256 || objects.findCalls != 2 || objects.putCalls != 1 ||
		authority.orphanCalls != 1 || authority.commitCalls != 1 {
		t.Fatalf("unknown outcome find/put/orphan/commit = %d/%d/%d/%d receipt=%+v", objects.findCalls, objects.putCalls, authority.orphanCalls, authority.commitCalls, receipt)
	}
}

func TestProcessorDrainRejectsNewAdmissionAndWaitsForActiveTasks(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{}
	processor := processorWithLoadedHTTP(t, enabledProcessorConfig(), authority, objects)
	if err := processor.acquire("tenant-active"); err != nil {
		t.Fatal(err)
	}
	processor.BeginDrain()
	if _, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response)); !errors.Is(err, ErrProcessorDraining) {
		t.Fatalf("Process() while draining error = %v", err)
	}
	shortCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := processor.WaitForDrain(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForDrain() active error = %v", err)
	}
	processor.release("tenant-active")
	if err := processor.WaitForDrain(context.Background()); err != nil {
		t.Fatalf("WaitForDrain() after release error = %v", err)
	}
}

func TestProcessorReplaysBeforeIndexOrObjectAccess(t *testing.T) {
	replayed := &CommitReceipt{TenantID: "tenant-a", RestorationID: uuid.NewString(), Revision: 1, Status: "complete", Replayed: true}
	authority := &fakeManifestAuthority{claimResult: AdmissionReceipt{Result: AdmissionReplay, Receipt: replayed}}
	objects := &fakeObjectStore{}
	index := &fakeSourceIndex{err: errors.New("index must not be called"), sessionError: errors.New("session must not be called")}
	processor, err := NewProcessor(enabledProcessorConfig(), index, objects, authority)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if err != nil {
		t.Fatalf("Process() replay error = %v", err)
	}
	if receipt != replayed || authority.claimCalls != 1 || len(index.sessionQueries) != 0 || len(index.queries) != 0 || objects.putCalls != 0 || authority.commitCalls != 0 || authority.releaseCalls != 0 {
		t.Fatalf("replay touched downstream authorities: claim/session/index/put/commit/release=%d/%d/%d/%d/%d/%d", authority.claimCalls, len(index.sessionQueries), len(index.queries), objects.putCalls, authority.commitCalls, authority.releaseCalls)
	}
}

func TestProcessorRejectsUnversionedSessionBeforePCAPRead(t *testing.T) {
	authority := &fakeManifestAuthority{}
	objects := &fakeObjectStore{}
	index := &fakeSourceIndex{sessionError: errors.New("restoration session authority uses an unsupported version")}
	processor, err := NewProcessor(enabledProcessorConfig(), index, objects, authority)
	if err != nil {
		t.Fatal(err)
	}
	_, err = processor.Process(context.Background(), processRequest(extractor.ProfileHTTP1Response))
	if err == nil || len(index.sessionQueries) != 1 || len(index.queries) != 0 || objects.findCalls != 0 || authority.commitCalls != 0 {
		t.Fatalf("session rejection error/session/index/find/commit = %v/%d/%d/%d/%d", err, len(index.sessionQueries), len(index.queries), objects.findCalls, authority.commitCalls)
	}
}

func TestCanonicalSourceRangesDeduplicatesAndTrimsOverlaps(t *testing.T) {
	ranges, err := canonicalSourceRanges([]reassembly.SourceRange{
		{SequenceStart: 106, SequenceEnd: 111, PacketIndex: 3, ObjectBucket: "pcap", ObjectKey: "source", ObjectVersion: "v1", ObjectRangeStart: 200, ObjectRangeEnd: 300, ObjectRangeExact: true, Length: 5},
		{SequenceStart: 100, SequenceEnd: 106, PacketIndex: 1, ObjectBucket: "pcap", ObjectKey: "source", ObjectVersion: "v1", ObjectRangeStart: 0, ObjectRangeEnd: 100, ObjectRangeExact: true, Length: 6},
		{SequenceStart: 100, SequenceEnd: 106, PacketIndex: 2, ObjectBucket: "pcap", ObjectKey: "source", ObjectVersion: "v1", ObjectRangeStart: 100, ObjectRangeEnd: 200, ObjectRangeExact: true, Length: 6},
		{SequenceStart: 104, SequenceEnd: 109, PacketIndex: 4, ObjectBucket: "pcap", ObjectKey: "source", ObjectVersion: "v1", ObjectRangeStart: 300, ObjectRangeEnd: 400, ObjectRangeExact: true, Length: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 3 || ranges[0].SequenceStart != 100 || ranges[0].SequenceEnd != 106 ||
		ranges[1].SequenceStart != 106 || ranges[1].SequenceEnd != 109 || ranges[1].Length != 3 ||
		ranges[2].SequenceStart != 109 || ranges[2].SequenceEnd != 111 || ranges[2].Length != 2 {
		t.Fatalf("canonical ranges = %+v", ranges)
	}
}
