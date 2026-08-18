package service

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func TestRegisterModelVersionPostgresAtomicity(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MODEL_REGISTRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("MODEL_REGISTRATION_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "model_registration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	defer admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`)

	scopedDSN, err := postgresDSNWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := createModelRegistrationFixture(ctx, db); err != nil {
		t.Fatal(err)
	}
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), DefaultModelServiceConfig())
	req := governedRegistrationRequest()
	opCtx := &OperationContext{TenantID: req.TenantID, UserID: "user-a", Username: "mlops", Authenticated: true}

	first, err := service.RegisterModelVersion(ctx, req, opCtx)
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}
	second, err := service.RegisterModelVersion(ctx, req, opCtx)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if first.RegistrationRequestSHA256 != second.RegistrationRequestSHA256 {
		t.Fatal("idempotent replay returned a different registration identity")
	}
	assertTableCount(t, db, "models", 1)
	assertTableCount(t, db, "model_versions", 1)
	assertTableCount(t, db, "audit_logs", 1)
	assertTableCount(t, db, "model_update_outbox", 0)

	conflict := *req
	conflict.PackageSHA256 = strings.Repeat("b", 64)
	if _, err := service.RegisterModelVersion(ctx, &conflict, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	if _, err := db.ExecContext(ctx, `DROP TABLE audit_logs`); err != nil {
		t.Fatal(err)
	}
	rollbackReq := *req
	rollbackReq.Version = "v1.2.4"
	rollbackReq.IdempotencyKey = "mlops-registration-000002"
	rollbackReq.PackageSHA256 = strings.Repeat("c", 64)
	if _, err := service.RegisterModelVersion(ctx, &rollbackReq, opCtx); err == nil {
		t.Fatal("registration succeeded after transactional audit storage was removed")
	}
	assertTableCount(t, db, "model_versions", 1)
	assertTableCount(t, db, "model_update_outbox", 0)
}

func postgresDSNWithSearchPath(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse PostgreSQL DSN: %w", err)
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func createModelRegistrationFixture(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE tenants (tenant_id TEXT PRIMARY KEY)`,
		`CREATE TABLE users (user_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL)`,
		`CREATE TABLE feature_sets (feature_set_id TEXT PRIMARY KEY)`,
		`CREATE TABLE models (model_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL, model_type TEXT NOT NULL, description TEXT, metadata JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name))`,
		`CREATE TABLE model_versions (model_version TEXT PRIMARY KEY, model_id TEXT NOT NULL REFERENCES models(model_id), tenant_id TEXT NOT NULL, feature_set_id TEXT NOT NULL, artifact_uri TEXT NOT NULL, artifact_manifest_uri TEXT NOT NULL DEFAULT '', package_id TEXT NOT NULL DEFAULT '', package_sha256 TEXT NOT NULL DEFAULT '', artifact_manifest_sha256 TEXT NOT NULL DEFAULT '', evaluation_sha256 TEXT NOT NULL DEFAULT '', explanation_sha256 TEXT NOT NULL DEFAULT '', graph_snapshot_id TEXT NOT NULL DEFAULT '', graph_snapshot_sha256 TEXT NOT NULL DEFAULT '', signing_key_id TEXT NOT NULL DEFAULT '', compatibility JSONB NOT NULL DEFAULT '{}'::jsonb, revision BIGINT NOT NULL DEFAULT 1, registration_idempotency_key TEXT NOT NULL DEFAULT '', registration_request_sha256 TEXT NOT NULL DEFAULT '', metrics JSONB NOT NULL DEFAULT '{}'::jsonb, status TEXT NOT NULL DEFAULT 'registered', created_by TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE UNIQUE INDEX uq_registration_idempotency ON model_versions(tenant_id,registration_idempotency_key) WHERE registration_idempotency_key <> ''`,
		`CREATE TABLE audit_logs (id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, user_id TEXT, action TEXT NOT NULL, object_type TEXT NOT NULL, object_id TEXT, detail JSONB NOT NULL, ip_addr TEXT, user_agent TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE model_update_outbox (id BIGSERIAL PRIMARY KEY)`,
		`INSERT INTO tenants VALUES ('tenant-a')`,
		`INSERT INTO users VALUES ('user-a','tenant-a')`,
		`INSERT INTO feature_sets VALUES ('feature-v1')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("fixture statement failed: %w", err)
		}
	}
	return nil
}

func assertTableCount(t *testing.T, db *sql.DB, table string, expected int) {
	t.Helper()
	var actual int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&actual); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if actual != expected {
		t.Fatalf("%s count=%d, want %d", table, actual, expected)
	}
}
