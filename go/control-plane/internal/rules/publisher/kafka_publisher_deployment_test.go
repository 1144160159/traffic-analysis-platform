package publisher

import (
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestBuildDeploymentEventPayloadV1Contract(t *testing.T) {
	occurredAt := time.UnixMilli(1720000000123).UTC()
	deployment := &model.Deployment{
		DeploymentID: "deployment-1",
		TenantID:     "tenant-1",
		RuleVersion:  "rule-v1",
		Scope:        map[string]interface{}{"percentage": 20},
		Status:       "gray",
	}
	payload := buildDeploymentEventPayload(deployment, "gray_started", "operator-1", "event-1", occurredAt)
	for _, key := range []string{"event_id", "schema_version", "event_type", "action", "deployment_id", "tenant_id", "scope", "status", "operator_id", "timestamp"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("deployment event v1 missing %s", key)
		}
	}
	if payload["schema_version"] != 1 || payload["event_type"] != "deployment_event" {
		t.Fatalf("unexpected deployment event contract: %#v", payload)
	}
	if payload["timestamp"] != occurredAt.UnixMilli() {
		t.Fatalf("timestamp = %v, want %d", payload["timestamp"], occurredAt.UnixMilli())
	}
}
