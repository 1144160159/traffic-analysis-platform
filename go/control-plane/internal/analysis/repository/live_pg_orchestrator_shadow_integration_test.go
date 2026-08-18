//go:build integration

// live PG 集成测试(§5.3 Orchestrator 影子接线):对真实非终态 run 装载事实并做
// 影子决策(只读):决策结构合法(有 wait 或 dispatchables/transition),不 panic;
// 状态映射与事实一致。切换写路径前以此类对照收集等价性证据。
package repository_test

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGOrchestratorShadowDecision(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()

	ids, err := repo.ListNonTerminalRunIDs(ctx, 20)
	if err != nil {
		t.Fatalf("list non-terminal: %v", err)
	}
	if len(ids) == 0 {
		t.Skip("no non-terminal runs for shadow decision")
	}
	shadow := service.NewOrchestratorShadow(repo)
	decided := 0
	for _, runID := range ids {
		facts, err := repo.LoadOrchestratorFacts(ctx, runID)
		if err != nil {
			t.Fatalf("facts %s: %v", runID, err)
		}
		inputs, err := service.OrchestratorInputsFromFacts(facts)
		if err != nil {
			t.Fatalf("inputs %s: %v", runID, err)
		}
		decision, err := shadow.ShadowOne(ctx, runID)
		if err != nil {
			t.Fatalf("shadow %s: %v", runID, err)
		}
		if decision == nil || (len(decision.Dispatchables) == 0 && decision.Wait == "" && decision.Transition == nil) {
			t.Fatalf("shadow %s: empty decision", runID)
		}
		// 收敛不变式:已确立事实不得回退为前置等待(等价性证据的强断言)。
		if facts.SubscriptionActive && facts.ReservationConsumed && facts.HasNodeLease {
			switch decision.Wait {
			case service.WaitPlanReady, service.WaitCapacity, service.WaitWindowStart:
				t.Fatalf("shadow %s: wait=%s contradicts established facts (active=%v consumed=%v lease=%v)",
					runID, decision.Wait, facts.SubscriptionActive, facts.ReservationConsumed, facts.HasNodeLease)
			}
		}
		t.Logf("[%s] state=%s planReady=%v active=%v consumed=%v nodes=%d dispatchables=%v wait=%s",
			runID[:8], facts.State, facts.PlanReady, facts.SubscriptionActive, facts.ReservationConsumed,
			len(inputs.Nodes), decision.Dispatchables, decision.Wait)
		decided++
	}
	t.Logf("orchestrator shadow PASS: %d/%d non-terminal runs decided", decided, len(ids))
}
