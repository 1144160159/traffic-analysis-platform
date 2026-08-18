// Package service Orchestrator 影子接线(§5.3):将权威事实装载为 OrchestratorInputs
// 并输出确定性决策。影子模式只读+日志,用于与现役双 loop 行为对照收集等价性证据;
// 证据收敛前不得切换到驱动写路径。
package service

import (
	"context"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// OrchestratorShadow 影子编排器:Load→Advance→Log;不执行任何写操作。
type OrchestratorShadow struct {
	repo         *repository.Repo
	orchestrator *Orchestrator
}

func NewOrchestratorShadow(repo *repository.Repo) *OrchestratorShadow {
	return &OrchestratorShadow{repo: repo, orchestrator: NewOrchestrator()}
}

// ShadowOne 对单个 run 做影子决策(只读)。
func (s *OrchestratorShadow) ShadowOne(ctx context.Context, runID string) (*AdvanceDecision, error) {
	facts, err := s.repo.LoadOrchestratorFacts(ctx, runID)
	if err != nil {
		return nil, err
	}
	inputs, err := OrchestratorInputsFromFacts(facts)
	if err != nil {
		return nil, err
	}
	return s.orchestrator.Advance(ctx, inputs)
}

// OrchestratorInputsFromFacts 事实快照 → 编排输入(纯映射)。
func OrchestratorInputsFromFacts(f *repository.OrchestratorFacts) (OrchestratorInputs, error) {
	inputs := OrchestratorInputs{
		RunState:            state.RunState(f.State),
		PlanReady:           f.PlanReady,
		AdmissionValid:      f.AdmissionValid,
		HasNodeLease:        f.HasNodeLease,
		SubscriptionActive:  f.SubscriptionActive,
		ReservationConsumed: f.ReservationConsumed,
		RunPreconditions: state.RunPreconditions{
			PlanReady:        f.PlanReady,
			AdmissionValid:   f.AdmissionValid && f.ReservationConsumed,
			HasNodeLease:     f.HasNodeLease,
			AllNodesTerminal: f.AllBusinessTerminal,
			ClosureCommitted: f.ClosureCommitted,
		},
	}
	for _, st := range f.Stages {
		inputs.Nodes = append(inputs.Nodes, NodeFact{
			ExecutionNodeID: st.ExecutionNodeID,
			BusinessPhaseID: st.BusinessPhaseID,
			ProviderMode:    st.ProviderMode,
			ActivationMode:  st.ActivationMode,
			Required:        true, // 核心主链 9 节点均为 required exact-set
			State:           state.StageAttemptState(st.State),
		})
	}
	if inputs.RunState == "" {
		return inputs, fmt.Errorf("run state empty")
	}
	return inputs, nil
}
