package service

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// stubAllowedReader 只读端口桩。
type stubAllowedReader struct {
	run    *repository.RunView
	stages []repository.RunStageView
	sum    *repository.ReportSummaryRef
	def    *repository.TaskDefinitionDetail
}

func (s *stubAllowedReader) GetRun(_ context.Context, _, _ string) (*repository.RunView, error) {
	return s.run, nil
}
func (s *stubAllowedReader) ListRunStages(_ context.Context, _, _ string) ([]repository.RunStageView, error) {
	return s.stages, nil
}
func (s *stubAllowedReader) GetRunSummaryHash(_ context.Context, _, _ string) (*repository.ReportSummaryRef, error) {
	return s.sum, nil
}
func (s *stubAllowedReader) GetTaskDefinitionDetail(_ context.Context, _, _ string) (*repository.TaskDefinitionDetail, error) {
	return s.def, nil
}

// §20/§21 allowedActions 服务端驱动判定表。
func TestAllowedActionsForRun(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		run  repository.RunView
		stgs []repository.RunStageView
		sum  repository.ReportSummaryRef
		want repository.RunView // 用 State 复述期望的布尔集合
		exp  map[string]bool
	}{
		{
			name: "running with failed attempt",
			run:  repository.RunView{RunID: "r1", State: "RUNNING"},
			stgs: []repository.RunStageView{{ExecutionNodeID: "S1", State: "FAILED"}},
			sum:  repository.ReportSummaryRef{},
			exp:  map[string]bool{"cancel": true, "retry_stage": true, "retry_task": false, "request_report": false},
		},
		{
			name: "running without failed attempt",
			run:  repository.RunView{RunID: "r2", State: "QUEUED"},
			stgs: []repository.RunStageView{{ExecutionNodeID: "S1", State: "RUNNING"}},
			sum:  repository.ReportSummaryRef{},
			exp:  map[string]bool{"cancel": true, "retry_stage": false, "retry_task": false, "request_report": false},
		},
		{
			name: "cancel requested",
			run:  repository.RunView{RunID: "r3", State: "CANCEL_REQUESTED"},
			sum:  repository.ReportSummaryRef{},
			exp:  map[string]bool{"cancel": false, "retry_stage": false, "retry_task": false, "request_report": false},
		},
		{
			name: "succeeded with summary, no report",
			run:  repository.RunView{RunID: "r4", State: "SUCCEEDED", ReportState: "NOT_REQUESTED"},
			sum:  repository.ReportSummaryRef{SummarySHA256: "sha-1", SummaryExists: true},
			exp:  map[string]bool{"cancel": false, "retry_stage": false, "retry_task": true, "request_report": true},
		},
		{
			name: "succeeded with available report",
			run:  repository.RunView{RunID: "r5", State: "SUCCEEDED", ReportState: "AVAILABLE"},
			sum:  repository.ReportSummaryRef{SummarySHA256: "sha-1", SummaryExists: true},
			exp:  map[string]bool{"cancel": false, "retry_stage": false, "retry_task": true, "request_report": false},
		},
		{
			name: "failed without summary",
			run:  repository.RunView{RunID: "r6", State: "FAILED", ReportState: "NOT_REQUESTED"},
			sum:  repository.ReportSummaryRef{SummaryExists: false},
			exp:  map[string]bool{"cancel": false, "retry_stage": false, "retry_task": true, "request_report": false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewAllowedActionsService(&stubAllowedReader{run: &tc.run, stages: tc.stgs, sum: &tc.sum})
			got, err := svc.ForRun(ctx, "default", tc.run.RunID)
			if err != nil {
				t.Fatalf("ForRun: %v", err)
			}
			gotMap := map[string]bool{"cancel": got.Cancel, "retry_stage": got.RetryStage, "retry_task": got.RetryTask, "request_report": got.RequestReport}
			for k, want := range tc.exp {
				if gotMap[k] != want {
					t.Fatalf("%s: want %v got %v", k, want, gotMap[k])
				}
			}
		})
	}
}

func TestAllowedActionsForDefinition(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		state string
		act   bool
		sus   bool
	}{
		{"DRAFT", true, false},
		{"ACTIVE", false, true},
		{"SUSPENDED", false, false},
	}
	for _, tc := range cases {
		svc := NewAllowedActionsService(&stubAllowedReader{def: &repository.TaskDefinitionDetail{ID: "d1", State: tc.state, Revision: 3}})
		got, err := svc.ForDefinition(ctx, "default", "d1")
		if err != nil {
			t.Fatalf("ForDefinition: %v", err)
		}
		if got.Activate != tc.act || got.Suspend != tc.sus {
			t.Fatalf("state %s: want act=%v sus=%v got act=%v sus=%v", tc.state, tc.act, tc.sus, got.Activate, got.Suspend)
		}
	}
}
