package api

import "testing"

func TestAppendUniqueStringKeepsStableOwnerOrder(t *testing.T) {
	owners := []string{"campaign-a", "campaign-b"}
	owners = appendUniqueString(owners, "campaign-a")
	owners = appendUniqueString(owners, "campaign-c")

	want := []string{"campaign-a", "campaign-b", "campaign-c"}
	if len(owners) != len(want) {
		t.Fatalf("owners=%v want=%v", owners, want)
	}
	for index := range want {
		if owners[index] != want[index] {
			t.Fatalf("owners=%v want=%v", owners, want)
		}
	}
}

func TestFallbackCampaignAlertsPreservesPerCampaignMembership(t *testing.T) {
	campaigns := []campaignDTO{
		{CampaignID: "campaign-a", Alerts: []string{"alert-1", "", "alert-2"}},
		{CampaignID: "campaign-b", Alerts: []string{"alert-3"}},
		{CampaignID: "campaign-empty"},
	}

	alerts := fallbackCampaignAlerts(campaigns)
	if got := []string{alerts["campaign-a"][0].AlertID, alerts["campaign-a"][1].AlertID}; got[0] != "alert-1" || got[1] != "alert-2" {
		t.Fatalf("campaign-a fallback=%v", got)
	}
	if got := alerts["campaign-b"]; len(got) != 1 || got[0].AlertID != "alert-3" {
		t.Fatalf("campaign-b fallback=%v", got)
	}
	if got := alerts["campaign-empty"]; len(got) != 0 {
		t.Fatalf("campaign-empty fallback=%v", got)
	}
}
