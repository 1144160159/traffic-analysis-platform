package restoration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/extractor"
	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

var (
	ErrAdmissionDisabled         = errors.New("file restoration admission is disabled")
	ErrProcessorDraining         = errors.New("file restoration processor is draining")
	ErrTenantConcurrencyExceeded = errors.New("file restoration tenant concurrency exceeded")
)

type SourceIndex interface {
	VerifyRestorationSession(context.Context, forensicsindex.RestorationSessionQuery) (forensicsindex.RestorationSessionAuthority, error)
	LookupRestorationSources(context.Context, forensicsindex.RestorationSourceQuery) ([]forensicsindex.RestorationSource, error)
}

type QuarantineObjectStore interface {
	VerifiedObjectReader
	FindQuarantineObject(
		context.Context,
		string, string, string, string, string,
		int64,
	) (s3client.ObjectAuthority, bool, error)
	PutQuarantineObject(
		context.Context,
		string, string, string, string,
		[]byte, string, string,
		time.Time,
	) (s3client.ObjectAuthority, error)
}

type ManifestAuthority interface {
	ClaimRequest(context.Context, string, string, string, string, time.Time, time.Time, int) (AdmissionReceipt, error)
	ReleaseRequestClaim(context.Context, string, string, string, uuid.UUID) error
	Commit(context.Context, CommitCommand) (*CommitReceipt, error)
	RecordOrphan(context.Context, string, uuid.UUID, s3client.ObjectAuthority) error
}

type ProcessorConfig struct {
	Enabled           bool
	QuarantineBucket  string
	MaxSourceBytes    int64
	MaxSourceObjects  int
	MaxPackets        uint64
	MaxStreamBytes    uint64
	MaxObjectBytes    int64
	MaxPartCount      int
	MaxMIMEDepth      int
	MaxExpansionRatio float64
	TaskTimeout       time.Duration
	TenantConcurrency int
	RetentionDuration time.Duration
}

func (config ProcessorConfig) Validate() error {
	if !config.Enabled {
		return nil
	}
	if strings.TrimSpace(config.QuarantineBucket) == "" || strings.ContainsAny(config.QuarantineBucket, "\r\n\x00") {
		return errors.New("restoration quarantine bucket is required")
	}
	if config.MaxSourceBytes <= 0 || config.MaxSourceObjects <= 0 || config.MaxSourceObjects > 1000 ||
		config.MaxPackets == 0 || config.MaxStreamBytes == 0 || config.MaxObjectBytes <= 0 ||
		config.MaxPartCount <= 0 || config.MaxMIMEDepth <= 0 || config.MaxExpansionRatio < 1 ||
		config.TaskTimeout <= 0 || config.TenantConcurrency <= 0 || config.RetentionDuration <= 0 {
		return errors.New("enabled restoration requires positive bounded safety limits")
	}
	if config.MaxObjectBytes > int64(config.MaxStreamBytes) || config.MaxStreamBytes > uint64(config.MaxSourceBytes) {
		return errors.New("restoration limits must satisfy max_object_bytes <= max_stream_bytes <= max_source_bytes")
	}
	return nil
}

type FTPDataRequest struct {
	CommunityID string    `json:"community_id"`
	FlowID      string    `json:"flow_id"`
	Tuple       FiveTuple `json:"five_tuple"`
}

type ProcessRequest struct {
	TenantID       string
	IdempotencyKey string
	SessionID      string
	CommunityID    string
	FlowIDs        []string
	FlowID         string
	Tuple          FiveTuple
	Direction      string
	StartTime      time.Time
	EndTime        time.Time
	ProfileID      string
	FTPData        *FTPDataRequest
	FTPTLSEnabled  bool
	ActorID        string
	Reason         string
	TraceID        string
}

func (request ProcessRequest) Validate() error {
	if strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" ||
		strings.TrimSpace(request.SessionID) == "" || strings.TrimSpace(request.CommunityID) == "" ||
		strings.TrimSpace(request.FlowID) == "" || strings.TrimSpace(request.ActorID) == "" ||
		strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.TraceID) == "" {
		return errors.New("restoration request identities and audit fields are required")
	}
	if len(request.IdempotencyKey) < 16 || len(request.IdempotencyKey) > 200 {
		return errors.New("restoration idempotency key length must be between 16 and 200")
	}
	if err := validateSortedUnique(request.FlowIDs, "flow_ids"); err != nil {
		return err
	}
	if len(request.FlowIDs) == 0 {
		return errors.New("restoration primary flow is not present in flow_ids")
	}
	flowPosition := sort.SearchStrings(request.FlowIDs, request.FlowID)
	if flowPosition == len(request.FlowIDs) || request.FlowIDs[flowPosition] != request.FlowID {
		return errors.New("restoration primary flow is not present in flow_ids")
	}
	expectedFlowCount := 1
	if request.FTPData != nil && request.FTPData.FlowID != request.FlowID {
		expectedFlowCount++
	}
	if len(request.FlowIDs) != expectedFlowCount {
		return errors.New("restoration flow_ids contains an unbound flow identity")
	}
	if err := (SegmentLoadQuery{TenantID: request.TenantID, ProbeID: "validated-during-processing", Tuple: request.Tuple, Start: request.StartTime, End: request.EndTime}).Validate(); err != nil {
		return err
	}
	switch request.ProfileID {
	case extractor.ProfileHTTP1Response:
		if request.Direction != "server_to_client" || request.FTPData != nil || request.FTPTLSEnabled {
			return errors.New("HTTP restoration requires server_to_client direction and no FTP fields")
		}
	case extractor.ProfileSMTPDataMIME:
		if request.Direction != "client_to_server" || request.FTPData != nil || request.FTPTLSEnabled {
			return errors.New("SMTP restoration requires client_to_server direction and no FTP fields")
		}
	case extractor.ProfileFTPPassive:
		if request.Direction != "server_to_client" {
			return errors.New("FTP restoration requires server_to_client file direction")
		}
		if request.FTPData != nil {
			if strings.TrimSpace(request.FTPData.CommunityID) == "" || strings.TrimSpace(request.FTPData.FlowID) == "" {
				return errors.New("FTP data community and flow identities are required")
			}
			if request.FTPData.FlowID == request.FlowID || request.FTPData.CommunityID == request.CommunityID {
				return errors.New("FTP passive data connection must have distinct flow and community identities")
			}
			position := sort.SearchStrings(request.FlowIDs, request.FTPData.FlowID)
			if position == len(request.FlowIDs) || request.FlowIDs[position] != request.FTPData.FlowID {
				return errors.New("FTP data flow is not present in flow_ids")
			}
			if err := (SegmentLoadQuery{TenantID: request.TenantID, ProbeID: "validated-during-processing", Tuple: request.FTPData.Tuple, Start: request.StartTime, End: request.EndTime}).Validate(); err != nil {
				return fmt.Errorf("invalid FTP data tuple: %w", err)
			}
		}
	default:
		if request.Direction != "client_to_server" && request.Direction != "server_to_client" {
			return errors.New("unsupported restoration profile still requires an explicit canonical direction")
		}
		if request.FTPData != nil || request.FTPTLSEnabled {
			return errors.New("unsupported restoration profile cannot carry FTP-only fields")
		}
	}
	return nil
}

type segmentLoader func(
	context.Context,
	VerifiedObjectReader,
	[]forensicsindex.RestorationSource,
	SegmentLoadQuery,
	SegmentLoadLimits,
) (LoadedSegments, error)

type Processor struct {
	config       ProcessorConfig
	index        SourceIndex
	objects      QuarantineObjectStore
	authority    ManifestAuthority
	loadSegments segmentLoader
	mu           sync.Mutex
	active       map[string]int
	accepting    bool
	activeTasks  sync.WaitGroup
}

func NewProcessor(config ProcessorConfig, sourceIndex SourceIndex, objects QuarantineObjectStore, authority ManifestAuthority) (*Processor, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Enabled && (sourceIndex == nil || objects == nil || authority == nil) {
		return nil, errors.New("enabled restoration requires index, object and manifest authorities")
	}
	return &Processor{
		config: config, index: sourceIndex, objects: objects, authority: authority,
		loadSegments: LoadVerifiedSegments, active: make(map[string]int), accepting: true,
	}, nil
}

func (processor *Processor) acquire(tenantID string) error {
	processor.mu.Lock()
	defer processor.mu.Unlock()
	if !processor.accepting {
		return ErrProcessorDraining
	}
	if processor.active[tenantID] >= processor.config.TenantConcurrency {
		return ErrTenantConcurrencyExceeded
	}
	processor.active[tenantID]++
	processor.activeTasks.Add(1)
	return nil
}

func (processor *Processor) release(tenantID string) {
	processor.mu.Lock()
	defer processor.mu.Unlock()
	processor.active[tenantID]--
	if processor.active[tenantID] <= 0 {
		delete(processor.active, tenantID)
	}
	processor.activeTasks.Done()
}

// BeginDrain closes admission before HTTP shutdown. WaitForDrain then proves
// all already-admitted tasks left their lease/object/manifest critical section.
func (processor *Processor) BeginDrain() {
	processor.mu.Lock()
	processor.accepting = false
	processor.mu.Unlock()
}

func (processor *Processor) WaitForDrain(ctx context.Context) error {
	drained := make(chan struct{})
	go func() {
		processor.activeTasks.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for restoration processor drain: %w", ctx.Err())
	}
}

func (processor *Processor) lookupAndLoad(ctx context.Context, request ProcessRequest, probeID, communityID, flowID string, tuple FiveTuple) (LoadedSegments, error) {
	sources, err := processor.index.LookupRestorationSources(ctx, forensicsindex.RestorationSourceQuery{
		TenantID: request.TenantID, ProbeID: probeID, CommunityID: communityID, FlowID: flowID,
		StartTime: request.StartTime, EndTime: request.EndTime, Limit: processor.config.MaxSourceObjects,
	})
	if err != nil {
		return LoadedSegments{}, fmt.Errorf("lookup immutable PCAP sources for flow %s: %w", flowID, err)
	}
	return processor.loadSegments(ctx, processor.objects, sources, SegmentLoadQuery{
		TenantID: request.TenantID, ProbeID: probeID, Tuple: tuple, Start: request.StartTime, End: request.EndTime,
	}, SegmentLoadLimits{MaxSourceBytes: processor.config.MaxSourceBytes, MaxPackets: processor.config.MaxPackets})
}

func normalizePacketRanges(values []PacketRange) []PacketRange {
	values = append([]PacketRange(nil), values...)
	sort.Slice(values, func(i, j int) bool {
		if values[i].ObjectBucket != values[j].ObjectBucket {
			return values[i].ObjectBucket < values[j].ObjectBucket
		}
		if values[i].ObjectKey == values[j].ObjectKey {
			if values[i].ObjectVersion != values[j].ObjectVersion {
				return values[i].ObjectVersion < values[j].ObjectVersion
			}
			if values[i].Start == values[j].Start {
				return values[i].End < values[j].End
			}
			return values[i].Start < values[j].Start
		}
		return values[i].ObjectKey < values[j].ObjectKey
	})
	result := make([]PacketRange, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1].ObjectBucket != value.ObjectBucket ||
			result[len(result)-1].ObjectKey != value.ObjectKey || result[len(result)-1].ObjectVersion != value.ObjectVersion ||
			result[len(result)-1].End < value.Start {
			result = append(result, value)
			continue
		}
		if value.End > result[len(result)-1].End {
			result[len(result)-1].End = value.End
		}
	}
	return result
}

func normalizeSourceReceipts(values []s3client.ObjectAuthority) ([]s3client.ObjectAuthority, error) {
	values = append([]s3client.ObjectAuthority(nil), values...)
	sort.Slice(values, func(i, j int) bool {
		if values[i].Bucket != values[j].Bucket {
			return values[i].Bucket < values[j].Bucket
		}
		if values[i].Key != values[j].Key {
			return values[i].Key < values[j].Key
		}
		return values[i].VersionID < values[j].VersionID
	})
	result := make([]s3client.ObjectAuthority, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1].Bucket != value.Bucket || result[len(result)-1].Key != value.Key || result[len(result)-1].VersionID != value.VersionID {
			result = append(result, value)
			continue
		}
		prior := result[len(result)-1]
		if prior.ETag != value.ETag || prior.SizeBytes != value.SizeBytes || prior.SHA256 != value.SHA256 {
			return nil, errors.New("one immutable source version has conflicting receipts")
		}
	}
	return result, nil
}

func objectWriteAllowed(result extractor.Result) bool {
	switch result.Status {
	case extractor.StatusComplete:
		return true
	case extractor.StatusPartial, extractor.StatusTruncated, extractor.StatusCorrupt:
		return len(result.Content) > 0
	default:
		return false
	}
}

func processRequestSHA256(request ProcessRequest) (string, error) {
	return canonicalSHA256(struct {
		TenantID       string          `json:"tenant_id"`
		IdempotencyKey string          `json:"idempotency_key"`
		SessionID      string          `json:"session_id"`
		CommunityID    string          `json:"community_id"`
		FlowIDs        []string        `json:"flow_ids"`
		FlowID         string          `json:"flow_id"`
		Tuple          FiveTuple       `json:"five_tuple"`
		Direction      string          `json:"direction"`
		StartTime      time.Time       `json:"capture_time_start"`
		EndTime        time.Time       `json:"capture_time_end"`
		ProfileID      string          `json:"protocol_profile_id"`
		FTPData        *FTPDataRequest `json:"ftp_data,omitempty"`
		FTPTLSEnabled  bool            `json:"ftp_tls_enabled"`
		ActorID        string          `json:"actor_id"`
		Reason         string          `json:"reason"`
	}{
		TenantID: request.TenantID, IdempotencyKey: request.IdempotencyKey, SessionID: request.SessionID,
		CommunityID: request.CommunityID, FlowIDs: request.FlowIDs, FlowID: request.FlowID, Tuple: request.Tuple,
		Direction: request.Direction, StartTime: request.StartTime, EndTime: request.EndTime, ProfileID: request.ProfileID,
		FTPData: request.FTPData, FTPTLSEnabled: request.FTPTLSEnabled, ActorID: request.ActorID, Reason: request.Reason,
	})
}

func (processor *Processor) Process(ctx context.Context, request ProcessRequest) (receipt *CommitReceipt, processErr error) {
	if !processor.config.Enabled {
		return nil, ErrAdmissionDisabled
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	requestSHA, err := processRequestSHA256(request)
	if err != nil {
		return nil, fmt.Errorf("hash restoration request: %w", err)
	}
	startedAt := time.Now().UTC()
	processingDeadline := startedAt.Add(processor.config.TaskTimeout)
	// The admission lease outlives the I/O deadline. That safety interval keeps
	// another replica from taking over while a canceled object request is still
	// unwinding, without extending the task's maximum processing time.
	leaseUntil := processingDeadline.Add(15 * time.Second)
	admission, claimErr := processor.authority.ClaimRequest(ctx, request.TenantID, request.IdempotencyKey,
		requestSHA, request.TraceID, startedAt, leaseUntil, processor.config.TenantConcurrency)
	if claimErr != nil {
		return nil, claimErr
	}
	switch admission.Result {
	case AdmissionReplay:
		if admission.Receipt == nil {
			return nil, errors.New("restoration replay admission lacks a receipt")
		}
		return admission.Receipt, nil
	case AdmissionInProgress:
		return nil, ErrRequestInProgress
	case AdmissionClaimed:
		if admission.ClaimToken == uuid.Nil {
			return nil, errors.New("restoration claimed admission lacks a fencing token")
		}
	default:
		return nil, fmt.Errorf("unexpected restoration admission result %q", admission.Result)
	}
	claimFinalized := false
	defer func() {
		if claimFinalized {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if releaseErr := processor.authority.ReleaseRequestClaim(cleanupCtx, request.TenantID, request.IdempotencyKey, requestSHA, admission.ClaimToken); releaseErr != nil {
			if processErr == nil {
				processErr = releaseErr
			} else {
				processErr = errors.Join(processErr, releaseErr)
			}
		}
	}()
	if err := processor.acquire(request.TenantID); err != nil {
		return nil, err
	}
	defer processor.release(request.TenantID)
	ctx, cancel := context.WithDeadline(ctx, processingDeadline)
	defer cancel()
	sessionAuthority, err := processor.index.VerifyRestorationSession(ctx, forensicsindex.RestorationSessionQuery{
		TenantID: request.TenantID, SessionID: request.SessionID, CommunityID: request.CommunityID,
		PrimaryFlowID: request.FlowID, StartTime: request.StartTime, EndTime: request.EndTime,
	})
	if err != nil {
		return nil, fmt.Errorf("verify restoration session authority: %w", err)
	}

	control, err := processor.lookupAndLoad(ctx, request, sessionAuthority.ProbeID, request.CommunityID, request.FlowID, request.Tuple)
	if err != nil {
		return nil, err
	}
	clientStream, err := reassembly.Reassemble(control.ClientToServer, processor.config.MaxStreamBytes)
	if err != nil {
		return nil, fmt.Errorf("reassemble client_to_server stream: %w", err)
	}
	serverStream, err := reassembly.Reassemble(control.ServerToClient, processor.config.MaxStreamBytes)
	if err != nil {
		return nil, fmt.Errorf("reassemble server_to_client stream: %w", err)
	}
	input := extractor.Input{
		ProfileID: request.ProfileID, ClientToServer: clientStream, ServerToClient: serverStream,
		ConnectionClosed: control.ServerToClientEnd != nil, ConnectionReset: control.ConnectionReset,
		FTPTLSEnabled: request.FTPTLSEnabled,
	}
	sourceReceipts := append([]s3client.ObjectAuthority(nil), control.SourceReceipts...)
	packetRanges := append([]PacketRange(nil), control.PacketRanges...)
	pcapIndexIDs := append([]string(nil), control.PcapIndexIDs...)
	proofRanges := serverStream.SourceRanges
	if request.Direction == "client_to_server" {
		proofRanges = clientStream.SourceRanges
	}
	if request.ProfileID == extractor.ProfileFTPPassive && request.FTPData != nil {
		data, loadErr := processor.lookupAndLoad(ctx, request, sessionAuthority.ProbeID, request.FTPData.CommunityID, request.FTPData.FlowID, request.FTPData.Tuple)
		if loadErr != nil {
			return nil, loadErr
		}
		dataStream, reassemblyErr := reassembly.Reassemble(data.ServerToClient, processor.config.MaxStreamBytes)
		if reassemblyErr != nil {
			return nil, fmt.Errorf("reassemble FTP data stream: %w", reassemblyErr)
		}
		input.FTPDataServerToClient = dataStream
		input.FTPDataConnections = 1
		input.FTPDataCorrelated = extractor.CorrelateFTPPassiveData(
			clientStream.Bytes, serverStream.Bytes, request.Tuple.DestinationIP,
			request.FTPData.Tuple.SourceIP, request.FTPData.Tuple.SourcePort,
			request.FTPData.Tuple.DestinationIP, request.FTPData.Tuple.DestinationPort,
		)
		input.FTPDataReset = data.ConnectionReset
		input.FTPDataBoundaryComplete = !data.ConnectionReset && data.ServerToClientStart != nil &&
			data.ServerToClientEnd != nil && ((len(dataStream.Bytes) == 0 && *data.ServerToClientStart == *data.ServerToClientEnd) ||
			(dataStream.BaseSequence == *data.ServerToClientStart && dataStream.EndSequence == *data.ServerToClientEnd))
		sourceReceipts = append(sourceReceipts, data.SourceReceipts...)
		packetRanges = append(packetRanges, data.PacketRanges...)
		pcapIndexIDs = append(pcapIndexIDs, data.PcapIndexIDs...)
		proofRanges = dataStream.SourceRanges
	}
	extraction, err := extractor.Extract(input, extractor.Limits{
		MaxObjectBytes: processor.config.MaxObjectBytes, MaxPartCount: processor.config.MaxPartCount,
		MaxMIMEDepth: processor.config.MaxMIMEDepth, MaxExpansionRate: processor.config.MaxExpansionRatio,
	})
	if err != nil {
		return nil, fmt.Errorf("extract bounded restoration content: %w", err)
	}

	restorationID := deterministicRestorationID(request.TenantID, request.IdempotencyKey)
	var object *s3client.ObjectAuthority
	if objectWriteAllowed(extraction) {
		contentType := extraction.DetectedMIMEType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		key := "tenants/" + request.TenantID + "/restorations/" + restorationID.String() + "/1/content.bin"
		receipt, found, findErr := processor.objects.FindQuarantineObject(ctx, processor.config.QuarantineBucket, key,
			request.TenantID, restorationID.String(), extraction.ContentSHA256, int64(len(extraction.Content)))
		if findErr != nil {
			return nil, fmt.Errorf("recover inert quarantine object: %w", findErr)
		}
		if !found {
			var putErr error
			receipt, putErr = processor.objects.PutQuarantineObject(ctx, processor.config.QuarantineBucket, key,
				request.TenantID, restorationID.String(), extraction.Content, contentType, extraction.ContentSHA256,
				startedAt.Add(processor.config.RetentionDuration))
			if putErr != nil {
				// A conditional PUT can race with a lease predecessor or return an
				// unknown network outcome after durable storage. Re-read authority;
				// only an exact immutable object converts that error into success.
				recovered, recoveredFound, recoverErr := processor.objects.FindQuarantineObject(ctx,
					processor.config.QuarantineBucket, key, request.TenantID, restorationID.String(),
					extraction.ContentSHA256, int64(len(extraction.Content)))
				if recoverErr != nil {
					return nil, errors.Join(fmt.Errorf("write inert quarantine object: %w", putErr),
						fmt.Errorf("recover conditional quarantine write: %w", recoverErr))
				}
				if !recoveredFound {
					return nil, fmt.Errorf("write inert quarantine object: %w", putErr)
				}
				receipt = recovered
			}
		}
		if orphanErr := processor.authority.RecordOrphan(ctx, request.TenantID, restorationID, receipt); orphanErr != nil {
			return nil, fmt.Errorf("record committed object before manifest: %w", orphanErr)
		}
		object = &receipt
	}
	sourceReceipts, err = normalizeSourceReceipts(sourceReceipts)
	if err != nil {
		return nil, err
	}
	packetRanges = normalizePacketRanges(packetRanges)
	sort.Strings(pcapIndexIDs)
	pcapIndexIDs = compactStrings(pcapIndexIDs)
	completedAt := time.Now().UTC()
	proofRanges, err = canonicalSourceRanges(proofRanges)
	if err != nil {
		return nil, err
	}
	flowSelections := []FlowSelection{{
		Role: "primary", CommunityID: request.CommunityID, FlowID: request.FlowID,
		FiveTuple: request.Tuple, Direction: request.Direction,
	}}
	if request.ProfileID == extractor.ProfileFTPPassive && request.FTPData != nil {
		flowSelections = append(flowSelections, FlowSelection{
			Role: "ftp_data", CommunityID: request.FTPData.CommunityID, FlowID: request.FTPData.FlowID,
			FiveTuple: request.FTPData.Tuple, Direction: "server_to_client",
		})
	}
	manifest := Manifest{
		RestorationID: restorationID, TenantID: request.TenantID, Revision: 1, IdempotencyKey: request.IdempotencyKey,
		SessionID: request.SessionID, CommunityID: request.CommunityID, SessionAuthority: sessionAuthority,
		PrimaryFlowID: request.FlowID, FlowIDs: append([]string(nil), request.FlowIDs...), FlowSelections: flowSelections,
		PcapIndexIDs: pcapIndexIDs, SourceObjectReceipts: sourceReceipts, FiveTuple: request.Tuple,
		Direction: request.Direction, CaptureTimeStart: request.StartTime, CaptureTimeEnd: request.EndTime,
		PacketRanges: packetRanges, TCPSequenceRanges: append([]reassembly.SourceRange(nil), proofRanges...),
		Extraction: extraction, Object: object, TraceID: request.TraceID, CreatedAt: startedAt, CompletedAt: completedAt,
	}
	receipt, processErr = processor.authority.Commit(ctx, CommitCommand{
		ActorID: request.ActorID, Reason: request.Reason, TraceID: request.TraceID,
		ClaimToken: admission.ClaimToken, IdempotencyKey: request.IdempotencyKey,
		RequestSHA256: requestSHA, Manifest: manifest,
	})
	if processErr == nil {
		claimFinalized = true
	}
	return receipt, processErr
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func sequenceBefore(left, right uint32) bool {
	return int32(left-right) < 0
}

func canonicalSourceRanges(values []reassembly.SourceRange) ([]reassembly.SourceRange, error) {
	values = append([]reassembly.SourceRange(nil), values...)
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].SequenceStart == values[j].SequenceStart {
			return values[i].PacketIndex < values[j].PacketIndex
		}
		return sequenceBefore(values[i].SequenceStart, values[j].SequenceStart)
	})
	result := make([]reassembly.SourceRange, 0, len(values))
	for _, value := range values {
		if value.SequenceStart == value.SequenceEnd || value.Length <= 0 || value.ObjectBucket == "" ||
			value.ObjectKey == "" || value.ObjectVersion == "" || value.ObjectRangeEnd <= value.ObjectRangeStart {
			return nil, errors.New("invalid TCP source proof range")
		}
		if len(result) == 0 {
			result = append(result, value)
			continue
		}
		prior := result[len(result)-1]
		if !sequenceBefore(value.SequenceStart, prior.SequenceEnd) {
			result = append(result, value)
			continue
		}
		if !sequenceBefore(prior.SequenceEnd, value.SequenceEnd) {
			continue
		}
		value.SequenceStart = prior.SequenceEnd
		value.Length = int(uint32(value.SequenceEnd - value.SequenceStart))
		result = append(result, value)
	}
	return result, nil
}
