package entityresolution

import (
	"testing"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestFourSourceProtoAdaptersPreserveRolesAndBindPayload(t *testing.T) {
	flow, err := ObservationFromFlow(&trafficv1.FlowEvent{
		Header: &trafficv1.EventHeader{
			EventId: "flow-1", TenantId: "tenant-a", ProbeId: "probe-1", OccurredAt: 1_000,
		},
		CommunityId: "1:flow",
		Tuple:       &trafficv1.FiveTuple{SrcIp: "192.0.2.1", DstIp: "198.51.100.2"},
		TsStart:     1_000,
	}, kafkaSource(RailFlow, "flow.events.v1", 0, 1))
	if err != nil {
		t.Fatalf("ObservationFromFlow() error = %v", err)
	}
	assertIdentifier(t, flow, IdentifierIP, "192.0.2.1", "source")
	assertIdentifier(t, flow, IdentifierIP, "198.51.100.2", "destination")
	assertIdentifier(t, flow, IdentifierProbeID, "probe-1", "sensor")
	assertIdentifier(t, flow, IdentifierCommunityID, "1:flow", "correlation")

	asset, err := ObservationFromAsset(&trafficv1.Asset{
		AssetId: "asset-1", TenantId: "tenant-a", IpAddress: "192.0.2.1",
		MacAddress: "aa:bb:cc:dd:ee:01", LastSeen: 1_000,
	}, directSource(RailAssetAuthority, "asset-service", "asset-event-1", 7))
	if err != nil {
		t.Fatalf("ObservationFromAsset() error = %v", err)
	}
	assertIdentifier(t, asset, IdentifierAssetID, "asset-1", "subject")

	log, err := ObservationFromDeviceLog(&trafficv1.DeviceLog{
		LogId: "log-1", TenantId: "tenant-a", DeviceIp: "192.0.2.1", Timestamp: 1_100,
	}, kafkaSource(RailDeviceLog, "device.logs.v1", 1, 2))
	if err != nil {
		t.Fatalf("ObservationFromDeviceLog() error = %v", err)
	}
	assertIdentifier(t, log, IdentifierIP, "192.0.2.1", "device")

	user, err := ObservationFromUserEvent(&trafficv1.UserEvent{
		EventId: "user-1", TenantId: "tenant-a", UserId: "account-1",
		SourceIp: "192.0.2.1", Timestamp: 1_200,
	}, kafkaSource(RailUserBehavior, "user.events.v1", 2, 3))
	if err != nil {
		t.Fatalf("ObservationFromUserEvent() error = %v", err)
	}
	assertIdentifier(t, user, IdentifierUserID, "account-1", "account")
	assertIdentifier(t, user, IdentifierIP, "192.0.2.1", "source")

	results, err := Resolve([]Observation{flow, asset, log, user}, 2_000)
	if err != nil {
		t.Fatalf("Resolve(adapted) error = %v", err)
	}
	flowResult := resultsByEvent(results)["flow-1"]
	assertEntityRole(t, flowResult, "asset:asset-1", "source")
	if entityWithRole(flowResult, "destination") != nil {
		t.Fatalf("unresolved destination was force-merged: %+v", flowResult.Entities)
	}
	if flowResult.Status != StatusPartial {
		t.Fatalf("flow status = %s, want partial for resolved source and unresolved destination", flowResult.Status)
	}
}

func TestAdaptersRejectSourceIdentityAndPayloadMismatch(t *testing.T) {
	log := &trafficv1.DeviceLog{
		LogId: "log-1", TenantId: "tenant-a", DeviceIp: "192.0.2.1", Timestamp: 1_100,
	}
	source := kafkaSource(RailDeviceLog, "device.logs.v1", 0, 1)
	source.EventID = "different"
	if _, err := ObservationFromDeviceLog(log, source); err == nil {
		t.Fatal("event identity mismatch must fail")
	}
	source = kafkaSource(RailDeviceLog, "device.logs.v1", 0, 1)
	source.PayloadSHA256 = testPayloadSHA
	if _, err := ObservationFromDeviceLog(log, source); err == nil {
		t.Fatal("payload digest mismatch must fail")
	}
	source = kafkaSource(RailFlow, "device.logs.v1", 0, 1)
	if _, err := ObservationFromDeviceLog(log, source); err == nil {
		t.Fatal("source rail mismatch must fail")
	}
}

func kafkaSource(rail SourceRail, topic string, partition int, offset int64) SourceReference {
	return SourceReference{
		Rail: rail, Authority: string(rail) + "-owner",
		Topic: topic, Partition: partition, Offset: offset,
	}
}

func directSource(
	rail SourceRail,
	authority string,
	eventID string,
	revision int64,
) SourceReference {
	return SourceReference{
		Rail: rail, Authority: authority, EventID: eventID, SourceRevision: revision,
	}
}

func assertIdentifier(
	t *testing.T,
	observation Observation,
	kind IdentifierKind,
	value string,
	role string,
) {
	t.Helper()
	for _, identifier := range observation.Identifiers {
		if identifier.Kind == kind && identifier.Value == value && identifier.Role == role {
			if len(observation.Source.PayloadSHA256) != 64 {
				t.Fatalf("source payload digest is missing: %+v", observation.Source)
			}
			return
		}
	}
	t.Fatalf("identifier %s=%s role=%s missing from %+v", kind, value, role, observation.Identifiers)
}

func assertEntityRole(t *testing.T, result ResolutionResult, entityID, role string) {
	t.Helper()
	entity := entityWithRole(result, role)
	if entity == nil || entity.EntityID != entityID {
		t.Fatalf("entity %s role=%s missing from result %+v", entityID, role, result)
	}
}

func entityWithRole(result ResolutionResult, role string) *EntityMatch {
	for index := range result.Entities {
		if result.Entities[index].Role == role {
			return &result.Entities[index]
		}
	}
	return nil
}
