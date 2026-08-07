package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const dashboardProfileSampleCount = 40

type dashboardProfileDistribution struct {
	Count int     `json:"count"`
	MinMS float64 `json:"min_ms"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

type dashboardProfileHarness struct {
	mu              sync.Mutex
	calls           int
	commands        map[string]string
	requestMS       []float64
	externalEffects map[string]bool
}

func newDashboardProfileHarness() *dashboardProfileHarness {
	return &dashboardProfileHarness{
		commands: make(map[string]string), externalEffects: make(map[string]bool),
	}
}

func (provider *dashboardProfileHarness) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	defer func() {
		provider.mu.Lock()
		provider.requestMS = append(provider.requestMS, float64(time.Since(started).Microseconds())/1000)
		provider.mu.Unlock()
	}()
	if request.Method != http.MethodPost || request.URL.Path != "/execute" ||
		request.Header.Get("Authorization") != "Bearer "+dashboardRealProviderToken {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	var envelope struct {
		SchemaVersion int                           `json:"schema_version"`
		Command       DashboardTaskExecutionRequest `json:"command"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.SchemaVersion != 1 {
		http.Error(writer, "invalid profile envelope", http.StatusBadRequest)
		return
	}
	command := envelope.Command
	if request.Header.Get("Idempotency-Key") != command.IdempotencyKey ||
		request.Header.Get("X-Tenant-ID") != command.TenantID ||
		request.Header.Get("X-Trace-ID") != command.TraceID {
		http.Error(writer, "profile metadata mismatch", http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.calls++
	digest := dashboardRealDigest(command)
	previous, exists := provider.commands[command.IdempotencyKey]
	if exists && previous != digest {
		provider.mu.Unlock()
		http.Error(writer, "profile idempotency collision", http.StatusConflict)
		return
	}
	provider.commands[command.IdempotencyKey] = digest
	provider.mu.Unlock()

	switch {
	case strings.HasPrefix(command.Target, "profile-close-"):
		dashboardRealCloseWithoutResponse(writer)
		return
	case strings.HasPrefix(command.Target, "profile-timeout-"):
		time.Sleep(120 * time.Millisecond)
		provider.recordExternalEffect(command.IdempotencyKey)
	case strings.HasPrefix(command.Target, "profile-slow-"):
		time.Sleep(25 * time.Millisecond)
		provider.recordExternalEffect(command.IdempotencyKey)
	default:
		provider.recordExternalEffect(command.IdempotencyKey)
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(DashboardTaskExecutionReceipt{
		Status: "completed", Provider: "bounded-loopback-provider",
		ProviderReceiptID: "profile-receipt-" + command.RequestEventID,
		EffectState:       "confirmed", EffectIDs: []string{"profile-effect-" + command.TaskID},
		Result:     map[string]interface{}{"task_id": command.TaskID, "profile": true},
		ExecutedAt: time.Now().UTC(),
	})
}

func (provider *dashboardProfileHarness) recordExternalEffect(key string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.externalEffects[key] = true
}

func (provider *dashboardProfileHarness) snapshot() (int, int, int, []float64) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls, len(provider.commands), len(provider.externalEffects), append([]float64(nil), provider.requestMS...)
}

func (provider *dashboardProfileHarness) reset() {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = 0
	provider.commands = make(map[string]string)
	provider.requestMS = nil
	provider.externalEffects = make(map[string]bool)
}

// TestDashboardTaskBoundedPerformanceProfile is an owned-environment G4
// preflight. Its deliberately loose ceilings are regression stop conditions,
// not production SLOs and not approved release-candidate performance evidence.
func TestDashboardTaskBoundedPerformanceProfile(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_PG_DSN")
	broker := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_KAFKA_BROKER")
	sentinel := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_SENTINEL")
	resultPath := os.Getenv("DASHBOARD_TASK_BOUNDED_PROFILE_RESULT")
	if dsn == "" || broker == "" || sentinel == "" || resultPath == "" {
		t.Skip("dashboard bounded-profile sentinel environment is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_dashboard_task_real_components_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != sentinel {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tenantID := "dashboard-profile-" + uuid.NewString()
	warmupTenantID := "dashboard-profile-warmup-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES
		($1,'Dashboard Bounded Profile'),($2,'Dashboard Bounded Profile Warmup')`, tenantID, warmupTenantID); err != nil {
		t.Fatal(err)
	}
	provider := newDashboardProfileHarness()
	providerServer := httptest.NewServer(provider)
	defer providerServer.Close()
	executor, err := NewHTTPDashboardTaskExecutor(providerServer.URL+"/execute", dashboardRealProviderToken, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: dashboardTaskKafkaTopic, BatchSize: 16,
		BatchTimeout: 5 * time.Millisecond, MaxAttempts: 3, RequiredAcks: "all",
		Compression: "none", Async: false, IdempotentKey: "dashboard-profile-partition-key",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	pipeline, err := NewDashboardTaskPipeline(db, executor, producer.Send, dashboardTaskKafkaTopic, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewDashboardTaskHandler(db, zap.NewNop(), true)
	groupID := "dashboard-task-bounded-profile-" + uuid.NewString()
	var committedMessages atomic.Int64
	var maximumCommittedOffset atomic.Int64
	maximumCommittedOffset.Store(-1)
	stopConsumer := startDashboardRealConsumer(t, broker, groupID, pipeline, &committedMessages, &maximumCommittedOffset)
	defer stopConsumer()
	consumerColdStarted := time.Now()
	warmup := performDashboardTaskCreate(t, handler, warmupTenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-profile-warmup-"+uuid.NewString(), DashboardTaskCreateRequest{
			Target: "profile-warmup", Priority: "high", SnapshotID: "snapshot-profile-warmup",
			Reason: "separate consumer cold-start warmup", Context: map[string]interface{}{"warmup": true},
		})
	if warmup.Code != http.StatusAccepted {
		t.Fatalf("profile warmup status=%d body=%s", warmup.Code, warmup.Body.String())
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-profile-warmup-outbox", 10); err != nil || drained != 1 {
		t.Fatalf("profile warmup outbox drained=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "profile warmup execution queue", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_execution_attempts WHERE tenant_id=$1 AND status='pending'`, warmupTenantID).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	if drained, err := pipeline.DrainExecutions(ctx, "dashboard-profile-warmup-executor", 10); err != nil || drained != 1 {
		t.Fatalf("profile warmup execution drained=%d err=%v", drained, err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-profile-warmup-result", 10); err != nil || drained != 1 {
		t.Fatalf("profile warmup result drained=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "profile warmup result consumption", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND event_type=$2`, warmupTenantID, dashboardTaskResultEvent).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	consumerColdStartMS := float64(time.Since(consumerColdStarted).Microseconds()) / 1000
	provider.reset()

	runtime.GC()
	var memoryBefore runtime.MemStats
	runtime.ReadMemStats(&memoryBefore)
	goroutinesBefore := runtime.NumGoroutine()
	profileStarted := time.Now()
	createLatencies := make([]float64, 0, dashboardProfileSampleCount)
	for index := 0; index < dashboardProfileSampleCount; index++ {
		target := fmt.Sprintf("profile-fast-%02d", index)
		switch {
		case index >= 24 && index < 32:
			target = fmt.Sprintf("profile-slow-%02d", index)
		case index >= 32 && index < 36:
			target = fmt.Sprintf("profile-close-%02d", index)
		case index >= 36:
			target = fmt.Sprintf("profile-timeout-%02d", index)
		}
		started := time.Now()
		created := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
			fmt.Sprintf("dashboard-profile-%02d-%s", index, uuid.NewString()), DashboardTaskCreateRequest{
				Target: target, Priority: "high", SnapshotID: fmt.Sprintf("snapshot-profile-%02d", index),
				Reason:  "owned bounded dashboard performance profile",
				Context: map[string]interface{}{"profile_index": index},
			})
		createLatencies = append(createLatencies, float64(time.Since(started).Microseconds())/1000)
		if created.Code != http.StatusAccepted {
			t.Fatalf("profile create %d status=%d body=%s", index, created.Code, created.Body.String())
		}
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-profile-outbox", 100); err != nil || drained != dashboardProfileSampleCount {
		t.Fatalf("profile request outbox drained=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "profile execution queue", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_execution_attempts WHERE tenant_id=$1 AND status='pending'`, tenantID).Scan(&count)
		return err == nil && count == dashboardProfileSampleCount, fmt.Sprintf("count=%d err=%v", count, err)
	})
	peakQueueDepth := dashboardProfileSampleCount
	if drained, err := pipeline.DrainExecutions(ctx, "dashboard-profile-executor", 50); err != nil || drained != dashboardProfileSampleCount {
		t.Fatalf("profile executions drained=%d err=%v", drained, err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-profile-results", 100); err != nil || drained != dashboardProfileSampleCount {
		t.Fatalf("profile result outbox drained=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "profile result consumption", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND event_type=$2`, tenantID, dashboardTaskResultEvent).Scan(&count)
		return err == nil && count == dashboardProfileSampleCount, fmt.Sprintf("count=%d err=%v", count, err)
	})
	// Let deliberately timed-out provider handlers finish so external-effect
	// ambiguity is measured rather than hidden by test teardown.
	time.Sleep(150 * time.Millisecond)
	stopConsumer()
	profileWall := time.Since(profileStarted)

	queueMS, terminalMS, resultPropagationMS, endToEndMS := queryDashboardProfileDurations(t, ctx, db, tenantID)
	var completedTasks, partialTasks, attempts, receipts, inbox, audits, duplicateEffects int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM dashboard_tasks WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_tasks WHERE tenant_id=$1 AND status='partial'),
		(SELECT count(*) FROM dashboard_task_execution_attempts WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_task_execution_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task'),
		(SELECT count(*) FROM (SELECT jsonb_array_elements_text(effect_ids),count(*)
		 FROM dashboard_task_execution_receipts WHERE tenant_id=$1 GROUP BY 1 HAVING count(*)>1) duplicates)`, tenantID).
		Scan(&completedTasks, &partialTasks, &attempts, &receipts, &inbox, &audits, &duplicateEffects); err != nil {
		t.Fatal(err)
	}
	providerCalls, uniqueCommands, externalEffects, providerRequestMS := provider.snapshot()
	endOffset := readDashboardProfileEndOffset(t, broker)
	committedOffset := maximumCommittedOffset.Load()
	finalLag := endOffset - committedOffset - 1
	runtime.GC()
	var memoryAfter runtime.MemStats
	runtime.ReadMemStats(&memoryAfter)
	heapGrowth := int64(memoryAfter.HeapAlloc) - int64(memoryBefore.HeapAlloc)
	goroutineGrowth := runtime.NumGoroutine() - goroutinesBefore
	throughput := float64(dashboardProfileSampleCount) / profileWall.Seconds()
	retryAmplification := float64(providerCalls) / float64(dashboardProfileSampleCount)

	metrics := map[string]interface{}{
		"create_api":                  distribution(createLatencies),
		"queue_wait":                  distribution(queueMS),
		"provider_request":            distribution(providerRequestMS),
		"terminal":                    distribution(terminalMS),
		"result_propagation":          distribution(resultPropagationMS),
		"end_to_end":                  distribution(endToEndMS),
		"profile_wall_ms":             float64(profileWall.Microseconds()) / 1000,
		"consumer_cold_start_ms":      consumerColdStartMS,
		"throughput_tasks_per_second": throughput,
		"peak_queue_depth":            peakQueueDepth,
		"provider_calls":              providerCalls,
		"provider_unique_commands":    uniqueCommands,
		"provider_external_effects":   externalEffects,
		"retry_amplification":         retryAmplification,
		"kafka_end_offset":            endOffset,
		"kafka_committed_offset":      committedOffset,
		"kafka_final_lag":             finalLag,
		"heap_alloc_before_bytes":     memoryBefore.HeapAlloc,
		"heap_alloc_after_bytes":      memoryAfter.HeapAlloc,
		"heap_growth_bytes":           heapGrowth,
		"goroutines_before":           goroutinesBefore,
		"goroutines_after":            runtime.NumGoroutine(),
		"goroutine_growth":            goroutineGrowth,
	}
	checks := map[string]bool{
		"sample_count_exact":                      len(endToEndMS) == dashboardProfileSampleCount,
		"terminal_oracle_exact":                   completedTasks == 32 && partialTasks == 8,
		"durable_facts_exact":                     attempts == 40 && receipts == 40 && inbox == 80 && audits == 120,
		"provider_idempotency_exact":              uniqueCommands == 40 && externalEffects == 36 && duplicateEffects == 0,
		"retry_amplification_bounded":             retryAmplification <= 1.50,
		"kafka_final_lag_zero":                    finalLag == 0 && committedMessages.Load() >= 82,
		"create_p99_below_owned_ceiling":          distribution(createLatencies).P99MS <= 1000,
		"queue_p99_below_owned_ceiling":           distribution(queueMS).P99MS <= 5000,
		"provider_p99_below_owned_ceiling":        distribution(providerRequestMS).P99MS <= 1000,
		"end_to_end_p99_below_owned_ceiling":      distribution(endToEndMS).P99MS <= 15000,
		"wall_time_below_owned_ceiling":           profileWall <= 30*time.Second,
		"consumer_cold_start_below_owned_ceiling": consumerColdStartMS <= 10000,
		"heap_growth_below_owned_ceiling":         heapGrowth <= 128*1024*1024,
		"goroutine_growth_below_owned_ceiling":    goroutineGrowth <= 32,
	}
	passed := true
	for _, check := range checks {
		passed = passed && check
	}
	result := map[string]interface{}{
		"schema_version":   1,
		"status":           map[bool]string{true: "PASS", false: "FAIL"}[passed],
		"coverage_status":  "OWNED_BOUNDED_POSTGRES_REDPANDA_HTTP_G4_PREFLIGHT_NOT_APPROVED_G4",
		"threshold_source": "OWNED_PREFLIGHT_CEILING_NOT_PRODUCTION_SLO",
		"sample_count":     dashboardProfileSampleCount,
		"workload":         map[string]int{"fast": 24, "slow_25ms": 8, "connection_loss": 4, "provider_timeout_50ms": 4},
		"task_states":      map[string]int{"completed": completedTasks, "partial": partialTasks},
		"fault_oracle":     map[string]int{"unknown_connection_loss": 4, "unknown_timeout_with_external_effect": 4},
		"metrics":          metrics,
		"checks":           checks,
		"stop_conditions": map[string]interface{}{
			"max_retry_amplification": 1.50, "max_create_p99_ms": 1000,
			"max_queue_p99_ms": 5000, "max_provider_p99_ms": 1000,
			"max_end_to_end_p99_ms": 15000, "max_wall_ms": 30000,
			"max_consumer_cold_start_ms": 10000,
			"max_heap_growth_bytes":      128 * 1024 * 1024, "max_goroutine_growth": 32,
			"required_final_kafka_lag": 0, "required_duplicate_effects": 0,
		},
		"approved_release_candidate": false,
		"production_applied":         false,
		"secrets_captured":           false,
	}
	writeDashboardProfileResult(t, resultPath, result)
	if !passed {
		t.Fatalf("bounded dashboard profile exceeded an owned preflight stop condition: %+v", checks)
	}
	t.Logf("dashboard_bounded_profile=pass samples=%d wall_ms=%.3f throughput=%.3f retry_amplification=%.3f kafka_lag=%d end_to_end_p99_ms=%.3f",
		dashboardProfileSampleCount, float64(profileWall.Microseconds())/1000, throughput,
		retryAmplification, finalLag, distribution(endToEndMS).P99MS)
}

func queryDashboardProfileDurations(t *testing.T, ctx context.Context, db *sql.DB, tenantID string) ([]float64, []float64, []float64, []float64) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT
		EXTRACT(EPOCH FROM (a.created_at-t.created_at))*1000,
		EXTRACT(EPOCH FROM (t.completed_at-t.created_at))*1000,
		EXTRACT(EPOCH FROM (i.processed_at-t.completed_at))*1000,
		EXTRACT(EPOCH FROM (i.processed_at-t.created_at))*1000
		FROM dashboard_tasks t
		JOIN dashboard_task_execution_attempts a ON a.task_id=t.task_id
		JOIN dashboard_task_event_inbox i ON i.task_id=t.task_id AND i.event_type=$2
		WHERE t.tenant_id=$1 ORDER BY t.task_id`, tenantID, dashboardTaskResultEvent)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	queue, terminal, propagation, endToEnd := []float64{}, []float64{}, []float64{}, []float64{}
	for rows.Next() {
		var values [4]float64
		if err := rows.Scan(&values[0], &values[1], &values[2], &values[3]); err != nil {
			t.Fatal(err)
		}
		queue = append(queue, values[0])
		terminal = append(terminal, values[1])
		propagation = append(propagation, values[2])
		endToEnd = append(endToEnd, values[3])
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return queue, terminal, propagation, endToEnd
}

func readDashboardProfileEndOffset(t *testing.T, broker string) int64 {
	t.Helper()
	connection, err := segmentkafka.DialLeader(context.Background(), "tcp", broker, dashboardTaskKafkaTopic, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, end, err := connection.ReadOffsets()
	if err != nil {
		t.Fatal(err)
	}
	return end
}

func distribution(samples []float64) dashboardProfileDistribution {
	if len(samples) == 0 {
		return dashboardProfileDistribution{}
	}
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	return dashboardProfileDistribution{
		Count: len(ordered), MinMS: ordered[0], P50MS: profilePercentile(ordered, 50),
		P95MS: profilePercentile(ordered, 95), P99MS: profilePercentile(ordered, 99), MaxMS: ordered[len(ordered)-1],
	}
}

func profilePercentile(ordered []float64, percentile int) float64 {
	if len(ordered) == 0 {
		return 0
	}
	index := (len(ordered)*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(ordered) {
		index = len(ordered)
	}
	return ordered[index-1]
}

func writeDashboardProfileResult(t *testing.T, path string, result map[string]interface{}) {
	t.Helper()
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(append(payload, '\n')); err != nil {
		t.Fatal(err)
	}
}
