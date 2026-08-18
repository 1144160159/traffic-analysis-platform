package service

import (
	"context"
	"strings"
	"testing"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestRollbackRuleCommitsCASVersionSnapshotAndOutboxAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(5, "current", "tenant-1", now.Add(-time.Hour))
	targetRule := rollbackTestRule(2, "restored", "tenant-1", now.Add(-2*time.Hour))
	target := rollbackTestVersion(t, targetRule)
	expectRuleRollbackReads(mock, current, target)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE rules").
		WithArgs(
			targetRule.Name, targetRule.Type, targetRule.Engine, targetRule.Description,
			sqlmock.AnyArg(), sqlmock.AnyArg(), targetRule.Severity, targetRule.Enabled,
			targetRule.Priority, int64(6), targetRule.Status, "operator-1", sqlmock.AnyArg(),
			current.RuleID, int64(5),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO rule_versions").
		WithArgs(
			"rule-1-v6", "rule-1", "tenant-1", int64(6), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"rollback", "restored from version 2: approved rollback", "operator-1", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO rule_outbox").
		WithArgs("rule-1", "update", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc := NewRuleService(repository.NewRuleRepositoryWithDB(db, zap.NewNop()), nil, zap.NewNop())
	result, err := svc.RollbackRule(context.Background(), "rule-1", 2, 5, "approved rollback", &OperationContext{
		TenantID: "tenant-1",
		UserID:   "operator-1",
	})
	if err != nil {
		t.Fatalf("RollbackRule() error = %v", err)
	}
	if result.Rule.Version != 6 || result.Rule.Name != "restored" {
		t.Fatalf("restored rule = %#v", result.Rule)
	}
	if result.RuntimeStatus != "pending" || result.ExpectedAcks != 4 || result.EventID == "" {
		t.Fatalf("rollback receipt = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackRuleRollsBackCASWhenVersionSnapshotCannotBeInserted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(5, "current", "tenant-1", now.Add(-time.Hour))
	targetRule := rollbackTestRule(2, "restored", "tenant-1", now.Add(-2*time.Hour))
	target := rollbackTestVersion(t, targetRule)
	expectRuleRollbackReads(mock, current, target)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE rules").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO rule_versions").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	svc := NewRuleService(repository.NewRuleRepositoryWithDB(db, zap.NewNop()), nil, zap.NewNop())
	_, err = svc.RollbackRule(context.Background(), "rule-1", 2, 5, "approved rollback", &OperationContext{
		TenantID: "tenant-1",
		UserID:   "operator-1",
	})
	if err == nil {
		t.Fatal("RollbackRule() error = nil, want version insert failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetRuleApplicationStatusDistinguishesBrokerFromRuntimeApplication(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(6, "restored", "tenant-1", now.Add(-time.Hour))
	expectCurrentRuleRead(mock, current)
	eventID := "11111111-1111-4111-8111-111111111111"
	mock.ExpectQuery("SELECT[[:space:]]+o.published").
		WithArgs("rule-1", eventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"published", "runtime_status", "runtime_applied_at", "runtime_last_error", "version", "action",
			"received", "successful", "stale", "conflict", "parallelism", "current_version",
		}).AddRow(true, "partial", nil, "", int64(6), "update", 2, 2, 0, 0, 4, int64(6)))

	svc := &RuleService{
		repo:   repository.NewRuleRepositoryWithDB(db, zap.NewNop()),
		db:     db,
		config: DefaultRuleServiceConfig(),
		logger: zap.NewNop(),
	}
	status, err := svc.GetRuleApplicationStatus(context.Background(), "rule-1", eventID, &OperationContext{
		TenantID: "tenant-1",
		UserID:   "operator-1",
	})
	if err != nil {
		t.Fatalf("GetRuleApplicationStatus() error = %v", err)
	}
	if !status.BrokerPublished || status.RuntimeStatus != "partial" || status.ReceivedAcks != 2 || status.ExpectedAcks != 4 {
		t.Fatalf("application status = %#v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRuleRollbackCandidateCreatesNewMonotonicVersion(t *testing.T) {
	createdAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	rollbackAt := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(5, "current_name", "tenant-1", createdAt)
	current.CreatedBy = "original-owner"
	targetRule := rollbackTestRule(2, "restored_name", "tenant-1", createdAt.Add(time.Hour))
	targetRule.Description = "historical definition"
	target := rollbackTestVersion(t, targetRule)

	restored, err := buildRuleRollbackCandidate(current, target, 5, "rollback-operator", rollbackAt)
	if err != nil {
		t.Fatalf("buildRuleRollbackCandidate() error = %v", err)
	}
	if restored.Version != 6 {
		t.Fatalf("restored version = %d, want 6", restored.Version)
	}
	if restored.Name != targetRule.Name || restored.Description != targetRule.Description {
		t.Fatalf("historical definition was not restored: %#v", restored)
	}
	if restored.RuleID != current.RuleID || restored.TenantID != current.TenantID ||
		restored.CreatedBy != current.CreatedBy || !restored.CreatedAt.Equal(current.CreatedAt) {
		t.Fatalf("immutable ownership was not preserved: %#v", restored)
	}
	if restored.UpdatedBy != "rollback-operator" || !restored.UpdatedAt.Equal(rollbackAt) {
		t.Fatalf("rollback provenance = %q %s", restored.UpdatedBy, restored.UpdatedAt)
	}
}

func TestBuildRuleRollbackCandidateRejectsConcurrentOrNonHistoricalTarget(t *testing.T) {
	now := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(5, "current", "tenant-1", now.Add(-time.Hour))
	targetRule := rollbackTestRule(2, "target", "tenant-1", now.Add(-2*time.Hour))
	target := rollbackTestVersion(t, targetRule)

	_, err := buildRuleRollbackCandidate(current, target, 4, "operator", now)
	if commonerrors.GetCode(err) != commonerrors.ErrCodeVersionConflict {
		t.Fatalf("concurrent version error = %v, code = %s", err, commonerrors.GetCode(err))
	}

	target.Version = current.Version
	_, err = buildRuleRollbackCandidate(current, target, current.Version, "operator", now)
	if commonerrors.GetCode(err) != commonerrors.ErrCodeInvalidStateTransition {
		t.Fatalf("non-historical target error = %v, code = %s", err, commonerrors.GetCode(err))
	}
}

func TestBuildRuleRollbackCandidateRejectsTamperedOrCrossTenantSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)
	current := rollbackTestRule(5, "current", "tenant-1", now.Add(-time.Hour))
	targetRule := rollbackTestRule(2, "target", "tenant-1", now.Add(-2*time.Hour))
	target := rollbackTestVersion(t, targetRule)
	target.ContentURI += " "

	_, err := buildRuleRollbackCandidate(current, target, current.Version, "operator", now)
	if commonerrors.GetCode(err) != commonerrors.ErrCodeInvalidStateTransition || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered target error = %v", err)
	}

	target = rollbackTestVersion(t, targetRule)
	target.TenantID = "tenant-2"
	_, err = buildRuleRollbackCandidate(current, target, current.Version, "operator", now)
	if commonerrors.GetCode(err) != commonerrors.ErrCodeInvalidStateTransition || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-tenant target error = %v", err)
	}
}

func rollbackTestRule(version int64, name, tenantID string, timestamp time.Time) *model.Rule {
	return &model.Rule{
		RuleID:     "rule-1",
		TenantID:   tenantID,
		Name:       name,
		Type:       string(model.RuleTypeBlacklist),
		Engine:     string(model.EngineInternal),
		Conditions: map[string]interface{}{"domain": "blocked.example"},
		Labels:     []string{"dns"},
		Severity:   string(model.SeverityHigh),
		Enabled:    true,
		Priority:   80,
		Version:    version,
		Status:     string(model.RuleStatusActive),
		CreatedBy:  "creator",
		UpdatedBy:  "editor",
		CreatedAt:  timestamp,
		UpdatedAt:  timestamp,
	}
}

func rollbackTestVersion(t *testing.T, rule *model.Rule) *model.RuleVersion {
	t.Helper()
	contentURI, checksum, err := model.EncodeRuleVersionSnapshot(rule)
	if err != nil {
		t.Fatal(err)
	}
	return &model.RuleVersion{
		RuleVersionID: rule.RuleID + "-v2",
		RuleID:        rule.RuleID,
		TenantID:      rule.TenantID,
		Version:       rule.Version,
		ContentURI:    contentURI,
		Checksum:      checksum,
		Status:        string(model.VersionStatusActive),
		CreatedBy:     rule.UpdatedBy,
		CreatedAt:     rule.UpdatedAt,
	}
}

func expectRuleRollbackReads(mock sqlmock.Sqlmock, current *model.Rule, target *model.RuleVersion) {
	expectCurrentRuleRead(mock, current)
	mock.ExpectQuery("SELECT rule_version, rule_id, tenant_id").
		WithArgs(target.RuleVersionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"rule_version", "rule_id", "tenant_id", "version", "content_uri", "checksum", "status", "change_log", "created_by", "created_at",
		}).AddRow(
			target.RuleVersionID, target.RuleID, target.TenantID, target.Version, target.ContentURI,
			target.Checksum, target.Status, target.ChangeLog, target.CreatedBy, target.CreatedAt,
		))
}

func expectCurrentRuleRead(mock sqlmock.Sqlmock, current *model.Rule) {
	mock.ExpectQuery("SELECT rule_id, tenant_id, name").
		WithArgs(current.RuleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"rule_id", "tenant_id", "name", "rule_type", "engine", "description", "conditions", "labels",
			"severity", "enabled", "priority", "version", "status", "created_by", "updated_by", "created_at", "updated_at",
		}).AddRow(
			current.RuleID, current.TenantID, current.Name, current.Type, current.Engine, current.Description,
			`{"domain":"current.example"}`, `{"dns"}`, current.Severity, current.Enabled, current.Priority,
			current.Version, current.Status, current.CreatedBy, current.UpdatedBy, current.CreatedAt, current.UpdatedAt,
		))
}
