package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestCorrelator() *Correlator {
	config := CorrelationConfig{
		TimeWindow:           24 * time.Hour,
		MaxAlertsPerCampaign: 1000,
		MinScoreForCampaign:  0.4,
		MinAlertsForCampaign: 2,
		MinPhasesForChain:    1,
	}
	return NewCorrelator(config, zap.NewNop())
}

func TestCorrelator_EmptyAlerts(t *testing.T) {
	c := newTestCorrelator()
	result := c.Correlate(context.Background(), nil)
	assert.Nil(t, result)
}

func TestCorrelator_BelowMinAlerts(t *testing.T) {
	c := newTestCorrelator()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "port_scan", Timestamp: time.Now(), Score: 0.8},
	}
	result := c.Correlate(context.Background(), alerts)
	assert.Nil(t, result)
}

func TestCorrelator_SameSrcIP(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "port_scan", SrcIP: "10.0.0.1", DstIP: "10.0.0.5", Timestamp: now, Score: 0.8},
		{AlertID: "a2", TenantID: "t1", AlertType: "host_scan", SrcIP: "10.0.0.1", DstIP: "10.0.0.6", Timestamp: now, Score: 0.7},
		{AlertID: "a3", TenantID: "t1", AlertType: "exploit_attempt", SrcIP: "10.0.0.1", DstIP: "10.0.0.5", Timestamp: now, Score: 0.9},
	}
	result := c.Correlate(context.Background(), alerts)
	require.NotNil(t, result)
	require.True(t, len(result) > 0, "should detect at least one campaign from same source IP")
}

func TestCorrelator_CommunityIDGrouping(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "c2_beacon", CommunityID: "1:abc123", SrcIP: "10.0.0.1", DstIP: "10.0.0.100", Timestamp: now, Score: 0.9},
		{AlertID: "a2", TenantID: "t1", AlertType: "data_exfiltration", CommunityID: "1:abc123", SrcIP: "10.0.0.1", DstIP: "10.0.0.100", Timestamp: now.Add(time.Minute), Score: 0.85},
	}
	result := c.Correlate(context.Background(), alerts)
	require.NotNil(t, result)
	assert.True(t, len(result) > 0, "should detect campaign from same community_id")
}

func TestCorrelator_AttackChainDetection(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "port_scan", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now, Score: 0.8},
		{AlertID: "a2", TenantID: "t1", AlertType: "exploit_attempt", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now.Add(5 * time.Minute), Score: 0.9},
		{AlertID: "a3", TenantID: "t1", AlertType: "malware_execution", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now.Add(10 * time.Minute), Score: 0.95},
	}
	result := c.Correlate(context.Background(), alerts)
	require.NotNil(t, result)
	assert.True(t, len(result) > 0, "should detect scan-and-exploit attack chain")
}

func TestCorrelator_MultiDimensionMerge(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	// Same source performing scan -> exploit -> movement
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "port_scan", SrcIP: "10.0.0.1", DstIP: "10.0.0.10", Timestamp: now, Score: 0.8},
		{AlertID: "a2", TenantID: "t1", AlertType: "brute_force", SrcIP: "10.0.0.1", DstIP: "10.0.0.10", Timestamp: now.Add(2 * time.Minute), Score: 0.85},
		{AlertID: "a3", TenantID: "t1", AlertType: "lateral_movement", SrcIP: "10.0.0.10", DstIP: "10.0.0.20", Timestamp: now.Add(5 * time.Minute), Score: 0.9},
		{AlertID: "a4", TenantID: "t1", AlertType: "credential_dump", SrcIP: "10.0.0.10", DstIP: "10.0.0.20", Timestamp: now.Add(7 * time.Minute), Score: 0.88},
	}
	result := c.Correlate(context.Background(), alerts)
	require.NotNil(t, result)
	// Should detect at least one campaign
	assert.True(t, len(result) > 0)
}

func TestBuildCampaign(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", AlertType: "port_scan", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now, Score: 0.8},
		{AlertID: "a2", TenantID: "t1", AlertType: "exploit_attempt", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now.Add(time.Minute), Score: 0.9},
		{AlertID: "a3", TenantID: "t1", AlertType: "malware_execution", SrcIP: "10.0.0.1", DstIP: "10.0.0.50", Timestamp: now.Add(2 * time.Minute), Score: 0.85},
	}
	event := c.buildCampaign(alerts)
	require.NotNil(t, event)
	assert.Equal(t, "t1", event.TenantID)
	assert.NotEmpty(t, event.CampaignID)
	assert.NotEmpty(t, event.Title)
	assert.NotEmpty(t, event.Description)
	assert.True(t, len(event.AlertIDs) == 3)
	assert.True(t, len(event.Phases) >= 1)
	assert.True(t, event.Score > 0)
	assert.NotEmpty(t, event.Severity)
}

func TestExtractPhases(t *testing.T) {
	c := newTestCorrelator()
	alerts := []AlertInfo{
		{AlertID: "a1", AlertType: "port_scan"},
		{AlertID: "a2", AlertType: "c2_beacon"},
		{AlertID: "a3", AlertType: "data_exfiltration"},
		{AlertID: "a4", AlertType: "port_scan"}, // duplicate phase
	}
	phases := c.extractPhases(alerts)
	assert.True(t, len(phases) == 3) // deduplicated to 3 unique phases
}

func TestIdentifyCampaignType(t *testing.T) {
	c := newTestCorrelator()

	tests := []struct {
		name     string
		phases   []AttackPhase
		expected CampaignType
	}{
		{"scan_exploit", []AttackPhase{PhaseReconnaissance, PhaseInitialAccess, PhaseExecution}, CampaignScanAndExploit},
		{"brute_force", []AttackPhase{PhaseReconnaissance, PhaseCredentialAccess}, CampaignBruteForce},
		{"c2", []AttackPhase{PhaseExecution, PhaseCommandControl}, CampaignC2Communication},
		{"data_exfil", []AttackPhase{PhaseCollection, PhaseExfiltration}, CampaignDataExfiltration},
		{"lateral", []AttackPhase{PhaseCredentialAccess, PhaseLateralMovement}, CampaignLateralMovement},
		{"ransomware", []AttackPhase{PhaseInitialAccess, PhaseExecution, PhaseImpact}, CampaignRansomware},
		{"apt_with_exfil", []AttackPhase{
			PhaseReconnaissance, PhaseInitialAccess, PhaseExecution,
			PhasePersistence, PhaseDefenseEvasion, PhaseCredentialAccess,
			PhaseDiscovery, PhaseLateralMovement, PhaseCollection,
			PhaseCommandControl, PhaseExfiltration,
		}, CampaignAPT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.identifyCampaignType(tt.phases)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDetermineCampaignSeverity(t *testing.T) {
	c := newTestCorrelator()
	assert.Equal(t, "critical", c.determineCampaignSeverity(0.9, 0.95))
	assert.Equal(t, "high", c.determineCampaignSeverity(0.6, 0.75))
	assert.Equal(t, "medium", c.determineCampaignSeverity(0.4, 0.5))
	assert.Equal(t, "low", c.determineCampaignSeverity(0.1, 0.2))
}

func TestPruneBuffer(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	c.alertBuffer = []AlertInfo{
		{AlertID: "old", Timestamp: now.Add(-48 * time.Hour)},
		{AlertID: "new", Timestamp: now.Add(-1 * time.Hour)},
	}
	c.pruneBuffer(now.Add(-24 * time.Hour))
	assert.Len(t, c.alertBuffer, 1)
	assert.Equal(t, "new", c.alertBuffer[0].AlertID)
}

func TestGenerateCampaignID(t *testing.T) {
	c := newTestCorrelator()
	now := time.Now()
	alerts := []AlertInfo{
		{AlertID: "a1", TenantID: "t1", Timestamp: now},
		{AlertID: "a2", TenantID: "t1", Timestamp: now},
	}
	id := c.generateCampaignID(alerts)
	assert.Contains(t, id, "campaign-")
	assert.Contains(t, id, "t1")
}

func TestPhaseOrder(t *testing.T) {
	c := newTestCorrelator()
	assert.Equal(t, 1, c.phaseOrder(PhaseReconnaissance))
	assert.Equal(t, 7, c.phaseOrder(PhaseCredentialAccess))
	assert.Equal(t, 13, c.phaseOrder(PhaseImpact))
}
