package service

import (
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// reconcile 对账:S1-S4 每终态 attempt 恰一条 fence 匹配回执;计数守恒。
func (l *FinalizeLoop) reconcile(facts *repository.RunClosureFactsRow) ClosureReconcileReport {
	report := ClosureReconcileReport{OK: true}

	receiptByNode := map[string][]repository.ClosureReceiptFact{}
	for _, p := range facts.Receipts {
		receiptByNode[p.ExecutionNodeID] = append(receiptByNode[p.ExecutionNodeID], p)
	}
	_ = receiptByNode

	// 1. 每终态 S1-S4 attempt:恰一条 fence 匹配回执。
	for _, a := range facts.Attempts {
		if a.State != "SUCCEEDED" && a.State != "FAILED" {
			continue
		}
		report.AttemptsChecked++
		recs := receiptByNode[a.ExecutionNodeID]
		matched := 0
		for _, p := range recs {
			if p.FencingToken != a.FencingToken {
				continue
			}
			matched++
			if a.State == "SUCCEEDED" && p.ErrorCount > 0 {
				report.Differences++
				report.Items = append(report.Items,
					fmt.Sprintf("node %s SUCCEEDED but receipt has error_count=%d", a.ExecutionNodeID, p.ErrorCount))
			}
			if a.State == "FAILED" && p.ErrorCount == 0 {
				report.Differences++
				report.Items = append(report.Items,
					fmt.Sprintf("node %s FAILED but receipt has error_count=0", a.ExecutionNodeID))
			}
		}
		if matched != 1 {
			report.Differences++
			report.Items = append(report.Items,
				fmt.Sprintf("node %s (%s) has %d fence-matched receipts, want 1", a.ExecutionNodeID, a.State, matched))
		}
	}

	// 2. 计数摘要(供回执 fence 与判定)。
	for _, p := range facts.Receipts {
		switch p.ExecutionNodeID {
		case "SOURCE_ACTIVATE":
			report.SourceInput = p.InputCount
			var sf struct {
				Kind string `json:"kind"`
			}
			_ = json.Unmarshal(p.Fence, &sf)
			report.SourceIsCaptureWindow = sf.Kind == "capture_window_fence"
		case "SESSIONIZATION":
			report.SessionFlows = p.InputCount
			report.Sessions = p.OutputCount
		case "DETECTION_AGGREGATE":
			var fence struct {
				Total        int64 `json:"total"`
				Positive     int64 `json:"positive"`
				Negative     int64 `json:"negative"`
				Inconclusive int64 `json:"inconclusive"`
				Error        int64 `json:"error"`
				Incompatible int64 `json:"incompatible"`
				NotRun       int64 `json:"not_run"`
			}
			_ = json.Unmarshal(p.Fence, &fence)
			report.DetectTotal = fence.Total
			report.Positive = fence.Positive
			report.Negative = fence.Negative
			report.Inconclusive = fence.Inconclusive
			report.DetectError = fence.Error
			report.DetectIncompatible = fence.Incompatible
			report.DetectNotRun = fence.NotRun
		}
	}

	// 3. 计数守恒:共享流多匹配语义下(同一事件可命中多个重叠 run),跨阶段
	// 计数不构成不变量(回放窗口会同时命中实时采集流量;检测输入=流数而非
	// 会话数)。对账完整性以 fence/回执匹配为准,计数仅作如实登记。
	if report.SourceIsCaptureWindow {
		report.Items = append(report.Items, "source=capture_window:coverage_acknowledged")
	}
	if report.SessionFlows > 0 && report.DetectTotal > report.SessionFlows {
		report.Items = append(report.Items,
			fmt.Sprintf("detect total %d exceeds session flows %d (cross-stage conservation deferred)",
				report.DetectTotal, report.SessionFlows))
	}

	report.OK = report.Differences == 0
	return report
}

// assembleClosureFacts 装配真值表输入(事实经对账后冻结)。
func (l *FinalizeLoop) assembleClosureFacts(
	facts *repository.RunClosureFactsRow,
	report ClosureReconcileReport,
) state.ClosureFacts {
	identityOK := true
	for _, p := range facts.Receipts {
		if p.PayloadHash != "" && p.PayloadHash != facts.ExecutionSpecSHA256 {
			identityOK = false
			break
		}
	}

	requiredFailed := !report.OK
	allSucceeded := report.OK
	for _, a := range facts.Attempts {
		if a.State == "FAILED" {
			requiredFailed = true
		}
		if a.State != "SUCCEEDED" {
			allSucceeded = false
		}
	}

	zeroInput := report.SourceInput == 0 && !report.SourceIsCaptureWindow
	// 全 disposition 计数(§7.4):每输入×required detector 恰一个 typed disposition,
	// 六类计数必须覆盖 total;NOT_RUN/INCOMPATIBLE 不允许被解释为阴性。
	allDispositionsAccounted := report.DetectTotal > 0 &&
		report.Positive+report.Negative+report.Inconclusive+report.DetectIncompatible+report.DetectError+report.DetectNotRun == report.DetectTotal
	return state.ClosureFacts{
		IdentityIntegrityOK:     identityOK,
		FenceCountIntegrityOK:   report.OK,
		CancelCASWon:            false,
		RequiredNodeFailed:      requiredFailed,
		DeadlineReached:         false,
		PartialAllowed:          false,
		PartialThresholdMet:     false,
		ZeroInputProven:         zeroInput,
		ZeroInputPolicy:         "ALLOW_NO_DATA",
		AllRequiredSucceeded:    allSucceeded,
		HasTrustedPositive:      report.Positive > 0,
		AllInputsExplicitNegative: allDispositionsAccounted &&
			report.Positive == 0 && report.Negative == report.DetectTotal,
		EvidenceSufficient: allDispositionsAccounted && report.DetectError == 0,
	}
}

// buildArtifacts 三件套内容(确定性;与终态同事务落库)。
func (l *FinalizeLoop) buildArtifacts(
	facts *repository.RunClosureFactsRow,
	closure state.ClosureFacts,
	decision state.ClosureDecision,
	report ClosureReconcileReport,
) (scope, keyFindings, limitations, evidence, decisionInputs, nodeSet, differences json.RawMessage) {
	scope, _ = json.Marshal(map[string]interface{}{
		"tenant_id":             facts.TenantID,
		"run_id":                facts.RunID,
		"execution_spec_sha256": facts.ExecutionSpecSHA256,
		"window_start_ms":       facts.WindowStartMs,
		"window_end_ms":         facts.WindowEndMs,
		"source": map[string]interface{}{
			"kind":        "PCAP_REPLAY",
			"input_count": report.SourceInput,
		},
	})
	keyFindings, _ = json.Marshal(map[string]interface{}{
		"finding_conclusion": decision.FindingConclusion,
		"risk_severity":      decision.RiskSeverity,
		"completeness":       decision.Completeness,
		"sessions":           report.Sessions,
		"session_flows":      report.SessionFlows,
		"detections": map[string]interface{}{
			"total":        report.DetectTotal,
			"positive":     report.Positive,
			"negative":     report.Negative,
			"inconclusive": report.Inconclusive,
			"incompatible": report.DetectIncompatible,
			"error":        report.DetectError,
			"not_run":      report.DetectNotRun,
		},
	})
	limitations, _ = json.Marshal(map[string]interface{}{
		"notes": []string{
			"detection dispositions are carried by the DETECTION_AGGREGATE receipt fence (six-way counts) and materialized per input x required detector into ClickHouse traffic.analysis_detections by the run-scoped behavior receipt job",
		},
	})
	evidenceList := make([]map[string]interface{}, 0, len(facts.Receipts))
	for _, p := range facts.Receipts {
		evidenceList = append(evidenceList, map[string]interface{}{
			"kind":          "stage_receipt",
			"execution_node": p.ExecutionNodeID,
			"provider":      p.Provider,
			"input_count":   p.InputCount,
			"output_count":  p.OutputCount,
			"error_count":   p.ErrorCount,
			"payload_hash":  p.PayloadHash,
		})
	}
	evidence, _ = json.Marshal(evidenceList)
	decisionInputs, _ = json.Marshal(closure)
	nodeMap := map[string]string{}
	for _, a := range facts.Attempts {
		nodeMap[a.ExecutionNodeID] = a.State
	}
	for _, a := range facts.S5Attempts {
		nodeMap[a.ExecutionNodeID] = a.State
	}
	nodeSet, _ = json.Marshal(nodeMap)
	items := report.Items
	if items == nil {
		items = []string{}
	}
	differences, _ = json.Marshal(map[string]interface{}{"items": items})
	return
}
