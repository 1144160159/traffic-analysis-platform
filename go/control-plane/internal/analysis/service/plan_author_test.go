package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

func testTemplateAndCatalog() (*DefaultTemplate, CatalogSnapshot) {
	tpl := &DefaultTemplate{
		TaskDefinitionID: "def-1",
		PlanSource:       "AUTO_DEFAULT",
		PlanRevision:     1,
		SourceKind:       "PCAP_REPLAY",
		SourceSpec:       json.RawMessage(`{"pcap_object":"s3://b/p1.pcap"}`),
		FeatureSetID:     "fs-v1",
		RecognitionModel: "enc@v1",
		DetectorRefs:     []string{"det@v1"},
		RuleRefs:         []string{"rule@v1"},
		StageDAG:         json.RawMessage(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy: json.RawMessage(`{"allow_partial":false}`),
		ResourceBudget:   json.RawMessage(`{"cpu":2}`),
	}
	catalog := CatalogSnapshot{
		Revision:          1,
		FeatureSets:       map[string][]string{"fs-v1": {"f1", "f2", "f3"}},
		RecognitionModels: []string{"enc@v1"},
		ThreatDetectors:   []string{"det@v1", "det@v2"},
		Rules:             []string{"rule@v1"},
		Probes:            []string{"probe-1"},
	}
	return tpl, catalog
}

func fakeLoader(tpl *DefaultTemplate, catalog CatalogSnapshot) TemplateLoader {
	return func(context.Context, string, string) (*DefaultTemplate, CatalogSnapshot, error) {
		return tpl, catalog, nil
	}
}

func TestPlanAuthorSaveCustomHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepo(db)
	tpl, catalog := testTemplateAndCatalog()
	compiler := NewPlanCompiler()
	svc := NewPlanAuthorService(repo, compiler, NewCustomPlanResolver(compiler), fakeLoader(tpl, catalog))

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT request_sha256 FROM analysis_materialization_ledger`).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO analysis_materialization_ledger`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(plan_revision\),0\)\+1`).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO analysis_plan_revisions`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO analysis_plan_governance_heads`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO analysis_history`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.SaveCustom(context.Background(), SaveCustomPlanRequest{
		TenantID:             "t1",
		TaskDefinitionID:     "def-1",
		CustomOverrides:      json.RawMessage(`{"selected_feature_ids":["f1","f2"],"threat_detector_refs":["det@v2"]}`),
		Actor:                "op-1",
		ClientIdempotencyKey: "k1",
	})
	if err != nil {
		t.Fatalf("save custom: %v", err)
	}
	if resp.PlanRevision != 2 || resp.PlanID == "" || resp.ExecutionSpecSHA256 == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPlanAuthorSaveCustomRejectsOverrideNotInCatalog(t *testing.T) {
	repo := repository.NewRepo(nil)
	tpl, catalog := testTemplateAndCatalog()
	compiler := NewPlanCompiler()
	svc := NewPlanAuthorService(repo, compiler, NewCustomPlanResolver(compiler), fakeLoader(tpl, catalog))

	_, err := svc.SaveCustom(context.Background(), SaveCustomPlanRequest{
		TenantID:             "t1",
		TaskDefinitionID:     "def-1",
		CustomOverrides:      json.RawMessage(`{"threat_detector_refs":["det@evil"]}`),
		Actor:                "op-1",
		ClientIdempotencyKey: "k1",
	})
	if err == nil || !containsCode(err, contract.ErrCodePlanNotApproved) {
		t.Fatalf("expected preflight catalog rejection, got %v", err)
	}
}

func TestPlanAuthorSaveCustomMissingIdempotencyKey(t *testing.T) {
	svc := NewPlanAuthorService(repository.NewRepo(nil), NewPlanCompiler(), NewCustomPlanResolver(NewPlanCompiler()), nil)
	_, err := svc.SaveCustom(context.Background(), SaveCustomPlanRequest{TenantID: "t1", TaskDefinitionID: "def-1"})
	if err == nil || !containsCode(err, contract.ErrCodeMissingIdempotencyKey) {
		t.Fatalf("expected missing idempotency key, got %v", err)
	}
}

func TestPlanAuthorApproveRequiresDistinctMakerChecker(t *testing.T) {
	svc := NewPlanAuthorService(repository.NewRepo(nil), NewPlanCompiler(), NewCustomPlanResolver(NewPlanCompiler()), nil)
	err := svc.Approve(context.Background(), ApprovePlanRequest{TenantID: "t1", PlanID: "p1", Maker: "a", Checker: "a"})
	if err == nil || !containsCode(err, contract.ErrCodeInvalidTransition) {
		t.Fatalf("expected maker/checker distinct rejection, got %v", err)
	}
}

func TestPlanAuthorApproveHappyPathActivates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepo(db)
	svc := NewPlanAuthorService(repo, NewPlanCompiler(), NewCustomPlanResolver(NewPlanCompiler()), nil)

	mock.ExpectQuery(`SELECT g.state, g.authority_revision`).WillReturnRows(sqlmock.NewRows([]string{"state", "rev"}).AddRow("DRAFT", 0))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE analysis_plan_governance_heads SET state`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO analysis_plan_approvals`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO analysis_history`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE analysis_task_definitions d SET active_plan_revision`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := svc.Approve(context.Background(), ApprovePlanRequest{TenantID: "t1", PlanID: "p1", Maker: "maker-a", Checker: "checker-b"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPlanAuthorApproveAlreadyActiveIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepo(db)
	svc := NewPlanAuthorService(repo, NewPlanCompiler(), NewCustomPlanResolver(NewPlanCompiler()), nil)

	mock.ExpectQuery(`SELECT g.state, g.authority_revision`).WillReturnRows(sqlmock.NewRows([]string{"state", "rev"}).AddRow("ACTIVE", 1))

	if err := svc.Approve(context.Background(), ApprovePlanRequest{TenantID: "t1", PlanID: "p1", Maker: "maker-a", Checker: "checker-b"}); err != nil {
		t.Fatalf("idempotent approve should be nil, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
