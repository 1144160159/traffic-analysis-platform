package main

import (
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
)

const testCampaignRailCandidate = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func setCampaignRailCanaryIdentityEnv(t *testing.T) campaignRailCanaryIdentity {
	t.Helper()
	t.Setenv("CAMPAIGN_RAIL_CANARY_RUN_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("CAMPAIGN_RAIL_CANARY_CANDIDATE_SHA256", testCampaignRailCandidate)
	t.Setenv("CAMPAIGN_RAIL_CANARY_TENANT_ID", "canary-m07-111111111111")
	identity, err := loadCampaignRailCanaryIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func testCampaignRailConfig() *config.Config {
	return &config.Config{Kafka: config.KafkaConfig{
		CampaignProtoTopic: campaignrail.ProtoTopic, CampaignEventTopic: campaignrail.AggregateJSONTopic,
		CampaignMemberTopic: campaignrail.MembershipJSONTopic,
	}}
}

func TestCampaignRailCanaryIdentityIsRunBoundAndDeterministic(t *testing.T) {
	first := setCampaignRailCanaryIdentityEnv(t)
	second, err := loadCampaignRailCanaryIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.ProtoEventID == first.JSONEventID || first.RelationID == first.JSONEventID {
		t.Fatalf("identity is not deterministic and separated: first=%+v second=%+v", first, second)
	}
	if first.TenantID != "canary-m07-111111111111" || first.CandidateSHA256 != testCampaignRailCandidate {
		t.Fatalf("unexpected identity: %+v", first)
	}
}

func TestCampaignRailCanaryStageRequiresExactSwitchVector(t *testing.T) {
	identity := setCampaignRailCanaryIdentityEnv(t)
	t.Setenv("CAMPAIGNS_PROTO_CANDIDATE_SHA256", testCampaignRailCandidate)
	t.Setenv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256", testCampaignRailCandidate)
	t.Setenv("CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256", campaignRailCorrelationContractSHA256)
	tests := []struct {
		stage string
		flags [4]bool
	}{
		{"proto-consumer", [4]bool{true, false, false, false}},
		{"json-consumer", [4]bool{false, true, false, false}},
		{"json-dispatcher", [4]bool{false, false, true, false}},
		{"correlation", [4]bool{false, false, false, true}},
	}
	for _, test := range tests {
		t.Run(test.stage, func(t *testing.T) {
			cfg := testCampaignRailConfig()
			cfg.Kafka.CampaignProtoEnabled = test.flags[0]
			cfg.Kafka.CampaignEventConsumerEnabled = test.flags[1]
			cfg.Kafka.CampaignEventDispatcherEnabled = test.flags[2]
			cfg.Kafka.CampaignRailCorrelationEnabled = test.flags[3]
			if err := validateCampaignRailCanaryStage(test.stage, cfg, identity); err != nil {
				t.Fatal(err)
			}
			cfg.Kafka.CampaignEventDispatcherEnabled = !cfg.Kafka.CampaignEventDispatcherEnabled
			if err := validateCampaignRailCanaryStage(test.stage, cfg, identity); err == nil {
				t.Fatal("stage accepted a second campaign rail switch")
			}
		})
	}
}

func TestCampaignRailCanaryRejectsAliasTopicAndStaleContract(t *testing.T) {
	identity := setCampaignRailCanaryIdentityEnv(t)
	t.Setenv("CAMPAIGNS_PROTO_CANDIDATE_SHA256", testCampaignRailCandidate)
	t.Setenv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256", testCampaignRailCandidate)
	t.Setenv("CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256", campaignRailCorrelationContractSHA256)
	cfg := testCampaignRailConfig()
	cfg.Kafka.CampaignRailCorrelationEnabled = true
	cfg.Kafka.CampaignProtoTopic = "campaigns-canary.v1"
	if err := validateCampaignRailCanaryStage("correlation", cfg, identity); err == nil {
		t.Fatal("alias campaign topic was accepted")
	}
	cfg.Kafka.CampaignProtoTopic = campaignrail.ProtoTopic
	t.Setenv("CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256", testCampaignRailCandidate)
	if err := validateCampaignRailCanaryStage("correlation", cfg, identity); err == nil {
		t.Fatal("stale correlation contract SHA was accepted")
	}
}

func TestCampaignRailCanaryReplayScopeKeepsImmutableCEPWindowAcrossPlannerBoundary(t *testing.T) {
	identity := setCampaignRailCanaryIdentityEnv(t)
	eventTimeEnd := time.Date(2026, 8, 14, 14, 14, 0, 0, time.UTC)
	asOf := time.Date(2026, 8, 14, 14, 21, 17, 0, time.UTC)
	scope, err := campaignRailCanaryReplayScope(identity, eventTimeEnd, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 8, 14, 14, 10, 0, 0, time.UTC); !scope.WindowFrom.Equal(want) {
		t.Fatalf("window_from=%s want=%s", scope.WindowFrom, want)
	}
	if want := time.Date(2026, 8, 14, 14, 15, 0, 0, time.UTC); !scope.WindowThrough.Equal(want) {
		t.Fatalf("window_through=%s want=%s", scope.WindowThrough, want)
	}
	if !scope.AsOf.Equal(asOf) || scope.TenantID != identity.TenantID || scope.MaxCampaigns != 100 {
		t.Fatalf("unexpected replay scope: %+v", scope)
	}
	if _, err := campaignRailCanaryReplayScope(identity, eventTimeEnd,
		time.Date(2026, 8, 14, 14, 14, 59, 0, time.UTC)); err == nil {
		t.Fatal("open CEP event-time window was accepted")
	}
}
