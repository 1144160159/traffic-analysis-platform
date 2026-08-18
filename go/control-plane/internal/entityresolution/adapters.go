package entityresolution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

// ObservationFromFlow maps both endpoint roles without treating two endpoints
// as an ambiguous single identifier.
func ObservationFromFlow(
	event *trafficv1.FlowEvent,
	source SourceReference,
) (Observation, error) {
	if event == nil || event.GetHeader() == nil || event.GetTuple() == nil {
		return Observation{}, fmt.Errorf("FlowEvent header and tuple are required")
	}
	header := event.GetHeader()
	observedAtMS := firstPositive(header.GetOccurredAt(), header.GetEventTs(), event.GetTsStart())
	bound, err := bindProtoSource(source, RailFlow, header.GetEventId(), observedAtMS, event)
	if err != nil {
		return Observation{}, err
	}
	identifiers := optionalIdentifiers(
		Identifier{Kind: IdentifierIP, Value: event.GetTuple().GetSrcIp(), Role: "source"},
		Identifier{Kind: IdentifierIP, Value: event.GetTuple().GetDstIp(), Role: "destination"},
		Identifier{Kind: IdentifierProbeID, Value: header.GetProbeId(), Role: "sensor"},
		Identifier{Kind: IdentifierCommunityID, Value: event.GetCommunityId(), Role: "correlation"},
	)
	return Observation{TenantID: header.GetTenantId(), Source: bound, Identifiers: identifiers}, nil
}

func ObservationFromAsset(
	asset *trafficv1.Asset,
	source SourceReference,
) (Observation, error) {
	if asset == nil {
		return Observation{}, fmt.Errorf("Asset is required")
	}
	observedAtMS := firstPositive(asset.GetLastSeen(), asset.GetFirstSeen())
	bound, err := bindProtoSource(source, RailAssetAuthority, source.EventID, observedAtMS, asset)
	if err != nil {
		return Observation{}, err
	}
	identifiers := optionalIdentifiers(
		Identifier{Kind: IdentifierAssetID, Value: asset.GetAssetId(), Role: "subject"},
		Identifier{Kind: IdentifierIP, Value: asset.GetIpAddress(), Role: "subject"},
		Identifier{Kind: IdentifierMAC, Value: asset.GetMacAddress(), Role: "subject"},
	)
	return Observation{TenantID: asset.GetTenantId(), Source: bound, Identifiers: identifiers}, nil
}

func ObservationFromMacIPBinding(
	binding *trafficv1.MacIpBinding,
	probeID string,
	source SourceReference,
) (Observation, error) {
	if binding == nil {
		return Observation{}, fmt.Errorf("MacIpBinding is required")
	}
	bound, err := bindProtoSource(
		source, RailAssetBinding, source.EventID, binding.GetObservedAt(), binding)
	if err != nil {
		return Observation{}, err
	}
	identifiers := optionalIdentifiers(
		Identifier{Kind: IdentifierMAC, Value: binding.GetMacAddress(), Role: "device"},
		Identifier{Kind: IdentifierIP, Value: binding.GetIpAddress(), Role: "device"},
		Identifier{Kind: IdentifierProbeID, Value: probeID, Role: "sensor"},
	)
	return Observation{TenantID: binding.GetTenantId(), Source: bound, Identifiers: identifiers}, nil
}

func ObservationFromDeviceLog(
	log *trafficv1.DeviceLog,
	source SourceReference,
) (Observation, error) {
	if log == nil {
		return Observation{}, fmt.Errorf("DeviceLog is required")
	}
	bound, err := bindProtoSource(source, RailDeviceLog, log.GetLogId(), log.GetTimestamp(), log)
	if err != nil {
		return Observation{}, err
	}
	return Observation{
		TenantID: log.GetTenantId(),
		Source:   bound,
		Identifiers: optionalIdentifiers(
			Identifier{Kind: IdentifierIP, Value: log.GetDeviceIp(), Role: "device"}),
	}, nil
}

func ObservationFromUserEvent(
	event *trafficv1.UserEvent,
	source SourceReference,
) (Observation, error) {
	if event == nil {
		return Observation{}, fmt.Errorf("UserEvent is required")
	}
	bound, err := bindProtoSource(
		source, RailUserBehavior, event.GetEventId(), event.GetTimestamp(), event)
	if err != nil {
		return Observation{}, err
	}
	return Observation{
		TenantID: event.GetTenantId(),
		Source:   bound,
		Identifiers: optionalIdentifiers(
			Identifier{Kind: IdentifierUserID, Value: event.GetUserId(), Role: "account"},
			Identifier{Kind: IdentifierIP, Value: event.GetSourceIp(), Role: "source"}),
	}, nil
}

func bindProtoSource(
	source SourceReference,
	expectedRail SourceRail,
	eventID string,
	observedAtMS int64,
	message proto.Message,
) (SourceReference, error) {
	if source.Rail != expectedRail {
		return SourceReference{}, fmt.Errorf(
			"source rail %q does not match %q adapter", source.Rail, expectedRail)
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return SourceReference{}, fmt.Errorf("domain event_id is required")
	}
	if source.EventID != "" && strings.TrimSpace(source.EventID) != eventID {
		return SourceReference{}, fmt.Errorf("source event_id does not match domain event")
	}
	if observedAtMS <= 0 {
		return SourceReference{}, fmt.Errorf("domain event time is required")
	}
	if source.ObservedAtMS != 0 && source.ObservedAtMS != observedAtMS {
		return SourceReference{}, fmt.Errorf("source observed_at_ms does not match domain event time")
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		return SourceReference{}, fmt.Errorf("deterministically marshal source protobuf: %w", err)
	}
	digest := sha256.Sum256(payload)
	payloadSHA := hex.EncodeToString(digest[:])
	if source.PayloadSHA256 != "" && source.PayloadSHA256 != payloadSHA {
		return SourceReference{}, fmt.Errorf("source payload_sha256 does not match domain event")
	}
	source.EventID = eventID
	source.ObservedAtMS = observedAtMS
	source.PayloadSHA256 = payloadSHA
	return source, nil
}

func optionalIdentifiers(values ...Identifier) []Identifier {
	result := make([]Identifier, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
