package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/modelcanary"
	segmentkafka "github.com/segmentio/kafka-go"
)

const controllerID = "traffic-model-canary-controller-v1"

type config struct {
	policyFile          string
	policySHA256        string
	shadowEvidenceFile  string
	decisionFile        string
	executionAuthorized bool
	brokers             []string
	topic               string
	groupID             string
	security            commonkafka.SecurityConfig
	apiBaseURL          string
	apiTokenFile        string
	apiCAFile           string
	maxRuntime          time.Duration
}

type shadowEvidenceReceipt struct {
	ScopedEvidenceStatus string `json:"scoped_evidence_status"`
	Candidate            struct {
		ModelPackageSHA256 string `json:"model_package_sha256"`
	} `json:"candidate"`
	ShadowObservationWindow struct {
		Status        string `json:"status"`
		Samples       int    `json:"samples"`
		WindowSeconds int    `json:"window_seconds"`
	} `json:"shadow_observation_window"`
}

type deploymentSnapshot struct {
	DeploymentID  string         `json:"deployment_id"`
	TenantID      string         `json:"tenant_id"`
	ModelVersion  string         `json:"model_version"`
	Status        string         `json:"status"`
	Scope         map[string]any `json:"scope"`
	GrayStartedAt *time.Time     `json:"gray_started_at"`
}

type deploymentResponse struct {
	Success bool               `json:"success"`
	Data    deploymentSnapshot `json:"data"`
	Message string             `json:"message"`
}

type deploymentClient struct {
	baseURL string
	token   string
	tenant  string
	client  *http.Client
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", controllerID, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	policyBytes, err := os.ReadFile(cfg.policyFile)
	if err != nil {
		return fmt.Errorf("read model canary policy: %w", err)
	}
	if digest(policyBytes) != cfg.policySHA256 {
		return errors.New("model canary policy sha256 does not match the approved binding")
	}
	policy, err := decodePolicy(policyBytes)
	if err != nil {
		return err
	}
	if !policy.Enabled || !cfg.executionAuthorized {
		return errors.New("model canary is default-off; policy and execution authorization must both be true")
	}
	if cfg.maxRuntime < time.Duration(policy.ObservationWindowSeconds+60)*time.Second {
		return errors.New("model canary max runtime cannot cover the observation window")
	}
	if err := verifyShadowEvidence(policy, cfg.shadowEvidenceFile); err != nil {
		return err
	}

	api, err := newDeploymentClient(cfg, policy.TenantID)
	if err != nil {
		return err
	}
	operationCtx, operationCancel := context.WithTimeout(context.Background(), 30*time.Second)
	canaryStartedAt, err := api.ensureGrayStarted(operationCtx, policy)
	if err != nil {
		operationCancel()
		return err
	}
	operationCancel()

	window, err := modelcanary.NewWindow(policy)
	if err != nil {
		return err
	}
	dialer, err := cfg.security.Dialer(controllerID)
	if err != nil {
		return fmt.Errorf("configure model canary Kafka security: %w", err)
	}
	reader := segmentkafka.NewReader(segmentkafka.ReaderConfig{
		Brokers:        cfg.brokers,
		Topic:          cfg.topic,
		GroupID:        cfg.groupID,
		Dialer:         dialer,
		MinBytes:       1,
		MaxBytes:       10 << 20,
		MaxWait:        500 * time.Millisecond,
		StartOffset:    segmentkafka.FirstOffset,
		CommitInterval: 0,
		GroupBalancers: []segmentkafka.GroupBalancer{segmentkafka.RoundRobinGroupBalancer{}},
	})
	defer reader.Close()

	signalCtx, signalCancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()
	consumeCtx, consumeCancel := context.WithTimeout(signalCtx, cfg.maxRuntime)
	defer consumeCancel()

	for {
		message, fetchErr := reader.FetchMessage(consumeCtx)
		if fetchErr != nil {
			decision := window.ForceStop(time.Now().UTC(), "observation_stream_unavailable")
			rollbackErr := rollbackWithFreshContext(api, policy, decision)
			_ = writeDecision(cfg.decisionFile, decision)
			if rollbackErr != nil {
				return fmt.Errorf("observation stream stopped (%v) and rollback failed: %w", fetchErr, rollbackErr)
			}
			return fmt.Errorf("observation stream stopped and canary rolled back: %w", fetchErr)
		}
		observation, decodeErr := modelcanary.DecodeObservation(message.Value)
		if decodeErr != nil {
			decision := window.ForceStop(time.Now().UTC(), "invalid_observation_contract")
			rollbackErr := rollbackWithFreshContext(api, policy, decision)
			_ = writeDecision(cfg.decisionFile, decision)
			if rollbackErr != nil {
				return fmt.Errorf("invalid observation (%v) and rollback failed: %w", decodeErr, rollbackErr)
			}
			return fmt.Errorf("invalid observation stopped and rolled back canary: %w", decodeErr)
		}
		if observation.ObservedAtMS < canaryStartedAt.UnixMilli() {
			// A new group may replay the N012 qualification window. Those records
			// prove shadow readiness, not post-cutover canary health.
			continue
		}
		decision, observeErr := window.Observe(observation, time.Now().UTC())
		if observeErr != nil && decision.State != modelcanary.StateStopped {
			return observeErr
		}
		switch decision.State {
		case modelcanary.StateIgnored, modelcanary.StateObserving:
			continue
		case modelcanary.StateStopped:
			if err := rollbackWithFreshContext(api, policy, decision); err != nil {
				_ = writeDecision(cfg.decisionFile, decision)
				return err
			}
			if err := reader.CommitMessages(context.Background(), message); err != nil {
				return fmt.Errorf("commit terminal rollback observation: %w", err)
			}
			if err := writeDecision(cfg.decisionFile, decision); err != nil {
				return err
			}
			return fmt.Errorf("model canary stopped and rolled back: %s", strings.Join(decision.StopReasons, ","))
		case modelcanary.StateWindowComplete:
			if err := reader.CommitMessages(context.Background(), message); err != nil {
				return fmt.Errorf("commit complete model canary window: %w", err)
			}
			if err := writeDecision(cfg.decisionFile, decision); err != nil {
				return err
			}
			// Deliberately no POST /activate: expansion needs a new approval.
			return nil
		default:
			return fmt.Errorf("unsupported model canary decision state %q", decision.State)
		}
	}
}

func loadConfig(getenv func(string) string) (config, error) {
	value := func(name, fallback string) string {
		if configured := strings.TrimSpace(getenv(name)); configured != "" {
			return configured
		}
		return fallback
	}
	seconds, err := strconv.Atoi(value("MODEL_CANARY_MAX_RUNTIME_SECONDS", "4200"))
	if err != nil || seconds < 360 || seconds > 7*24*60*60+3600 {
		return config{}, errors.New("MODEL_CANARY_MAX_RUNTIME_SECONDS is invalid")
	}
	authorized, err := strconv.ParseBool(value("MODEL_CANARY_EXECUTION_AUTHORIZED", "false"))
	if err != nil {
		return config{}, errors.New("MODEL_CANARY_EXECUTION_AUTHORIZED must be boolean")
	}
	brokers := splitNonEmpty(value("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092"))
	cfg := config{
		policyFile:          value("MODEL_CANARY_POLICY_FILE", "/etc/model-canary/policy.json"),
		policySHA256:        value("MODEL_CANARY_POLICY_SHA256", ""),
		shadowEvidenceFile:  value("MODEL_CANARY_SHADOW_EVIDENCE_FILE", "/etc/model-canary/shadow-evidence.json"),
		decisionFile:        value("MODEL_CANARY_DECISION_FILE", "/var/run/model-canary/decision.json"),
		executionAuthorized: authorized,
		brokers:             brokers,
		topic:               value("MODEL_CANARY_OBSERVATION_TOPIC", "model-shadow-observations.v1"),
		groupID:             value("MODEL_CANARY_CONSUMER_GROUP", "rule-manager-model-tenant-canary-v1"),
		apiBaseURL:          strings.TrimRight(value("MODEL_CANARY_API_BASE_URL", ""), "/"),
		apiTokenFile:        value("MODEL_CANARY_API_TOKEN_FILE", "/var/run/secrets/model-canary/token"),
		apiCAFile:           value("MODEL_CANARY_API_CA_FILE", "/var/run/secrets/model-canary/ca.crt"),
		maxRuntime:          time.Duration(seconds) * time.Second,
		security: commonkafka.SecurityConfig{
			SecurityProtocol: value("KAFKA_SECURITY_PROTOCOL", "SASL_SSL"),
			SASLMechanism:    value("KAFKA_SASL_MECHANISM", "SCRAM-SHA-512"),
			SASLUsername:     strings.TrimSpace(getenv("KAFKA_SASL_USERNAME")),
			SASLPassword:     getenv("KAFKA_SASL_PASSWORD"),
			TLSCAFile:        value("KAFKA_TLS_CA_FILE", "/etc/kafka/tls/ca.crt"),
			TLSServerName:    value("KAFKA_TLS_SERVER_NAME", "kafka-bootstrap.middleware.svc"),
			ClientID:         controllerID,
		},
	}
	if len(cfg.brokers) == 0 || cfg.topic == "" || cfg.groupID == "" {
		return config{}, errors.New("model canary Kafka brokers, topic and consumer group are required")
	}
	if len(cfg.policySHA256) != sha256.Size*2 {
		return config{}, errors.New("MODEL_CANARY_POLICY_SHA256 must be an approved sha256")
	}
	parsed, err := url.Parse(cfg.apiBaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return config{}, errors.New("MODEL_CANARY_API_BASE_URL must be an absolute HTTPS URL")
	}
	return cfg, nil
}

func decodePolicy(payload []byte) (modelcanary.Policy, error) {
	var policy modelcanary.Policy
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return policy, fmt.Errorf("decode model canary policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return policy, errors.New("decode model canary policy: trailing JSON")
	}
	if err := policy.Validate(); err != nil {
		return policy, err
	}
	return policy, nil
}

func verifyShadowEvidence(policy modelcanary.Policy, path string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read model shadow evidence: %w", err)
	}
	if digest(payload) != policy.ShadowEvidence.SHA256 {
		return errors.New("model shadow evidence sha256 does not match the canary policy")
	}
	var receipt shadowEvidenceReceipt
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode model shadow evidence: %w", err)
	}
	if receipt.ScopedEvidenceStatus != policy.ShadowEvidence.RequiredStatus ||
		receipt.ShadowObservationWindow.Status != policy.ShadowEvidence.RequiredStatus {
		return errors.New("model shadow evidence has not passed its complete observation window")
	}
	if receipt.Candidate.ModelPackageSHA256 != policy.CandidatePackageSHA256 {
		return errors.New("model shadow evidence candidate package does not match the canary policy")
	}
	if receipt.ShadowObservationWindow.Samples < policy.ShadowEvidence.MinimumSamples ||
		receipt.ShadowObservationWindow.WindowSeconds < policy.ShadowEvidence.MinimumWindowSeconds {
		return errors.New("model shadow evidence sample count or observation window is insufficient")
	}
	return nil
}

func newDeploymentClient(cfg config, tenant string) (*deploymentClient, error) {
	token, err := os.ReadFile(cfg.apiTokenFile)
	if err != nil {
		return nil, fmt.Errorf("read model canary API token: %w", err)
	}
	if strings.TrimSpace(string(token)) == "" {
		return nil, errors.New("model canary API token is empty")
	}
	caPEM, err := os.ReadFile(cfg.apiCAFile)
	if err != nil {
		return nil, fmt.Errorf("read model canary API CA: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("model canary API CA contains no certificates")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	return &deploymentClient{
		baseURL: cfg.apiBaseURL,
		token:   strings.TrimSpace(string(token)),
		tenant:  tenant,
		client:  &http.Client{Transport: transport, Timeout: 20 * time.Second},
	}, nil
}

func (client *deploymentClient) ensureGrayStarted(
	ctx context.Context, policy modelcanary.Policy,
) (time.Time, error) {
	snapshot, err := client.get(ctx, policy.DeploymentID)
	if err != nil {
		return time.Time{}, err
	}
	if snapshot.TenantID != policy.TenantID || snapshot.ModelVersion != policy.CandidateVersion {
		return time.Time{}, errors.New("candidate deployment identity does not match the tenant canary policy")
	}
	rolloutPercentage, ok := percentage(snapshot.Scope)
	if !ok || rolloutPercentage != policy.RolloutPercentage {
		return time.Time{}, errors.New("candidate deployment percentage does not match the tenant canary policy")
	}
	switch snapshot.Status {
	case "gray":
		if snapshot.GrayStartedAt == nil || snapshot.GrayStartedAt.IsZero() {
			return time.Time{}, errors.New("gray deployment has no durable start time")
		}
		return snapshot.GrayStartedAt.UTC(), nil
	case "planned":
		if err := client.post(ctx, policy.DeploymentID+"/gray", nil); err != nil {
			return time.Time{}, fmt.Errorf("start tenant model canary: %w", err)
		}
		confirmed, err := client.get(ctx, policy.DeploymentID)
		if err != nil {
			return time.Time{}, err
		}
		if confirmed.Status != "gray" || confirmed.GrayStartedAt == nil ||
			confirmed.GrayStartedAt.IsZero() {
			return time.Time{}, fmt.Errorf("candidate deployment did not enter timestamped gray state: %s", confirmed.Status)
		}
		return confirmed.GrayStartedAt.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("candidate deployment status %q cannot start or resume canary", snapshot.Status)
	}
}

func percentage(scope map[string]any) (int, bool) {
	value, ok := scope["percentage"]
	if !ok {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		if number < 0 || number > 100 || math.Trunc(number) != number {
			return 0, false
		}
		return int(number), true
	case int:
		return number, number >= 0 && number <= 100
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		return parsed, err == nil && parsed >= 0 && parsed <= 100
	default:
		return 0, false
	}
}

func (client *deploymentClient) rollback(ctx context.Context, policy modelcanary.Policy, reason string) error {
	snapshot, err := client.get(ctx, policy.DeploymentID)
	if err != nil {
		return err
	}
	if snapshot.Status == "rolled_back" {
		return nil
	}
	payload := map[string]string{
		"target_deployment_id": policy.RollbackDeploymentID,
		"reason":               reason,
	}
	return client.post(ctx, policy.DeploymentID+"/rollback", payload)
}

func (client *deploymentClient) get(ctx context.Context, deploymentID string) (deploymentSnapshot, error) {
	var response deploymentResponse
	if err := client.request(ctx, http.MethodGet, deploymentID, nil, &response); err != nil {
		return deploymentSnapshot{}, err
	}
	if !response.Success || response.Data.DeploymentID != deploymentID {
		return deploymentSnapshot{}, errors.New("deployment API returned an inconsistent snapshot")
	}
	return response.Data, nil
}

func (client *deploymentClient) post(ctx context.Context, suffix string, payload any) error {
	var response deploymentResponse
	if err := client.request(ctx, http.MethodPost, suffix, payload, &response); err != nil {
		return err
	}
	if !response.Success {
		return errors.New("deployment API did not acknowledge the operation")
	}
	return nil
}

func (client *deploymentClient) request(
	ctx context.Context, method, suffix string, payload any, output any,
) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	parts := strings.SplitN(suffix, "/", 2)
	endpoint := client.baseURL + "/api/v1/deployments/" + url.PathEscape(parts[0])
	if len(parts) == 2 {
		if parts[1] != "gray" && parts[1] != "rollback" {
			return fmt.Errorf("unsupported deployment operation %q", parts[1])
		}
		endpoint += "/" + parts[1]
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("X-Tenant-ID", client.tenant)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", controllerID)
	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	limited, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("deployment API status %d: %s", response.StatusCode, strings.TrimSpace(string(limited)))
	}
	if err := json.Unmarshal(limited, output); err != nil {
		return fmt.Errorf("decode deployment API response: %w", err)
	}
	return nil
}

func rollbackWithFreshContext(
	client *deploymentClient, policy modelcanary.Policy, decision modelcanary.Decision,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reason := "M08 N013 automatic stop: " + strings.Join(decision.StopReasons, ",")
	return client.rollback(ctx, policy, reason)
}

func writeDecision(path string, decision modelcanary.Decision) error {
	payload, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create model canary decision directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write model canary decision: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish model canary decision: %w", err)
	}
	return nil
}

func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func splitNonEmpty(value string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
