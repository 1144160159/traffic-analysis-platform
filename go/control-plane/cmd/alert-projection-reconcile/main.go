package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/projection"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func main() {
	var tenantID, mode, requestedBy, traceID, startText, endText, idsText, targetVersion string
	var expectedClusterUUID, expectedReadTarget, expectedWriteAlias, expectedWriteIndex string
	var reviewPackage, approvalBundle, expectedReviewSHA, expectedApprovalSHA, expectedToolImage string
	var maxDocuments int
	var confirmRepair bool
	flag.StringVar(&tenantID, "tenant", "", "required tenant scope")
	flag.StringVar(&mode, "mode", "plan", "plan or repair")
	flag.StringVar(&requestedBy, "requested-by", "", "required operator identity")
	flag.StringVar(&traceID, "trace-id", uuid.NewString(), "stable audit trace ID")
	flag.StringVar(&startText, "start", "", "optional RFC3339 lower bound")
	flag.StringVar(&endText, "end", "", "optional RFC3339 upper bound")
	flag.StringVar(&idsText, "alert-ids", "", "optional comma-separated business IDs")
	flag.StringVar(&targetVersion, "target-index-version", "", "exact approved index generation")
	flag.StringVar(&expectedClusterUUID, "expected-cluster-uuid", "", "repair-only approved OpenSearch cluster UUID")
	flag.StringVar(&expectedReadTarget, "expected-read-target", "", "repair-only exact approved projection read target")
	flag.StringVar(&expectedWriteAlias, "expected-write-alias", "", "repair-only exact approved projection write alias")
	flag.StringVar(&expectedWriteIndex, "expected-write-index", "", "repair-only exact physical write index")
	flag.StringVar(&reviewPackage, "review-package", "", "repair-only immutable non-authorizing review JSON")
	flag.StringVar(&approvalBundle, "approval-bundle", "", "repair-only immutable four-party approval JSON")
	flag.StringVar(&expectedReviewSHA, "expected-review-sha256", "", "repair-only exact review file SHA-256")
	flag.StringVar(&expectedApprovalSHA, "expected-approval-sha256", "", "repair-only exact approval file SHA-256")
	flag.StringVar(&expectedToolImage, "expected-tool-image", "", "repair-only immutable repository@sha256 image reference")
	flag.IntVar(&maxDocuments, "max-documents", 0, "bounded document count")
	flag.BoolVar(&confirmRepair, "confirm-repair", false, "required for mode=repair")
	flag.Parse()
	if tenantID == "" || requestedBy == "" {
		fatalf("--tenant and --requested-by are required")
	}
	if mode == "repair" && !confirmRepair {
		fatalf("--confirm-repair is required for mode=repair")
	}
	if err := validateRepairApproval(
		mode, requestedBy, reviewPackage, approvalBundle, expectedReviewSHA, expectedApprovalSHA,
		expectedToolImage, time.Now().UTC(), os.Args,
	); err != nil {
		fatalf("repair approval: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		fatalf("load config: %v", err)
	}
	if err := validateModeConfiguration(mode, cfg.OpenSearch); err != nil {
		fatalf("%v", err)
	}
	logger := zap.NewExample()
	defer logger.Sync()
	ctx := context.Background()
	chClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: cfg.ClickHouse.GetHosts(), Database: cfg.ClickHouse.GetDatabase(), Username: cfg.ClickHouse.GetUsername(), Password: cfg.ClickHouse.GetPassword(),
		MaxOpenConns: cfg.ClickHouse.MaxOpenConns, MaxIdleConns: cfg.ClickHouse.MaxIdleConns,
		ConnMaxLifetime: time.Hour, DialTimeout: 10 * time.Second, CompressionLZ4: true,
	}, logger)
	if err != nil {
		fatalf("connect ClickHouse: %v", err)
	}
	defer chClient.Close()
	osWriter, err := persistence.NewOpenSearchReconcileTarget(
		cfg.OpenSearch.Addresses,
		cfg.OpenSearch.Username,
		cfg.OpenSearch.Password,
		cfg.OpenSearch.ReadTarget(),
		cfg.OpenSearch.WriteTarget(),
		cfg.OpenSearch.V2Enabled,
		!cfg.OpenSearch.V2Enabled && strings.TrimSpace(cfg.OpenSearch.LegacyReadTarget) != "",
		logger,
	)
	if err != nil {
		fatalf("connect OpenSearch: %v", err)
	}
	defer osWriter.Close()
	metadata, err := osWriter.ProjectionMetadata(ctx)
	if err != nil {
		fatalf("read OpenSearch projection target identity: %v", err)
	}
	if err := validateRepairTargetBinding(mode, metadata, expectedClusterUUID, expectedReadTarget, expectedWriteAlias, expectedWriteIndex); err != nil {
		fatalf("repair target binding: %v", err)
	}
	pg, err := sql.Open("postgres", cfg.Auth.ConnectionString())
	if err != nil {
		fatalf("open PostgreSQL: %v", err)
	}
	defer pg.Close()
	if err := pg.PingContext(ctx); err != nil {
		fatalf("connect PostgreSQL: %v", err)
	}
	store := persistence.NewProjectionDebtStore(pg)
	if err := store.CheckSchema(ctx); err != nil {
		fatalf("schema readiness: %v", err)
	}
	if targetVersion == "" {
		targetVersion = osWriter.TargetVersion()
	}
	if maxDocuments == 0 {
		maxDocuments = cfg.AlertProjection.MaxDocuments
	}
	scope := persistence.ProjectionScope{
		TenantID: tenantID, StartTime: parseTime(startText), EndTime: parseTime(endText),
		BusinessIDs: splitIDs(idsText), TargetIndexVersion: targetVersion, MaxDocuments: maxDocuments,
	}
	reconciler, err := projection.NewReconciler(projection.ReconcileConfig{
		MaxDocuments: cfg.AlertProjection.MaxDocuments, StopErrorCount: cfg.AlertProjection.StopErrorCount,
		RepairPerSecond: cfg.AlertProjection.RepairPerSecond,
	}, repository.NewAlertRepository(chClient, logger), osWriter, store)
	if err != nil {
		fatalf("create reconciler: %v", err)
	}
	result, err := reconciler.Run(ctx, projection.ReconcileRequest{Mode: mode, RequestedBy: requestedBy, TraceID: traceID, Scope: scope})
	if err != nil {
		fatalf("reconcile: %v", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatalf("encode result: %v", err)
	}
}

func validateRepairTargetBinding(
	mode string,
	metadata persistence.OpenSearchProjectionMetadata,
	expectedClusterUUID, expectedReadTarget, expectedWriteAlias, expectedWriteIndex string,
) error {
	if !strings.EqualFold(strings.TrimSpace(mode), "repair") {
		return nil
	}
	expectedClusterUUID = strings.TrimSpace(expectedClusterUUID)
	expectedReadTarget = strings.TrimSpace(expectedReadTarget)
	expectedWriteAlias = strings.TrimSpace(expectedWriteAlias)
	expectedWriteIndex = strings.TrimSpace(expectedWriteIndex)
	if expectedClusterUUID == "" || expectedReadTarget == "" || expectedWriteAlias == "" || expectedWriteIndex == "" {
		return fmt.Errorf("repair requires expected cluster UUID, read target, write alias and physical write index")
	}
	for name, value := range map[string]string{
		"read target": expectedReadTarget, "write alias": expectedWriteAlias, "write index": expectedWriteIndex,
	} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("expected %s must be exact and non-wildcard", name)
		}
	}
	if metadata.ClusterUUID != expectedClusterUUID || metadata.ReadTarget != expectedReadTarget || metadata.WriteAlias != expectedWriteAlias {
		return fmt.Errorf(
			"approved target identity drifted: cluster=%s read=%s write_alias=%s",
			metadata.ClusterUUID, metadata.ReadTarget, metadata.WriteAlias,
		)
	}
	writeIndices := make([]string, 0, 1)
	for _, index := range metadata.WriteIndices {
		if index.IsWriteIndex {
			writeIndices = append(writeIndices, index.Index)
		}
	}
	if len(writeIndices) != 1 || writeIndices[0] != expectedWriteIndex {
		return fmt.Errorf("approved write index drifted: current=%v expected=%s", writeIndices, expectedWriteIndex)
	}
	return nil
}

func validateModeConfiguration(mode string, cfg config.OpenSearchConfig) error {
	if strings.EqualFold(strings.TrimSpace(mode), "repair") && !cfg.V2Enabled {
		return fmt.Errorf("repair requires OPENSEARCH_ALERTS_V2_ENABLED=true and an approved versioned write alias")
	}
	return nil
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		fatalf("invalid RFC3339 time %q: %v", value, err)
	}
	return parsed
}

func splitIDs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			result = append(result, id)
		}
	}
	return result
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
