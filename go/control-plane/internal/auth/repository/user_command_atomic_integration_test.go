package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	authRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

const (
	userAtomicTenantA = "auth-user-atomic-a"
	userAtomicTenantB = "auth-user-atomic-b"
)

type recordingUserEventProducer struct{ count int }

func (p *recordingUserEventProducer) SendProto(_ context.Context, _ string, _ proto.Message, _ ...commonkafka.MessageHeader) error {
	p.count++
	return nil
}

func TestUserCommandAtomicPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("AUTH_USER_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("AUTH_USER_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	if err := cleanupUserAtomicTenants(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupUserAtomicTenants(db); err != nil {
			t.Errorf("cleanup user atomic tenants: %v", err)
		}
	}()
	for _, tenantID := range []string{userAtomicTenantA, userAtomicTenantB} {
		if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "User Atomic "+tenantID); err != nil {
			t.Fatal(err)
		}
		for _, role := range []string{"viewer", "admin"} {
			if _, err := db.Exec(`INSERT INTO roles(role_id,tenant_id,name) VALUES($1,$2,$3)`, uuid.New(), tenantID, role); err != nil {
				t.Fatal(err)
			}
		}
	}
	repo := authRepository.NewUserRepository(db, zap.NewNop())
	local := &model.User{TenantID: userAtomicTenantA, Username: "local-analyst", Email: "local@example.test"}
	if err := repo.Create(context.Background(), local, "initial-secret-123"); err != nil {
		t.Fatalf("create local user: %v", err)
	}
	expectedOne := int64(1)
	profileMeta := authRepository.UserCommandMetadata{
		TenantID: userAtomicTenantA, ActorID: local.UserID, ActionID: authRepository.UserProfileUpdateAction,
		Reason: "integration profile update", IdempotencyKey: "auth-profile-integration-key", ExpectedRevision: &expectedOne,
		TraceID: "trace-auth-profile-integration",
	}
	profile, err := repo.UpdateProfileAtomic(context.Background(), local.UserID, "updated@example.test", profileMeta)
	if err != nil || profile.Revision != 2 || profile.IdempotentReuse {
		t.Fatalf("profile result=%+v err=%v", profile, err)
	}
	replay, err := repo.UpdateProfileAtomic(context.Background(), local.UserID, "updated@example.test", profileMeta)
	if err != nil || !replay.IdempotentReuse || replay.Revision != 2 {
		t.Fatalf("profile replay=%+v err=%v", replay, err)
	}
	if _, err := repo.UpdateProfileAtomic(context.Background(), local.UserID, "different@example.test", profileMeta); err == nil {
		t.Fatal("expected profile idempotency conflict")
	}
	expectedTwo := int64(2)
	passwordMeta := authRepository.UserCommandMetadata{
		TenantID: userAtomicTenantA, ActorID: local.UserID, ActionID: authRepository.UserPasswordChangeAction,
		Reason: "integration password change", IdempotencyKey: "auth-password-integration-key", ExpectedRevision: &expectedTwo,
		TraceID: "trace-auth-password-integration",
	}
	passwordResult, err := repo.ChangePasswordAtomic(context.Background(), local.UserID, "initial-secret-123", "rotated-secret-456", passwordMeta)
	if err != nil || passwordResult.Revision != 3 {
		t.Fatalf("password result=%+v err=%v", passwordResult, err)
	}
	updatedLocal, err := repo.GetByID(context.Background(), local.UserID)
	if err != nil || !repo.VerifyPassword(updatedLocal, "rotated-secret-456") {
		t.Fatalf("rotated password verification err=%v", err)
	}

	claims := &model.OIDCClaims{Subject: "oidc-subject-integration", Email: "oidc@example.test", PreferredUsername: "oidc-user"}
	oidcMeta := authRepository.UserCommandMetadata{TenantID: userAtomicTenantA, ActionID: authRepository.UserOIDCSyncAction,
		Reason: "integration OIDC sync", IdempotencyKey: "auth-oidc-create-integration", TraceID: "trace-auth-oidc-create"}
	oidcUser, oidcResult, err := repo.SyncOIDCUserAtomic(context.Background(), claims, []string{"viewer", "admin"}, oidcMeta)
	if err != nil || oidcResult.Revision != 1 || oidcUser.TenantID != userAtomicTenantA {
		t.Fatalf("OIDC create user=%+v result=%+v err=%v", oidcUser, oidcResult, err)
	}
	_, oidcReplay, err := repo.SyncOIDCUserAtomic(context.Background(), claims, []string{"admin", "viewer"}, oidcMeta)
	if err != nil || !oidcReplay.IdempotentReuse || oidcReplay.Revision != 1 {
		t.Fatalf("OIDC reordered replay=%+v err=%v", oidcReplay, err)
	}
	crossTenant := oidcMeta
	crossTenant.TenantID = userAtomicTenantB
	crossTenant.IdempotencyKey = "auth-oidc-cross-tenant-integration"
	if _, _, err := repo.SyncOIDCUserAtomic(context.Background(), claims, []string{"admin"}, crossTenant); err == nil {
		t.Fatal("expected cross-tenant OIDC identity rejection")
	}
	oidcUpdate := oidcMeta
	oidcUpdate.IdempotencyKey = "auth-oidc-update-integration"
	oidcUpdate.TraceID = "trace-auth-oidc-update"
	_, oidcUpdated, err := repo.SyncOIDCUserAtomic(context.Background(), claims, []string{"admin"}, oidcUpdate)
	if err != nil || oidcUpdated.Revision != 2 {
		t.Fatalf("OIDC update=%+v err=%v", oidcUpdated, err)
	}
	roles, err := repo.GetUserRoles(context.Background(), oidcUser.UserID)
	if err != nil || len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("OIDC roles=%v err=%v", roles, err)
	}

	var history, requests, outbox, audits, leaked int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM user_command_history WHERE tenant_id=$1),
		(SELECT count(*) FROM user_command_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM user_command_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action LIKE 'auth-user-%'),
		(SELECT count(*) FROM user_command_history h FULL JOIN user_command_outbox o ON false
		 WHERE (h.tenant_id=$1 AND h.new_value::text LIKE '%rotated-secret-456%')
		    OR (o.tenant_id=$1 AND o.payload::text LIKE '%rotated-secret-456%'))`, userAtomicTenantA).
		Scan(&history, &requests, &outbox, &audits, &leaked); err != nil {
		t.Fatal(err)
	}
	if history != 5 || requests != 5 || outbox != 5 || audits != 5 || leaked != 0 {
		t.Fatalf("counts history=%d requests=%d outbox=%d audits=%d leaked=%d", history, requests, outbox, audits, leaked)
	}
	producer := &recordingUserEventProducer{}
	worker := authRepository.NewUserSettingsOutboxWorker(db, producer, zap.NewNop())
	processed, err := worker.DrainUserCommands(context.Background(), 20)
	if err != nil || processed != outbox || producer.count != outbox {
		t.Fatalf("drain processed=%d produced=%d outbox=%d err=%v", processed, producer.count, outbox, err)
	}
	var published int
	if err := db.QueryRow(`SELECT count(*) FROM user_command_outbox WHERE tenant_id=$1 AND status='published'`, userAtomicTenantA).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != outbox {
		t.Fatalf("published=%d want=%d", published, outbox)
	}
}

func cleanupUserAtomicTenants(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM user_command_requests WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM user_command_outbox WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM user_command_history WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM audit_logs WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM tenants WHERE tenant_id IN ($1,$2)`,
	} {
		if _, err := tx.Exec(query, userAtomicTenantA, userAtomicTenantB); err != nil {
			return err
		}
	}
	return tx.Commit()
}
