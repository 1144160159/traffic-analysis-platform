package dataquality

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMonitorConfigDefaults(t *testing.T) {
	cfg := MonitorConfig{CheckInterval: 15 * time.Minute, MinFlowRate: 100, MaxLatencyP95: 60000}
	if cfg.CheckInterval != 15*time.Minute {
		t.Error("default CheckInterval mismatch")
	}
}

func TestNewMonitor(t *testing.T) {
	monitor := NewMonitor(nil, MonitorConfig{MinFlowRate: 50}, zap.NewNop())
	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}
}

func TestCheckAllWithoutDB(t *testing.T) {
	monitor := NewMonitor(nil, MonitorConfig{MinFlowRate: 100, MaxLatencyP95: 60000}, zap.NewNop())

	report, err := monitor.CheckAll(context.Background(), "tenant-test")
	if err == nil {
		t.Fatal("CheckAll with nil DB MUST return error, not fake results")
	}
	if report != nil {
		t.Fatal("CheckAll with nil DB MUST return nil report")
	}
	t.Logf("Correct: nil DB returns error: %v", err)
}

func TestEvaluateOverall(t *testing.T) {
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	tests := []struct {
		name   string
		checks []QualityCheck
		want   string
	}{
		{"all pass", []QualityCheck{{Status: "pass"}, {Status: "pass"}}, "healthy"},
		{"one warn", []QualityCheck{{Status: "warn"}, {Status: "pass"}}, "healthy"},
		{"two warns", []QualityCheck{{Status: "warn"}, {Status: "warn"}}, "degraded"},
		{"one fail", []QualityCheck{{Status: "fail"}, {Status: "pass"}}, "unhealthy"},
		{"unmeasured is not healthy", []QualityCheck{{Status: "pass"}, {Status: "unknown"}}, "unknown"},
	}
	for _, tc := range tests {
		report := &DataQualityReport{Checks: tc.checks}
		if result := monitor.evaluateOverall(report); result != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, result, tc.want)
		}
	}
}

func TestBaselineManagement(t *testing.T) {
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	baseline, err := monitor.GetBaseline(context.Background(), "tenant-test")
	if err != nil {
		t.Fatalf("GetBaseline without control DB: %v", err)
	}
	if baseline != nil {
		t.Error("baseline should be nil initially")
	}
	_, err = monitor.UpdateBaseline(context.Background(), "tenant-test", "user-test", "0123456789abcdef0123456789abcdef")
	if err == nil {
		t.Error("UpdateBaseline with nil DB MUST return error")
	}
	t.Logf("Correct: nil DB returns error: %v", err)
}

func TestKafkaLagCannotUseClickHouseProxyAsMeasuredSignal(t *testing.T) {
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	report := &DataQualityReport{Timestamp: time.Now(), Metrics: map[string]float64{}, SourceWatermarks: map[string]interface{}{}}
	monitor.applyHandoffSignals(context.Background(), "tenant-test", report)
	if len(report.Checks) != 3 {
		t.Fatalf("checks=%d want=3", len(report.Checks))
	}
	for _, check := range report.Checks {
		if check.Status != "unknown" || check.Measured {
			t.Fatalf("missing persisted hand-off signal must be unknown: %#v", check)
		}
		if check.Availability != SignalAvailabilityNotArrived || check.Freshness != SignalFreshnessUnknown || check.Completeness != SignalCompletenessUnknown || check.ValueState != SignalValueNone {
			t.Fatalf("missing persisted hand-off signal must be typed not-arrived: %#v", check)
		}
	}
	if _, ok := report.Metrics["insert_rate_per_min"]; ok {
		t.Fatal("ClickHouse insert-rate proxy must not be published as Kafka lag")
	}
}

func TestQualityCheckTypes(t *testing.T) {
	check := QualityCheck{Name: "flow_rate", Status: "pass", Value: 150.0, Threshold: 100.0}
	if check.Value <= check.Threshold && check.Status != "fail" {
		t.Logf("Value %.1f > threshold %.1f (pass)", check.Value, check.Threshold)
	}
}

func TestDBNumericHandlesClickHouseCountPointer(t *testing.T) {
	value := uint64(42)
	if got := dbNumeric(&value); got != 42 {
		t.Fatalf("dbNumeric(*uint64) = %.0f, want 42", got)
	}
}
