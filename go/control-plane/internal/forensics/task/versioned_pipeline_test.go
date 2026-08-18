package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/restoration"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type fakeVersionedCutter struct{}

func (fakeVersionedCutter) CutPCAPVersioned(_ context.Context, _ *cutter.CutQuery, _ []string, _ cutter.VerifiedCutLimits, output io.Writer, progress cutter.ProgressCallback) (*cutter.CutResult, error) {
	payload := []byte("versioned-pcap-evidence")
	if _, err := output.Write(payload); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(1, 1, 3)
	}
	return &cutter.CutResult{
		TotalPackets: 3, TotalBytes: int64(len(payload)), FilesScanned: 1,
		PcapIndexIDs: []string{strings.Repeat("a", 64)},
		SourceReceipts: []s3client.ObjectAuthority{{
			Bucket: "pcap", Key: "source.pcap", VersionID: "source-v1", ETag: "source-etag",
			SizeBytes: 100, SHA256: strings.Repeat("b", 64), ObservedAt: time.Now().UTC(),
		}},
	}, nil
}

type fakeImmutableResultStore struct {
	found     bool
	puts      int
	authority s3client.ObjectAuthority
}

func (store *fakeImmutableResultStore) FindForensicsResultObject(context.Context, string, string, string, string, int64) (s3client.ObjectAuthority, bool, error) {
	return store.authority, store.found, nil
}

func (store *fakeImmutableResultStore) PutForensicsResultObject(_ context.Context, key, tenantID, taskID string, content io.ReadSeeker, size int64, expectedSHA string, retention time.Time) (s3client.ObjectAuthority, error) {
	store.puts++
	hasher := sha256.New()
	if _, err := io.Copy(hasher, content); err != nil {
		return s3client.ObjectAuthority{}, err
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedSHA {
		return s3client.ObjectAuthority{}, io.ErrUnexpectedEOF
	}
	store.authority = s3client.ObjectAuthority{
		Bucket: "results", Key: key, VersionID: "result-v1", ETag: "result-etag", SizeBytes: size,
		SHA256: expectedSHA, ObservedAt: time.Now().UTC(), RetentionUntil: retention,
	}
	return store.authority, nil
}

type fakeTaskRestorationProcessor struct{ status string }

func (processor fakeTaskRestorationProcessor) Process(_ context.Context, request restoration.ProcessRequest) (*restoration.CommitReceipt, error) {
	return &restoration.CommitReceipt{
		TenantID: request.TenantID, RestorationID: "restoration-" + request.IdempotencyKey,
		Revision: 1, Status: processor.status, EventID: "event-1", OutboxStatus: "pending", CreatedAt: time.Now().UTC(),
	}, nil
}

func versionedPipelineRequest() CutTaskRequest {
	now := time.Now().UTC()
	return CutTaskRequest{
		TenantID: "tenant-a", UserID: "analyst-a", ProbeIDs: []string{"probe-a"},
		SrcIP: "192.0.2.1", DstIP: "198.51.100.2", SrcPort: 51000, DstPort: 80, Protocol: 6,
		CommunityID: "1:test", StartTime: now.Add(-time.Minute).UnixMilli(), EndTime: now.Add(time.Minute).UnixMilli(),
		Purpose: "incident-response", PermissionSnapshot: []string{"pcap:write"}, RetentionPolicy: "standard",
		RestorationContractVersion: 1, TraceID: "trace-a",
		Restorations: []RestorationTaskSpec{{
			RequestID: "file-a", SessionID: "session-a", CommunityID: "1:test", FlowIDs: []string{"flow-a"}, FlowID: "flow-a",
			Tuple:     restoration.FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
			Direction: "server_to_client", ProfileID: "http1-response-body-v1",
		}},
	}
}

func TestVersionedPipelineCommitsInertPartialManifestThroughM03Receipt(t *testing.T) {
	store := &fakeImmutableResultStore{}
	pipeline, err := NewVersionedPipeline(VersionedPipelineConfig{
		Enabled: true, MaxSourceObjects: 10, MaxSourceBytes: 1 << 20,
		SourceRetention: 90 * 24 * time.Hour, ResultRetention: 24 * time.Hour,
	}, fakeVersionedCutter{}, store, fakeTaskRestorationProcessor{status: "partial"})
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	result, err := pipeline.Process(context.Background(), "task-a", versionedPipelineRequest(), nil, func(phase string, _ any) error {
		stages = append(stages, phase)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Status != "partial" || result.Manifest.Executable || result.Manifest.AutomaticOpen || store.puts != 1 {
		t.Fatalf("unexpected manifest/store result: %+v puts=%d", result.Manifest, store.puts)
	}
	if len(result.Manifest.RestorationReceipts) != 1 || result.Manifest.RestorationReceipts[0].Status != "partial" {
		t.Fatalf("M03 receipt missing from manifest: %+v", result.Manifest.RestorationReceipts)
	}
	if !strings.HasPrefix(result.Manifest.ResultObject.Key, "tenants/tenant-a/forensics/jobs/task-a/") {
		t.Fatalf("result key escaped tenant/task authority: %s", result.Manifest.ResultObject.Key)
	}
	wantStages := "reading_source,verifying,publishing,restoring_sessions,restoring_files"
	if strings.Join(stages, ",") != wantStages {
		t.Fatalf("stages = %v, want %s", stages, wantStages)
	}
}

func TestVersionedPipelineResumesExactObjectWithoutDuplicatePut(t *testing.T) {
	request := versionedPipelineRequest()
	digest := sha256.Sum256([]byte("versioned-pcap-evidence"))
	store := &fakeImmutableResultStore{found: true, authority: s3client.ObjectAuthority{
		Bucket: "results", Key: "tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap",
		VersionID: "result-v1", ETag: "result-etag", SizeBytes: int64(len("versioned-pcap-evidence")),
		SHA256: hex.EncodeToString(digest[:]), ObservedAt: time.Now().UTC(), RetentionUntil: time.Now().UTC().Add(time.Hour),
	}}
	pipeline, err := NewVersionedPipeline(VersionedPipelineConfig{
		Enabled: true, MaxSourceObjects: 10, MaxSourceBytes: 1 << 20,
		SourceRetention: 90 * 24 * time.Hour, ResultRetention: 24 * time.Hour,
	}, fakeVersionedCutter{}, store, fakeTaskRestorationProcessor{status: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Process(context.Background(), "task-a", request, nil, nil); err != nil {
		t.Fatal(err)
	}
	if store.puts != 0 {
		t.Fatalf("resume created %d duplicate object puts", store.puts)
	}
}
