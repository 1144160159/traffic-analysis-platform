package service

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

type fakeSystemSettingsStore struct {
	settings  *model.SystemSettings
	revision  int64
	updatedAt time.Time
	roles     []model.SystemRole
	tokens    model.SystemTokenSummary
	audit     []string
	auditErr  error
	missAt    time.Time
	missEvent string
	missTrace string
	supportAt time.Time
}

type fakeIntegrationChecker struct{ failures map[string]error }

func (f fakeIntegrationChecker) Check(_ context.Context, integration model.IntegrationSetting) error {
	return f.failures[integration.ID]
}

func (f *fakeSystemSettingsStore) GetTenant(context.Context, string) (string, string, error) {
	return "默认租户", "active", nil
}

func (f *fakeSystemSettingsStore) GetSettings(context.Context, string) (*model.SystemSettings, int64, time.Time, error) {
	return f.settings, f.revision, f.updatedAt, nil
}

func (f *fakeSystemSettingsStore) SaveSettingsWithAudit(_ context.Context, _ string, _ uuid.UUID, expectedRevision int64, settings model.SystemSettings, auditAction string, _ map[string]interface{}) (int64, time.Time, error) {
	if expectedRevision != f.revision {
		return 0, time.Time{}, commonerrors.New(commonerrors.ErrCodeVersionConflict, "system settings revision conflict")
	}
	if f.auditErr != nil {
		return 0, time.Time{}, f.auditErr
	}
	f.revision++
	f.settings = &settings
	f.updatedAt = time.Date(2026, 7, 20, 4, 0, 0, 0, time.UTC)
	f.audit = append(f.audit, auditAction)
	return f.revision, f.updatedAt, nil
}

func (f *fakeSystemSettingsStore) ListRoles(context.Context, string) ([]model.SystemRole, error) {
	return f.roles, nil
}

func (f *fakeSystemSettingsStore) GetTokenSummary(context.Context, string) (model.SystemTokenSummary, error) {
	return f.tokens, nil
}

func (f *fakeSystemSettingsStore) InsertAudit(_ context.Context, _, _, action string, _ map[string]interface{}) error {
	f.audit = append(f.audit, action)
	return nil
}

func (f *fakeSystemSettingsStore) RecordNavigationMiss(_ context.Context, _, _, eventID, traceID, _ string) (time.Time, string, error) {
	if f.auditErr != nil {
		return time.Time{}, "", f.auditErr
	}
	f.missEvent = eventID
	f.missTrace = traceID
	f.audit = append(f.audit, "navigation_not_found")
	if f.missAt.IsZero() {
		f.missAt = time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	}
	return f.missAt, f.missTrace, nil
}

func (f *fakeSystemSettingsStore) RecordNavigationSupportRequest(_ context.Context, _, _, eventID, traceID string) (time.Time, string, error) {
	if f.auditErr != nil {
		return time.Time{}, "", f.auditErr
	}
	f.missEvent = eventID
	f.missTrace = traceID
	f.audit = append(f.audit, "navigation_support_requested")
	if f.supportAt.IsZero() {
		f.supportAt = time.Date(2026, 7, 20, 8, 5, 0, 0, time.UTC)
	}
	return f.supportAt, f.missTrace, nil
}

func TestSystemSettingsWorkbenchUsesTenantDataAndDefaults(t *testing.T) {
	store := &fakeSystemSettingsStore{
		roles:  []model.SystemRole{{ID: "role-admin", Name: "admin", Permissions: []string{"*"}}},
		tokens: model.SystemTokenSummary{Total: 4, Active: 2, ExpiringSoon: 1, Revoked: 2},
	}
	service := NewSystemSettingsService(store, nil).WithIntegrationChecker(fakeIntegrationChecker{})
	service.clock = func() time.Time { return time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC) }

	workbench, err := service.GetWorkbench(context.Background(), "default")
	if err != nil {
		t.Fatalf("GetWorkbench() error = %v", err)
	}
	if workbench.TenantName != "默认租户" || workbench.Revision != 0 {
		t.Fatalf("unexpected tenant workbench: %#v", workbench)
	}
	if len(workbench.Settings.Sites) != 11 || len(workbench.Settings.Integrations) != 7 {
		t.Fatalf("default settings are incomplete: sites=%d integrations=%d", len(workbench.Settings.Sites), len(workbench.Settings.Integrations))
	}
	if workbench.Tokens.Active != 2 || len(workbench.Roles) != 1 {
		t.Fatalf("live aggregates missing: tokens=%#v roles=%#v", workbench.Tokens, workbench.Roles)
	}
}

func TestSystemSettingsUpdatePersistsRevisionAndAudit(t *testing.T) {
	store := &fakeSystemSettingsStore{}
	service := NewSystemSettingsService(store, nil)
	settings := model.DefaultSystemSettings()
	settings.Security.RefreshIntervalSec = 45

	workbench, err := service.UpdateSettings(context.Background(), "default", uuid.New(), UpdateSystemSettingsRequest{
		ExpectedRevision: 0,
		Settings:         settings,
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if workbench.Revision != 1 || store.settings.Security.RefreshIntervalSec != 45 {
		t.Fatalf("settings were not persisted: revision=%d settings=%#v", workbench.Revision, store.settings)
	}
	if len(store.audit) != 1 || store.audit[0] != "system_settings_update" {
		t.Fatalf("expected settings audit, got %#v", store.audit)
	}

	_, err = service.UpdateSettings(context.Background(), "default", uuid.New(), UpdateSystemSettingsRequest{
		ExpectedRevision: 0,
		Settings:         settings,
	})
	if !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestSystemSettingsUpdateDoesNotPersistWhenAtomicAuditFails(t *testing.T) {
	original := model.DefaultSystemSettings()
	store := &fakeSystemSettingsStore{
		settings: &original,
		revision: 5,
		auditErr: stderrors.New("audit insert failed"),
	}
	service := NewSystemSettingsService(store, nil)
	changed := original
	changed.Security.RefreshIntervalSec = 45

	_, err := service.UpdateSettings(context.Background(), "default", uuid.New(), UpdateSystemSettingsRequest{
		ExpectedRevision: 5,
		Settings:         changed,
	})
	if err == nil {
		t.Fatal("expected atomic audit failure")
	}
	if store.revision != 5 || store.settings.Security.RefreshIntervalSec == 45 || len(store.audit) != 0 {
		t.Fatalf("settings changed despite failed atomic audit: revision=%d settings=%#v audit=%#v", store.revision, store.settings, store.audit)
	}
}

func TestSystemSettingsActionsPersistConnectionTestAndReportSecurity(t *testing.T) {
	settings := model.DefaultSystemSettings()
	store := &fakeSystemSettingsStore{settings: &settings, revision: 3, updatedAt: time.Now()}
	service := NewSystemSettingsService(store, nil)
	service.clock = func() time.Time { return time.Date(2026, 7, 20, 5, 0, 0, 0, time.UTC) }
	userID := uuid.New()

	result, err := service.RunAction(context.Background(), "default", userID, "test-integration", SystemSettingsActionRequest{
		ExpectedRevision: 3,
		TargetID:         "kafka",
	})
	if err != nil {
		t.Fatalf("RunAction(test-integration) error = %v", err)
	}
	if result.Revision != 4 || len(result.Integrations) != 7 {
		t.Fatalf("unexpected connection result: %#v", result)
	}
	for _, integration := range store.settings.Integrations {
		if integration.ID == "kafka" && integration.LastTestedAt == nil {
			t.Fatal("kafka test timestamp was not persisted")
		}
	}

	auditResult, err := service.RunAction(context.Background(), "default", userID, "security-audit", SystemSettingsActionRequest{ExpectedRevision: 4})
	if err != nil {
		t.Fatalf("RunAction(security-audit) error = %v", err)
	}
	if auditResult.Status != "warning" || len(auditResult.Findings) == 0 {
		t.Fatalf("expected unisolated-site warning, got %#v", auditResult)
	}
	if len(store.audit) != 2 {
		t.Fatalf("expected action audits, got %#v", store.audit)
	}
}

func TestSystemSettingsRejectsInlineIntegrationSecrets(t *testing.T) {
	store := &fakeSystemSettingsStore{}
	service := NewSystemSettingsService(store, nil)
	settings := model.DefaultSystemSettings()
	settings.Integrations[0].SecretRef = "password=plain-text"

	_, err := service.UpdateSettings(context.Background(), "default", uuid.New(), UpdateSystemSettingsRequest{Settings: settings})
	if !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidParameter) {
		t.Fatalf("expected invalid parameter, got %v", err)
	}
}

func TestSystemSettingsConnectionTestReportsRealFailure(t *testing.T) {
	settings := model.DefaultSystemSettings()
	store := &fakeSystemSettingsStore{settings: &settings, revision: 2}
	service := NewSystemSettingsService(store, nil).WithIntegrationChecker(fakeIntegrationChecker{failures: map[string]error{"kafka": stderrors.New("connection refused")}})
	result, err := service.RunAction(context.Background(), "default", uuid.New(), "test-integration", SystemSettingsActionRequest{ExpectedRevision: 2, TargetID: "kafka"})
	if err != nil {
		t.Fatalf("RunAction() error = %v", err)
	}
	if result.Status != "warning" || len(result.Findings) != 1 || store.settings.Integrations[2].Status != "degraded" {
		t.Fatalf("connection failure was reported as success: %#v settings=%#v", result, store.settings.Integrations[2])
	}
}

func TestSystemSettingsScopeReviewDoesNotClaimMutation(t *testing.T) {
	settings := model.DefaultSystemSettings()
	store := &fakeSystemSettingsStore{settings: &settings, revision: 2, roles: []model.SystemRole{{Name: "legacy", Permissions: []string{"unknown:scope"}}}}
	service := NewSystemSettingsService(store, nil)
	result, err := service.RunAction(context.Background(), "default", uuid.New(), "scope-review", SystemSettingsActionRequest{ExpectedRevision: 2})
	if err != nil {
		t.Fatalf("RunAction() error = %v", err)
	}
	if result.Status != "warning" || len(result.Findings) != 1 || !strings.Contains(result.Message, "未执行权限变更") {
		t.Fatalf("scope review result is not truthful: %#v", result)
	}
}

func TestNavigationMissUsesTenantDataAndPersistsOpaqueAudit(t *testing.T) {
	settings := model.DefaultSystemSettings()
	store := &fakeSystemSettingsStore{settings: &settings}
	service := NewSystemSettingsService(store, nil)
	traceID := uuid.NewString()
	eventID := "nav-" + uuid.NewString()

	result, err := service.RecordNavigationMiss(context.Background(), "default", uuid.NewString(), traceID, "内网访问", NavigationMissRequest{EventID: eventID, Source: "web-ui"})
	if err != nil {
		t.Fatalf("RecordNavigationMiss() error = %v", err)
	}
	if !result.Persisted || result.TraceID != traceID || result.TenantName != "默认租户" || result.SiteName == "" {
		t.Fatalf("unexpected navigation context: %#v", result)
	}
	if store.missEvent != eventID || store.missTrace != traceID || len(result.Statuses) != 4 || len(store.audit) != 1 {
		t.Fatalf("navigation audit was not persisted exactly once: result=%#v store=%#v", result, store)
	}
}

func TestNavigationMissRejectsRawOrNonOpaqueIdentifiers(t *testing.T) {
	service := NewSystemSettingsService(&fakeSystemSettingsStore{}, nil)
	_, err := service.RecordNavigationMiss(context.Background(), "default", uuid.NewString(), uuid.NewString(), "内网访问", NavigationMissRequest{EventID: "/internal/admin", Source: "web-ui"})
	if !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidParameter) {
		t.Fatalf("expected invalid parameter for raw route, got %v", err)
	}
}

func TestNavigationSupportRequestPersistsObservableQueueRecord(t *testing.T) {
	store := &fakeSystemSettingsStore{}
	service := NewSystemSettingsService(store, nil)
	eventID := "nav-" + uuid.NewString()
	traceID := uuid.NewString()

	result, err := service.RecordNavigationSupportRequest(context.Background(), "default", uuid.NewString(), traceID, NavigationSupportRequest{EventID: eventID})
	if err != nil {
		t.Fatalf("RecordNavigationSupportRequest() error = %v", err)
	}
	if !result.Persisted || result.Status != "queued" || result.Queue != "平台值班管理员" || result.TraceID != traceID {
		t.Fatalf("unexpected support context: %#v", result)
	}
	if result.NavigationEventID != eventID || result.SupportRequestID != "support-"+strings.TrimPrefix(eventID, "nav-") || len(store.audit) != 1 {
		t.Fatalf("support audit was not persisted: result=%#v store=%#v", result, store)
	}
}
