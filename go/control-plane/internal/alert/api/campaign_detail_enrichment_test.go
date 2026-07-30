package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCampaignEvidenceSummaryKeepsUnavailableDenominatorsExplicit(t *testing.T) {
	items := campaignEvidenceSummary(
		[]campaignAlertDTO{{AlertID: "alert-1"}, {AlertID: "alert-2"}},
		[]campaignPhaseDTO{{Phase: "initial_access", EvidenceCount: 3}, {Phase: "execution", EvidenceCount: 4}},
		true,
		2,
		true,
	)

	require.Len(t, items, 5)
	require.Equal(t, "告警", items[0].Label)
	require.NotNil(t, items[0].Current)
	require.Equal(t, uint64(2), *items[0].Current)
	require.Nil(t, items[0].Expected)
	require.True(t, items[1].Available)
	require.NotNil(t, items[1].Current)
	require.Equal(t, uint64(7), *items[1].Current)
	require.False(t, items[2].Available)
	require.Nil(t, items[2].Current)
	require.True(t, items[4].Available)
	require.Equal(t, uint64(2), *items[4].Current)
}

func TestAppendCurrentCampaignTransitionUsesRealWorkbenchTimestamp(t *testing.T) {
	start := time.Date(2026, time.June, 19, 9, 15, 0, 0, time.UTC)
	updated := time.Date(2026, time.June, 19, 10, 2, 0, 0, time.UTC)
	transitions := []campaignStatusTransitionDTO{{
		Status:    "new",
		ChangedAt: start.Format(time.RFC3339Nano),
		Source:    "campaign",
	}}

	result := appendCurrentCampaignTransition(transitions, "investigating", updated.Format(time.RFC3339Nano), 0)

	require.Len(t, result, 2)
	require.Equal(t, "investigating", result[1].Status)
	require.Equal(t, updated.Format(time.RFC3339Nano), result[1].ChangedAt)
	require.Equal(t, "workbench_state", result[1].Source)
}

func TestCampaignTimestampRFC3339SupportsMilliseconds(t *testing.T) {
	expected := time.Date(2026, time.June, 20, 3, 22, 0, 0, time.UTC)
	require.Equal(t, expected.Format(time.RFC3339Nano), campaignTimestampRFC3339(expected.UnixMilli()))
}

func TestCampaignImpactServicesUseObservedPortProtocolAndHighestRisk(t *testing.T) {
	services := campaignImpactServicesFromObservations([]campaignImpactObservation{
		{DstIP: "10.0.0.7", DstPort: 5432, Protocol: 6, Severity: "medium"},
		{DstIP: "10.0.0.7", DstPort: 5432, Protocol: 6, Severity: "high"},
		{DstIP: "10.0.0.9", DstPort: 53, Protocol: 17, Severity: "low"},
		{DstIP: "10.0.0.8", DstPort: 0, Protocol: 6, Severity: "critical"},
	})

	require.Equal(t, []campaignImpactServiceDTO{
		{
			ServiceName:  "PostgreSQL",
			PortProtocol: "5432/TCP",
			Risk:         "高危",
			Dependency:   "10.0.0.7",
		},
		{
			ServiceName:  "DNS",
			PortProtocol: "53/UDP",
			Risk:         "低危",
			Dependency:   "10.0.0.9",
		},
	}, services)
}

func TestCampaignImpactAccountClassificationAndWorkflowProgress(t *testing.T) {
	require.Equal(t, "服务账号", campaignAccountType("svc_backup", "user-1"))
	require.Equal(t, "管理账号", campaignAccountType("temp_admin", "user-2"))
	require.Equal(t, "人员账号", campaignAccountType("li.ming", "user-3"))
	require.Equal(t, "高危", campaignAccountRisk("temp_admin", "success", 0))
	require.Equal(t, "中危", campaignAccountRisk("li.ming", "failed", 1))
	require.Equal(t, "低危", campaignAccountRisk("li.ming", "success", 0))
	require.Equal(t, "10.0.0.8 -> db-prod", campaignLoginPath("10.0.0.8", "db-prod"))
	require.Equal(t, 40, campaignStatusProgress("active"))
	require.Equal(t, 70, campaignStatusProgress("contained"))
	require.Equal(t, 100, campaignStatusProgress("closed"))
}

func TestCampaignImpactMetadataProjectionAndDeduplication(t *testing.T) {
	metadata := map[string]interface{}{
		"campaign_accounts": []interface{}{
			map[string]interface{}{
				"account": "sec_analyst", "account_type": "人员账号",
				"permission_risk": "高危", "login_path": "统一认证 -> 10.0.5.8",
			},
		},
		"open_services": []interface{}{
			map[string]interface{}{
				"service": "VXLAN", "port": float64(8472), "protocol": "UDP",
				"risk_level": "中危", "dependency": "集群容器网络",
			},
		},
	}

	accounts := campaignImpactAccountsFromMetadata(metadata)
	services := campaignImpactServicesFromMetadata(metadata, "全流量分析平台")

	require.Equal(t, []campaignImpactAccountDTO{{
		Account: "sec_analyst", AccountType: "人员账号", PermissionRisk: "高危", LoginPath: "统一认证 -> 10.0.5.8",
	}}, accounts)
	require.Equal(t, []campaignImpactServiceDTO{{
		ServiceName: "VXLAN", PortProtocol: "8472/UDP", Risk: "中危", Dependency: "集群容器网络",
	}}, services)
	require.Len(t, mergeCampaignImpactAccounts(accounts, accounts), 1)
	require.Len(t, mergeCampaignImpactServices(services, services), 1)
	require.Equal(t, "高危", campaignAlertRiskLabel("SEVERITY_CRITICAL"))
}

func TestAttackChainPhasesCarryObservedEndpointsEvidenceAndMitreTechnique(t *testing.T) {
	campaign := campaignDTO{
		CampaignID:   "attack-chain-demo",
		TenantID:     "default",
		AttackPhases: []string{"initial_access"},
		TsStart:      100,
		TsEnd:        300,
		Score:        0.92,
	}
	alerts := []campaignAlertDTO{{
		AlertID:     "alert-initial-access",
		AlertType:   "Web 漏洞利用",
		Severity:    "high",
		LastSeen:    200,
		AttackPhase: "initial_access",
		Entity:      "FW-01",
		SrcIP:       "198.51.100.27",
		DstIP:       "10.12.5.23",
		EvidenceIDs: []string{"pcap-initial-access"},
	}}

	phases := campaignToPhasesWithAlerts(campaign, alerts)

	require.Len(t, phases, 1)
	require.Equal(t, []string{"alert-initial-access"}, phases[0].AlertIDs)
	require.Equal(t, int64(200), phases[0].StartTime)
	require.Equal(t, int64(200), phases[0].EndTime)
	require.Len(t, phases[0].KeyEvents, 1)
	require.Equal(t, "FW-01", phases[0].KeyEvents[0].Entity)
	require.Equal(t, "198.51.100.27", phases[0].KeyEvents[0].SrcIP)
	require.Equal(t, "10.12.5.23", phases[0].KeyEvents[0].DstIP)
	require.Equal(t, "TA0001", phases[0].KeyEvents[0].Technique)
	require.Equal(t, []string{"pcap-initial-access"}, phases[0].KeyEvents[0].EvidenceIDs)
}

func TestAttackChainPagedResourcesKeepDistinctRecommendationTabs(t *testing.T) {
	alert := campaignAlertDTO{
		AlertID:     "alert-c2",
		AlertType:   "C2 隧道通信",
		Severity:    "critical",
		LastSeen:    1785047111000,
		AttackPhase: "command_control",
		Entity:      "c2.example.com",
		SrcIP:       "10.12.8.45",
		DstIP:       "198.51.100.27",
		EvidenceIDs: []string{"evidence-tls-session"},
	}

	path := attackChainPathFromAlert(alert, 0)
	require.Equal(t, "TA0011", path.Technique)
	require.Equal(t, "evidence-tls-session", path.EvidenceID)
	require.Equal(t, "confirmed", path.Status)

	block := attackChainRecommendationFromAlert("block", alert, 0)
	isolate := attackChainRecommendationFromAlert("isolate", alert, 0)
	allowlist := attackChainRecommendationFromAlert("allowlist", alert, 0)
	playbook := attackChainRecommendationFromAlert("playbook", alert, 0)

	require.Equal(t, "阻断 c2.example.com", block.Action)
	require.Equal(t, "隔离 c2.example.com", isolate.Action)
	require.Equal(t, "复核白名单 c2.example.com", allowlist.Action)
	require.Equal(t, "执行 C2 阻断剧本", playbook.Action)
	require.Equal(t, []string{"低影响", "中等影响", "需审批", "自动化"}, []string{
		block.Impact, isolate.Impact, allowlist.Impact, playbook.Impact,
	})
}

func TestAttackChainEvidenceAndRecommendationQueryValuesAreAllowlisted(t *testing.T) {
	for input, expected := range map[string]string{
		"": "block", "阻断点": "block", "隔离建议": "isolate", "白名单风险": "allowlist", "剧本推荐": "playbook",
	} {
		actual, err := normalizeAttackChainRecommendationCategory(input)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	for input, expected := range map[string]string{
		"": "", "全部": "", "PCAP": "pcap", "Session": "session", "日志": "log", "图谱": "graph", "规则/模型": "rule_model",
	} {
		actual, err := normalizeAttackChainEvidenceType(input)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	_, err := normalizeAttackChainEvidenceType("unsupported")
	require.Error(t, err)
	_, err = normalizeAttackChainRecommendationCategory("unsupported")
	require.Error(t, err)
}
