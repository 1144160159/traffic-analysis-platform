package consumer

import (
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestDetectionBatchPreservesNotificationRoutingContract(t *testing.T) {
	firstSeen := time.Now().Add(-20 * time.Minute).Truncate(time.Millisecond)
	lastSeen := time.Now().Truncate(time.Millisecond)
	detection := &pb.DetectionBatch{Behaviors: []*pb.DetectionBehavior{{
		Header:     &pb.EventHeader{TenantId: "default", EventId: "event-1", EventTs: lastSeen.UnixMilli()},
		ObjectType: "server", ObjectId: "SRV-0007", TopLabel: "port_scan", TopScore: 0.93,
		Labels: []string{"port_scan", "asset_scope:核心资产", "campus:主园区"},
	}}}
	alert := (&Consumer{logger: zap.NewNop()}).buildAlert(detection, "fingerprint-1", &dedup.DedupResult{IsNew: false, Count: 3, FirstSeen: firstSeen.UnixMilli(), LastSeen: lastSeen.UnixMilli()})
	if alert.AlertType != "攻击告警" || alert.Severity != "critical" {
		t.Fatalf("normalized fields type=%q severity=%q", alert.AlertType, alert.Severity)
	}
	if !alert.FirstSeen.Equal(firstSeen) || !alert.LastSeen.Equal(lastSeen) {
		t.Fatalf("dedup times first=%s last=%s", alert.FirstSeen, alert.LastSeen)
	}
	for _, expected := range []string{"object_type:server", "object_id:SRV-0007", "asset_scope:核心资产", "campus:主园区"} {
		if !slices.Contains(alert.Labels, expected) {
			t.Fatalf("missing routing label %q in %#v", expected, alert.Labels)
		}
	}
	scope, campus, objectType, objectID := notificationDimensions(alert.Labels)
	if scope != "核心资产" || campus != "主园区" || objectType != "server" || objectID != "SRV-0007" {
		t.Fatalf("dimensions=%q,%q,%q,%q", scope, campus, objectType, objectID)
	}
}
