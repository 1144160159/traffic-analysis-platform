package service

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func governedRegistrationRequest() *model.RegisterModelRequest {
	expectedRevision := int64(0)
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return &model.RegisterModelRequest{
		ModelID: "behavior-classifier", ModelType: "onnx", Version: "v1.2.3",
		ArtifactURI:         "s3://traffic-models/package/baseline-model.onnx",
		ArtifactManifestURI: "s3://traffic-models/package/model-artifact-manifest.json",
		PackageID:           "76c8debb-d938-596c-b14d-183d20799ef7", PackageSHA256: digest,
		ArtifactManifestSHA256: digest, EvaluationSHA256: digest, ExplanationSHA256: digest,
		GraphSnapshotID: "graph-1", GraphSnapshotSHA256: digest, SigningKeyID: "kms/model/key-1",
		Compatibility:     map[string]interface{}{"runtime_contract": "traffic.behavior.inference.v1"},
		GovernanceVersion: "model-registration.v1", ExpectedRevision: &expectedRevision,
		IdempotencyKey: "mlops-registration-000001", FeatureSetID: "feature-v1", TenantID: "tenant-a",
		Metrics: map[string]interface{}{"f1_score": 0.91}, Status: "registered",
	}
}

func TestRegisterModelVersionCommitsMetadataAndAuditWithoutOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req := governedRegistrationRequest()
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), DefaultModelServiceConfig())
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))")).
		WithArgs(req.TenantID, req.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("FROM model_versions mv JOIN models").
		WithArgs(req.TenantID, req.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"model_version"}))
	mock.ExpectQuery("FROM models WHERE tenant_id = \\$1 AND name = \\$2 FOR UPDATE").
		WithArgs(req.TenantID, req.ModelID).
		WillReturnRows(sqlmock.NewRows([]string{"model_id", "tenant_id", "name", "model_type", "description"}).
			AddRow("11111111-1111-4111-8111-111111111111", req.TenantID, req.ModelID, req.ModelType, nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM model_versions WHERE tenant_id = $1 AND model_id = $2")).
		WithArgs(req.TenantID, "11111111-1111-4111-8111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("INSERT INTO model_versions").
		WithArgs(registrationInsertArguments(req)...).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mv, err := service.RegisterModelVersion(context.Background(), req, &OperationContext{
		TenantID: req.TenantID, UserID: "user-a", Username: "mlops", Authenticated: true,
	})
	if err != nil {
		t.Fatalf("RegisterModelVersion returned error: %v", err)
	}
	if mv.Status != "registered" || mv.Revision != 1 || mv.PackageSHA256 != req.PackageSHA256 {
		t.Fatalf("unexpected registered model version: %+v", mv)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("registration transaction did not match metadata+audit-only contract: %v", err)
	}
}

func registrationInsertArguments(req *model.RegisterModelRequest) []driver.Value {
	requestSHA, _ := modelRegistrationRequestSHA256(req)
	return []driver.Value{
		req.Version, "11111111-1111-4111-8111-111111111111", req.TenantID, req.FeatureSetID,
		req.ArtifactURI, req.ArtifactManifestURI, req.PackageID, req.PackageSHA256,
		req.ArtifactManifestSHA256, req.EvaluationSHA256, req.ExplanationSHA256,
		req.GraphSnapshotID, req.GraphSnapshotSHA256, req.SigningKeyID,
		`{"runtime_contract":"traffic.behavior.inference.v1"}`, req.IdempotencyKey,
		requestSHA, `{"f1_score":0.91}`, "user-a",
	}
}

func TestGovernedRegistrationRejectsActivationAndMissingConcurrencyGuard(t *testing.T) {
	req := governedRegistrationRequest()
	req.Status = "active"
	if err := req.Validate(); err == nil {
		t.Fatal("registration accepted active status")
	}
	req = governedRegistrationRequest()
	req.ExpectedRevision = nil
	if err := req.Validate(); err == nil {
		t.Fatal("registration accepted a missing expected_revision")
	}
	req = governedRegistrationRequest()
	req.IdempotencyKey = "short"
	if err := req.Validate(); err == nil {
		t.Fatal("registration accepted an invalid Idempotency-Key")
	}
}
