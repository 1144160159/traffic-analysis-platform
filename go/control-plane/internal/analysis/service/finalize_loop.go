package service

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// FinalizeLoop 终局循环(权威自身):S1-S4 全部终态后,
// 1) 开闸 S5(RECONCILE/MACHINE_FINALIZATION,权威自产回执,无外部执行器);
// 2) 对账(reconcile):每终态 attempt 恰一条 fence 匹配回执 + 计数守恒;
// 3) 事实装配 → EvaluateRunClosure → 三件套同事务 + 唯一终态。
type FinalizeLoop struct {
	repo   *repository.Repo
	logger *zap.Logger
}

// NewFinalizeLoop 构造。
func NewFinalizeLoop(repo *repository.Repo, logger *zap.Logger) *FinalizeLoop {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FinalizeLoop{repo: repo, logger: logger}
}

// ClosureReconcileReport 对账报告(RECONCILE 回执 fence + closure manifest differences)。
type ClosureReconcileReport struct {
	OK              bool     `json:"ok"`
	AttemptsChecked int      `json:"attempts_checked"`
	Differences     int      `json:"differences"`
	Items           []string `json:"items"`
	// ReceiptFence 回执 fence 的会话/检测覆盖摘要(供 RECONCILE 回执 fence 内嵌)。
	SourceInput  int64 `json:"source_input"`
	SessionFlows int64 `json:"session_flows"`
	Sessions     int64 `json:"sessions"`
	DetectTotal  int64 `json:"detect_total"`
	SourceIsCaptureWindow bool `json:"source_is_capture_window"`
	Positive     int64 `json:"positive"`
	Negative     int64 `json:"negative"`
	Inconclusive int64 `json:"inconclusive"`
	DetectError  int64 `json:"detect_error"`
	DetectIncompatible int64 `json:"detect_incompatible"`
	DetectNotRun int64 `json:"detect_not_run"`
}

// FinalizeOnce 终局一个候选 run;无候选返回 (false, nil)。
func (l *FinalizeLoop) FinalizeOnce(ctx context.Context) (bool, error) {
	cand, err := l.repo.NextFinalizeCandidate(ctx)
	if err != nil {
		return false, fmt.Errorf("next finalize candidate: %w", err)
	}
	if cand == nil {
		return false, nil
	}
	log := l.logger.With(zap.String("run_id", cand.RunID))

	// 1. 迁移到 FINALIZING(幂等:已是 FINALIZING 则继续)。
	switch cand.RunState {
	case "ACCEPTED", "PREPARING", "QUEUED", "RUNNING":
		if err := l.repo.TransitionRunAtomic(ctx, cand.TenantID, cand.RunID, cand.RunState, "FINALIZING"); err != nil {
			log.Warn("finalizing transition lost (concurrent finalizer?)", zap.Error(err))
			return false, nil
		}
		log.Info("run entering FINALIZING")
	case "FINALIZING":
		// 续跑(上次循环在中间步骤失败):继续。
	default:
		return false, nil
	}

	// 2. 开闸 S5(若 PENDING):权威自身阶段,同一 token 批量开闸。
	facts, err := l.repo.LoadRunClosureFacts(ctx, cand.RunID)
	if err != nil {
		return false, fmt.Errorf("load closure facts: %w", err)
	}
	if facts == nil {
		return false, fmt.Errorf("run %s not found", cand.RunID)
	}
	pendingS5 := false
	for _, a := range facts.S5Attempts {
		if a.State == "PENDING" {
			pendingS5 = true
			break
		}
	}
	if pendingS5 {
		for _, a := range facts.S5Attempts {
			if a.State != "PENDING" {
				continue
			}
			// 物化时已预分配 S5 共享 token:直接使用(旧 run 兜底新生成)。
			token := a.FencingToken
			if token == "" {
				token = repository.NewFencingToken()
			}
			ok, err := l.repo.MarkAttemptRunningAtomic(ctx, facts.TenantID, a.ID, token)
			if err != nil {
				return false, fmt.Errorf("gate S5 %s: %w", a.ExecutionNodeID, err)
			}
			if !ok {
				return false, fmt.Errorf("gate S5 %s CAS lost", a.ExecutionNodeID)
			}
			log.Info("S5 gated (authority-owned)", zap.String("node", a.ExecutionNodeID), zap.String("token", token[:8]))
		}
		// 重载:S5 已 RUNNING(带 token)
		facts, err = l.repo.LoadRunClosureFacts(ctx, cand.RunID)
		if err != nil {
			return false, fmt.Errorf("reload closure facts: %w", err)
		}
	}

	// 3. 对账(reconcile)。
	report := l.reconcile(facts)
	reconcileFence, err := json.Marshal(map[string]interface{}{
		"kind":             "reconcile_fence",
		"ok":               report.OK,
		"attempts_checked": report.AttemptsChecked,
		"differences":      report.Differences,
		"source_input":     report.SourceInput,
		"session_flows":    report.SessionFlows,
		"sessions":         report.Sessions,
		"detect_total":     report.DetectTotal,
	})
	if err != nil {
		return false, fmt.Errorf("marshal reconcile fence: %w", err)
	}

	// 4. RECONCILE 回执(自产,provider=analysis-service)。
	reconcileNode := s5Attempt(facts, "RECONCILE")
	if reconcileNode == nil {
		return false, fmt.Errorf("RECONCILE attempt missing")
	}
	reconcileNewState := "SUCCEEDED"
	if !report.OK {
		reconcileNewState = "FAILED"
	}
	if err := l.applyAuthorityReceipt(ctx, facts, reconcileNode, reconcileNewState,
		int64(report.AttemptsChecked), int64(report.AttemptsChecked-report.Differences),
		boolInt(!report.OK), reconcileFence); err != nil {
		return false, fmt.Errorf("apply RECONCILE receipt: %w", err)
	}
	log.Info("RECONCILE receipt applied", zap.Bool("ok", report.OK))

	// 5. 事实装配 + 真值表判定。
	closureFacts := l.assembleClosureFacts(facts, report)
	decision := state.EvaluateRunClosure(closureFacts)
	if !state.IsTerminal(decision.RunState) {
		// 事实不足不终局(如 S4 回执尚未回流);下一轮重试。
		log.Warn("closure not terminal; retry later",
			zap.String("run_state", string(decision.RunState)))
		return false, nil
	}

	// 6. MACHINE_FINALIZATION 回执(自产):携带五轴判定。
	finalizeNode := s5Attempt(facts, "MACHINE_FINALIZATION")
	if finalizeNode == nil {
		return false, fmt.Errorf("MACHINE_FINALIZATION attempt missing")
	}
	summaryFence, err := json.Marshal(map[string]interface{}{
		"kind":               "machine_finalization_fence",
		"run_state":          string(decision.RunState),
		"finding_conclusion": decision.FindingConclusion,
		"risk_severity":      decision.RiskSeverity,
		"completeness":       decision.Completeness,
		"integrity_state":    decision.IntegrityState,
	})
	if err != nil {
		return false, fmt.Errorf("marshal finalization fence: %w", err)
	}
	if err := l.applyAuthorityReceipt(ctx, facts, finalizeNode, "SUCCEEDED",
		1, 1, 0, summaryFence); err != nil {
		return false, fmt.Errorf("apply MACHINE_FINALIZATION receipt: %w", err)
	}
	log.Info("MACHINE_FINALIZATION receipt applied", zap.String("conclusion", decision.FindingConclusion))

	// 7. 三件套同事务 + 唯一终态。
	scope, keyFindings, limitations, evidence, decisionInputs, nodeSet, differences :=
		l.buildArtifacts(facts, closureFacts, decision, report)
	in := FinalizeInput{
		TenantID:        facts.TenantID,
		RunID:           facts.RunID,
		Facts:           closureFacts,
		ScopeJSON:       scope,
		KeyFindingsJSON: keyFindings,
		LimitationsJSON: limitations,
		EvidenceEntries: evidence,
		DecisionInputs:  decisionInputs,
		NodeExactSet:    nodeSet,
		Differences:     differences,
	}
	finalizer := NewFinalizerService(l.repo)
	if _, err := finalizer.Finalize(ctx, in); err != nil {
		return false, fmt.Errorf("finalize run: %w", err)
	}
	log.Info("run finalized",
		zap.String("state", string(decision.RunState)),
		zap.String("conclusion", decision.FindingConclusion))
	return true, nil
}

func s5Attempt(facts *repository.RunClosureFactsRow, node string) *repository.ClosureAttemptFact {
	for i := range facts.S5Attempts {
		if facts.S5Attempts[i].ExecutionNodeID == node {
			return &facts.S5Attempts[i]
		}
	}
	return nil
}

// applyAuthorityReceipt 权威自产回执(经与外部回执同一条不可变路径落库)。
func (l *FinalizeLoop) applyAuthorityReceipt(
	ctx context.Context,
	facts *repository.RunClosureFactsRow,
	attempt *repository.ClosureAttemptFact,
	newState string,
	inputCount, outputCount, errorCount int64,
	fence json.RawMessage,
) error {
	eventID := fmt.Sprintf("authority-receipt:%s:%s:%d", facts.RunID, attempt.ExecutionNodeID, attempt.Attempt)
	tuple := fmt.Sprintf("%s|%s|%d", facts.RunID, attempt.ExecutionNodeID, attempt.Attempt)
	_, err := l.repo.ApplyStageReceiptAtomic(ctx, repository.ReceiptCommand{
		TenantID:        facts.TenantID,
		RunID:           facts.RunID,
		EventID:         eventID,
		TupleHash:       tuple,
		ExecutionNodeID: attempt.ExecutionNodeID,
		Attempt:         attempt.Attempt,
		FencingToken:    attempt.FencingToken,
		Provider:        "analysis-service",
		InputCount:      inputCount,
		OutputCount:     outputCount,
		ErrorCount:      errorCount,
		RejectCount:     0,
		WatermarkMs:     0,
		FenceJSON:       fence,
		PayloadHash:     facts.ExecutionSpecSHA256,
		ExpectedState:   "RUNNING",
		NewState:        newState,
	})
	if err != nil {
		return err
	}
	return nil
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
