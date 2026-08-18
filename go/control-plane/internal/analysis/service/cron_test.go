package service

import (
	"testing"
	"time"
)

func TestParseCronAndNextFire(t *testing.T) {
	c, err := ParseCron("0 * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	base := time.Date(2026, 8, 17, 10, 5, 0, 0, time.UTC)
	next, err := c.NextFire(base)
	if err != nil || !next.Equal(time.Date(2026, 8, 17, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("hourly next fire: %v err=%v", next, err)
	}
	prev, err := c.PrevFireOrNow(base)
	if err != nil || !prev.Equal(time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("hourly prev fire: %v err=%v", prev, err)
	}
}

func TestParseCronStepsListsRanges(t *testing.T) {
	c, err := ParseCron("*/15 1,2 1-5 * 0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sun := time.Date(2026, 8, 23, 1, 30, 0, 0, time.UTC) // 周日 1:30 5 日
	if !c.matches(sun) {
		t.Fatalf("expected match for */15 1,2 1-5 * 0")
	}
	mon := time.Date(2026, 8, 24, 1, 30, 0, 0, time.UTC) // 周一
	if c.matches(mon) {
		t.Fatalf("dow=0 must not match Monday")
	}
}

func TestParseCronRejectsMalformed(t *testing.T) {
	for _, spec := range []string{"", "60 * * * *", "* 24 * * *", "* * * *", "* * * * * *", "a * * * *", "* * 32 * *", "* * * 13 *"} {
		if _, err := ParseCron(spec); err == nil {
			t.Fatalf("cron %q must be rejected", spec)
		}
	}
}

func TestEvaluateLateActivation(t *testing.T) {
	start := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	prepareAt := ComputePrepareAt(start, 300_000)
	if prepareAt.Equal(start.Add(-5 * time.Minute)) == false {
		t.Fatalf("prepare_at = start - lead, got %v", prepareAt)
	}
	// 提前于 prepare_at
	if d := EvaluateLateActivation(start.Add(-6*time.Minute), start, prepareAt, LateActivationFailWindow, false); d.Action != LateActivateNow {
		t.Fatalf("before prepare_at must hold: %+v", d)
	}
	// prepare 窗口内
	if d := EvaluateLateActivation(start.Add(-2*time.Minute), start, prepareAt, LateActivationFailWindow, false); d.Action != LateActivateNow {
		t.Fatalf("within prepare window must activate: %+v", d)
	}
	// 已迟到:FAIL_WINDOW
	if d := EvaluateLateActivation(start.Add(time.Minute), start, prepareAt, LateActivationFailWindow, false); d.Action != LateFailWindow {
		t.Fatalf("late + FAIL_WINDOW must fail: %+v", d)
	}
	// DELAY_WINDOW
	if d := EvaluateLateActivation(start.Add(time.Minute), start, prepareAt, LateActivationDelayWindow, false); d.Action != LateDelayWindow {
		t.Fatalf("late + DELAY_WINDOW must delay: %+v", d)
	}
	// BOUNDED_REPLAY 未证明回放输入 → fail closed
	if d := EvaluateLateActivation(start.Add(time.Minute), start, prepareAt, LateActivationBoundedReplay, false); d.Action != LateFailWindow {
		t.Fatalf("BOUNDED_REPLAY without proof must fail closed: %+v", d)
	}
	if d := EvaluateLateActivation(start.Add(time.Minute), start, prepareAt, LateActivationBoundedReplay, true); d.Action != LateReplayOnly {
		t.Fatalf("BOUNDED_REPLAY with proven input must replay: %+v", d)
	}
}
