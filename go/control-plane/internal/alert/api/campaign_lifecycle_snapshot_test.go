package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCampaignLifecycleSnapshotIsOrderIndependentAndWatermarkBound(t *testing.T) {
	base := campaignDTO{
		TenantID: "tenant-a", CampaignID: "campaign-a",
		StateVersion: 4, MemberCount: 2,
		LastEventID: "00000000-0000-4000-8000-000000000004",
		EventID:     "ch-event-a", IngestTs: 100, TsStart: 10, TsEnd: 20,
		Status: "investigating", Assignee: "owner-a", Summary: "campaign summary",
		Score: 0.91, CampaignType: "apt",
		Entities: []string{"asset-b", "asset-a"}, AttackPhases: []string{"execution", "initial_access"},
		RuleIDs: []string{"rule-b", "rule-a"}, ModelIDs: []string{"model-b", "model-a"},
	}
	reordered := base
	reordered.Entities = []string{"asset-a", "asset-b"}
	reordered.AttackPhases = []string{"initial_access", "execution"}
	reordered.RuleIDs = []string{"rule-a", "rule-b"}
	reordered.ModelIDs = []string{"model-a", "model-b"}

	require.NoError(t, stampCampaignLifecycleSnapshot(&base))
	require.NoError(t, stampCampaignLifecycleSnapshot(&reordered))
	require.Equal(t, base.SnapshotID, reordered.SnapshotID)
	require.Equal(t, base.SnapshotSHA256, reordered.SnapshotSHA256)
	require.Len(t, base.SnapshotSHA256, 64)
	require.True(t, strings.HasPrefix(base.SnapshotID, "campaign:campaign-a:revision:4:"))

	for name, mutate := range map[string]func(*campaignDTO){
		"postgres revision":           func(value *campaignDTO) { value.StateVersion++ },
		"postgres last event":         func(value *campaignDTO) { value.LastEventID = "different-event" },
		"authoritative member count":  func(value *campaignDTO) { value.MemberCount++ },
		"clickhouse event":            func(value *campaignDTO) { value.EventID = "ch-event-b" },
		"clickhouse ingest watermark": func(value *campaignDTO) { value.IngestTs++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			require.NoError(t, stampCampaignLifecycleSnapshot(&changed))
			require.NotEqual(t, base.SnapshotID, changed.SnapshotID)
			require.NotEqual(t, base.SnapshotSHA256, changed.SnapshotSHA256)
		})
	}
}

func TestCampaignLifecycleSnapshotRequiresTenantAndCampaign(t *testing.T) {
	require.Error(t, stampCampaignLifecycleSnapshot(&campaignDTO{CampaignID: "campaign-a"}))
	require.Error(t, stampCampaignLifecycleSnapshot(&campaignDTO{TenantID: "tenant-a"}))
}
