package fusion

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

func TestDecodeAndMergeSourceFactsUsesStrongIdentifiers(t *testing.T) {
	flowPayload, err := proto.Marshal(&trafficv1.FlowEvent{
		Header: &trafficv1.EventHeader{EventId: "flow-event", TenantId: "tenant-a"},
		Tuple:  &trafficv1.FiveTuple{SrcIp: "10.0.0.8", DstIp: "10.0.0.9"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assetPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id": "tenant-a", "asset_id": "asset-a",
		"asset": map[string]interface{}{
			"tenant_id": "tenant-a", "asset_id": "asset-a", "asset_type": "server",
			"ip_address": "10.0.0.8", "mac_address": "00:11:22:33:44:55", "hostname": "Srv-A.EXAMPLE.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	flowEntities, flowRelations, err := DecodeSourceFacts("traffic", "tenant-a", []SourceFact{{
		EventID: "flow-event", EventTime: time.Unix(100, 0).UTC(), SourceTopic: "flow.events.v1",
		SourcePartition: 1, SourceOffset: 2, PayloadBase64: base64.StdEncoding.EncodeToString(flowPayload),
	}})
	if err != nil {
		t.Fatalf("decode flow facts: %v", err)
	}
	assetEntities, _, err := DecodeSourceFacts("asset", "tenant-a", []SourceFact{{
		EventID: "asset-event", EventTime: time.Unix(100, 0).UTC(), SourceTopic: "asset.events.v2",
		SourcePartition: 2, SourceOffset: 3, PayloadBase64: base64.StdEncoding.EncodeToString(assetPayload),
	}})
	if err != nil {
		t.Fatalf("decode asset facts: %v", err)
	}
	bound := make([]BoundSourceEntityFact, 0, len(flowEntities)+len(assetEntities))
	for _, entity := range flowEntities {
		bound = append(bound, BoundSourceEntityFact{SourceID: "traffic", SourceSnapshotID: "flow-snapshot", Fact: entity})
	}
	for _, entity := range assetEntities {
		bound = append(bound, BoundSourceEntityFact{SourceID: "asset", SourceSnapshotID: "asset-snapshot", Fact: entity})
	}
	boundRelations := make([]BoundSourceRelationFact, 0, len(flowRelations))
	for _, relation := range flowRelations {
		boundRelations = append(boundRelations, BoundSourceRelationFact{SourceID: "traffic", SourceSnapshotID: "flow-snapshot", Fact: relation})
	}
	entities, relations, err := MergeSourceEntities(bound, boundRelations)
	if err != nil {
		t.Fatalf("merge source entities: %v", err)
	}
	if len(entities) != 2 || len(relations) != 1 {
		t.Fatalf("expected two merged endpoints and one observed relation, got entities=%d relations=%d", len(entities), len(relations))
	}
	var merged *CanonicalEntity
	for index := range entities {
		if entities[index].Identifiers["asset_id"] == "asset-a" {
			merged = &entities[index]
		}
	}
	if merged == nil || merged.Identifiers["ip"] != "10.0.0.8" || merged.SourceCount != 2 || merged.Confidence != 0.5 {
		t.Fatalf("unexpected merged entity: %#v", merged)
	}
	if relations[0].EdgeOrigin != "observed" || relations[0].Confidence != 1 {
		t.Fatalf("source relation must remain observed with explicit confidence: %#v", relations[0])
	}
}

func TestMergeSourceEntitiesFailsClosedOnAuthoritativeIdentityCollision(t *testing.T) {
	entities := []BoundSourceEntityFact{
		{SourceID: "asset", SourceSnapshotID: "one", Fact: SourceEntityFact{
			SourceEntityID: "asset:one", EntityKind: "asset", Identifiers: map[string]string{"asset_id": "one", "ip": "10.0.0.8"},
		}},
		{SourceID: "asset", SourceSnapshotID: "two", Fact: SourceEntityFact{
			SourceEntityID: "asset:two", EntityKind: "asset", Identifiers: map[string]string{"asset_id": "two", "ip": "10.0.0.8"},
		}},
	}
	_, _, err := MergeSourceEntities(entities, nil)
	if !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("expected identity conflict, got %v", err)
	}
}

func TestDecodeSourceFactsRejectsCrossTenantPayload(t *testing.T) {
	payload, err := proto.Marshal(&trafficv1.DeviceLog{LogId: "log-a", TenantId: "tenant-b", DeviceIp: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = DecodeSourceFacts("log", "tenant-a", []SourceFact{{
		EventID: "log-a", PayloadBase64: base64.StdEncoding.EncodeToString(payload),
	}})
	if !errors.Is(err, ErrInvalidSourceFact) {
		t.Fatalf("expected cross-tenant source fact rejection, got %v", err)
	}
}
