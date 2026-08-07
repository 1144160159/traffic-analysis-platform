package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dashboardTaskProviderResponseLimit = 1 << 20

// DashboardTaskExecutionRequest is the immutable command sent to the external
// executor. RequestEventID is also the provider idempotency key.
type DashboardTaskExecutionRequest struct {
	RequestEventID string                 `json:"request_event_id"`
	TenantID       string                 `json:"tenant_id"`
	TaskID         string                 `json:"task_id"`
	ActionID       string                 `json:"action_id"`
	TaskType       string                 `json:"task_type"`
	Target         string                 `json:"target"`
	Priority       string                 `json:"priority"`
	SnapshotID     string                 `json:"snapshot_id"`
	Reason         string                 `json:"reason"`
	RequestedBy    string                 `json:"requested_by"`
	TraceID        string                 `json:"trace_id"`
	Context        map[string]interface{} `json:"context"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

// DashboardTaskExecutionReceipt is accepted only when the provider supplies a
// durable identity and an explicit external-effect state. HTTP 2xx alone is
// never treated as business success.
type DashboardTaskExecutionReceipt struct {
	Status            string                 `json:"status"`
	Provider          string                 `json:"provider"`
	ProviderReceiptID string                 `json:"provider_receipt_id"`
	EffectState       string                 `json:"effect_state"`
	EffectIDs         []string               `json:"effect_ids"`
	Result            map[string]interface{} `json:"result"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	ExecutedAt        time.Time              `json:"executed_at"`
}

type DashboardTaskExecutor interface {
	ExecuteDashboardTask(context.Context, DashboardTaskExecutionRequest) (DashboardTaskExecutionReceipt, error)
}

// DashboardTaskCompensationRequest is sent only after PostgreSQL proves that
// the original task has a confirmed, durable execution receipt.
type DashboardTaskCompensationRequest struct {
	RequestEventID          string                 `json:"request_event_id"`
	TenantID                string                 `json:"tenant_id"`
	TaskID                  string                 `json:"task_id"`
	ActionID                string                 `json:"action_id"`
	SnapshotID              string                 `json:"snapshot_id"`
	Reason                  string                 `json:"reason"`
	RequestedBy             string                 `json:"requested_by"`
	TraceID                 string                 `json:"trace_id"`
	OriginalProvider        string                 `json:"original_provider"`
	OriginalReceiptID       string                 `json:"original_provider_receipt_id"`
	OriginalEffectIDs       []string               `json:"original_effect_ids"`
	OriginalResult          map[string]interface{} `json:"original_result"`
	CompensationIdempotency string                 `json:"idempotency_key"`
}

type DashboardTaskCompensationReceipt struct {
	Status               string                 `json:"status"`
	Provider             string                 `json:"provider"`
	ProviderReceiptID    string                 `json:"provider_receipt_id"`
	EffectState          string                 `json:"effect_state"`
	CompensatedEffectIDs []string               `json:"compensated_effect_ids"`
	Result               map[string]interface{} `json:"result"`
	ErrorCode            string                 `json:"error_code,omitempty"`
	ErrorMessage         string                 `json:"error_message,omitempty"`
	CompensatedAt        time.Time              `json:"compensated_at"`
}

type DashboardTaskCompensator interface {
	CompensateDashboardTask(context.Context, DashboardTaskCompensationRequest) (DashboardTaskCompensationReceipt, error)
}

type HTTPDashboardTaskExecutor struct {
	endpoint string
	token    string
	client   *http.Client
}

type HTTPDashboardTaskCompensator struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewHTTPDashboardTaskExecutor(endpoint, token string, timeout time.Duration) (*HTTPDashboardTaskExecutor, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("dashboard task executor URL must be an absolute http(s) URL without embedded credentials")
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	return &HTTPDashboardTaskExecutor{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    strings.TrimSpace(token),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return fmt.Errorf("dashboard task executor redirects are disabled")
			},
		},
	}, nil
}

func NewHTTPDashboardTaskCompensator(endpoint, token string, timeout time.Duration) (*HTTPDashboardTaskCompensator, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("dashboard task compensator URL must be an absolute http(s) URL without embedded credentials")
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	return &HTTPDashboardTaskCompensator{
		endpoint: strings.TrimRight(endpoint, "/"), token: strings.TrimSpace(token),
		client: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("dashboard task compensator redirects are disabled")
		}},
	}, nil
}

func (executor *HTTPDashboardTaskExecutor) ExecuteDashboardTask(ctx context.Context, command DashboardTaskExecutionRequest) (DashboardTaskExecutionReceipt, error) {
	payload, err := json.Marshal(map[string]interface{}{"schema_version": 1, "command": command})
	if err != nil {
		return DashboardTaskExecutionReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.endpoint, bytes.NewReader(payload))
	if err != nil {
		return DashboardTaskExecutionReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", command.IdempotencyKey)
	request.Header.Set("X-Tenant-ID", command.TenantID)
	request.Header.Set("X-Trace-ID", command.TraceID)
	if executor.token != "" {
		request.Header.Set("Authorization", "Bearer "+executor.token)
	}
	response, err := executor.client.Do(request)
	if err != nil {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("dashboard task executor request failed: %w", err)
	}
	defer response.Body.Close()
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, dashboardTaskProviderResponseLimit+1))
	if err != nil {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("read dashboard task executor response: %w", err)
	}
	if len(responsePayload) > dashboardTaskProviderResponseLimit {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("dashboard task executor response exceeds %d bytes", dashboardTaskProviderResponseLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("dashboard task executor returned HTTP %d", response.StatusCode)
	}
	var receipt DashboardTaskExecutionReceipt
	decoder := json.NewDecoder(bytes.NewReader(responsePayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("decode dashboard task executor receipt: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return DashboardTaskExecutionReceipt{}, fmt.Errorf("dashboard task executor returned trailing response data")
	}
	if err := validateDashboardTaskExecutionReceipt(receipt); err != nil {
		return DashboardTaskExecutionReceipt{}, err
	}
	return normalizeDashboardTaskExecutionReceipt(receipt), nil
}

func (compensator *HTTPDashboardTaskCompensator) CompensateDashboardTask(ctx context.Context, command DashboardTaskCompensationRequest) (DashboardTaskCompensationReceipt, error) {
	payload, err := json.Marshal(map[string]interface{}{"schema_version": 1, "command": command})
	if err != nil {
		return DashboardTaskCompensationReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, compensator.endpoint, bytes.NewReader(payload))
	if err != nil {
		return DashboardTaskCompensationReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", command.CompensationIdempotency)
	request.Header.Set("X-Tenant-ID", command.TenantID)
	request.Header.Set("X-Trace-ID", command.TraceID)
	if compensator.token != "" {
		request.Header.Set("Authorization", "Bearer "+compensator.token)
	}
	response, err := compensator.client.Do(request)
	if err != nil {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("dashboard task compensator request failed: %w", err)
	}
	defer response.Body.Close()
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, dashboardTaskProviderResponseLimit+1))
	if err != nil {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("read dashboard task compensator response: %w", err)
	}
	if len(responsePayload) > dashboardTaskProviderResponseLimit {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("dashboard task compensator response exceeds %d bytes", dashboardTaskProviderResponseLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("dashboard task compensator returned HTTP %d", response.StatusCode)
	}
	var receipt DashboardTaskCompensationReceipt
	decoder := json.NewDecoder(bytes.NewReader(responsePayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("decode dashboard task compensator receipt: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return DashboardTaskCompensationReceipt{}, fmt.Errorf("dashboard task compensator returned trailing response data")
	}
	if err := validateDashboardTaskCompensationReceipt(receipt); err != nil {
		return DashboardTaskCompensationReceipt{}, err
	}
	return normalizeDashboardTaskCompensationReceipt(receipt), nil
}

func normalizeDashboardTaskExecutionReceipt(receipt DashboardTaskExecutionReceipt) DashboardTaskExecutionReceipt {
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	receipt.Provider = strings.TrimSpace(receipt.Provider)
	receipt.ProviderReceiptID = strings.TrimSpace(receipt.ProviderReceiptID)
	receipt.EffectState = strings.ToLower(strings.TrimSpace(receipt.EffectState))
	receipt.ErrorCode = strings.TrimSpace(receipt.ErrorCode)
	receipt.ErrorMessage = strings.TrimSpace(receipt.ErrorMessage)
	for index := range receipt.EffectIDs {
		receipt.EffectIDs[index] = strings.TrimSpace(receipt.EffectIDs[index])
	}
	if receipt.Result == nil {
		receipt.Result = map[string]interface{}{}
	}
	return receipt
}

func validateDashboardTaskExecutionReceipt(receipt DashboardTaskExecutionReceipt) error {
	receipt = normalizeDashboardTaskExecutionReceipt(receipt)
	if receipt.Status != "completed" && receipt.Status != "partial" && receipt.Status != "failed" {
		return fmt.Errorf("dashboard task executor receipt has an invalid status")
	}
	if receipt.Provider == "" || receipt.ProviderReceiptID == "" || receipt.ExecutedAt.IsZero() {
		return fmt.Errorf("dashboard task executor receipt lacks a durable provider identity")
	}
	if receipt.EffectState != "confirmed" && receipt.EffectState != "none" && receipt.EffectState != "unknown" {
		return fmt.Errorf("dashboard task executor receipt has an invalid effect_state")
	}
	seen := make(map[string]struct{}, len(receipt.EffectIDs))
	for _, effectID := range receipt.EffectIDs {
		effectID = strings.TrimSpace(effectID)
		if effectID == "" {
			return fmt.Errorf("dashboard task executor receipt contains an empty effect identity")
		}
		if _, exists := seen[effectID]; exists {
			return fmt.Errorf("dashboard task executor receipt contains duplicate effect identities")
		}
		seen[effectID] = struct{}{}
	}
	if receipt.Status == "completed" && (receipt.EffectState != "confirmed" || len(receipt.EffectIDs) == 0) {
		return fmt.Errorf("dashboard task completion requires confirmed external effects")
	}
	if receipt.Status == "failed" && receipt.EffectState == "unknown" {
		return fmt.Errorf("unknown external effect must be represented as partial, not failed")
	}
	return nil
}

func normalizeDashboardTaskCompensationReceipt(receipt DashboardTaskCompensationReceipt) DashboardTaskCompensationReceipt {
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	receipt.Provider = strings.TrimSpace(receipt.Provider)
	receipt.ProviderReceiptID = strings.TrimSpace(receipt.ProviderReceiptID)
	receipt.EffectState = strings.ToLower(strings.TrimSpace(receipt.EffectState))
	receipt.ErrorCode = strings.TrimSpace(receipt.ErrorCode)
	receipt.ErrorMessage = strings.TrimSpace(receipt.ErrorMessage)
	for index := range receipt.CompensatedEffectIDs {
		receipt.CompensatedEffectIDs[index] = strings.TrimSpace(receipt.CompensatedEffectIDs[index])
	}
	if receipt.Result == nil {
		receipt.Result = map[string]interface{}{}
	}
	return receipt
}

func validateDashboardTaskCompensationReceipt(receipt DashboardTaskCompensationReceipt) error {
	receipt = normalizeDashboardTaskCompensationReceipt(receipt)
	if receipt.Status != "compensated" && receipt.Status != "compensation_partial" && receipt.Status != "compensation_failed" {
		return fmt.Errorf("dashboard task compensator receipt has an invalid status")
	}
	if receipt.Provider == "" || receipt.ProviderReceiptID == "" || receipt.CompensatedAt.IsZero() {
		return fmt.Errorf("dashboard task compensator receipt lacks a durable provider identity")
	}
	if receipt.EffectState != "confirmed" && receipt.EffectState != "none" && receipt.EffectState != "unknown" {
		return fmt.Errorf("dashboard task compensator receipt has an invalid effect_state")
	}
	seen := make(map[string]struct{}, len(receipt.CompensatedEffectIDs))
	for _, effectID := range receipt.CompensatedEffectIDs {
		if effectID == "" {
			return fmt.Errorf("dashboard task compensator receipt contains an empty effect identity")
		}
		if _, exists := seen[effectID]; exists {
			return fmt.Errorf("dashboard task compensator receipt contains duplicate effect identities")
		}
		seen[effectID] = struct{}{}
	}
	if receipt.Status == "compensated" && (receipt.EffectState != "confirmed" || len(receipt.CompensatedEffectIDs) == 0) {
		return fmt.Errorf("dashboard task compensation requires confirmed external effects")
	}
	if receipt.Status == "compensation_failed" && receipt.EffectState == "unknown" {
		return fmt.Errorf("unknown compensation effect must be represented as compensation_partial")
	}
	return nil
}
