package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphnebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
	graphprojection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
)

type output struct {
	Mode   string                             `json:"mode"`
	Before graphprojection.ReconcileManifest  `json:"before"`
	After  *graphprojection.ReconcileManifest `json:"after,omitempty"`
}

func main() { os.Exit(run()) }

func run() int {
	mode := flag.String("mode", "compare", "compare or repair")
	runID := flag.String("run-id", uuid.NewString(), "immutable reconcile run UUID")
	tenantID := flag.String("tenant", "", "required tenant ID")
	windowFrom := flag.String("window-from", "", "required closed-window RFC3339 start")
	windowThrough := flag.String("window-through", "", "required closed-window RFC3339 end")
	maxFacts := flag.Int("max-facts", 10000, "maximum authority or target facts")
	maxDuration := flag.Duration("max-duration", 30*time.Second, "combined query deadline")
	requestedBy := flag.String("requested-by", "", "repair requester identity")
	approvedBy := flag.String("approved-by", "", "repair approver identity, distinct from requester")
	approvedAtText := flag.String("approved-at", "", "repair approval RFC3339 timestamp")
	repairMaxItems := flag.Int("repair-max-items", 0, "approved missing/stale repair limit")
	confirmRepair := flag.Bool("confirm-repair", false, "required explicit repair confirmation")
	flag.Parse()

	if *mode != "compare" && *mode != "repair" {
		return usageError("--mode must be compare or repair")
	}
	if uuid.Validate(*runID) != nil || strings.TrimSpace(*tenantID) == "" {
		return usageError("--run-id must be a UUID and --tenant is required")
	}
	from, err := time.Parse(time.RFC3339, *windowFrom)
	if err != nil {
		return usageError("--window-from must be RFC3339")
	}
	through, err := time.Parse(time.RFC3339, *windowThrough)
	if err != nil {
		return usageError("--window-through must be RFC3339")
	}
	var approvedAt time.Time
	if *mode == "repair" {
		if !*confirmRepair || *repairMaxItems <= 0 {
			return usageError("repair requires --confirm-repair and positive --repair-max-items")
		}
		approvedAt, err = time.Parse(time.RFC3339, *approvedAtText)
		if err != nil {
			return usageError("repair requires RFC3339 --approved-at")
		}
	}
	scope := graphprojection.ReconcileScope{
		TenantID: *tenantID, WindowFrom: from, WindowThrough: through,
		MaxFacts: *maxFacts, MaxDuration: *maxDuration,
	}

	cfg, err := config.Load()
	if err != nil {
		return fail("load graph configuration", err)
	}
	if !cfg.Nebula.Enabled {
		return fail("validate target", fmt.Errorf("NEBULA_ENABLED=true is required"))
	}
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if dsn == "" {
		return fail("validate PostgreSQL configuration", fmt.Errorf("POSTGRES_DSN is required"))
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fail("open PostgreSQL", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fail("connect PostgreSQL", err)
	}
	repository, err := graphprojection.NewPostgresReconcileRepository(db)
	if err != nil {
		return fail("create reconcile repository", err)
	}
	if err := repository.VerifySchema(ctx); err != nil {
		return fail("verify reconcile schema", err)
	}
	logger := zap.NewExample()
	defer logger.Sync()
	store, err := graphnebula.NewWorkbenchStore(cfg.Nebula, logger)
	if err != nil {
		return fail("connect NebulaGraph", err)
	}
	defer store.Close()
	if err := store.ReadyProjection(ctx); err != nil {
		return fail("verify NebulaGraph projection target", err)
	}
	target, err := graphnebula.NewProjectionTarget(store)
	if err != nil {
		return fail("create graph projection target", err)
	}
	service, err := graphprojection.NewReconcileService(repository, store, repository, target, repository)
	if err != nil {
		return fail("create graph reconcile service", err)
	}
	before, err := service.Compare(ctx, *runID, "before", scope)
	if err != nil {
		return fail("compare graph projection", err)
	}
	result := output{Mode: *mode, Before: before}
	if *mode == "repair" {
		after, repairErr := service.Repair(ctx, before, graphprojection.RepairAuthorization{
			RunID: *runID, RequestedBy: strings.TrimSpace(*requestedBy), ApprovedBy: strings.TrimSpace(*approvedBy),
			ApprovedAt: approvedAt, MaxItems: *repairMaxItems,
		})
		result.After = &after
		if repairErr != nil {
			_ = encode(result)
			return fail("repair graph projection", repairErr)
		}
	}
	if err := encode(result); err != nil {
		return fail("encode graph reconcile result", err)
	}
	return 0
}

func encode(value output) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usageError(message string) int {
	_, _ = fmt.Fprintln(os.Stderr, message)
	return 2
}

func fail(operation string, err error) int {
	_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	return 1
}
