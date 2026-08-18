package fusion

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

var ErrInvalidSourceFact = errors.New("invalid fusion source fact")

type sourceFactReference struct {
	EventID             string `json:"event_id"`
	ProjectionIdentity  string `json:"projection_identity"`
	ProjectionHash      string `json:"projection_hash"`
	SourceTopic         string `json:"source_topic"`
	SourcePartition     int    `json:"source_partition"`
	SourceOffset        int64  `json:"source_offset"`
	SourcePayloadSHA256 string `json:"source_payload_sha256"`
}

type assetUpsertWire struct {
	TenantID string `json:"tenant_id"`
	AssetID  string `json:"asset_id"`
	Asset    struct {
		AssetID    string `json:"asset_id"`
		TenantID   string `json:"tenant_id"`
		AssetType  string `json:"asset_type"`
		IPAddress  string `json:"ip_address"`
		MACAddress string `json:"mac_address"`
		Hostname   string `json:"hostname"`
	} `json:"asset"`
}

func DecodeSourceFacts(sourceID, tenantID string, facts []SourceFact) ([]SourceEntityFact, []SourceRelationFact, error) {
	entities := make(map[string]*SourceEntityFact)
	relations := make(map[string]SourceRelationFact)
	for _, fact := range facts {
		payload, err := base64.StdEncoding.DecodeString(fact.PayloadBase64)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: event %s payload is not base64: %v", ErrInvalidSourceFact, fact.EventID, err)
		}
		reference := sourceFactReference{
			EventID: fact.EventID, ProjectionIdentity: fact.ProjectionIdentity,
			ProjectionHash: fact.ProjectionHash, SourceTopic: fact.SourceTopic,
			SourcePartition: fact.SourcePartition, SourceOffset: fact.SourceOffset,
			SourcePayloadSHA256: fact.SourcePayloadSHA256,
		}
		switch sourceID {
		case "traffic":
			var event trafficv1.FlowEvent
			if err := proto.Unmarshal(payload, &event); err != nil || event.GetHeader() == nil || event.GetTuple() == nil {
				return nil, nil, fmt.Errorf("%w: flow event %s cannot be decoded", ErrInvalidSourceFact, fact.EventID)
			}
			if event.GetHeader().GetTenantId() != tenantID || event.GetHeader().GetEventId() != fact.EventID {
				return nil, nil, fmt.Errorf("%w: flow event %s tenant or event identity mismatch", ErrInvalidSourceFact, fact.EventID)
			}
			sourceEntityID, err := addIPEntity(entities, sourceID, event.GetTuple().GetSrcIp(), fact.EventID, reference)
			if err != nil {
				return nil, nil, err
			}
			targetEntityID, err := addIPEntity(entities, sourceID, event.GetTuple().GetDstIp(), fact.EventID, reference)
			if err != nil {
				return nil, nil, err
			}
			addSourceRelation(relations, sourceID, fact, sourceEntityID, targetEntityID, "network_flow", reference)
		case "asset":
			var event assetUpsertWire
			if err := json.Unmarshal(payload, &event); err != nil {
				return nil, nil, fmt.Errorf("%w: asset event %s cannot be decoded", ErrInvalidSourceFact, fact.EventID)
			}
			assetID := strings.TrimSpace(event.Asset.AssetID)
			if assetID == "" {
				assetID = strings.TrimSpace(event.AssetID)
			}
			if event.TenantID != tenantID || event.Asset.TenantID != tenantID || assetID == "" {
				return nil, nil, fmt.Errorf("%w: asset event %s tenant or asset identity mismatch", ErrInvalidSourceFact, fact.EventID)
			}
			identifiers := map[string]string{"asset_id": assetID}
			if ip := normalizedIP(event.Asset.IPAddress); ip != "" {
				identifiers["ip"] = ip
			}
			if mac := normalizedMAC(event.Asset.MACAddress); mac != "" {
				identifiers["mac"] = mac
			}
			if hostname := normalizedHostname(event.Asset.Hostname); hostname != "" {
				identifiers["hostname"] = hostname
			}
			addSourceEntity(entities, sourceID+":asset:"+assetID, normalizedEntityKind(event.Asset.AssetType, "asset"), identifiers, fact.EventID, reference)
		case "log":
			var event trafficv1.DeviceLog
			if err := proto.Unmarshal(payload, &event); err != nil {
				return nil, nil, fmt.Errorf("%w: device log %s cannot be decoded", ErrInvalidSourceFact, fact.EventID)
			}
			if event.GetTenantId() != tenantID || event.GetLogId() != fact.EventID {
				return nil, nil, fmt.Errorf("%w: device log %s tenant or event identity mismatch", ErrInvalidSourceFact, fact.EventID)
			}
			entityID, err := addIPEntity(entities, sourceID, event.GetDeviceIp(), fact.EventID, reference)
			if err != nil {
				return nil, nil, err
			}
			entities[entityID].EntityKind = normalizedEntityKind(event.GetDeviceType(), "device")
		case "behavior":
			var event trafficv1.UserEvent
			if err := proto.Unmarshal(payload, &event); err != nil {
				return nil, nil, fmt.Errorf("%w: user event %s cannot be decoded", ErrInvalidSourceFact, fact.EventID)
			}
			if event.GetTenantId() != tenantID || event.GetEventId() != fact.EventID || strings.TrimSpace(event.GetUserId()) == "" {
				return nil, nil, fmt.Errorf("%w: user event %s tenant or event identity mismatch", ErrInvalidSourceFact, fact.EventID)
			}
			userID := strings.TrimSpace(event.GetUserId())
			userEntityID := sourceID + ":user:" + userID
			identifiers := map[string]string{"user_id": userID}
			if username := normalizedUsername(event.GetUsername()); username != "" {
				identifiers["username"] = username
			}
			addSourceEntity(entities, userEntityID, "user", identifiers, fact.EventID, reference)
			if ip := normalizedIP(event.GetSourceIp()); ip != "" {
				ipEntityID, err := addIPEntity(entities, sourceID, ip, fact.EventID, reference)
				if err != nil {
					return nil, nil, err
				}
				addSourceRelation(relations, sourceID, fact, userEntityID, ipEntityID, "user_observed_from_ip", reference)
			}
		default:
			return nil, nil, fmt.Errorf("%w: unsupported source %q", ErrInvalidSourceFact, sourceID)
		}
	}
	entityResult := make([]SourceEntityFact, 0, len(entities))
	for _, entity := range entities {
		sort.Strings(entity.EvidenceEventIDs)
		entityResult = append(entityResult, *entity)
	}
	sort.Slice(entityResult, func(i, j int) bool { return entityResult[i].SourceEntityID < entityResult[j].SourceEntityID })
	relationResult := make([]SourceRelationFact, 0, len(relations))
	for _, relation := range relations {
		sort.Strings(relation.EvidenceEventIDs)
		relationResult = append(relationResult, relation)
	}
	sort.Slice(relationResult, func(i, j int) bool { return relationResult[i].SourceRelationID < relationResult[j].SourceRelationID })
	return entityResult, relationResult, nil
}

func addIPEntity(entities map[string]*SourceEntityFact, sourceID, rawIP, eventID string, reference sourceFactReference) (string, error) {
	ip := normalizedIP(rawIP)
	if ip == "" {
		return "", fmt.Errorf("%w: event %s contains an invalid IP identity", ErrInvalidSourceFact, eventID)
	}
	entityID := sourceID + ":ip:" + ip
	addSourceEntity(entities, entityID, "ip", map[string]string{"ip": ip}, eventID, reference)
	return entityID, nil
}

func addSourceEntity(entities map[string]*SourceEntityFact, entityID, kind string, identifiers map[string]string, eventID string, reference sourceFactReference) {
	entity := entities[entityID]
	if entity == nil {
		entity = &SourceEntityFact{
			SourceEntityID: entityID, EntityKind: kind, Identifiers: map[string]string{},
			Provenance: map[string]interface{}{"source_fact_refs": []sourceFactReference{}},
		}
		entities[entityID] = entity
	}
	for key, value := range identifiers {
		if value != "" {
			entity.Identifiers[key] = value
		}
	}
	if !containsString(entity.EvidenceEventIDs, eventID) {
		entity.EvidenceEventIDs = append(entity.EvidenceEventIDs, eventID)
		refs, _ := entity.Provenance["source_fact_refs"].([]sourceFactReference)
		entity.Provenance["source_fact_refs"] = append(refs, reference)
	}
}

func addSourceRelation(relations map[string]SourceRelationFact, sourceID string, fact SourceFact, sourceEntityID, targetEntityID, kind string, reference sourceFactReference) {
	if sourceEntityID == targetEntityID {
		return
	}
	relationID := stableHex(sourceID, kind, sourceEntityID, targetEntityID, fact.EventID)
	relations[relationID] = SourceRelationFact{
		SourceRelationID: relationID, SourceEntityID: sourceEntityID, TargetEntityID: targetEntityID,
		RelationKind: kind, EventTime: fact.EventTime, EvidenceEventIDs: []string{fact.EventID},
		Provenance: map[string]interface{}{"source_fact_ref": reference},
	}
}

func normalizedIP(value string) string {
	parsed := net.ParseIP(strings.TrimSpace(value))
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

func normalizedMAC(value string) string {
	parsed, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.String())
}

func normalizedHostname(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizedUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedEntityKind(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "server", "endpoint", "network-device", "switch", "router", "firewall", "host":
		return "host"
	case "service", "business-system":
		return "service"
	case "device":
		return "device"
	case "user":
		return "user"
	case "asset":
		return "asset"
	default:
		return fallback
	}
}

func stableHex(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
