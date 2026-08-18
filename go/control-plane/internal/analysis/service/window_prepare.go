// Package service 窗口前置准备与迟到激活(§76.45.4):
// prepare_at = window_start - prepare_lead_time;调度器在 prepare_at 前冻结
// Trigger/Task/Run/PlanReady/Reservation;window_start 前完成 PIPELINED provider
// 准备与 ACTIVE 订阅。迟到按 FAIL_WINDOW / DELAY_WINDOW / BOUNDED_REPLAY_IF_PROVEN
// 裁决(LIVE Router 不回填已错过数据,replay 必须冻结 offset/PCAP manifest)。
package service

import (
	"fmt"
	"time"
)

// LateActivationPolicy 迟到激活策略(§76.45.4)。
type LateActivationPolicy string

const (
	LateActivationFailWindow      LateActivationPolicy = "FAIL_WINDOW"
	LateActivationDelayWindow     LateActivationPolicy = "DELAY_WINDOW"
	LateActivationBoundedReplay   LateActivationPolicy = "BOUNDED_REPLAY_IF_PROVEN"
)

// LateActivationAction 裁决动作。
type LateActivationAction string

const (
	LateActivateNow      LateActivationAction = "ACTIVATE_NOW"    // 仍在 prepare 窗口内,正常激活
	LateFailWindow       LateActivationAction = "FAIL_WINDOW"     // 已错过,形成 FAILED/NOT_EVALUATED closure
	LateDelayWindow      LateActivationAction = "DELAY_WINDOW"    // 业务允许时生成新窗口 identity 延迟
	LateReplayOnly       LateActivationAction = "BOUNDED_REPLAY"  // 仅冻结 offset/PCAP 的 replay 路径
)

// LateActivationDecision 裁决结果。
type LateActivationDecision struct {
	Action LateActivationAction
	Reason string
}

// ComputePrepareAt prepare_at = window_start - prepare_lead_time(§76.45.4)。
func ComputePrepareAt(windowStart time.Time, prepareLeadMs int64) time.Time {
	if prepareLeadMs <= 0 {
		return windowStart
	}
	return windowStart.Add(-time.Duration(prepareLeadMs) * time.Millisecond)
}

// EvaluateLateActivation 迟到裁决(纯函数):
// now < window_start 且 ≥ prepare_at → 正常;now < prepare_at → 尚未到准备点;
// now ≥ window_start → 迟到,按策略裁决。BOUNDED_REPLAY 仅在 replayProven=true
// (冻结了 partition offset 范围/PCAP manifest)时允许,否则退回 FAIL_WINDOW。
func EvaluateLateActivation(now, windowStart, prepareAt time.Time, policy LateActivationPolicy, replayProven bool) LateActivationDecision {
	if now.Before(windowStart) {
		if now.Before(prepareAt) {
			return LateActivationDecision{Action: LateActivateNow, Reason: "before prepare_at; hold"}
		}
		return LateActivationDecision{Action: LateActivateNow, Reason: "within prepare window"}
	}
	switch policy {
	case LateActivationDelayWindow:
		return LateActivationDecision{Action: LateDelayWindow, Reason: "window missed; delay allowed by policy"}
	case LateActivationBoundedReplay:
		if replayProven {
			return LateActivationDecision{Action: LateReplayOnly, Reason: "replay input frozen (offsets/PCAP manifest)"}
		}
		return LateActivationDecision{Action: LateFailWindow, Reason: "BOUNDED_REPLAY without proven replay input; fail closed"}
	default:
		return LateActivationDecision{Action: LateFailWindow, Reason: fmt.Sprintf("window missed; policy=%s", policy)}
	}
}
