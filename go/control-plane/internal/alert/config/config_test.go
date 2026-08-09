package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("JWT_SECRET_KEY") == "" {
		_ = os.Setenv("JWT_SECRET_KEY", "unit-test-only-jwt-signing-key-32-bytes")
	}
	os.Exit(m.Run())
}

func TestSavedViewEventTopicDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.SavedViewEventTopic != "alert.saved-view.events.v1" {
		t.Fatalf("SavedViewEventTopic = %q", cfg.Kafka.SavedViewEventTopic)
	}
}

func TestWhitelistEventPipelineDefaultsFailClosed(t *testing.T) {
	t.Setenv("WHITELIST_EVENT_PIPELINE_V2_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.WhitelistEventPipelineEnabled {
		t.Fatal("whitelist event pipeline must be disabled by default")
	}
	if cfg.Kafka.WhitelistEventTopic != "whitelist.events.v2" {
		t.Fatalf("WhitelistEventTopic = %q", cfg.Kafka.WhitelistEventTopic)
	}
}

func TestAuthEnabledRejectsMissingJWTSecret(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("JWT_SECRET_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("auth-enabled configuration must reject a missing JWT secret")
	}
}

func TestAuthDisabledAllowsMissingJWTSecret(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("JWT_SECRET_KEY", "")
	if _, err := Load(); err != nil {
		t.Fatalf("auth-disabled configuration unexpectedly failed: %v", err)
	}
}

func TestNotificationGovernanceEventTopicDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.NotificationGovernanceEventTopic != "notification.governance.events.v1" {
		t.Fatalf("NotificationGovernanceEventTopic = %q", cfg.Kafka.NotificationGovernanceEventTopic)
	}
}

func TestDataQualityRuleEvaluationDefaultsFailClosed(t *testing.T) {
	t.Setenv("DATA_QUALITY_RULE_EVALUATION_ENABLED", "false")
	t.Setenv("DATA_QUALITY_REPAIR_EXECUTION_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataQuality.EvaluationEnabled {
		t.Fatal("data quality rule evaluation must be disabled by default")
	}
	if cfg.DataQuality.RepairExecutionEnabled {
		t.Fatal("data quality repair execution must be disabled by default")
	}
	if cfg.DataQuality.RepairProjectionEnabled {
		t.Fatal("data quality repair projection must be disabled by default")
	}
	if cfg.DataQuality.EvaluationInterval != 5*time.Minute || cfg.DataQuality.EvaluationTimeout != 30*time.Second {
		t.Fatalf("unexpected evaluation defaults: interval=%s timeout=%s", cfg.DataQuality.EvaluationInterval, cfg.DataQuality.EvaluationTimeout)
	}
	if cfg.DataQuality.RepairEvidenceTimeout != 15*time.Second {
		t.Fatalf("unexpected repair evidence timeout: %s", cfg.DataQuality.RepairEvidenceTimeout)
	}
	if cfg.DataQuality.RepairProjectionTopic != "flow.projection-replay.v1" || cfg.DataQuality.RepairProjectionGroup != "alert-service-flow-replay-projection-v1" || cfg.DataQuality.RepairWorkerInterval != 5*time.Second {
		t.Fatalf("unexpected repair projection defaults: %+v", cfg.DataQuality)
	}
}

func TestDataQualityRuleEvaluationEnvironmentIsParsed(t *testing.T) {
	t.Setenv("DATA_QUALITY_RULE_EVALUATION_ENABLED", "true")
	t.Setenv("DATA_QUALITY_RULE_EVALUATION_INTERVAL", "2m")
	t.Setenv("DATA_QUALITY_RULE_EVALUATION_TIMEOUT", "17s")
	t.Setenv("DATA_QUALITY_REPAIR_EXECUTION_ENABLED", "true")
	t.Setenv("DATA_QUALITY_REPAIR_PROJECTION_ENABLED", "true")
	t.Setenv("DATA_QUALITY_REPAIR_EVIDENCE_TIMEOUT", "9s")
	t.Setenv("DATA_QUALITY_REPAIR_PROJECTION_TOPIC", "flow.projection-replay.v1")
	t.Setenv("DATA_QUALITY_REPAIR_PROJECTION_GROUP", "dq-replay-test-v1")
	t.Setenv("DATA_QUALITY_REPAIR_WORKER_INTERVAL", "2s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DataQuality.EvaluationEnabled || cfg.DataQuality.EvaluationInterval != 2*time.Minute || cfg.DataQuality.EvaluationTimeout != 17*time.Second || !cfg.DataQuality.RepairExecutionEnabled || !cfg.DataQuality.RepairProjectionEnabled || cfg.DataQuality.RepairEvidenceTimeout != 9*time.Second || cfg.DataQuality.RepairProjectionGroup != "dq-replay-test-v1" || cfg.DataQuality.RepairWorkerInterval != 2*time.Second {
		t.Fatalf("unexpected evaluation config: %+v", cfg.DataQuality)
	}
}

func TestClickHouseHostsNormalizeCommaSeparatedEnvironmentValue(t *testing.T) {
	cfg := ClickHouseConfig{Hosts: []string{"clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000"}}

	got := cfg.GetHosts()
	want := []string{"clickhouse-1.middleware.svc:9000", "clickhouse-2.middleware.svc:9000"}
	if len(got) != len(want) {
		t.Fatalf("GetHosts() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("GetHosts()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestClickHouseHostsNormalizeDefaultDSN(t *testing.T) {
	cfg := ClickHouseConfig{DSN: "clickhouse://default:@clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000/traffic"}

	got := cfg.GetHosts()
	want := []string{"clickhouse-1.middleware.svc:9000", "clickhouse-2.middleware.svc:9000"}
	if len(got) != len(want) {
		t.Fatalf("GetHosts() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("GetHosts()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAuthConnectionStringFromParts(t *testing.T) {
	cfg := AuthConfig{
		PostgresHost:           "postgres-primary.databases.svc",
		PostgresPort:           5432,
		PostgresDatabase:       "traffic_platform",
		PostgresUsername:       "postgres",
		PostgresPassword:       "pass word/@:",
		PostgresSSLMode:        "disable",
		PostgresConnectTimeout: 10,
	}

	got := cfg.ConnectionString()
	want := "postgres://postgres:pass%20word%2F%40%3A@postgres-primary.databases.svc:5432/traffic_platform?connect_timeout=10&sslmode=disable"
	if got != want {
		t.Fatalf("ConnectionString() = %q, want %q", got, want)
	}
}

func TestAuthConnectionStringPrefersExplicitDSN(t *testing.T) {
	cfg := AuthConfig{
		PostgresDSN:      "postgres://explicit",
		PostgresHost:     "postgres-primary.databases.svc",
		PostgresDatabase: "traffic_platform",
		PostgresUsername: "postgres",
	}

	if got := cfg.ConnectionString(); got != cfg.PostgresDSN {
		t.Fatalf("ConnectionString() = %q, want explicit DSN", got)
	}
}

func TestCampaignProjectionDefaultsFailClosed(t *testing.T) {
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_V2_ENABLED", "false")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_NEBULA_PASSWORD", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	projection := cfg.CampaignProjection
	if projection.Enabled {
		t.Fatal("campaign target projection must be disabled by default")
	}
	if projection.Interval != 500*time.Millisecond || projection.Lease != 45*time.Second || projection.MaxAttempts != 8 {
		t.Fatalf("unexpected worker defaults: interval=%s lease=%s attempts=%d", projection.Interval, projection.Lease, projection.MaxAttempts)
	}
	if projection.ClickHouseTable != "traffic.campaign_projection_events_v2" || projection.OpenSearchWriteAlias != "campaign-projections-v2-write" {
		t.Fatalf("unexpected target defaults: table=%q alias=%q", projection.ClickHouseTable, projection.OpenSearchWriteAlias)
	}
}

func TestCampaignProjectionEnvironmentIsParsed(t *testing.T) {
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_V2_ENABLED", "true")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_INTERVAL", "2s")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_LEASE", "90s")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_MAX_ATTEMPTS", "11")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_CLICKHOUSE_TABLE", "traffic.campaign_projection_events_v2_shadow")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_OS_WRITE_ALIAS", "campaign-projections-v2-canary")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_NEBULA_ADDRESSES", "nebula-a:9669,nebula-b:9669")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_NEBULA_USERNAME", "campaign_projector")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_NEBULA_PASSWORD", "ephemeral-test-password")
	t.Setenv("CAMPAIGN_TARGET_PROJECTION_NEBULA_SPACE", "traffic_graph_canary")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	projection := cfg.CampaignProjection
	if !projection.Enabled || projection.Interval != 2*time.Second || projection.Lease != 90*time.Second || projection.MaxAttempts != 11 {
		t.Fatalf("unexpected parsed worker config: %+v", projection)
	}
	if projection.ClickHouseTable != "traffic.campaign_projection_events_v2_shadow" ||
		projection.OpenSearchWriteAlias != "campaign-projections-v2-canary" ||
		projection.Nebula.Username != "campaign_projector" ||
		projection.Nebula.Password != "ephemeral-test-password" ||
		projection.Nebula.Space != "traffic_graph_canary" ||
		!reflect.DeepEqual(projection.Nebula.Addresses, []string{"nebula-a:9669", "nebula-b:9669"}) {
		t.Fatalf("unexpected parsed target config: %+v", projection)
	}
}

func TestOpenSearchV2TargetsFailClosedByDefault(t *testing.T) {
	cfg := OpenSearchConfig{
		Index:      "legacy-alerts",
		ReadAlias:  "alerts-v2-read",
		WriteAlias: "alerts-v2-write",
	}
	if got := cfg.ReadTarget(); got != "legacy-alerts-*" {
		t.Fatalf("ReadTarget() = %q, want legacy wildcard", got)
	}
	if got := cfg.WriteTarget(); got != "legacy-alerts" {
		t.Fatalf("WriteTarget() = %q, want legacy base", got)
	}
}

func TestOpenSearchV2TargetsRequireExplicitEnable(t *testing.T) {
	cfg := OpenSearchConfig{
		Index:      "legacy-alerts",
		V2Enabled:  true,
		ReadAlias:  "alerts-v2-read",
		WriteAlias: "alerts-v2-write",
	}
	if got := cfg.ReadTarget(); got != "alerts-v2-read" {
		t.Fatalf("ReadTarget() = %q, want read alias", got)
	}
	if got := cfg.WriteTarget(); got != "alerts-v2-write" {
		t.Fatalf("WriteTarget() = %q, want write alias", got)
	}
}

func TestOpenSearchLegacyReadTargetCanBindFrozenProductionIndex(t *testing.T) {
	cfg := OpenSearchConfig{
		Index:            "traffic-alerts",
		LegacyReadTarget: " alerts ",
		ReadAlias:        "alerts-v2-read",
		WriteAlias:       "alerts-v2-write",
	}
	if got := cfg.ReadTarget(); got != "alerts" {
		t.Fatalf("ReadTarget() = %q, want frozen legacy index", got)
	}
	cfg.V2Enabled = true
	if got := cfg.ReadTarget(); got != "alerts-v2-read" {
		t.Fatalf("v2 ReadTarget() = %q, want approved read alias", got)
	}
}

func TestOpenSearchSearchCursorBudgetsParseAndDefaultOff(t *testing.T) {
	t.Setenv("OPENSEARCH_SEARCH_CURSOR_V1_ENABLED", "false")
	t.Setenv("OPENSEARCH_SEARCH_SHALLOW_RESULT_LIMIT", "750")
	t.Setenv("OPENSEARCH_SEARCH_MAX_PAGE_SIZE", "125")
	t.Setenv("OPENSEARCH_SEARCH_QUERY_TIMEOUT", "1500ms")
	t.Setenv("OPENSEARCH_SEARCH_CURSOR_TTL", "90s")
	t.Setenv("OPENSEARCH_SEARCH_TRACK_TOTAL_HITS_UP_TO", "5000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	search := cfg.OpenSearch
	if search.SearchCursorEnabled || search.SearchShallowLimit != 750 || search.SearchMaxPageSize != 125 ||
		search.SearchQueryTimeout != 1500*time.Millisecond || search.SearchCursorTTL != 90*time.Second ||
		search.SearchTrackTotal != 5000 {
		t.Fatalf("unexpected OpenSearch cursor config: %+v", search)
	}
}

func TestAlertProjectionReconcileBudgetsParseAndDefaultOff(t *testing.T) {
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED", "false")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_RECONCILE_INTERVAL", "2s")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_RECONCILE_LEASE", "90s")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_RECONCILE_BATCH_SIZE", "75")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_RECONCILE_MAX_ATTEMPTS", "11")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_REBUILD_MAX_DOCUMENTS", "7500")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_STOP_ERROR_COUNT", "17")
	t.Setenv("OPENSEARCH_ALERT_PROJECTION_REPAIR_PER_SECOND", "55")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	projection := cfg.AlertProjection
	if projection.ReconcileEnabled || projection.Interval != 2*time.Second || projection.Lease != 90*time.Second ||
		projection.BatchSize != 75 || projection.MaxAttempts != 11 || projection.MaxDocuments != 7500 ||
		projection.StopErrorCount != 17 || projection.RepairPerSecond != 55 {
		t.Fatalf("unexpected alert projection config: %+v", projection)
	}
}

func TestPlaybookExecutionEventPipelineDefaultsFailClosedAndParseOverrides(t *testing.T) {
	t.Setenv("PLAYBOOK_EXECUTION_EVENT_PIPELINE_V2_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.PlaybookEventEnabled {
		t.Fatal("playbook execution event pipeline must be disabled by default")
	}
	if cfg.Kafka.PlaybookEventTopic != "playbook.execution.events.v2" ||
		cfg.Kafka.PlaybookEventGroup != "alert-service-playbook-execution-projection-v2" {
		t.Fatalf("unexpected playbook pipeline defaults: %+v", cfg.Kafka)
	}

	t.Setenv("PLAYBOOK_EXECUTION_EVENT_PIPELINE_V2_ENABLED", "true")
	t.Setenv("KAFKA_PLAYBOOK_EXECUTION_EVENT_TOPIC", "playbook.execution.events.v2.canary")
	t.Setenv("KAFKA_PLAYBOOK_EXECUTION_EVENT_GROUP", "playbook-projection-canary")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Kafka.PlaybookEventEnabled ||
		cfg.Kafka.PlaybookEventTopic != "playbook.execution.events.v2.canary" ||
		cfg.Kafka.PlaybookEventGroup != "playbook-projection-canary" {
		t.Fatalf("unexpected parsed playbook pipeline config: %+v", cfg.Kafka)
	}
}
