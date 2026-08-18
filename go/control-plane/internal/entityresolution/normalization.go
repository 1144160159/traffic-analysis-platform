package entityresolution

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/eventtime"
)

var (
	hexSHA256          = regexp.MustCompile(`^[0-9a-f]{64}$`)
	communityIDPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9+/=_-]+$`)
)

var allowedIdentifierRoles = map[string]struct{}{
	"subject": {}, "source": {}, "destination": {}, "device": {},
	"sensor": {}, "account": {}, "correlation": {},
}

var allowedIdentifiersByRail = map[SourceRail]map[IdentifierKind]struct{}{
	RailFlow: {
		IdentifierIP: {}, IdentifierProbeID: {}, IdentifierCommunityID: {},
	},
	RailAssetAuthority: {
		IdentifierAssetID: {}, IdentifierIP: {}, IdentifierMAC: {}, IdentifierProbeID: {},
	},
	RailAssetBinding: {
		IdentifierIP: {}, IdentifierMAC: {}, IdentifierProbeID: {},
	},
	RailDeviceLog: {
		IdentifierIP: {},
	},
	RailUserBehavior: {
		IdentifierUserID: {}, IdentifierIP: {},
	},
	RailProbeIngest: {
		IdentifierIP: {}, IdentifierMAC: {}, IdentifierProbeID: {},
	},
}

func normalizeIdentifier(identifier Identifier) (Identifier, error) {
	value := strings.TrimSpace(identifier.Value)
	if value == "" {
		return Identifier{}, fmt.Errorf("empty %s identifier", identifier.Kind)
	}
	switch identifier.Kind {
	case IdentifierAssetID, IdentifierUserID, IdentifierProbeID:
		if err := validateOpaqueIdentifier(value); err != nil {
			return Identifier{}, fmt.Errorf("invalid %s: %w", identifier.Kind, err)
		}
	case IdentifierIP:
		address, err := netip.ParseAddr(value)
		if err != nil {
			return Identifier{}, fmt.Errorf("invalid IP literal: %w", err)
		}
		value = address.Unmap().String()
	case IdentifierMAC:
		address, err := net.ParseMAC(value)
		if err != nil || len(address) != 6 {
			return Identifier{}, fmt.Errorf("invalid IEEE 48-bit MAC address")
		}
		value = strings.ToLower(address.String())
	case IdentifierCommunityID:
		if len(value) > 256 || !communityIDPattern.MatchString(value) {
			return Identifier{}, fmt.Errorf("invalid Community ID")
		}
	default:
		return Identifier{}, fmt.Errorf("unsupported identifier kind %q", identifier.Kind)
	}
	role := strings.TrimSpace(identifier.Role)
	if role == "" {
		role = "subject"
	}
	if _, ok := allowedIdentifierRoles[role]; !ok {
		return Identifier{}, fmt.Errorf("unsupported identifier role %q", role)
	}
	return Identifier{Kind: identifier.Kind, Value: value, Role: role}, nil
}

func validateOpaqueIdentifier(value string) error {
	if len(value) > 256 {
		return fmt.Errorf("identifier exceeds 256 bytes")
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return fmt.Errorf("identifier contains whitespace or control characters")
		}
	}
	return nil
}

func normalizeObservation(input Observation, asOfMS int64) (normalizedObservation, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	if err := validateOpaqueIdentifier(tenantID); err != nil || tenantID == "" {
		return normalizedObservation{}, fmt.Errorf("invalid tenant_id")
	}
	if asOfMS <= 0 {
		return normalizedObservation{}, fmt.Errorf("as_of_ms must be positive")
	}
	if !eventtime.ObservedWithinAsOf(input.Source.ObservedAtMS, asOfMS) {
		return normalizedObservation{}, fmt.Errorf("source observed_at_ms is outside as_of boundary")
	}
	allowed, ok := allowedIdentifiersByRail[input.Source.Rail]
	if !ok {
		return normalizedObservation{}, fmt.Errorf("unsupported source rail %q", input.Source.Rail)
	}
	if !hexSHA256.MatchString(input.Source.PayloadSHA256) {
		return normalizedObservation{}, fmt.Errorf("source payload_sha256 must be lowercase SHA-256")
	}

	input.Source.Authority = strings.TrimSpace(input.Source.Authority)
	input.Source.Topic = strings.TrimSpace(input.Source.Topic)
	input.Source.EventID = strings.TrimSpace(input.Source.EventID)
	if err := validateOpaqueIdentifier(input.Source.Authority); err != nil || input.Source.Authority == "" {
		return normalizedObservation{}, fmt.Errorf("source authority is required")
	}
	if err := validateOpaqueIdentifier(input.Source.EventID); err != nil || input.Source.EventID == "" {
		return normalizedObservation{}, fmt.Errorf("source event_id is required")
	}

	sourceTuple := ""
	if input.Source.Topic != "" {
		if input.Source.Partition < 0 || input.Source.Offset < 0 {
			return normalizedObservation{}, fmt.Errorf("Kafka source coordinates are negative")
		}
		sourceTuple = fmt.Sprintf("kafka:%s:%d:%d", input.Source.Topic,
			input.Source.Partition, input.Source.Offset)
	} else {
		if input.Source.SourceRevision <= 0 {
			return normalizedObservation{}, fmt.Errorf("direct authority source_revision must be positive")
		}
		sourceTuple = fmt.Sprintf("authority:%s:%s:%d", input.Source.Authority,
			input.Source.EventID, input.Source.SourceRevision)
	}

	identifiers := make([]Identifier, 0, len(input.Identifiers))
	seen := make(map[string]struct{}, len(input.Identifiers))
	for _, raw := range input.Identifiers {
		if _, allowedForRail := allowed[raw.Kind]; !allowedForRail {
			return normalizedObservation{}, fmt.Errorf(
				"identifier %s is not allowed on source rail %s", raw.Kind, input.Source.Rail)
		}
		normalized, err := normalizeIdentifier(raw)
		if err != nil {
			return normalizedObservation{}, err
		}
		key := string(normalized.Kind) + "\x00" + normalized.Role + "\x00" + normalized.Value
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		identifiers = append(identifiers, normalized)
	}
	if len(identifiers) == 0 {
		return normalizedObservation{}, fmt.Errorf("observation has no identifiers")
	}
	sortIdentifiers(identifiers)

	return newNormalizedObservation(tenantID, input.Source, sourceTuple, identifiers), nil
}

func sortIdentifiers(values []Identifier) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].Role != values[j].Role {
			return values[i].Role < values[j].Role
		}
		return values[i].Value < values[j].Value
	})
}
