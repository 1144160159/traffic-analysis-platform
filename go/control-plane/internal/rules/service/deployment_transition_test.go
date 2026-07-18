package service

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestResumeDeploymentStatusRestoresPausedOrigin(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     model.DeploymentStatus
	}{
		{name: "gray origin", metadata: map[string]interface{}{"paused_from": "gray"}, want: model.DeploymentStatusGray},
		{name: "active origin", metadata: map[string]interface{}{"paused_from": "active"}, want: model.DeploymentStatusActive},
		{name: "legacy paused record", metadata: map[string]interface{}{}, want: model.DeploymentStatusActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resumeDeploymentStatus(tt.metadata); got != tt.want {
				t.Fatalf("resumeDeploymentStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
