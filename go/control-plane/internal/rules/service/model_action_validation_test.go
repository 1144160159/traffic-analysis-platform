package service

import (
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestValidateModelActionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *model.ModelActionRequest
		wantErr string
	}{
		{
			name: "feedback reference accepted",
			req: &model.ModelActionRequest{Action: "append-feedback-samples", Payload: map[string]interface{}{
				"dataset_id": "feedback-latest", "sample_count": float64(5231),
			}},
		},
		{
			name: "feedback inline samples rejected",
			req: &model.ModelActionRequest{Action: "append-feedback-samples", Payload: map[string]interface{}{
				"dataset_id": "feedback-latest", "sample_count": float64(1), "samples": []interface{}{"raw"},
			}},
			wantErr: "inline sample payloads are forbidden",
		},
		{
			name: "retrain requires strategy",
			req: &model.ModelActionRequest{Action: "request-retraining", Payload: map[string]interface{}{
				"dataset_id": "ds_ueba_latest", "reason": "drift",
			}},
			wantErr: "strategy must be incremental or full",
		},
		{
			name: "evaluation preserves version and dataset references",
			req: &model.ModelActionRequest{Action: "request-evaluation", Version: "v2", Payload: map[string]interface{}{
				"dataset_id": "validation-latest", "include_explanations": true,
			}},
		},
		{
			name:    "rollback requires reason",
			req:     &model.ModelActionRequest{Action: "rollback-version", Version: "v1", Payload: map[string]interface{}{}},
			wantErr: "rollback reason is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateModelActionRequest(test.req)
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}
