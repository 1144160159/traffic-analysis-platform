package entityresolution

import (
	"reflect"
	"strings"
	"testing"
)

const testPayloadSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestResolveFourRailsWithDeterministicTenantScopedMatches(t *testing.T) {
	observations := []Observation{
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 10, "asset-event-1", 10_000,
			Identifier{Kind: IdentifierAssetID, Value: "asset-1"},
			Identifier{Kind: IdentifierIP, Value: "192.0.2.8"},
			Identifier{Kind: IdentifierMAC, Value: "AA-BB-CC-DD-EE-01"}),
		observation("tenant-a", RailDeviceLog, "device.logs.v1", 1, 11, "log-1", 11_000,
			Identifier{Kind: IdentifierIP, Value: "192.0.2.8"}),
		observation("tenant-a", RailUserBehavior, "user.events.v1", 2, 12, "user-event-1", 11_500,
			Identifier{Kind: IdentifierUserID, Value: "user-1"},
			Identifier{Kind: IdentifierIP, Value: "192.0.2.8"}),
		observation("tenant-a", RailFlow, "flow.events.v1", 3, 13, "flow-1", 12_000,
			Identifier{Kind: IdentifierIP, Value: "192.0.2.8"},
			Identifier{Kind: IdentifierProbeID, Value: "probe-1"},
			Identifier{Kind: IdentifierCommunityID, Value: "1:abcDEF_123"}),
		observation("tenant-a", RailAssetBinding, "asset.bindings.v1", 4, 14, "binding-1", 12_500,
			Identifier{Kind: IdentifierMAC, Value: "aa:bb:cc:dd:ee:01"},
			Identifier{Kind: IdentifierIP, Value: "192.0.2.8"},
			Identifier{Kind: IdentifierProbeID, Value: "probe-1"}),
	}

	first, err := Resolve(observations, 20_000)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	reversed := append([]Observation(nil), observations...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	second, err := Resolve(reversed, 20_000)
	if err != nil {
		t.Fatalf("Resolve(reversed) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("input ordering changed deterministic result:\nfirst=%+v\nsecond=%+v", first, second)
	}

	byEvent := resultsByEvent(first)
	assertEntity(t, byEvent["asset-event-1"], "asset", "asset:asset-1", AssetExactConfidencePPM)
	assertEntity(t, byEvent["log-1"], "asset", "asset:asset-1", IPAssetConfidencePPM)
	assertEntity(t, byEvent["user-event-1"], "asset", "asset:asset-1", IPAssetConfidencePPM)
	assertEntity(t, byEvent["user-event-1"], "user", "user:user-1", UserExactConfidencePPM)
	assertEntity(t, byEvent["flow-1"], "probe", "probe:probe-1", ProbeExactConfidencePPM)
	assertEntity(t, byEvent["flow-1"], "asset", "asset:asset-1", IPAssetConfidencePPM)
	if got := byEvent["flow-1"].Correlations; len(got) != 1 || got[0].RuleID != ruleCommunity {
		t.Fatalf("flow correlation = %+v, want one Community ID correlation", got)
	}
	assertEntity(t, byEvent["binding-1"], "asset", "asset:asset-1", MACAssetConfidencePPM)
	assertEntity(t, byEvent["binding-1"], "probe", "probe:probe-1", ProbeExactConfidencePPM)
	for _, result := range first {
		if result.Status != StatusAccepted {
			t.Errorf("event %s status = %s, want accepted (issues=%+v)",
				result.Source.EventID, result.Status, result.Issues)
		}
		if !strings.HasPrefix(result.ResolutionID, "er1-") || len(result.DecisionSHA256) != 64 {
			t.Errorf("result identity not complete: %+v", result)
		}
	}
}

func TestResolvePreservesAmbiguousCandidatesWithoutForcedMerge(t *testing.T) {
	observations := []Observation{
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 1, "asset-a", 10_000,
			Identifier{Kind: IdentifierAssetID, Value: "asset-a"},
			Identifier{Kind: IdentifierIP, Value: "192.0.2.9"}),
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 2, "asset-b", 10_100,
			Identifier{Kind: IdentifierAssetID, Value: "asset-b"},
			Identifier{Kind: IdentifierIP, Value: "192.0.2.9"}),
		observation("tenant-a", RailDeviceLog, "device.logs.v1", 0, 3, "log-ambiguous", 10_200,
			Identifier{Kind: IdentifierIP, Value: "192.0.2.9"}),
	}

	results, err := Resolve(observations, 20_000)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	result := resultsByEvent(results)["log-ambiguous"]
	if result.Status != StatusAmbiguous || len(result.Entities) != 0 || len(result.Issues) != 1 {
		t.Fatalf("ambiguous result = %+v", result)
	}
	want := []string{"asset:asset-a", "asset:asset-b"}
	if !reflect.DeepEqual(result.Issues[0].CandidateEntityIDs, want) {
		t.Fatalf("candidate IDs = %v, want %v", result.Issues[0].CandidateEntityIDs, want)
	}
}

func TestResolveUsesEventTimeWindowsAndNeverCrossesTenant(t *testing.T) {
	observations := []Observation{
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 1, "asset-a", 1_000,
			Identifier{Kind: IdentifierAssetID, Value: "asset-a"},
			Identifier{Kind: IdentifierIP, Value: "2001:0db8::1"}),
		observation("tenant-a", RailDeviceLog, "device.logs.v1", 0, 2, "expired", 1_000+IPMaximumLinkAgeMS+1,
			Identifier{Kind: IdentifierIP, Value: "2001:db8:0:0:0:0:0:1"}),
		observation("tenant-b", RailDeviceLog, "device.logs.v1", 0, 3, "other-tenant", 2_000,
			Identifier{Kind: IdentifierIP, Value: "2001:db8::1"}),
	}

	results, err := Resolve(observations, 1_000+IPMaximumLinkAgeMS+10)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	byEvent := resultsByEvent(results)
	for _, eventID := range []string{"expired", "other-tenant"} {
		result := byEvent[eventID]
		if result.Status != StatusInsufficient || len(result.Entities) != 0 {
			t.Errorf("event %s crossed time/tenant boundary: %+v", eventID, result)
		}
	}
	if got := byEvent["expired"].NormalizedIdentifiers[0].Value; got != "2001:db8::1" {
		t.Errorf("IPv6 normalization = %q, want RFC5952 form", got)
	}
}

func TestResolveRejectsSourceTupleCollisionAndInvalidRailIdentifiers(t *testing.T) {
	first := observation("tenant-a", RailDeviceLog, "device.logs.v1", 0, 7, "log-1", 1_000,
		Identifier{Kind: IdentifierIP, Value: "192.0.2.1"})
	duplicate := first
	results, err := Resolve([]Observation{first, duplicate}, 2_000)
	if err != nil || len(results) != 1 {
		t.Fatalf("identical replay must deduplicate, results=%d err=%v", len(results), err)
	}

	collision := first
	collision.Identifiers = []Identifier{{Kind: IdentifierIP, Value: "192.0.2.2"}}
	if _, err := Resolve([]Observation{first, collision}, 2_000); err == nil ||
		!strings.Contains(err.Error(), "identity collision") {
		t.Fatalf("source tuple collision error = %v", err)
	}

	badRail := first
	badRail.Identifiers = []Identifier{{Kind: IdentifierUserID, Value: "user-1"}}
	if _, err := Resolve([]Observation{badRail}, 2_000); err == nil ||
		!strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("invalid rail identifier error = %v", err)
	}
}

func TestResolveReportsExplicitAnchorAndIdentifierConflict(t *testing.T) {
	observations := []Observation{
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 1, "asset-a", 10_000,
			Identifier{Kind: IdentifierAssetID, Value: "asset-a"},
			Identifier{Kind: IdentifierMAC, Value: "aa:bb:cc:dd:ee:ff"}),
		observation("tenant-a", RailAssetAuthority, "asset.events.v2", 0, 2, "asset-b", 10_100,
			Identifier{Kind: IdentifierAssetID, Value: "asset-b"},
			Identifier{Kind: IdentifierMAC, Value: "aa:bb:cc:dd:ee:ff"}),
	}
	results, err := Resolve(observations, 20_000)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for _, result := range results {
		if result.Status != StatusConflict || len(result.Entities) != 1 {
			t.Errorf("explicit anchor conflict result = %+v", result)
		}
	}
}

func observation(
	tenant string,
	rail SourceRail,
	topic string,
	partition int,
	offset int64,
	eventID string,
	observedAtMS int64,
	identifiers ...Identifier,
) Observation {
	return Observation{
		TenantID: tenant,
		Source: SourceReference{
			Rail:          rail,
			Authority:     string(rail) + "-owner",
			Topic:         topic,
			Partition:     partition,
			Offset:        offset,
			EventID:       eventID,
			ObservedAtMS:  observedAtMS,
			PayloadSHA256: testPayloadSHA,
		},
		Identifiers: identifiers,
	}
}

func resultsByEvent(results []ResolutionResult) map[string]ResolutionResult {
	byEvent := make(map[string]ResolutionResult, len(results))
	for _, result := range results {
		byEvent[result.Source.EventID] = result
	}
	return byEvent
}

func assertEntity(t *testing.T, result ResolutionResult, entityType, entityID string, confidence int) {
	t.Helper()
	for _, entity := range result.Entities {
		if entity.EntityType == entityType && entity.EntityID == entityID {
			if entity.ConfidencePPM != confidence {
				t.Fatalf("entity %s confidence = %d, want %d", entityID, entity.ConfidencePPM, confidence)
			}
			return
		}
	}
	t.Fatalf("entity %s missing from %+v", entityID, result.Entities)
}
