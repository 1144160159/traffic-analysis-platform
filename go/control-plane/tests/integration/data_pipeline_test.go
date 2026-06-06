package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"go.uber.org/zap"
)

// =============================================================================
// Asset Service Pipeline Tests
// =============================================================================

func TestAssetServicePipeline(t *testing.T) {
	logger := zap.NewNop()
	_ = service.New(nil, nil, logger)

	// OUI lookup
	vendor := service.LookupVendor("18:c0:09:11:22:33")
	assert.Equal(t, "Broadcom Limited", vendor)

	vendor = service.LookupVendor("ff:ff:ff:11:22:33")
	assert.Equal(t, "Unknown", vendor)
}

func TestAssetRecordFields(t *testing.T) {
	rec := &config.AssetRecord{
		AssetID:    "a-1",
		TenantID:   "t1",
		IPAddress:  "192.168.1.1",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		Hostname:   "server-01",
		Vendor:     "Intel",
		OSType:     "Linux",
		Source:     "arp",
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	assert.Equal(t, "a-1", rec.AssetID)
	assert.Equal(t, "t1", rec.TenantID)
}

// =============================================================================
// Data Pipeline: FlowEvent → Alert (完整链路验证)
// =============================================================================

func TestFlowToAlertPipeline(t *testing.T) {
	now := time.Now().UnixMilli()

	// 1. FlowEvent → protobuf serialization (simulate probe → Kafka)
	flow := &pb.FlowEvent{
		FlowId:      uuid.New().String(),
		CommunityId: "1:abc123def456",
		Tuple: &pb.FiveTuple{
			SrcIp: "192.168.1.100", DstIp: "10.0.0.50",
			SrcPort: 54321, DstPort: 443, Protocol: 6,
		},
		TsStart: now, TsEnd: now + 100,
		Header: &pb.EventHeader{
			EventId: uuid.New().String(), TenantId: "test-tenant",
			RunId: "realtime", ProbeId: "probe-001", FeatureSetId: "v1-default",
			EventTs: now, IngestTs: now,
		},
		PacketsFwd: 10, PacketsBwd: 8, BytesFwd: 5000, BytesBwd: 3000,
	}
	data, err := proto.Marshal(flow)
	require.NoError(t, err)
	var decodedFlow pb.FlowEvent
	require.NoError(t, proto.Unmarshal(data, &decodedFlow))
	assert.Equal(t, flow.CommunityId, decodedFlow.CommunityId)

	// 2. Alert → protobuf serialization (simulate Flink → Kafka → Go consumer)
	alert := &pb.Alert{
		AlertId: uuid.New().String(), TenantId: "test-tenant",
		CommunityId: "1:abc123def456", SessionId: uuid.New().String(),
		AlertType: "scan", Severity: pb.Severity_SEVERITY_MEDIUM, Score: 0.85,
		SrcIp: "192.168.1.100", DstIp: "10.0.0.50",
		SrcPort: 54321, DstPort: 443, Protocol: 6,
		FirstSeen: now, LastSeen: now,
		Status: pb.AlertStatus_ALERT_STATUS_NEW,
		RuleVersion: "v1", ModelVersion: "xgboost-v1",
	}
	alertData, err := proto.Marshal(alert)
	require.NoError(t, err)
	var decodedAlert pb.Alert
	require.NoError(t, proto.Unmarshal(alertData, &decodedAlert))
	assert.Equal(t, alert.AlertId, decodedAlert.AlertId)

	// 3. Fingerprint generation
	fp := dedup.CalculateAlertFingerprint(
		alert.TenantId, alert.AlertType, alert.SrcIp, alert.DstIp,
		alert.DstPort, alert.Severity.String(), alert.FirstSeen, 5,
	)
	assert.NotEmpty(t, fp)
	assert.Len(t, fp, 32)

	// 4. Persistence format
	p := &persistence.Alert{
		TenantID: alert.TenantId, AlertID: alert.AlertId, Fingerprint: fp,
		CommunityID: alert.CommunityId, SessionID: alert.SessionId,
		SrcIP: alert.SrcIp, DstIP: alert.DstIp,
		SrcPort: uint16(alert.SrcPort), DstPort: uint16(alert.DstPort), Protocol: uint8(alert.Protocol),
		AlertType: alert.AlertType, Labels: alert.Labels, Score: alert.Score, Severity: "medium",
		FirstSeen: time.UnixMilli(alert.FirstSeen), LastSeen: time.UnixMilli(alert.LastSeen),
		Count: 1, Status: "new", UpdatedTs: time.Now(),
		ModelVersion: alert.ModelVersion, RuleVersion: alert.RuleVersion,
	}
	assert.Equal(t, "medium", p.Severity)
	assert.Equal(t, "192.168.1.100", p.SrcIP)

	// 5. Proto round-trip for persistence
	protoAlert := p.ToProto()
	assert.Equal(t, p.AlertID, protoAlert.AlertId)
	assert.Equal(t, p.SrcIP, protoAlert.SrcIp)
}

// =============================================================================
// DetectionBatch → Alert Pipeline
// =============================================================================

func TestDetectionToAlertPipeline(t *testing.T) {
	now := time.Now().UnixMilli()

	batch := &pb.DetectionBatch{
		BatchId: uuid.New().String(), TenantId: "test", RunId: "realtime",
		Businesses: []*pb.DetectionBusiness{{
			Header:        &pb.EventHeader{EventId: uuid.New().String(), TenantId: "test", EventTs: now, IngestTs: now},
			CommunityId:   "1:abc", SessionId: uuid.New().String(),
			DetectionType: "ddos", Label: "high_traffic", Score: 0.95,
			RuleVersion: "v2", ModelVersion: "xgb-v2", Ts: now,
		}},
		CreatedAt: now,
	}
	data, _ := proto.Marshal(batch)
	var decoded pb.DetectionBatch
	require.NoError(t, proto.Unmarshal(data, &decoded))
	require.Len(t, decoded.Businesses, 1)
	assert.Equal(t, "ddos", decoded.Businesses[0].DetectionType)
}

// =============================================================================
// Audit Log Pipeline
// =============================================================================

func TestAuditLogPipeline(t *testing.T) {
	log := &pb.AuditLog{
		EventId: uuid.New().String(), TenantId: "test", UserId: "u1",
		Action: "ALERT_CLOSE", ObjectType: "alert", ObjectId: "a-1",
		Detail: `{"reason":"false_positive"}`, IpAddr: "10.0.0.1",
		UserAgent: "test/1.0", CreatedAt: time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(log)
	var decoded pb.AuditLog
	require.NoError(t, proto.Unmarshal(data, &decoded))
	assert.Equal(t, "ALERT_CLOSE", decoded.Action)
	assert.Contains(t, decoded.Detail, "false_positive")
}

func TestUserEventPipeline(t *testing.T) {
	ue := &pb.UserEvent{
		EventId: "ev-1", TenantId: "t1", UserId: "u1", Username: "admin",
		EventType: "login", SourceIp: "10.0.0.1", UserAgent: "Chrome",
		Resource: "/api/v1/alerts", Action: "GET", Result: "success",
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(ue)
	var decoded pb.UserEvent
	require.NoError(t, proto.Unmarshal(data, &decoded))
	assert.Equal(t, "ev-1", decoded.EventId)
	assert.Equal(t, "login", decoded.EventType)
}

func TestDeviceLogPipeline(t *testing.T) {
	dl := &pb.DeviceLog{
		LogId: "log-1", TenantId: "t1", DeviceIp: "10.0.0.1",
		DeviceType: "switch", Facility: 16, Severity: 3,
		Timestamp: time.Now().UnixMilli(), Message: "port down", Source: "syslog",
	}
	data, _ := proto.Marshal(dl)
	var decoded pb.DeviceLog
	require.NoError(t, proto.Unmarshal(data, &decoded))
	assert.Equal(t, "log-1", decoded.LogId)
}

func TestDeadLetterPipeline(t *testing.T) {
	dl := &pb.DeadLetter{
		EventId: "dl-1", TenantId: "t1", SourceTopic: "flow.events.v1",
		SourceKey: "key1", ErrorMsg: "parse error", RetryCount: 1,
		CreatedAt: time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(dl)
	var decoded pb.DeadLetter
	require.NoError(t, proto.Unmarshal(data, &decoded))
	assert.Equal(t, "dl-1", decoded.EventId)
	assert.Equal(t, "parse error", decoded.ErrorMsg)
}
