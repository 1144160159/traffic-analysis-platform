package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/projection"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func main() {
	var tenantID, requestedBy, traceID, environmentID string
	var startText, endText, readTarget, writeAlias string
	var output string
	var maxDocuments, timeoutSeconds int
	var legacyKeywordFields bool
	flag.StringVar(&tenantID, "tenant", "", "required explicit tenant scope")
	flag.StringVar(&requestedBy, "requested-by", "", "required operator identity")
	flag.StringVar(&traceID, "trace-id", "", "required stable audit trace ID")
	flag.StringVar(&environmentID, "environment-id", "", "required approved environment identity")
	flag.StringVar(&startText, "start", "", "required RFC3339 closed-window lower bound")
	flag.StringVar(&endText, "end", "", "required RFC3339 closed-window upper bound")
	flag.StringVar(&readTarget, "read-target", "alerts-v2-read", "exact frozen OpenSearch V2 projection read target")
	flag.StringVar(&writeAlias, "target-write-alias", "alerts-v2-write", "exact approved V2 write alias")
	flag.StringVar(&output, "output", "", "required new local JSON manifest path")
	flag.IntVar(&maxDocuments, "max-documents", 10_000, "bounded comparison count, maximum 10000")
	flag.IntVar(&timeoutSeconds, "timeout-seconds", 120, "overall read-only timeout")
	flag.BoolVar(&legacyKeywordFields, "legacy-keyword-fields", true, "use legacy keyword subfields while reading the frozen target")
	flag.Parse()

	start, err := time.Parse(time.RFC3339, startText)
	if err != nil {
		fatalf("invalid --start: %v", err)
	}
	end, err := time.Parse(time.RFC3339, endText)
	if err != nil {
		fatalf("invalid --end: %v", err)
	}
	if strings.TrimSpace(output) == "" {
		fatalf("--output is required")
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		fatalf("resolve output: %v", err)
	}
	if _, err := os.Stat(outputPath); err == nil {
		fatalf("refusing to overwrite shadow manifest: %s", outputPath)
	} else if !os.IsNotExist(err) {
		fatalf("inspect output: %v", err)
	}
	if timeoutSeconds < 1 || timeoutSeconds > 300 {
		fatalf("--timeout-seconds must be between 1 and 300")
	}

	cfg, err := config.Load()
	if err != nil {
		fatalf("load config: %v", err)
	}
	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	clickHouse, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: cfg.ClickHouse.GetHosts(), Database: cfg.ClickHouse.GetDatabase(), Username: cfg.ClickHouse.GetUsername(), Password: cfg.ClickHouse.GetPassword(),
		MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Hour, DialTimeout: 10 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, logger)
	if err != nil {
		fatalf("connect ClickHouse: %v", err)
	}
	defer clickHouse.Close()
	openSearch, err := persistence.NewOpenSearchReconcileTarget(
		cfg.OpenSearch.Addresses, cfg.OpenSearch.Username, cfg.OpenSearch.Password,
		strings.TrimSpace(readTarget), strings.TrimSpace(writeAlias), true, legacyKeywordFields, logger,
	)
	if err != nil {
		fatalf("connect OpenSearch: %v", err)
	}
	defer openSearch.Close()
	metadata, err := openSearch.ProjectionMetadata(ctx)
	if err != nil {
		fatalf("read OpenSearch target metadata: %v", err)
	}
	writeIndices := make([]projection.ShadowWriteIndex, 0, len(metadata.WriteIndices))
	for _, index := range metadata.WriteIndices {
		writeIndices = append(writeIndices, projection.ShadowWriteIndex{Index: index.Index, IsWriteIndex: index.IsWriteIndex})
	}
	scope := persistence.ProjectionScope{
		TenantID: strings.TrimSpace(tenantID), StartTime: start, EndTime: end,
		TargetIndexVersion: strings.TrimSpace(writeAlias), MaxDocuments: maxDocuments,
	}
	manifest, err := projection.BuildShadowManifest(
		ctx,
		projection.ShadowConfig{MaxDocuments: 10_000, MaxWindow: time.Hour, MinimumWindowAge: 15 * time.Minute},
		repository.NewAlertRepository(clickHouse, logger), openSearch,
		projection.ShadowRequest{
			RequestedBy: requestedBy, TraceID: traceID, EnvironmentID: environmentID, Scope: scope,
			Target: projection.ShadowTargetMetadata{
				ClusterUUID: metadata.ClusterUUID, ReadTarget: metadata.ReadTarget,
				WriteAlias: metadata.WriteAlias, WriteIndices: writeIndices,
			},
		},
	)
	if err != nil {
		fatalf("build read-only shadow manifest: %v", err)
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatalf("encode shadow manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(outputPath, append(payload, '\n'), 0o644); err != nil {
		fatalf("write shadow manifest: %v", err)
	}
	fmt.Printf("status=%s approval_readiness=%s missing=%d stale=%d extra=%d binding_sha256=%s output=%s production_mutations=0\n",
		manifest.Status, manifest.ApprovalReadiness, manifest.MissingCount, manifest.StaleCount,
		manifest.ExtraCount, manifest.BindingSHA256, outputPath)
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
