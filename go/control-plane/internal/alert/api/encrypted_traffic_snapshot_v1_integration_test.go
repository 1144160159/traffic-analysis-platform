package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"go.uber.org/zap"
)

func TestEncryptedTrafficSnapshotKubernetesReadOnlyIntegration(t *testing.T) {
	if os.Getenv("ENCRYPTED_SNAPSHOT_K8S_INTEGRATION") != "1" {
		t.Skip("set ENCRYPTED_SNAPSHOT_K8S_INTEGRATION=1 inside the Kubernetes canary")
	}
	hosts := splitEncryptedSnapshotHosts(os.Getenv("CLICKHOUSE_HOSTS"))
	if len(hosts) == 0 {
		t.Fatal("CLICKHOUSE_HOSTS is required")
	}
	database := strings.TrimSpace(os.Getenv("CLICKHOUSE_DATABASE"))
	if database == "" {
		database = "traffic"
	}
	username := strings.TrimSpace(os.Getenv("CLICKHOUSE_USERNAME"))
	if username == "" {
		username = "default"
	}
	client, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: hosts, Database: database, Username: username, Password: os.Getenv("CLICKHOUSE_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
		DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("connect to Kubernetes ClickHouse service: %v", err)
	}
	defer client.Close()

	discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), encryptedTrafficSnapshotTimeout)
	defer discoveryCancel()
	tenantID := strings.TrimSpace(os.Getenv("ENCRYPTED_SNAPSHOT_INTEGRATION_TENANT_ID"))
	var discoveredTenant string
	var latestSessionStart int64
	discoveryRow, discoveryErr := client.QueryRow(discoveryCtx, `SELECT tenant_id,max(ts_start)
		FROM traffic.sessions
		WHERE dst_port IN (443,8443,853,993,995,465,22)
		GROUP BY tenant_id ORDER BY max(ts_start) DESC LIMIT 1`)
	if discoveryErr == nil && discoveryRow.Scan(&discoveredTenant, &latestSessionStart) == nil && tenantID == "" {
		tenantID = discoveredTenant
	}
	if tenantID == "" {
		tenantID = "default"
	}
	windowEnd := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	if latestSessionStart > 0 {
		windowEnd = time.UnixMilli(latestSessionStart).UTC()
	}
	query := encryptedTrafficSnapshotQuery{
		TenantID: tenantID, Start: windowEnd.Add(-time.Hour), End: windowEnd,
		Limit: 5, PcapReadAllowed: false, PcapDownloadAllowed: false,
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), encryptedTrafficSnapshotTimeout)
	reader := &encryptedTrafficSnapshotProductionReader{clickhouse: client, logger: zap.NewNop()}
	_, _, _, _, sessionErr := reader.readSessions(readCtx, query)
	readCancel()
	if sessionErr != nil {
		t.Fatalf("authoritative session query failed within the server budget: %v", sessionErr)
	}
	readCtx, readCancel = context.WithTimeout(context.Background(), encryptedTrafficSnapshotTimeout)
	defer readCancel()
	started := time.Now()
	result := reader.ReadEncryptedTrafficSnapshot(readCtx, query)
	elapsed := time.Since(started)
	if elapsed > encryptedTrafficSnapshotTimeout {
		t.Fatalf("read-only snapshot exceeded server budget: %s", elapsed)
	}
	if result.FlowMetadata.Availability == "unavailable" || result.SideChannel.Availability == "unavailable" {
		t.Fatalf("authoritative session source unavailable: flow=%+v side=%+v", result.FlowMetadata, result.SideChannel)
	}
	if result.PlaintextVisible.Availability == "unavailable" || result.RandomnessStatistics.Availability == "unavailable" {
		t.Fatalf("authoritative fingerprint source unavailable: plaintext=%+v randomness=%+v", result.PlaintextVisible, result.RandomnessStatistics)
	}
	if result.RawReference.Availability != "forbidden" {
		t.Fatalf("pcap reference must remain field-forbidden without pcap:read: %+v", result.RawReference)
	}
	if result.SourceWatermarks["clickhouse.sessions"] == "" {
		t.Fatalf("sessions source watermark is absent: %+v", result.SourceWatermarks)
	}
	if result.FlowMetadata.SampleCount > query.Limit || result.PlaintextVisible.SampleCount > query.Limit {
		t.Fatalf("bounded query returned too many facts: flow=%d plaintext=%d", result.FlowMetadata.SampleCount, result.PlaintextVisible.SampleCount)
	}
}

func splitEncryptedSnapshotHosts(value string) []string {
	parts := strings.Split(value, ",")
	hosts := make([]string, 0, len(parts))
	for _, part := range parts {
		if host := strings.TrimSpace(part); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
