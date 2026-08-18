package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// ScheduleSource 调度源(仓储只读视图)。
type ScheduleSource interface {
	ListActiveSchedules(ctx context.Context, tenantID string) ([]repository.ScheduleRow, error)
}

// TriggerSink 触发登记与物化入队。
type TriggerSink interface {
	InsertTriggerInstance(ctx context.Context, tenantID, identityKind, canonicalHash, requestSHA, triggerKind, windowID, taskDefinitionID string, planRevision int64, actor, effectiveClass, resourceRestrictions string, scheduleRevision int64) (triggerID string, created bool, err error)
	EnqueueMaterialize(ctx context.Context, triggerID string) error
	HasActiveRunForDefinition(ctx context.Context, tenantID, taskDefinitionID string) (bool, error)
	SuppressTrigger(ctx context.Context, triggerID, reason string) (bool, error)
}

// Scheduler 窗口调度器:Tick 扫描 active schedules,按窗口对齐产生 TriggerInstance。
// 单实例经 InsertTriggerInstance 唯一约束幂等;物化由 MaterializeWorker 领取
// (EnqueueMaterialize 为兜底登记,worker 直接轮询 PENDING_MATERIALIZATION)。
type Scheduler struct {
	source ScheduleSource
	sink   TriggerSink
	now    func() time.Time
}

func NewScheduler(source ScheduleSource, sink TriggerSink) *Scheduler {
	return &Scheduler{source: source, sink: sink, now: time.Now}
}

// TickResult 单轮结果。
type TickResult struct {
	Triggered  int
	Misfires   int
	Skipped    int
	Suppressed int
}

// Tick 一轮调度:window 对齐 + prepare lead + misfire 策略(FAIL/DELAY/BOUNDED_REPLAY)。
func (s *Scheduler) Tick(ctx context.Context, tenantID string) (*TickResult, error) {
	schedules, err := s.source.ListActiveSchedules(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	result := &TickResult{}
	for _, sch := range schedules {
		if sch.TriggerKind == "ON_DEMAND" || sch.TriggerKind == "EVENT_DRIVEN" {
			result.Skipped++
			continue // 按需不走窗口调度;事件桥未发布,只登记跳过
		}
		window := parseWindow(sch.WindowOrCron, sch.PrepareLeadTimeMs, s.now())
		if sch.TriggerKind == "CRON_WINDOW" {
			window = parseCronWindow(sch.WindowOrCron, sch.PrepareLeadTimeMs, s.now())
		}
		if window.windowID == "" {
			result.Skipped++
			continue
		}
		// 窗口前置准备(§76.45.4):未到 prepare_at 不触发(冻结 Trigger/Task/Run 前
		// 必须先完成 PlanReady 与准入预留)。
		if s.now().Before(window.prepareAt) {
			result.Skipped++
			continue
		}

		// misfire:当前时刻已越过窗口终点(整个窗口已错过)
		if !s.now().Before(window.windowEnd) {
			switch sch.MisfirePolicy {
			case "MISFIRE_DELAY":
				window = window.shiftedTo(s.now())
			case "MISFIRE_BOUNDED_REPLAY":
				// 只允许 Kafka bounded replay/PCAP 路径,不得假装 LIVE 回填;
				// 该路径由 P3 的 PcapReplayAdapter 承接,此处登记 misfire。
				result.Misfires++
				continue
			default: // MISFIRE_FAIL
				result.Misfires++
				continue
			}
		}

		// FORBID_OVERLAP:定义下存在非终态 run → 触发事实转 SUPPRESSED(原因可查),
		// 不创建 Task(§76.45.4;SUPPRESSED ≠ 假 CANCELLED)。
		if sch.ConcurrencyPolicy == "FORBID_OVERLAP" {
			active, err := s.sink.HasActiveRunForDefinition(ctx, tenantID, sch.TaskDefinitionID)
			if err != nil {
				return nil, fmt.Errorf("overlap check: %w", err)
			}
			if active {
				canonicalHash := identityHash(tenantID, "schedule", sch.ScheduleID, window.windowID)
				requestSHA := identityHash(canonicalHash, sch.ExecutionSpecSHA256, window.windowID)
				triggerID, created, err := s.sink.InsertTriggerInstance(ctx, tenantID, "schedule", canonicalHash, requestSHA, sch.TriggerKind, window.windowID, sch.TaskDefinitionID, sch.ApprovedPlanRevision, "scheduler", classOrDefault(sch.SchedulingClass), sch.ResourceRestrictions, sch.Revision)
				if err != nil {
					return nil, fmt.Errorf("insert suppressed trigger: %w", err)
				}
				if created {
					if _, err := s.sink.SuppressTrigger(ctx, triggerID, "FORBID_OVERLAP_CONFLICT"); err != nil {
						return nil, fmt.Errorf("suppress overlapping trigger: %w", err)
					}
					result.Suppressed++
				}
				continue
			}
		}

		canonicalHash := identityHash(tenantID, "schedule", sch.ScheduleID, window.windowID)
		requestSHA := identityHash(canonicalHash, sch.ExecutionSpecSHA256, window.windowID)
		triggerID, created, err := s.sink.InsertTriggerInstance(ctx, tenantID, "schedule", canonicalHash, requestSHA, sch.TriggerKind, window.windowID, sch.TaskDefinitionID, sch.ApprovedPlanRevision, "scheduler", classOrDefault(sch.SchedulingClass), sch.ResourceRestrictions, sch.Revision)
		if err != nil {
			return nil, fmt.Errorf("insert trigger: %w", err)
		}
		if !created {
			continue // 同窗幂等
		}
		if err := s.sink.EnqueueMaterialize(ctx, triggerID); err != nil {
			return nil, fmt.Errorf("enqueue materialize: %w", err)
		}
		result.Triggered++
	}
	return result, nil
}

// classOrDefault 调度类别缺省(有效策略解析步骤 1 的 schedule 层输入)。
func classOrDefault(class string) string {
	if class == "" {
		return "BASELINE"
	}
	return class
}

type windowSpec struct {
	windowID    string
	windowStart time.Time
	windowEnd   time.Time
	prepareAt   time.Time
	duration    time.Duration
}

func (w windowSpec) shiftedTo(t time.Time) windowSpec {
	w.windowStart = t
	w.windowEnd = t.Add(w.duration)
	w.prepareAt = w.windowStart.Add(-w.duration / 2) // 简化:lead 取半个窗口
	return w
}

// parseCronWindow CRON_WINDOW 窗口定义:{"cron":".. .. .. .. ..","window_ms":N}。
// fire = 最近一次(含 now)的 cron 触发时刻;窗口 = [fire, fire+window_ms);
// windowID 编码 c-{fire_ms}-{window_ms}(同 fire 幂等)。
func parseCronWindow(raw []byte, leadMs int64, now time.Time) windowSpec {
	if len(raw) == 0 {
		return windowSpec{}
	}
	var def struct {
		Cron     string `json:"cron"`
		WindowMS int64  `json:"window_ms"`
	}
	if err := jsonUnmarshal(raw, &def); err != nil || def.Cron == "" {
		return windowSpec{}
	}
	if def.WindowMS <= 0 {
		def.WindowMS = 60000
	}
	schedule, err := ParseCron(def.Cron)
	if err != nil {
		return windowSpec{} // fail-closed:非法 cron 不触发
	}
	fire, err := schedule.PrevFireOrNow(now)
	if err != nil {
		return windowSpec{}
	}
	d := time.Duration(def.WindowMS) * time.Millisecond
	lead := time.Duration(leadMs) * time.Millisecond
	return windowSpec{
		windowID:    fmt.Sprintf("c-%d-%d", fire.UnixMilli(), def.WindowMS),
		windowStart: fire,
		windowEnd:   fire.Add(d),
		prepareAt:   fire.Add(-lead),
		duration:    d,
	}
}

// parseWindow 解析窗口定义:{"window_ms":N} 对齐滚动窗口;
// {"start_ms":S,"window_ms":N} 显式有界窗口(PCAP_REPLAY/确定性窗口);cron 不在核心卷。
func parseWindow(raw []byte, leadMs int64, now time.Time) windowSpec {
	if len(raw) == 0 {
		return windowSpec{}
	}
	var def struct {
		WindowMS  int64  `json:"window_ms"`
		StartMS   int64  `json:"start_ms"`
		Alignment string `json:"alignment"`
		Cron      string `json:"cron"`
	}
	if err := jsonUnmarshal(raw, &def); err != nil {
		return windowSpec{}
	}
	if def.Cron != "" {
		return windowSpec{} // cron 支撑不在核心卷
	}
	if def.WindowMS <= 0 {
		def.WindowMS = 60000
	}
	d := time.Duration(def.WindowMS) * time.Millisecond
	lead := time.Duration(leadMs) * time.Millisecond
	if def.StartMS > 0 {
		start := time.UnixMilli(def.StartMS)
		windowID := fmt.Sprintf("w-%d-%d-%d", def.StartMS, def.WindowMS, leadMs)
		return windowSpec{
			windowID:    windowID,
			windowStart: start,
			windowEnd:   start.Add(d),
			prepareAt:   start.Add(-lead),
			duration:    d,
		}
	}
	// unix 对齐滚动窗口
	start := now.Truncate(d)
	windowID := fmt.Sprintf("w-%d-%d", start.UnixMilli(), def.WindowMS)
	return windowSpec{
		windowID:    windowID,
		windowStart: start,
		windowEnd:   start.Add(d),
		prepareAt:   start.Add(-lead),
		duration:    d,
	}
}

// jsonUnmarshal 轻量 JSON 解析。
func jsonUnmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

var _ = contract.ErrCodeWindowMisfired
