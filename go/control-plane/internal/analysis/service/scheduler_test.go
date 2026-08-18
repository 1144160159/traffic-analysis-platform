package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

type fakeScheduleSource struct{ rows []repository.ScheduleRow }
func (f *fakeScheduleSource) ListActiveSchedules(context.Context, string) ([]repository.ScheduleRow, error) {
	return f.rows, nil
}

type fakeTriggerSink struct {
	triggers   []string
	enqueued   []string
	suppressed []string
	activeRun  bool
}
func (f *fakeTriggerSink) InsertTriggerInstance(_ context.Context, _ string, _ string, canonicalHash, requestSHA, triggerKind, windowID, _ string, _ int64, _ string, _ string, _ string, _ int64) (string, bool, error) {
	f.triggers = append(f.triggers, windowID)
	return "t-" + windowID, true, nil
}
func (f *fakeTriggerSink) EnqueueMaterialize(_ context.Context, triggerID string) error {
	f.enqueued = append(f.enqueued, triggerID)
	return nil
}
func (f *fakeTriggerSink) HasActiveRunForDefinition(context.Context, string, string) (bool, error) {
	return f.activeRun, nil
}
func (f *fakeTriggerSink) SuppressTrigger(_ context.Context, triggerID, reason string) (bool, error) {
	f.suppressed = append(f.suppressed, reason)
	return true, nil
}

func TestSchedulerTickFixedWindow(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000) // 任意固定时刻
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-1", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 1,
		ApprovedPlanRevision: 1, ExecutionSpecSHA256: "spec-1",
		TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron: []byte(`{"window_ms":60000}`), PrepareLeadTimeMs: 5000,
		MisfirePolicy: "MISFIRE_FAIL", SchedulingClass: "BASELINE",
	}}}
	sink := &fakeTriggerSink{}
	s := NewScheduler(src, sink)
	s.now = func() time.Time { return now }

	result, err := s.Tick(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Triggered != 1 || len(sink.enqueued) != 1 {
		t.Fatalf("expected 1 trigger: %+v triggers=%v", result, sink.triggers)
	}
}

func TestSchedulerTickMisfireFail(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	// 显式有界窗口 start_ms 在 now 之前且已越过终点 → MISFIRE_FAIL 不触发
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-2", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 1,
		ExecutionSpecSHA256: "spec-1", TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron: []byte(`{"start_ms":1799999000000,"window_ms":500}`), PrepareLeadTimeMs: 100,
		MisfirePolicy: "MISFIRE_FAIL", SchedulingClass: "BASELINE",
	}}}
	sink := &fakeTriggerSink{}
	s := NewScheduler(src, sink)
	s.now = func() time.Time { return now }

	result, _ := s.Tick(context.Background(), "tenant-a")
	if result.Triggered != 0 || result.Misfires != 1 {
		t.Fatalf("misfired window must not trigger (FAIL): %+v", result)
	}
}

func TestSchedulerTickMisfireDelayShifts(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-2b", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 1,
		ExecutionSpecSHA256: "spec-1", TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron: []byte(`{"start_ms":1799999000000,"window_ms":500}`), PrepareLeadTimeMs: 100,
		MisfirePolicy: "MISFIRE_DELAY", SchedulingClass: "BASELINE",
	}}}
	sink := &fakeTriggerSink{}
	s := NewScheduler(src, sink)
	s.now = func() time.Time { return now }

	result, _ := s.Tick(context.Background(), "tenant-a")
	if result.Triggered != 1 || result.Misfires != 0 {
		t.Fatalf("MISFIRE_DELAY must shift and trigger: %+v", result)
	}
}

func TestSchedulerTickSkipsOnDemand(t *testing.T) {
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-3", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 1,
		ExecutionSpecSHA256: "spec-1", TriggerKind: "ON_DEMAND", Timezone: "UTC",
		WindowOrCron: []byte(`{"window_ms":60000}`),
	}}}
	sink := &fakeTriggerSink{}
	s := NewScheduler(src, sink)
	result, _ := s.Tick(context.Background(), "tenant-a")
	if result.Triggered != 0 || result.Skipped != 1 {
		t.Fatalf("on-demand must be skipped by scheduler: %+v", result)
	}
}

func TestSchedulerTickCronWindowTriggers(t *testing.T) {
	// 整点 cron:now 落在 [fire, fire+10min) → 触发(windowID=c-{fire}-{window_ms})。
	now := time.UnixMilli(1_800_000_180_000) // 10:03:00(整点 10:00 之后 3 分钟)
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-4", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 1,
		ApprovedPlanRevision: 1, ExecutionSpecSHA256: "spec-1", TriggerKind: "CRON_WINDOW", Timezone: "UTC",
		WindowOrCron: []byte(`{"cron":"0 * * * *","window_ms":600000}`), PrepareLeadTimeMs: 5000,
		MisfirePolicy: "MISFIRE_FAIL", SchedulingClass: "BASELINE",
	}}}
	sink := &fakeTriggerSink{}
	s := NewScheduler(src, sink)
	s.now = func() time.Time { return now }
	result, err := s.Tick(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Triggered != 1 || len(sink.triggers) != 1 {
		t.Fatalf("cron window must trigger once: %+v triggers=%v", result, sink.triggers)
	}
	if !strings.HasPrefix(sink.triggers[0], "c-") {
		t.Fatalf("cron window id must carry fire timestamp: %q", sink.triggers[0])
	}
}

func TestSchedulerTickForbidOverlapSuppresses(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	src := &fakeScheduleSource{rows: []repository.ScheduleRow{{
		ScheduleID: "s-5", TenantID: "tenant-a", TaskDefinitionID: "def-1", Revision: 2,
		ApprovedPlanRevision: 1, ExecutionSpecSHA256: "spec-1",
		TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron: []byte(`{"window_ms":60000}`), PrepareLeadTimeMs: 5000,
		MisfirePolicy: "MISFIRE_FAIL", ConcurrencyPolicy: "FORBID_OVERLAP", SchedulingClass: "BASELINE",
	}}}
	sink := &fakeTriggerSink{activeRun: true}
	s := NewScheduler(src, sink)
	s.now = func() time.Time { return now }
	result, err := s.Tick(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Triggered != 0 || result.Suppressed != 1 {
		t.Fatalf("FORBID_OVERLAP with active run must suppress without task: %+v", result)
	}
	if len(sink.suppressed) != 1 || sink.suppressed[0] != "FORBID_OVERLAP_CONFLICT" {
		t.Fatalf("suppress reason mismatch: %v", sink.suppressed)
	}
	if len(sink.enqueued) != 0 {
		t.Fatalf("suppressed trigger must not enqueue materialize: %v", sink.enqueued)
	}
}
