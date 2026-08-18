package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// FinalizerService 机器终局:Reconcile 事实 + EvaluateRunClosure → 三件套同事务 → 唯一终态。
type FinalizerService struct {
	repo *repository.Repo
}

func NewFinalizerService(repo *repository.Repo) *FinalizerService { return &FinalizerService{repo: repo} }

// FinalizeInput 终局输入(冻结事实)。
type FinalizeInput struct {
	TenantID string
	RunID    string
	Facts    state.ClosureFacts
	// 摘要内容
	ScopeJSON       json.RawMessage
	KeyFindingsJSON json.RawMessage
	LimitationsJSON json.RawMessage
	EvidenceEntries json.RawMessage
	DecisionInputs  json.RawMessage
	NodeExactSet    json.RawMessage
	Differences     json.RawMessage
}

// Finalize 判定+提交。Run 终态由真值表唯一决定。
func (f *FinalizerService) Finalize(ctx context.Context, in FinalizeInput) (*state.ClosureDecision, error) {
	decision := state.EvaluateRunClosure(in.Facts)
	if !state.IsTerminal(decision.RunState) {
		return nil, fmt.Errorf("closure facts do not produce a terminal state")
	}

	expected := "FINALIZING"
	if in.Facts.CancelCASWon {
		expected = "CANCEL_REQUESTED"
	}

	summaryHash := identityHash(in.RunID, string(in.ScopeJSON), string(in.KeyFindingsJSON))
	closureHash := identityHash(in.RunID, string(decision.RunState), string(in.DecisionInputs))
	evidenceHash := identityHash(in.RunID, string(in.EvidenceEntries))

	if err := f.repo.FinalizeRunAtomic(ctx, repository.FinalizeCommand{
		TenantID:            in.TenantID,
		RunID:               in.RunID,
		ExpectedState:       expected,
		NewState:            string(decision.RunState),
		FindingConclusion:   decision.FindingConclusion,
		RiskSeverity:        decision.RiskSeverity,
		Completeness:        decision.Completeness,
		IntegrityState:      decision.IntegrityState,
		ScopeJSON:           in.ScopeJSON,
		KeyFindingsJSON:     in.KeyFindingsJSON,
		LimitationsJSON:     in.LimitationsJSON,
		EvidenceEntriesJSON: in.EvidenceEntries,
		DecisionInputsJSON:  in.DecisionInputs,
		NodeExactSetJSON:    in.NodeExactSet,
		DifferencesJSON:     in.Differences,
		Priority:            0,
		SummarySHA256:       summaryHash,
		ClosureSHA256:       closureHash,
		EvidenceSHA256:      evidenceHash,
	}); err != nil {
		return nil, fmt.Errorf("finalize run: %w", err)
	}
	return &decision, nil
}

// CancelService 取消(请求→逐 attempt→closure)。
type CancelService struct {
	repo *repository.Repo
}

func NewCancelService(repo *repository.Repo) *CancelService { return &CancelService{repo: repo} }

// RequestCancel 进入 CANCEL_REQUESTED(仅冻结 CancelTargetManifest 的目标;终态先胜由 CAS 保证)。
func (c *CancelService) RequestCancel(ctx context.Context, tenantID, runID string) error {
	// 终态不可取消:非终态 CAS 到 CANCEL_REQUESTED
	if err := c.repo.TransitionRunAtomic(ctx, tenantID, runID, "ACCEPTED", "CANCEL_REQUESTED"); err == nil {
		return nil
	}
	if err := c.repo.TransitionRunAtomic(ctx, tenantID, runID, "PREPARING", "CANCEL_REQUESTED"); err == nil {
		return nil
	}
	if err := c.repo.TransitionRunAtomic(ctx, tenantID, runID, "QUEUED", "CANCEL_REQUESTED"); err == nil {
		return nil
	}
	if err := c.repo.TransitionRunAtomic(ctx, tenantID, runID, "RUNNING", "CANCEL_REQUESTED"); err == nil {
		return nil
	}
	if err := c.repo.TransitionRunAtomic(ctx, tenantID, runID, "FINALIZING", "CANCEL_REQUESTED"); err == nil {
		return nil
	}
	return fmt.Errorf("run is not cancelable (terminal or unknown state)")
}

// FinalizeCancel 取消终局:取消型三件套 + CANCELLED(终态先胜由 FinalizeRunAtomic CAS 保证)。
func (c *CancelService) FinalizeCancel(ctx context.Context, in FinalizeInput) error {
	in.Facts.CancelCASWon = true
	in.Facts.IdentityIntegrityOK = true
	in.Facts.FenceCountIntegrityOK = true
	decision := state.EvaluateRunClosure(in.Facts)
	if decision.RunState != state.RunCancelled {
		return fmt.Errorf("cancel closure did not yield CANCELLED")
	}
	summaryHash := identityHash(in.RunID, "cancel", string(in.ScopeJSON))
	closureHash := identityHash(in.RunID, "cancel", string(in.DecisionInputs))
	evidenceHash := identityHash(in.RunID, string(in.EvidenceEntries))
	return c.repo.FinalizeRunAtomic(ctx, repository.FinalizeCommand{
		TenantID:            in.TenantID,
		RunID:               in.RunID,
		ExpectedState:       "CANCEL_REQUESTED",
		NewState:            "CANCELLED",
		FindingConclusion:   decision.FindingConclusion,
		RiskSeverity:        decision.RiskSeverity,
		Completeness:        decision.Completeness,
		IntegrityState:      decision.IntegrityState,
		ScopeJSON:           in.ScopeJSON,
		KeyFindingsJSON:     in.KeyFindingsJSON,
		LimitationsJSON:     in.LimitationsJSON,
		EvidenceEntriesJSON: in.EvidenceEntries,
		DecisionInputsJSON:  in.DecisionInputs,
		NodeExactSetJSON:    in.NodeExactSet,
		DifferencesJSON:     in.Differences,
		Priority:            0,
		SummarySHA256:       summaryHash,
		ClosureSHA256:       closureHash,
		EvidenceSHA256:      evidenceHash,
	})
}
