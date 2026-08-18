package apitoken

import (
	"context"
	"testing"

	"net/http/httptest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/security"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
)

func TestCandidatePrefixes(t *testing.T) {
	got := candidatePrefixes("tap_abcdefgh12345678_wxyz9876")
	if len(got) != 1 || got[0] != "tap_abcdefgh" {
		t.Fatalf("candidatePrefixes = %v, want [tap_abcdefgh]", got)
	}
	got = candidatePrefixes("api_meusorcohatynt4_2q86_csszr")
	if len(got) != 1 || got[0] != "api_meusorco" {
		t.Fatalf("legacy candidatePrefixes = %v", got)
	}
	if len(candidatePrefixes("jwt.token.value")) != 0 {
		// 随机段过短不构成合法 API Key
		t.Fatalf("jwt-like input produced candidates")
	}
	if len(candidatePrefixes("nodash")) != 0 {
		t.Fatal("no underscore should yield no candidates")
	}
}

func TestLooksLikeAPIKey(t *testing.T) {
	cases := map[string]bool{
		"tap_abcdefgh1234_wxyz5678": true,
		"api_3igelzat529uxt":        true,
		"probe-token-default-001":   false, // 连字符格式非 API Key
		"eyJhbGciOi.abc.def":        false, // JWT 形态
		"":                          false,
	}
	for in, want := range cases {
		if got := LooksLikeAPIKey(in); got != want {
			t.Errorf("LooksLikeAPIKey(%q)=%v want %v", in, got, want)
		}
	}
}

func TestValidatorValidate(t *testing.T) {
	// 现役契约:sha256 十六进制(与 TokenService 存储一致)
	hasher := security.NewTokenHasher()
	plain := "api_meusorcohatynt4_2q86_csszr2tbalw3hcrg4cn1"
	hash, err := hasher.HashToken(plain)
	if err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	tokenID := uuid.New()
	rows := sqlmock.NewRows([]string{
		"token_id", "tenant_id", "user_id", "name", "description", "token_type",
		"token_hash", "token_prefix", "scopes", "status", "expires_at",
		"last_used_at", "usage_count", "created_by", "created_at", "updated_at",
		"revoked_at", "rotation_enabled", "rotation_interval", "last_rotated_at",
		"previous_token_id", "ip_whitelist", "metadata", "probe_id",
	}).AddRow(
		tokenID, "default", nil, "ci-token", "test", "api",
		hash, "api_meusorcohatynt", "[\"alert:read\"]", "active", nil,
		nil, 0, nil, time.Now(), time.Now(),
		nil, false, nil, nil, nil, "", "{}", "",
	)
	mock.ExpectQuery("LIKE token_prefix").WillReturnRows(rows)

	validator := NewValidator(repository.NewTokenRepository(db, zap.NewNop()), zap.NewNop())
	principal, err := validator.Validate(context.Background(), nil, plain)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Kind != authz.PrincipalKindAPIToken {
		t.Fatalf("kind = %q", principal.Kind)
	}
	if principal.Subject != "api-token:"+tokenID.String() {
		t.Fatalf("subject = %q", principal.Subject)
	}
	if principal.TenantID != "default" || principal.Username != "ci-token" {
		t.Fatalf("principal = %+v", principal)
	}
	if len(principal.Permissions) != 1 || principal.Permissions[0] != "alert:read" {
		t.Fatalf("permissions = %v", principal.Permissions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// 错 token:bcrypt 不匹配 → invalid
	if _, err := validator.Validate(context.Background(), nil, "api_meusorcohatynt4_2q86_othervalue"); err == nil {
		t.Fatal("mismatched token must fail")
	}
}

func TestValidatorValidateNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("LIKE token_prefix").WillReturnRows(sqlmock.NewRows([]string{
		"token_id", "tenant_id", "user_id", "name", "description", "token_type",
		"token_hash", "token_prefix", "scopes", "status", "expires_at",
		"last_used_at", "usage_count", "created_by", "created_at", "updated_at",
		"revoked_at", "rotation_enabled", "rotation_interval", "last_rotated_at",
		"previous_token_id", "ip_whitelist", "metadata", "probe_id",
	}))
	validator := NewValidator(repository.NewTokenRepository(db, zap.NewNop()), zap.NewNop())
	if _, err := validator.Validate(context.Background(), nil, "api_meusorcohatynt4_2q86_csszr2tbalw3hcrg4cn1"); err == nil {
		t.Fatal("no rows must fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidatorIPWhitelistEnforced(t *testing.T) {
	hasher := security.NewTokenHasher()
	plain := "api_meusorcohatynt4_2q86_csszr2tbalw3hcrg4cn1"
	hash, err := hasher.HashToken(plain)
	if err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tokenID := uuid.New()
	rows := sqlmock.NewRows([]string{
		"token_id", "tenant_id", "user_id", "name", "description", "token_type",
		"token_hash", "token_prefix", "scopes", "status", "expires_at",
		"last_used_at", "usage_count", "created_by", "created_at", "updated_at",
		"revoked_at", "rotation_enabled", "rotation_interval", "last_rotated_at",
		"previous_token_id", "ip_whitelist", "metadata", "probe_id",
	}).AddRow(
		tokenID, "default", nil, "ci-token", "test", "api",
		hash, "api_meusorcohatynt", "[\"alert:read\"]", "active", nil,
		nil, 0, nil, time.Now(), time.Now(),
		nil, false, nil, nil, nil, "[\"10.0.0.1\"]", "{}", "",
	)
	mock.ExpectQuery("LIKE token_prefix").WillReturnRows(rows)
	rows2 := sqlmock.NewRows([]string{
		"token_id", "tenant_id", "user_id", "name", "description", "token_type",
		"token_hash", "token_prefix", "scopes", "status", "expires_at",
		"last_used_at", "usage_count", "created_by", "created_at", "updated_at",
		"revoked_at", "rotation_enabled", "rotation_interval", "last_rotated_at",
		"previous_token_id", "ip_whitelist", "metadata", "probe_id",
	}).AddRow(
		tokenID, "default", nil, "ci-token", "test", "api",
		hash, "api_meusorcohatynt", "[\"alert:read\"]", "active", nil,
		nil, 0, nil, time.Now(), time.Now(),
		nil, false, nil, nil, nil, "[\"10.0.0.1\"]", "{}", "",
	)
	mock.ExpectQuery("LIKE token_prefix").WillReturnRows(rows2)

	validator := NewValidator(repository.NewTokenRepository(db, zap.NewNop()), zap.NewNop())
	// 白名单外的来源 IP → 拒绝(fail-closed)
	req := httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	if _, err := validator.Validate(context.Background(), req, plain); err == nil {
		t.Fatal("ip whitelist must reject non-listed source ip")
	}
	// 白名单内来源 IP → 通过
	req = httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	if _, err := validator.Validate(context.Background(), req, plain); err != nil {
		t.Fatalf("whitelisted ip must pass: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
