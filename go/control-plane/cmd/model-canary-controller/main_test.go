package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/modelcanary"
)

func TestLoadConfigIsDefaultOffAndRequiresPolicyBinding(t *testing.T) {
	if _, err := loadConfig(func(name string) string {
		if name == "MODEL_CANARY_POLICY_SHA256" {
			return strings.Repeat("a", 64)
		}
		return ""
	}); err == nil {
		t.Fatal("config without an approved HTTPS API endpoint was accepted")
	}
	environment := map[string]string{
		"MODEL_CANARY_POLICY_SHA256": strings.Repeat("a", 64),
		"MODEL_CANARY_API_BASE_URL":  "https://approved-apisix.example",
	}
	cfg, err := loadConfig(func(name string) string { return environment[name] })
	if err != nil {
		t.Fatalf("valid default-off config rejected: %v", err)
	}
	if cfg.executionAuthorized {
		t.Fatal("model canary execution unexpectedly defaults on")
	}
	if cfg.groupID != "rule-manager-model-tenant-canary-v1" ||
		cfg.topic != "model-shadow-observations.v1" {
		t.Fatalf("unexpected Kafka binding: group=%s topic=%s", cfg.groupID, cfg.topic)
	}

	delete(environment, "MODEL_CANARY_POLICY_SHA256")
	if _, err := loadConfig(func(name string) string { return environment[name] }); err == nil {
		t.Fatal("config without approved policy hash was accepted")
	}
}

func TestVerifyShadowEvidenceRequiresExactFullWindow(t *testing.T) {
	policy := controllerPolicy()
	receipt := shadowEvidenceReceipt{ScopedEvidenceStatus: "PASS"}
	receipt.Candidate.ModelPackageSHA256 = policy.CandidatePackageSHA256
	receipt.ShadowObservationWindow.Status = "PASS"
	receipt.ShadowObservationWindow.Samples = policy.ShadowEvidence.MinimumSamples
	receipt.ShadowObservationWindow.WindowSeconds = policy.ShadowEvidence.MinimumWindowSeconds
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shadow.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	policy.ShadowEvidence.SHA256 = digest(payload)
	if err := verifyShadowEvidence(policy, path); err != nil {
		t.Fatalf("exact shadow window rejected: %v", err)
	}

	policy.ShadowEvidence.MinimumSamples++
	if err := verifyShadowEvidence(policy, path); err == nil {
		t.Fatal("insufficient shadow sample window was accepted")
	}
}

func TestDeploymentClientStartsGrayAndRollbackOnly(t *testing.T) {
	policy := controllerPolicy()
	var mutex sync.Mutex
	status := "planned"
	startedAt := time.Time{}
	operations := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" ||
			request.Header.Get("X-Tenant-ID") != policy.TenantID {
			http.Error(response, "missing controller identity", http.StatusUnauthorized)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/deployments/"+policy.DeploymentID:
			_ = json.NewEncoder(response).Encode(deploymentResponse{
				Success: true,
				Data: deploymentSnapshot{DeploymentID: policy.DeploymentID, TenantID: policy.TenantID,
					ModelVersion: policy.CandidateVersion, Status: status,
					Scope: map[string]any{"percentage": float64(policy.RolloutPercentage)},
					GrayStartedAt: func() *time.Time {
						if startedAt.IsZero() {
							return nil
						}
						copy := startedAt
						return &copy
					}()},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/deployments/"+policy.DeploymentID+"/gray":
			operations = append(operations, "gray")
			status = "gray"
			startedAt = time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
			_ = json.NewEncoder(response).Encode(deploymentResponse{Success: true})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/deployments/"+policy.DeploymentID+"/rollback":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil ||
				body["target_deployment_id"] != policy.RollbackDeploymentID ||
				!strings.Contains(body["reason"], "automatic stop") {
				http.Error(response, "invalid rollback", http.StatusBadRequest)
				return
			}
			operations = append(operations, "rollback")
			status = "rolled_back"
			_ = json.NewEncoder(response).Encode(deploymentResponse{Success: true})
		default:
			http.Error(response, "unexpected operation", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &deploymentClient{
		baseURL: server.URL,
		token:   "token",
		tenant:  policy.TenantID,
		client:  server.Client(),
	}
	gotStartedAt, err := client.ensureGrayStarted(context.Background(), policy)
	if err != nil {
		t.Fatalf("start gray failed: %v", err)
	}
	if !gotStartedAt.Equal(startedAt) {
		t.Fatalf("unexpected durable gray start: got %s want %s", gotStartedAt, startedAt)
	}
	if err := client.rollback(context.Background(), policy, "M08 N013 automatic stop: threshold"); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if strings.Join(operations, ",") != "gray,rollback" {
		t.Fatalf("unexpected operations %v", operations)
	}
}

func TestDeploymentClientRejectsFractionalRolloutPercentage(t *testing.T) {
	policy := controllerPolicy()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		startedAt := time.Now().UTC()
		_ = json.NewEncoder(response).Encode(deploymentResponse{
			Success: true,
			Data: deploymentSnapshot{
				DeploymentID:  policy.DeploymentID,
				TenantID:      policy.TenantID,
				ModelVersion:  policy.CandidateVersion,
				Status:        "gray",
				Scope:         map[string]any{"percentage": 5.9},
				GrayStartedAt: &startedAt,
			},
		})
	}))
	defer server.Close()

	client := &deploymentClient{
		baseURL: server.URL,
		token:   "token",
		tenant:  policy.TenantID,
		client:  server.Client(),
	}
	if _, err := client.ensureGrayStarted(context.Background(), policy); err == nil {
		t.Fatal("fractional rollout percentage was accepted")
	}
}

func TestDecodePolicyRejectsUnknownField(t *testing.T) {
	policy := controllerPolicy()
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePolicy(payload); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	withUnknown := append(payload[:len(payload)-1], []byte(`,"unknown":true}`)...)
	if _, err := decodePolicy(withUnknown); err == nil {
		t.Fatal("unknown policy field was accepted")
	}
}

func controllerPolicy() modelcanary.Policy {
	return modelcanary.Policy{
		SchemaVersion:              modelcanary.SchemaVersion,
		CanaryID:                   "m08-n013-canary-1",
		Enabled:                    true,
		TenantID:                   "tenant-a",
		DeploymentID:               "candidate-deployment",
		RollbackDeploymentID:       "champion-deployment",
		CandidateModelID:           "model-a",
		CandidateVersion:           "v2",
		CandidatePackageSHA256:     strings.Repeat("a", 64),
		CandidateAggregateRevision: 2,
		ChampionVersion:            "v1",
		RolloutPercentage:          5,
		MinimumSamples:             100,
		MaximumSamples:             1000,
		ObservationWindowSeconds:   300,
		MaximumClockSkewSeconds:    60,
		Thresholds: modelcanary.Thresholds{
			MaximumErrorRate:              0.01,
			MaximumTimeoutRate:            0.005,
			MaximumDecisionChangeRate:     0.1,
			MaximumLabelChangeRate:        0.1,
			MaximumAbsoluteScoreDeltaP95:  0.15,
			MaximumLatencyRatioP95:        1.5,
			MaximumChallengerHeapDeltaP95: 64 << 20,
			MaximumConsecutiveNonCompared: 3,
		},
		ShadowEvidence: modelcanary.Evidence{
			Path:                 "shadow.json",
			SHA256:               strings.Repeat("b", 64),
			RequiredStatus:       "PASS",
			MinimumSamples:       100,
			MinimumWindowSeconds: 300,
		},
	}
}
