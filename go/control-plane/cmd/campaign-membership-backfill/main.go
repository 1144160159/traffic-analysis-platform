package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func main() {
	os.Exit(run())
}

func run() int {
	manifestPath := flag.String("manifest", "", "path to the immutable ClickHouse export manifest")
	dsnEnvironment := flag.String("dsn-env", "POSTGRES_DSN", "environment variable containing the PostgreSQL DSN")
	flag.Parse()
	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "-manifest is required")
		return 2
	}
	dsn := os.Getenv(*dsnEnvironment)
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", *dsnEnvironment)
		return 2
	}
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		return 2
	}
	var manifest api.CampaignMembershipBackfillManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		fmt.Fprintf(os.Stderr, "decode manifest: %v\n", err)
		return 2
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, "decode manifest: trailing JSON value")
		return 2
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open PostgreSQL: %v\n", err)
		return 1
	}
	defer db.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "connect PostgreSQL: %v\n", err)
		return 1
	}
	result, runErr := api.RunCampaignMembershipBackfill(ctx, db, zap.NewNop(), manifest)
	encoded, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", marshalErr)
		return 1
	}
	fmt.Println(string(encoded))
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "backfill: %v\n", runErr)
		return 1
	}
	return 0
}
