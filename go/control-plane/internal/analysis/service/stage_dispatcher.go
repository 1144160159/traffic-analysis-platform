package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// SourceExecutor 采集执行器端口(contract.SourceExecutor 别名;测试可桩)。
type SourceExecutor = contract.SourceExecutor

// SourceSpecPcap 冻结源规格(plan source_spec 内字段,PCAP_REPLAY)。
// ProbeID:人工执行链选定的探针(回放发生在探针位置)。
// Interface:测试阶段 wire 回放注入目标(虚拟网卡输入端);空 = 进程内共享分支喂入。
type SourceSpecPcap struct {
	PcapObject  string `json:"pcap_object"`
	PcapSHA256  string `json:"pcap_sha256"`
	PacketLimit int64  `json:"packet_limit"`
	ByteLimit   int64  `json:"byte_limit"`
	ProbeID     string `json:"probe_id"`
	Interface   string `json:"interface,omitempty"`
}

// SourceSpecCaptureWindow 冻结源规格(PROBE_CAPTURE_WINDOW/LIVE_STREAM_WINDOW,
// 实时采集:探针常驻采集覆盖窗口,流量经共享分支按订阅窗口归属 run)。
type SourceSpecCaptureWindow struct {
	ProbeID         string `json:"probe_id"`
	Interface       string `json:"interface"`
	BPFFilter       string `json:"bpf_filter,omitempty"`
	PacketLimit     int64  `json:"packet_limit"`
	ByteLimit       int64  `json:"byte_limit"`
	SpoolQuotaBytes int64  `json:"spool_quota_bytes,omitempty"`
	LeaseEpoch      int64  `json:"lease_epoch,omitempty"`
}

// SourceCommandBuilder 按 SourceKind 构造采集命令(OCP:注册表扩展,不修改派发循环)。
type SourceCommandBuilder func(att *repository.PendingSourceAttempt, token string) (contract.SourceStageCommand, error)

// defaultSourceCommandBuilders 核心卷注册表:新增 SourceKind 只注册,不改派发循环。
func defaultSourceCommandBuilders() map[string]SourceCommandBuilder {
	return map[string]SourceCommandBuilder{
		"PCAP_REPLAY":          buildPcapReplayCommand,
		"PROBE_CAPTURE_WINDOW": buildCaptureWindowCommand,
		"LIVE_STREAM_WINDOW":   buildCaptureWindowCommand,
	}
}

// StageDispatcher SOURCE_ACTIVATE 派发循环:
// 领取 PENDING 尝试 → CAS RUNNING(带 fencing token)→ 执行器 Dispatch → 回执 APPLIED。
// 执行失败同样落 FAILED 回执(fail-closed,不重试;重试由上层幂等机制裁决)。
type StageDispatcher struct {
	repo      *repository.Repo
	executors map[string]SourceExecutor
	logger    *zap.Logger
	now       func() time.Time
	builders  map[string]SourceCommandBuilder
	walker    *RunStateWalker
	leaseTTL  time.Duration
	// TenantScope 非空时仅派发该租户(分片/测试隔离);为空派发全部。
	TenantScope string
}

// NewStageDispatcher 构造派发器(默认命令构造注册表;executor 作为全 kind 缺省执行器)。
func NewStageDispatcher(repo *repository.Repo, executor SourceExecutor, logger *zap.Logger) *StageDispatcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	d := &StageDispatcher{repo: repo, logger: logger, now: time.Now, leaseTTL: 5 * time.Minute,
		builders: defaultSourceCommandBuilders(), executors: map[string]SourceExecutor{}}
	if executor != nil {
		d.executors[""] = executor
	}
	return d
}

// SetExecutor 按 SourceKind 注册执行器(OCP:"" 为全 kind 缺省)。
func (d *StageDispatcher) SetExecutor(kind string, executor SourceExecutor) {
	if d.executors == nil {
		d.executors = map[string]SourceExecutor{}
	}
	d.executors[kind] = executor
}

// SetRunStateWalker 注入状态机行走器(准入消费 + QUEUED→RUNNING;nil 时跳过)。
func (d *StageDispatcher) SetRunStateWalker(w *RunStateWalker) { d.walker = w }

// executorFor 取该 SourceKind 的执行器(未注册回退缺省;无缺省返回 nil=fail-closed)。
func (d *StageDispatcher) executorFor(kind string) SourceExecutor {
	if e, ok := d.executors[kind]; ok {
		return e
	}
	return d.executors[""]
}

// RegisterSourceBuilder 注册 SourceKind 命令构造器(OCP 扩展点)。
func (d *StageDispatcher) RegisterSourceBuilder(kind string, b SourceCommandBuilder) {
	if d.builders == nil {
		d.builders = map[string]SourceCommandBuilder{}
	}
	d.builders[kind] = b
}

// DispatchOnce 处理最多 limit 条候选(多副本安全:SKIP LOCKED + CAS)。
func (d *StageDispatcher) DispatchOnce(ctx context.Context, limit int) (dispatched int, err error) {
	if len(d.executors) == 0 {
		return 0, fmt.Errorf("source executor is not configured")
	}
	for i := 0; i < limit; i++ {
		// §76.45.3 单事务领取:DRR 稳定排序选中 + queue/attempt CAS + DRR 更新 +
		// 预留消费 + lease + ACTIVE 订阅 outbox + 审计同事务(多副本 SKIP LOCKED 安全)。
		lease, err := d.repo.ClaimStageLeaseAtomic(ctx, d.TenantScope, d.leaseTTL, d.now())
		if err != nil {
			return dispatched, fmt.Errorf("claim stage lease: %w", err)
		}
		if lease == nil {
			return dispatched, nil
		}
		if err := d.dispatchOne(ctx, lease); err != nil {
			d.logger.Warn("source attempt dispatch failed", zap.String("run_id", lease.RunID), zap.Error(err))
			continue
		}
		dispatched++
	}
	return dispatched, nil
}

func (d *StageDispatcher) dispatchOne(ctx context.Context, lease *repository.ClaimedStageLease) error {
	att := &lease.PendingSourceAttempt
	token := lease.FencingToken
	// 领取已由 ClaimStageLeaseAtomic 单事务完成(DISPATCHED CAS + lease + DRR +
	// 预留消费 + ACTIVE 订阅 outbox + 审计);此处只做协议转换与状态行走。
	// Run 状态行走(QUEUED→RUNNING;事实驱动 Sync 幂等;ACTIVE 订阅由 outbox 中继后
	// 中继侧再次 Sync)。
	if d.walker != nil {
		d.walker.Sync(ctx, att.TenantID, att.RunID)
	}

	cmd, buildErr := d.buildCommand(att, token)
	var receipt *contract.ProviderOperationReceipt
	var err error
	exec := d.executorFor(att.SourceKind)
	if buildErr == nil && exec != nil {
		receipt, err = exec.Dispatch(ctx, cmd)
	} else if buildErr == nil && exec == nil {
		buildErr = fmt.Errorf("no source executor registered for source_kind %q", att.SourceKind)
	}

	if buildErr != nil {
		receipt = &contract.ProviderOperationReceipt{
			OperationID: att.RunID + ":SOURCE_ACTIVATE",
			State:       "FAILED",
			InputCount:  0, OutputCount: 0, ErrorCount: 1,
			Fence: json.RawMessage(`{"kind":"source_fence","detail":"invalid frozen source spec"}`),
		}
		err = buildErr
	}
	if receipt == nil {
		// 执行器未产出回执即失败语义:合成 FAILED 回执(fail-closed,防 nil 崩)
		detail := "executor returned no receipt"
		if err != nil {
			detail = err.Error()
		} else {
			err = fmt.Errorf("executor returned no receipt")
		}
		fence, _ := json.Marshal(map[string]string{"kind": "source_fence", "detail": detail})
		receipt = &contract.ProviderOperationReceipt{
			OperationID: att.RunID + ":SOURCE_ACTIVATE",
			State:       "FAILED",
			InputCount:  0, OutputCount: 0, ErrorCount: 1,
			Fence: fence,
		}
	}

	// 协议转换式派发:ACCEPTED 表示命令已投递探针,终态回执由
	// 探针→gateway→analysis.receipts.v1 异步回流。
	// 接受是生命周期事实(DISPATCHED→RUNNING)而非终态回执:直接 CAS,
	// 不写 stage receipt(每终态 attempt 恰一条 fence 匹配回执的对账不变式)。
	if receipt.State == "ACCEPTED" {
		if ok, casErr := d.repo.MarkAttemptAcceptedAtomic(ctx, att.TenantID, att.AttemptID, token); casErr != nil || !ok {
			d.logger.Warn("accept CAS lost", zap.String("run_id", att.RunID), zap.Error(casErr))
		}
		d.logger.Info("source command accepted (async receipt expected)",
			zap.String("run_id", att.RunID), zap.String("operation_id", receipt.OperationID))
		return nil
	}

	newState := "SUCCEEDED"
	errorCount := receipt.ErrorCount
	if err != nil || receipt.State == "FAILED" {
		newState = "FAILED"
		if errorCount == 0 {
			errorCount = 1
		}
	}
	d.applySourceReceipt(ctx, att, token, receipt, newState, errorCount, err)
	return nil
}

// applySourceReceipt 应用同步派发回执(dispatch receipt 或同步终态回执)。
// expected state 为 DISPATCHED:命令领取后执行器即时返回的状态。
func (d *StageDispatcher) applySourceReceipt(ctx context.Context, att *repository.PendingSourceAttempt, token string, receipt *contract.ProviderOperationReceipt, newState string, errorCount int64, dispatchErr error) {
	fenceJSON := receipt.Fence
	if fenceJSON == nil {
		fenceJSON = json.RawMessage(`{}`)
	}
	tuple := fmt.Sprintf("%s|%s|%d", att.RunID, att.ExecutionNodeID, att.Attempt)
	hash := sha256.Sum256([]byte(string(fenceJSON) + "|" + newState))
	payloadHash := hex.EncodeToString(hash[:])
	_, applyErr := d.repo.ApplyStageReceiptAtomic(ctx, repository.ReceiptCommand{
		TenantID:        att.TenantID,
		RunID:           att.RunID,
		EventID:         "dispatch-" + att.AttemptID,
		TupleHash:       tuple,
		ExecutionNodeID: att.ExecutionNodeID,
		Attempt:         att.Attempt,
		FencingToken:    token,
		Provider:        "analysis-replay",
		InputCount:      receipt.InputCount,
		OutputCount:     receipt.OutputCount,
		ErrorCount:      errorCount,
		RejectCount:     receipt.RejectCount,
		WatermarkMs:     receipt.WatermarkMs,
		FenceJSON:       fenceJSON,
		PayloadHash:     payloadHash,
		ExpectedState:   "DISPATCHED",
		NewState:        newState,
	})
	if applyErr != nil {
		d.logger.Warn("apply source receipt failed",
			zap.String("run_id", att.RunID), zap.Error(applyErr))
		return
	}
	if newState == "FAILED" {
		d.logger.Warn("source attempt failed",
			zap.String("run_id", att.RunID), zap.String("detail", string(fenceJSON)))
		// 回执已按 FAILED 落库(执行失败语义),不再向上重抛
		return
	}
	d.logger.Info("source attempt completed",
		zap.String("run_id", att.RunID), zap.Int64("packets", receipt.InputCount),
		zap.Int64("flows", receipt.OutputCount), zap.String("state", newState))
}

func (d *StageDispatcher) buildCommand(att *repository.PendingSourceAttempt, token string) (contract.SourceStageCommand, error) {
	builder, ok := d.builders[att.SourceKind]
	if !ok {
		return contract.SourceStageCommand{}, fmt.Errorf("unknown source_kind %q", att.SourceKind)
	}
	return builder(att, token)
}

// buildPcapReplayCommand PCAP_REPLAY 命令构造(冻结源规格 + run 级 fencing)。
func buildPcapReplayCommand(att *repository.PendingSourceAttempt, token string) (contract.SourceStageCommand, error) {
	var cmd contract.SourceStageCommand
	var spec SourceSpecPcap
	if len(att.SourceSpec) == 0 || string(att.SourceSpec) == "null" {
		return cmd, fmt.Errorf("source_spec is empty")
	}
	if err := json.Unmarshal(att.SourceSpec, &spec); err != nil {
		return cmd, fmt.Errorf("source_spec malformed: %w", err)
	}
	if spec.ProbeID == "" {
		return cmd, fmt.Errorf("source_spec.probe_id is required (manual chain selects the probe)")
	}
	cmd = contract.SourceStageCommand{
		TenantID:            att.TenantID,
		TaskID:              att.TaskID,
		RunID:               att.RunID,
		ExecutionSpecSHA256: att.ExecutionSpecSHA256,
		SourceKind:          "PCAP_REPLAY",
		ProbeID:             spec.ProbeID,
		ObjectRef:           spec.PcapObject,
		ObjectSHA256:        spec.PcapSHA256,
		Interface:           spec.Interface,
		WindowStartMs:       att.WindowStartMs,
		WindowEndMs:         att.WindowEndMs,
		PacketLimit:         spec.PacketLimit,
		ByteLimit:           spec.ByteLimit,
		FencingToken:        token,
	}
	return cmd, nil
}

// buildCaptureWindowCommand PROBE_CAPTURE_WINDOW/LIVE_STREAM_WINDOW 命令构造
// (实时采集:冻结源规格 + run 级 fencing;窗口由探针常驻采集覆盖)。
func buildCaptureWindowCommand(att *repository.PendingSourceAttempt, token string) (contract.SourceStageCommand, error) {
	var cmd contract.SourceStageCommand
	var spec SourceSpecCaptureWindow
	if len(att.SourceSpec) == 0 || string(att.SourceSpec) == "null" {
		return cmd, fmt.Errorf("source_spec is empty")
	}
	if err := json.Unmarshal(att.SourceSpec, &spec); err != nil {
		return cmd, fmt.Errorf("source_spec malformed: %w", err)
	}
	if spec.ProbeID == "" {
		return cmd, fmt.Errorf("source_spec.probe_id is required (manual chain selects the probe)")
	}
	if spec.Interface == "" {
		return cmd, fmt.Errorf("source_spec.interface is required (monitored probe interface)")
	}
	if spec.PacketLimit <= 0 && spec.ByteLimit <= 0 {
		return cmd, fmt.Errorf("source_spec.packet_limit or byte_limit is required (bounded capture)")
	}
	cmd = contract.SourceStageCommand{
		TenantID:            att.TenantID,
		TaskID:              att.TaskID,
		RunID:               att.RunID,
		ExecutionSpecSHA256: att.ExecutionSpecSHA256,
		SourceKind:          "PROBE_CAPTURE_WINDOW",
		ProbeID:             spec.ProbeID,
		Interface:           spec.Interface,
		BPFFilter:           spec.BPFFilter,
		WindowStartMs:       att.WindowStartMs,
		WindowEndMs:         att.WindowEndMs,
		PacketLimit:         spec.PacketLimit,
		ByteLimit:           spec.ByteLimit,
		SpoolQuotaBytes:     spec.SpoolQuotaBytes,
		LeaseEpoch:          spec.LeaseEpoch,
		FencingToken:        token,
	}
	return cmd, nil
}

// publishActiveSubscription S1 领取后发布 ACTIVE 订阅(带预分配流水线 fence;
// revision 取当前 run revision,PREPARE rev1 之后单调)。
func (d *StageDispatcher) publishActiveSubscription(ctx context.Context, att *repository.PendingSourceAttempt) error {
	run, err := d.repo.GetRun(ctx, att.TenantID, att.RunID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	taskID, planRevision, sourceKind, policySHA, err := d.repo.GetRunSubscriptionFacts(ctx, att.RunID)
	if err != nil {
		return fmt.Errorf("load subscription facts: %w", err)
	}
	fence, err := d.repo.PipelinedFenceToken(ctx, att.RunID)
	if err != nil {
		return fmt.Errorf("pipeline fence: %w", err)
	}
	sub := contract.RunSubscription{
		SchemaVersion:         "1",
		TenantID:              att.TenantID,
		RunID:                 att.RunID,
		TaskID:                taskID,
		Revision:              run.Revision,
		State:                 "ACTIVE",
		ExecutionSpecSHA256:   run.ExecutionSpecSHA256,
		PlanRevision:          planRevision,
		SourceKind:            sourceKind,
		WindowStartMs:         run.WindowStartMs,
		WindowEndMs:           run.WindowEndMs,
		PrepareAtMs:           run.WindowStartMs,
		LeaseEpoch:            1,
		EffectivePolicySHA256: policySHA,
		ExpiresAtMs:           run.WindowEndMs + 60_000,
	}
	if fence != "" {
		sub.Fence = json.RawMessage(fmt.Sprintf("%q", fence))
	}
	return d.repo.EnqueueRunSubscriptionUpdate(ctx, sub)
}
