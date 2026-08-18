package baseline

import (
	"errors"
	"testing"
	"time"
)

func TestBuildRequestSeparatesDynamicAndStaticSampleSemantics(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	dynamic := validBuildRequest()
	dynamic.WindowStart, dynamic.WindowEnd = &start, &end
	dynamic.MinimumEligibleRows = 100
	if err := dynamic.Validate(); err != nil {
		t.Fatalf("valid dynamic build rejected: %v", err)
	}
	if dynamic.SamplePolicy["minimum_eligible_rows"] != int64(100) {
		t.Fatalf("minimum sample was not frozen into policy: %#v", dynamic.SamplePolicy)
	}
	static := validBuildRequest()
	static.BaselineKind = "static"
	static.WindowStart, static.WindowEnd = &start, &end
	if err := static.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("static baseline accepted a learned window: %v", err)
	}
}

func TestDynamicSampleRejectsCompleteWithPartialReasons(t *testing.T) {
	result := DynamicSampleResult{
		TenantID: "tenant-a", JobID: "job-a",
		CandidateSHA256: repeatHex("a"), RowCount: 10, EligibleRowCount: 10,
		QualityStatus: "complete", PartialReasons: []string{"not_arrived"},
		SourceWatermark: map[string]interface{}{"offset": 1}, SourceQuerySHA256: repeatHex("b"),
		Statistics: map[string]interface{}{}, Provenance: map[string]interface{}{}, CompletedBy: "worker-a",
	}
	if err := result.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("complete sample accepted a partial reason: %v", err)
	}
}

func TestEvaluationRequestRejectsEvidenceThatBecomesEmptyAndEventOutsideWindow(t *testing.T) {
	observed := time.Unix(500, 0).UTC()
	start, end := observed.Add(-time.Hour), observed.Add(-time.Minute)
	request := EvaluationRequest{TenantID: "tenant-a", BaselineID: "asset:asset-a", MetricName: "bytes",
		ObservedValue: 1, ObservedAt: observed, WindowStart: &start, WindowEnd: &end,
		EvidenceRefs: []string{"event-a"}, TraceID: "trace-a"}
	if err := request.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("evaluation outside its closed window was accepted: %v", err)
	}
	request.WindowStart, request.WindowEnd = nil, nil
	request.EvidenceRefs = []string{"  "}
	if err := request.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("blank-only evidence set was accepted: %v", err)
	}
}

func validBuildRequest() BuildRequest {
	return BuildRequest{
		TenantID: "tenant-a", BaselineID: "asset:asset-a", BaselineKind: "dynamic",
		EntityType: "asset", EntityID: "asset-a", ExpectedRevision: 1,
		AlgorithmVersion: "behavior-zscore-v1", SamplePolicy: map[string]interface{}{"max_active_age_seconds": 86400.0},
		ThresholdSpec:     map[string]interface{}{"warning": 2.0, "alert": 3.0},
		ExpectedConsumers: []string{"flink-behavior-v1"}, CandidateSHA256: repeatHex("c"),
		IdempotencyKey: "build-a", RequestedBy: "user-a", Reason: "rebuild", TraceID: "trace-a",
	}
}

func repeatHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
