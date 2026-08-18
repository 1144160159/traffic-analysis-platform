package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/restoration"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type RestorationTaskProcessor interface {
	Process(context.Context, restoration.ProcessRequest) (*restoration.CommitReceipt, error)
}

type VersionedPcapCutter interface {
	CutPCAPVersioned(context.Context, *cutter.CutQuery, []string, cutter.VerifiedCutLimits, io.Writer, cutter.ProgressCallback) (*cutter.CutResult, error)
}

type ImmutableResultStore interface {
	FindForensicsResultObject(context.Context, string, string, string, string, int64) (s3client.ObjectAuthority, bool, error)
	PutForensicsResultObject(context.Context, string, string, string, io.ReadSeeker, int64, string, time.Time) (s3client.ObjectAuthority, error)
}

type VersionedPipelineConfig struct {
	Enabled            bool
	MaxSourceObjects   int
	MaxSourceBytes     int64
	SourceRetention    time.Duration
	ResultRetention    time.Duration
	TemporaryDirectory string
}

func (config VersionedPipelineConfig) Validate() error {
	if !config.Enabled {
		return nil
	}
	if err := (cutter.VerifiedCutLimits{
		MaxSourceObjects: config.MaxSourceObjects,
		MaxSourceBytes:   config.MaxSourceBytes,
		SourceRetention:  config.SourceRetention,
	}).Validate(); err != nil {
		return err
	}
	if config.ResultRetention <= 0 {
		return errors.New("versioned pipeline result retention must be positive")
	}
	return nil
}

type VersionedJobManifest struct {
	ManifestVersion     int                         `json:"manifest_version"`
	TenantID            string                      `json:"tenant_id"`
	TaskID              string                      `json:"task_id"`
	RestorationContract int                         `json:"restoration_contract_version"`
	PcapIndexIDs        []string                    `json:"pcap_index_ids"`
	SourceReceipts      []s3client.ObjectAuthority  `json:"source_object_receipts"`
	ResultObject        s3client.ObjectAuthority    `json:"result_object"`
	RestorationReceipts []restoration.CommitReceipt `json:"restoration_receipts"`
	Status              string                      `json:"status"`
	CreatedAt           time.Time                   `json:"created_at"`
	CompletedAt         time.Time                   `json:"completed_at"`
	Executable          bool                        `json:"executable"`
	AutomaticOpen       bool                        `json:"automatic_open"`
}

type VersionedTaskResult struct {
	Manifest     VersionedJobManifest
	ManifestSHA  string
	Packets      int64
	Bytes        int64
	FilesScanned int
}

type VersionedStageCallback func(phase string, checkpoint any) error

type VersionedPipeline struct {
	config      VersionedPipelineConfig
	cutter      VersionedPcapCutter
	objects     ImmutableResultStore
	restoration RestorationTaskProcessor
}

func NewVersionedPipeline(
	config VersionedPipelineConfig,
	pcapCutter VersionedPcapCutter,
	objects ImmutableResultStore,
	restorationProcessor RestorationTaskProcessor,
) (*VersionedPipeline, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Enabled && (pcapCutter == nil || objects == nil) {
		return nil, errors.New("enabled versioned pipeline requires cutter and immutable result store")
	}
	return &VersionedPipeline{config: config, cutter: pcapCutter, objects: objects, restoration: restorationProcessor}, nil
}

func hashManifest(manifest VersionedJobManifest) (string, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (pipeline *VersionedPipeline) Process(
	ctx context.Context,
	taskID string,
	request CutTaskRequest,
	progress cutter.ProgressCallback,
	stage VersionedStageCallback,
) (*VersionedTaskResult, error) {
	if !pipeline.config.Enabled {
		return nil, errors.New("versioned forensics worker is disabled")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.RestorationContractVersion != 1 {
		return nil, errors.New("versioned forensics worker requires restoration contract v1")
	}
	startedAt := time.Now().UTC()
	temporary, err := os.CreateTemp(pipeline.config.TemporaryDirectory, "forensics-pcap-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create bounded forensics result staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	hasher := sha256.New()
	if stage != nil {
		if err := stage("reading_source", map[string]any{"probe_ids": request.ProbeIDs}); err != nil {
			return nil, err
		}
	}
	cutResult, err := pipeline.cutter.CutPCAPVersioned(ctx, request.ToCutQuery(), request.ProbeIDs,
		cutter.VerifiedCutLimits{
			MaxSourceObjects: pipeline.config.MaxSourceObjects,
			MaxSourceBytes:   pipeline.config.MaxSourceBytes,
			SourceRetention:  pipeline.config.SourceRetention,
		}, io.MultiWriter(temporary, hasher), progress)
	if err != nil {
		return nil, err
	}
	if stage != nil {
		if err := stage("verifying", map[string]any{"pcap_index_ids": cutResult.PcapIndexIDs}); err != nil {
			return nil, err
		}
	}
	info, err := temporary.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat staged forensics result: %w", err)
	}
	resultSHA := hex.EncodeToString(hasher.Sum(nil))
	resultKey := "tenants/" + request.TenantID + "/forensics/jobs/" + taskID + "/pcap/result.pcap"
	if stage != nil {
		if err := stage("publishing", map[string]any{"result_key": resultKey, "result_sha256": resultSHA}); err != nil {
			return nil, err
		}
	}
	resultObject, found, err := pipeline.objects.FindForensicsResultObject(ctx, resultKey, request.TenantID, taskID, resultSHA, info.Size())
	if err != nil {
		return nil, fmt.Errorf("recover immutable forensics result: %w", err)
	}
	if !found {
		if _, err := temporary.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("rewind staged forensics result: %w", err)
		}
		resultObject, err = pipeline.objects.PutForensicsResultObject(ctx, resultKey, request.TenantID, taskID,
			temporary, info.Size(), resultSHA, startedAt.Add(pipeline.config.ResultRetention))
		if err != nil {
			// A timed-out PUT may already be durable. Accept only an exact immutable
			// recovery; otherwise the task remains failed and unmanifested.
			recovered, recoveredFound, recoveryErr := pipeline.objects.FindForensicsResultObject(ctx, resultKey,
				request.TenantID, taskID, resultSHA, info.Size())
			if recoveryErr != nil {
				return nil, errors.Join(fmt.Errorf("put immutable forensics result: %w", err), recoveryErr)
			}
			if !recoveredFound {
				return nil, fmt.Errorf("put immutable forensics result: %w", err)
			}
			resultObject = recovered
		}
	}

	restorationReceipts := make([]restoration.CommitReceipt, 0, len(request.Restorations))
	if len(request.Restorations) > 0 && pipeline.restoration == nil {
		return nil, errors.New("task requests file restoration but M03 processor is unavailable")
	}
	if len(request.Restorations) > 0 && stage != nil {
		if err := stage("restoring_sessions", map[string]any{"restoration_count": len(request.Restorations)}); err != nil {
			return nil, err
		}
	}
	for _, spec := range request.Restorations {
		if stage != nil {
			if err := stage("restoring_files", map[string]any{"request_id": spec.RequestID}); err != nil {
				return nil, err
			}
		}
		receipt, processErr := pipeline.restoration.Process(ctx, restoration.ProcessRequest{
			TenantID: request.TenantID, IdempotencyKey: "forensics-job-" + taskID + "-" + spec.RequestID,
			SessionID: spec.SessionID, CommunityID: spec.CommunityID, FlowIDs: append([]string(nil), spec.FlowIDs...),
			FlowID: spec.FlowID, Tuple: spec.Tuple, Direction: spec.Direction,
			StartTime: time.UnixMilli(request.StartTime), EndTime: time.UnixMilli(request.EndTime),
			ProfileID: spec.ProfileID, FTPData: spec.FTPData, FTPTLSEnabled: spec.FTPTLSEnabled,
			ActorID: request.UserID, Reason: request.Purpose, TraceID: request.TraceID + ":" + spec.RequestID,
		})
		if processErr != nil {
			return nil, fmt.Errorf("M03 restoration %s: %w", spec.RequestID, processErr)
		}
		if receipt == nil {
			return nil, fmt.Errorf("M03 restoration %s returned no receipt", spec.RequestID)
		}
		restorationReceipts = append(restorationReceipts, *receipt)
	}
	sort.Slice(restorationReceipts, func(i, j int) bool {
		return restorationReceipts[i].RestorationID < restorationReceipts[j].RestorationID
	})
	terminalStatus := "completed"
	for _, receipt := range restorationReceipts {
		if receipt.Status != "complete" {
			terminalStatus = "partial"
			break
		}
	}
	manifest := VersionedJobManifest{
		ManifestVersion: 1, TenantID: request.TenantID, TaskID: taskID,
		RestorationContract: request.RestorationContractVersion,
		PcapIndexIDs:        append([]string(nil), cutResult.PcapIndexIDs...),
		SourceReceipts:      append([]s3client.ObjectAuthority(nil), cutResult.SourceReceipts...),
		ResultObject:        resultObject, RestorationReceipts: restorationReceipts,
		Status: terminalStatus, CreatedAt: startedAt, CompletedAt: time.Now().UTC(),
		Executable: false, AutomaticOpen: false,
	}
	manifestSHA, err := hashManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("hash versioned forensics manifest: %w", err)
	}
	return &VersionedTaskResult{
		Manifest: manifest, ManifestSHA: manifestSHA, Packets: cutResult.TotalPackets,
		Bytes: cutResult.TotalBytes, FilesScanned: cutResult.FilesScanned,
	}, nil
}
