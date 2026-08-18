package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/modeldrift"
)

func driftPersistenceFixture(state modeldrift.DecisionState) (*automatedMLOpsScope, modeldrift.Snapshot, modeldrift.Decision, string) {
	now := time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC)
	scope := &automatedMLOpsScope{
		TenantID: "tenant-a", ModelID: "11111111-1111-4111-8111-111111111111",
		ModelVersion: "model-v1", FeatureSetID: "feature-v1",
	}
	snapshot := modeldrift.Snapshot{
		TenantID: scope.TenantID, ModelID: scope.ModelID, ModelVersion: scope.ModelVersion, FeatureSetID: scope.FeatureSetID,
		EvaluatedAt: now, BaselineWindowStart: now.Add(-8 * 24 * time.Hour), BaselineWindowEnd: now.Add(-24 * time.Hour),
		CurrentWindowStart: now.Add(-24 * time.Hour), CurrentWindowEnd: now,
		FeatureWatermark: now.Add(-time.Minute), FeedbackWatermark: now.Add(-time.Hour),
		CurrentFeatureCount: 1000, FeedbackCount: 100, FalsePositiveCount: 30,
		FeatureDistributions: map[string]modeldrift.Distribution{
			"bps":         {Baseline: []uint64{500, 500}, Current: []uint64{500, 500}},
			"iat_mean_ms": {Baseline: []uint64{500, 500}, Current: []uint64{500, 500}},
			"pktlen_mean": {Baseline: []uint64{500, 500}, Current: []uint64{500, 500}},
			"pps":         {Baseline: []uint64{900, 100}, Current: []uint64{100, 900}},
		},
	}
	decision := modeldrift.Decision{
		State: state, Reasons: []string{"psi_threshold_exceeded"}, PSI: map[string]float64{"pps": 3.5},
		MaxObservedPSI: 3.5, FalsePositiveRate: 0.3,
		PolicySHA256: strings.Repeat("a", 64), SignalSHA256: strings.Repeat("b", 64), ActivationAuthorized: false,
	}
	evaluationID := uuid.NewSHA1(driftEvaluationNamespace, []byte(decision.PolicySHA256+":"+decision.SignalSHA256)).String()
	return scope, snapshot, decision, evaluationID
}

func TestRecordGovernedDriftDecisionCreatesCandidateOnlyReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scope, snapshot, decision, evaluationID := driftPersistenceFixture(modeldrift.DecisionCandidate)
	candidateID := uuid.NewSHA1(driftEvaluationNamespace, []byte("candidate:"+evaluationID)).String()
	workflowName := "mlops-drift-" + strings.ReplaceAll(candidateID, "-", "")[:20]

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_drift_evaluation_receipt").
		WithArgs(sqlmock.AnyArg(), scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID,
			decision.PolicySHA256, decision.SignalSHA256, decision.State, sqlmock.AnyArg(), sqlmock.AnyArg(),
			decision.MaxObservedPSI, decision.FalsePositiveRate, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			snapshot.BaselineWindowStart, snapshot.BaselineWindowEnd, snapshot.CurrentWindowStart, snapshot.CurrentWindowEnd, snapshot.EvaluatedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT evaluation_id::text,decision_state").
		WithArgs(decision.PolicySHA256, decision.SignalSHA256).
		WillReturnRows(sqlmock.NewRows([]string{"evaluation_id", "decision_state", "tenant_id", "model_id", "model_version", "feature_set_id"}).
			AddRow(evaluationID, string(decision.State), scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID))
	mock.ExpectExec("INSERT INTO model_retrain_candidate_request").
		WithArgs(candidateID, evaluationID, scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID,
			workflowName, driftCandidateWorkflowPending, "traffic-analysis", "mlops-training-template").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT candidate.candidate_id::text,candidate.evaluation_id::text").
		WithArgs(scope.TenantID, scope.ModelID, scope.ModelVersion).
		WillReturnRows(sqlmock.NewRows([]string{"candidate_id", "evaluation_id", "workflow_name", "candidate_state", "policy_sha256", "signal_sha256", "reasons"}).
			AddRow(candidateID, evaluationID, workflowName, driftCandidateWorkflowPending, decision.PolicySHA256, decision.SignalSHA256, `["psi_threshold_exceeded"]`))
	mock.ExpectCommit()

	orch := NewMLOpsOrchestrator(nil, db, DefaultMLOpsOrchestratorConfig(), zap.NewNop())
	candidate, err := orch.recordGovernedDriftDecision(context.Background(), scope, snapshot, decision)
	if err != nil {
		t.Fatalf("record candidate: %v", err)
	}
	if candidate == nil || candidate.WorkflowName != workflowName || candidate.State != driftCandidateWorkflowPending {
		t.Fatalf("unexpected candidate receipt: %+v", candidate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordBlockedDriftDecisionDoesNotCreateCandidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scope, snapshot, decision, evaluationID := driftPersistenceFixture(modeldrift.DecisionBlocked)
	decision.Reasons = []string{"feedback_signals_stale"}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_drift_evaluation_receipt").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT evaluation_id::text,decision_state").
		WithArgs(decision.PolicySHA256, decision.SignalSHA256).
		WillReturnRows(sqlmock.NewRows([]string{"evaluation_id", "decision_state", "tenant_id", "model_id", "model_version", "feature_set_id"}).
			AddRow(evaluationID, string(decision.State), scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID))
	mock.ExpectCommit()

	orch := NewMLOpsOrchestrator(nil, db, DefaultMLOpsOrchestratorConfig(), zap.NewNop())
	candidate, err := orch.recordGovernedDriftDecision(context.Background(), scope, snapshot, decision)
	if err != nil || candidate != nil {
		t.Fatalf("blocked decision created a candidate: candidate=%+v err=%v", candidate, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordGovernedDriftDecisionReusesCandidateForActiveBaseline(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scope, snapshot, decision, evaluationID := driftPersistenceFixture(modeldrift.DecisionCandidate)
	priorEvaluationID := "88888888-8888-4888-8888-888888888888"
	priorCandidateID := uuid.NewSHA1(driftEvaluationNamespace, []byte("candidate:"+priorEvaluationID)).String()
	priorWorkflowName := "mlops-drift-" + strings.ReplaceAll(priorCandidateID, "-", "")[:20]
	priorPolicySHA := strings.Repeat("c", 64)
	priorSignalSHA := strings.Repeat("d", 64)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_drift_evaluation_receipt").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT evaluation_id::text,decision_state").
		WithArgs(decision.PolicySHA256, decision.SignalSHA256).
		WillReturnRows(sqlmock.NewRows([]string{"evaluation_id", "decision_state", "tenant_id", "model_id", "model_version", "feature_set_id"}).
			AddRow(evaluationID, string(decision.State), scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID))
	mock.ExpectExec("INSERT INTO model_retrain_candidate_request").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT candidate.candidate_id::text,candidate.evaluation_id::text").
		WithArgs(scope.TenantID, scope.ModelID, scope.ModelVersion).
		WillReturnRows(sqlmock.NewRows([]string{"candidate_id", "evaluation_id", "workflow_name", "candidate_state", "policy_sha256", "signal_sha256", "reasons"}).
			AddRow(priorCandidateID, priorEvaluationID, priorWorkflowName, driftCandidateWorkflowFailed, priorPolicySHA, priorSignalSHA, `["psi_threshold_exceeded"]`))
	mock.ExpectCommit()

	orch := NewMLOpsOrchestrator(nil, db, DefaultMLOpsOrchestratorConfig(), zap.NewNop())
	candidate, err := orch.recordGovernedDriftDecision(context.Background(), scope, snapshot, decision)
	if err != nil {
		t.Fatalf("reuse baseline candidate: %v", err)
	}
	if candidate.EvaluationID != priorEvaluationID || candidate.WorkflowName != priorWorkflowName || candidate.PolicySHA256 != priorPolicySHA || candidate.SignalSHA256 != priorSignalSHA {
		t.Fatalf("active baseline created a competing candidate: %+v", candidate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateWorkflowConflictReconcilesExactIdentity(t *testing.T) {
	decision := &RetrainDecision{
		ShouldRetrain: true, Trigger: TriggerDrift, Disposition: "CANDIDATE",
		TenantID: "tenant-a", ModelID: "model-a", ModelVersion: "model-v1", FeatureSetID: "feature-v1",
		EvaluationID: "22222222-2222-4222-8222-222222222222",
		SignalSHA256: strings.Repeat("b", 64), PolicySHA256: strings.Repeat("a", 64),
	}
	candidateID := uuid.NewSHA1(driftEvaluationNamespace, []byte("candidate:"+decision.EvaluationID)).String()
	workflowName := "mlops-drift-" + strings.ReplaceAll(candidateID, "-", "")[:20]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Error(w, "already exists", http.StatusConflict)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"metadata":{"name":"` + workflowName + `","namespace":"argo"},"spec":{"workflowTemplateRef":{"name":"mlops-training-template"},"arguments":{"parameters":[` +
				`{"name":"tenant-id","value":"tenant-a"},{"name":"model-id","value":"model-a"},{"name":"feature-set-id","value":"feature-v1"},` +
				`{"name":"baseline-model-version","value":"model-v1"},{"name":"drift-evaluation-id","value":"22222222-2222-4222-8222-222222222222"},` +
				`{"name":"drift-signal-sha256","value":"` + decision.SignalSHA256 + `"},{"name":"drift-policy-sha256","value":"` + decision.PolicySHA256 + `"},` +
				`{"name":"candidate-only","value":"true"},{"name":"auto-activate","value":"false"}]}},"status":{"phase":"Running"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	if err := orch.submitArgoWorkflow(context.Background(), decision, workflowName); err != nil {
		t.Fatalf("exact existing candidate workflow did not reconcile: %v", err)
	}
}
