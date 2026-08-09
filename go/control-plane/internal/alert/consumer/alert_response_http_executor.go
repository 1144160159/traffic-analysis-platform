package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const alertResponseProviderResponseLimit = 1 << 20

var errAlertResponseAuthorityLookupNotConfigured = errors.New("alert response provider authority lookup is not configured")
var errAlertResponseCompensationLookupNotConfigured = errors.New("alert response compensation authority lookup is not configured")

// AlertResponseExecutionCommand is the immutable command submitted to an
// external response provider. IdempotencyKey is derived from the stable event
// identity and must be reused for every retry or authority lookup.
type AlertResponseExecutionCommand struct {
	EventID          string `json:"event_id"`
	JobID            string `json:"job_id"`
	TenantID         string `json:"tenant_id"`
	AlertID          string `json:"alert_id"`
	ActionID         string `json:"action_id"`
	Action           string `json:"action"`
	Target           string `json:"target"`
	Reason           string `json:"reason"`
	RequestedBy      string `json:"requested_by"`
	ApprovedBy       string `json:"approved_by"`
	ApprovalReason   string `json:"approval_reason"`
	TraceID          string `json:"trace_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	IdempotencyKey   string `json:"idempotency_key"`
}

// AlertResponseExecutionReceipt is business authority. HTTP success without a
// durable provider receipt and an explicit effect state is rejected.
type AlertResponseExecutionReceipt struct {
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

type AlertResponseExecutor interface {
	ExecuteAlertResponse(context.Context, AlertResponseExecutionCommand) (AlertResponseExecutionReceipt, error)
}

type AlertResponseExecutionAuthorityLookup struct {
	EventID        string                         `json:"event_id"`
	JobID          string                         `json:"job_id"`
	TenantID       string                         `json:"tenant_id"`
	IdempotencyKey string                         `json:"idempotency_key"`
	TraceID        string                         `json:"trace_id"`
	State          string                         `json:"state"`
	Provider       string                         `json:"provider"`
	CheckedAt      time.Time                      `json:"checked_at"`
	Receipt        *AlertResponseExecutionReceipt `json:"receipt,omitempty"`
}

type AlertResponseExecutionAuthority interface {
	LookupAlertResponseExecution(context.Context, AlertResponseExecutionCommand) (AlertResponseExecutionAuthorityLookup, error)
}

// AlertResponseCompensationCommand identifies one approved inverse operation.
// The provider idempotency key is derived from the durable control request and
// is never reused for execution or another compensation.
type AlertResponseCompensationCommand struct {
	RequestID                 string   `json:"request_id"`
	EventID                   string   `json:"event_id"`
	JobID                     string   `json:"job_id"`
	TenantID                  string   `json:"tenant_id"`
	AlertID                   string   `json:"alert_id"`
	OriginalActionID          string   `json:"original_action_id"`
	CompensationActionID      string   `json:"compensation_action_id"`
	OriginalProvider          string   `json:"original_provider"`
	OriginalProviderReceiptID string   `json:"original_provider_receipt_id"`
	OriginalEffectIDs         []string `json:"original_effect_ids"`
	RequestedBy               string   `json:"requested_by"`
	Reason                    string   `json:"reason"`
	TraceID                   string   `json:"trace_id"`
	AggregateVersion          int64    `json:"aggregate_version"`
	IdempotencyKey            string   `json:"idempotency_key"`
}

type AlertResponseCompensationReceipt struct {
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

type AlertResponseCompensationAuthorityLookup struct {
	RequestID      string                            `json:"request_id"`
	EventID        string                            `json:"event_id"`
	JobID          string                            `json:"job_id"`
	TenantID       string                            `json:"tenant_id"`
	IdempotencyKey string                            `json:"idempotency_key"`
	TraceID        string                            `json:"trace_id"`
	State          string                            `json:"state"`
	Provider       string                            `json:"provider"`
	CheckedAt      time.Time                         `json:"checked_at"`
	Receipt        *AlertResponseCompensationReceipt `json:"receipt,omitempty"`
}

type AlertResponseCompensator interface {
	CompensateAlertResponse(context.Context, AlertResponseCompensationCommand) (AlertResponseCompensationReceipt, error)
}

type AlertResponseCompensationAuthority interface {
	LookupAlertResponseCompensation(context.Context, AlertResponseCompensationCommand) (AlertResponseCompensationAuthorityLookup, error)
}

type HTTPAlertResponseExecutor struct {
	endpoint                   string
	lookupEndpoint             string
	compensationEndpoint       string
	compensationLookupEndpoint string
	token                      string
	client                     *http.Client
}

func NewHTTPAlertResponseExecutor(endpoint, token string, timeout time.Duration) (*HTTPAlertResponseExecutor, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("alert response executor endpoint must be an absolute http(s) URL")
	}
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 30 * time.Second
	}
	return &HTTPAlertResponseExecutor{
		endpoint: endpoint,
		token:    strings.TrimSpace(token),
		client:   &http.Client{Timeout: timeout},
	}, nil
}

func (executor *HTTPAlertResponseExecutor) ConfigureAuthorityLookup(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("alert response authority lookup endpoint must be an absolute http(s) URL")
	}
	executor.lookupEndpoint = endpoint
	return nil
}

func (executor *HTTPAlertResponseExecutor) ConfigureCompensation(endpoint, lookupEndpoint string) error {
	parsedEndpoint, err := parseAlertResponseProviderEndpoint(endpoint, "compensation")
	if err != nil {
		return err
	}
	parsedLookup, err := parseAlertResponseProviderEndpoint(lookupEndpoint, "compensation authority lookup")
	if err != nil {
		return err
	}
	executor.compensationEndpoint = parsedEndpoint
	executor.compensationLookupEndpoint = parsedLookup
	return nil
}

func parseAlertResponseProviderEndpoint(endpoint, label string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("alert response %s endpoint must be an absolute http(s) URL", label)
	}
	return endpoint, nil
}

func (executor *HTTPAlertResponseExecutor) ExecuteAlertResponse(
	ctx context.Context,
	command AlertResponseExecutionCommand,
) (AlertResponseExecutionReceipt, error) {
	var receipt AlertResponseExecutionReceipt
	if err := validateAlertResponseExecutionCommand(command); err != nil {
		return receipt, err
	}
	body, err := json.Marshal(command)
	if err != nil {
		return receipt, fmt.Errorf("marshal alert response command: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.endpoint, bytes.NewReader(body))
	if err != nil {
		return receipt, fmt.Errorf("create alert response executor request: %w", err)
	}
	executor.applyHeaders(request, command)
	response, err := executor.client.Do(request)
	if err != nil {
		return receipt, fmt.Errorf("execute alert response provider request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return receipt, fmt.Errorf("alert response provider returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := decodeAlertResponseProviderJSON(response.Body, &receipt); err != nil {
		return receipt, fmt.Errorf("decode alert response provider receipt: %w", err)
	}
	receipt = normalizeAlertResponseExecutionReceipt(receipt)
	if err := validateAlertResponseExecutionReceipt(receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (executor *HTTPAlertResponseExecutor) LookupAlertResponseExecution(
	ctx context.Context,
	command AlertResponseExecutionCommand,
) (AlertResponseExecutionAuthorityLookup, error) {
	var lookup AlertResponseExecutionAuthorityLookup
	if strings.TrimSpace(executor.lookupEndpoint) == "" {
		return lookup, errAlertResponseAuthorityLookupNotConfigured
	}
	if err := validateAlertResponseExecutionCommand(command); err != nil {
		return lookup, err
	}
	endpoint, err := url.Parse(executor.lookupEndpoint)
	if err != nil {
		return lookup, fmt.Errorf("parse alert response authority endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("event_id", command.EventID)
	query.Set("job_id", command.JobID)
	query.Set("tenant_id", command.TenantID)
	query.Set("idempotency_key", command.IdempotencyKey)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return lookup, fmt.Errorf("create alert response authority request: %w", err)
	}
	executor.applyHeaders(request, command)
	response, err := executor.client.Do(request)
	if err != nil {
		return lookup, fmt.Errorf("lookup alert response provider authority: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return lookup, fmt.Errorf("alert response authority returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := decodeAlertResponseProviderJSON(response.Body, &lookup); err != nil {
		return lookup, fmt.Errorf("decode alert response authority: %w", err)
	}
	lookup = normalizeAlertResponseExecutionAuthorityLookup(lookup)
	if err := validateAlertResponseExecutionAuthorityLookup(command, lookup); err != nil {
		return lookup, err
	}
	return lookup, nil
}

func (executor *HTTPAlertResponseExecutor) CompensateAlertResponse(
	ctx context.Context,
	command AlertResponseCompensationCommand,
) (AlertResponseCompensationReceipt, error) {
	var receipt AlertResponseCompensationReceipt
	if strings.TrimSpace(executor.compensationEndpoint) == "" {
		return receipt, fmt.Errorf("alert response compensation endpoint is not configured")
	}
	if err := validateAlertResponseCompensationCommand(command); err != nil {
		return receipt, err
	}
	body, err := json.Marshal(command)
	if err != nil {
		return receipt, fmt.Errorf("marshal alert response compensation command: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.compensationEndpoint, bytes.NewReader(body))
	if err != nil {
		return receipt, fmt.Errorf("create alert response compensation request: %w", err)
	}
	executor.applyCompensationHeaders(request, command)
	response, err := executor.client.Do(request)
	if err != nil {
		return receipt, fmt.Errorf("execute alert response compensation provider request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return receipt, fmt.Errorf("alert response compensation provider returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := decodeAlertResponseProviderJSON(response.Body, &receipt); err != nil {
		return receipt, fmt.Errorf("decode alert response compensation receipt: %w", err)
	}
	receipt = normalizeAlertResponseCompensationReceipt(receipt)
	if err := validateAlertResponseCompensationReceipt(command, receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (executor *HTTPAlertResponseExecutor) LookupAlertResponseCompensation(
	ctx context.Context,
	command AlertResponseCompensationCommand,
) (AlertResponseCompensationAuthorityLookup, error) {
	var lookup AlertResponseCompensationAuthorityLookup
	if strings.TrimSpace(executor.compensationLookupEndpoint) == "" {
		return lookup, errAlertResponseCompensationLookupNotConfigured
	}
	if err := validateAlertResponseCompensationCommand(command); err != nil {
		return lookup, err
	}
	endpoint, err := url.Parse(executor.compensationLookupEndpoint)
	if err != nil {
		return lookup, fmt.Errorf("parse alert response compensation authority endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("request_id", command.RequestID)
	query.Set("event_id", command.EventID)
	query.Set("job_id", command.JobID)
	query.Set("tenant_id", command.TenantID)
	query.Set("idempotency_key", command.IdempotencyKey)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return lookup, fmt.Errorf("create alert response compensation authority request: %w", err)
	}
	executor.applyCompensationHeaders(request, command)
	response, err := executor.client.Do(request)
	if err != nil {
		return lookup, fmt.Errorf("lookup alert response compensation authority: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return lookup, fmt.Errorf("alert response compensation authority returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := decodeAlertResponseProviderJSON(response.Body, &lookup); err != nil {
		return lookup, fmt.Errorf("decode alert response compensation authority: %w", err)
	}
	lookup = normalizeAlertResponseCompensationAuthorityLookup(lookup)
	if err := validateAlertResponseCompensationAuthorityLookup(command, lookup); err != nil {
		return lookup, err
	}
	return lookup, nil
}

func (executor *HTTPAlertResponseExecutor) applyHeaders(request *http.Request, command AlertResponseExecutionCommand) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", command.IdempotencyKey)
	request.Header.Set("X-Event-ID", command.EventID)
	request.Header.Set("X-Tenant-ID", command.TenantID)
	request.Header.Set("X-Trace-ID", command.TraceID)
	if executor.token != "" {
		request.Header.Set("Authorization", "Bearer "+executor.token)
	}
}

func (executor *HTTPAlertResponseExecutor) applyCompensationHeaders(request *http.Request, command AlertResponseCompensationCommand) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", command.IdempotencyKey)
	request.Header.Set("X-Request-ID", command.RequestID)
	request.Header.Set("X-Event-ID", command.EventID)
	request.Header.Set("X-Tenant-ID", command.TenantID)
	request.Header.Set("X-Trace-ID", command.TraceID)
	if executor.token != "" {
		request.Header.Set("Authorization", "Bearer "+executor.token)
	}
}

func decodeAlertResponseProviderJSON(reader io.Reader, target interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(reader, alertResponseProviderResponseLimit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("provider response must contain exactly one JSON object")
	}
	return nil
}

func validateAlertResponseExecutionCommand(command AlertResponseExecutionCommand) error {
	values := []string{command.EventID, command.JobID, command.TenantID, command.AlertID, command.ActionID,
		command.Action, command.Target, command.Reason, command.RequestedBy, command.ApprovedBy,
		command.ApprovalReason, command.TraceID, command.IdempotencyKey}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("alert response execution command is incomplete")
		}
	}
	if command.AggregateVersion < 2 || command.RequestedBy == command.ApprovedBy {
		return fmt.Errorf("alert response execution command lacks independent approval authority")
	}
	return nil
}

func normalizeAlertResponseExecutionReceipt(receipt AlertResponseExecutionReceipt) AlertResponseExecutionReceipt {
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	receipt.Provider = strings.TrimSpace(receipt.Provider)
	receipt.ProviderReceiptID = strings.TrimSpace(receipt.ProviderReceiptID)
	receipt.EffectState = strings.ToLower(strings.TrimSpace(receipt.EffectState))
	receipt.ErrorCode = strings.TrimSpace(receipt.ErrorCode)
	receipt.ErrorMessage = strings.TrimSpace(receipt.ErrorMessage)
	if receipt.EffectIDs == nil {
		receipt.EffectIDs = []string{}
	} else {
		for index := range receipt.EffectIDs {
			receipt.EffectIDs[index] = strings.TrimSpace(receipt.EffectIDs[index])
		}
	}
	if receipt.Result == nil {
		receipt.Result = map[string]interface{}{}
	}
	return receipt
}

func validateAlertResponseExecutionReceipt(receipt AlertResponseExecutionReceipt) error {
	if receipt.Status != "completed" && receipt.Status != "partial" && receipt.Status != "failed" {
		return fmt.Errorf("alert response provider receipt has invalid status")
	}
	if receipt.Provider == "" || receipt.ProviderReceiptID == "" || receipt.ExecutedAt.IsZero() {
		return fmt.Errorf("alert response provider receipt lacks durable identity")
	}
	if receipt.EffectState != "confirmed" && receipt.EffectState != "none" && receipt.EffectState != "unknown" {
		return fmt.Errorf("alert response provider receipt has invalid effect_state")
	}
	if receipt.Status == "completed" && (receipt.EffectState != "confirmed" || len(receipt.EffectIDs) == 0) {
		return fmt.Errorf("completed alert response receipt lacks confirmed effect identity")
	}
	if receipt.Status == "failed" && receipt.EffectState != "none" {
		return fmt.Errorf("failed alert response receipt cannot hide a possible external effect")
	}
	if receipt.EffectState == "confirmed" && len(receipt.EffectIDs) == 0 {
		return fmt.Errorf("confirmed alert response receipt lacks effect identity")
	}
	seenEffectIDs := make(map[string]struct{}, len(receipt.EffectIDs))
	for _, effectID := range receipt.EffectIDs {
		if effectID == "" {
			return fmt.Errorf("alert response provider receipt has empty effect identity")
		}
		if _, duplicate := seenEffectIDs[effectID]; duplicate {
			return fmt.Errorf("alert response provider receipt has duplicate effect identity")
		}
		seenEffectIDs[effectID] = struct{}{}
	}
	if (receipt.Status == "partial" || receipt.Status == "failed") && receipt.ErrorCode == "" {
		return fmt.Errorf("non-success alert response receipt lacks error_code")
	}
	return nil
}

func normalizeAlertResponseExecutionAuthorityLookup(lookup AlertResponseExecutionAuthorityLookup) AlertResponseExecutionAuthorityLookup {
	lookup.EventID = strings.TrimSpace(lookup.EventID)
	lookup.JobID = strings.TrimSpace(lookup.JobID)
	lookup.TenantID = strings.TrimSpace(lookup.TenantID)
	lookup.IdempotencyKey = strings.TrimSpace(lookup.IdempotencyKey)
	lookup.TraceID = strings.TrimSpace(lookup.TraceID)
	lookup.State = strings.ToLower(strings.TrimSpace(lookup.State))
	lookup.Provider = strings.TrimSpace(lookup.Provider)
	if lookup.Receipt != nil {
		receipt := normalizeAlertResponseExecutionReceipt(*lookup.Receipt)
		lookup.Receipt = &receipt
	}
	return lookup
}

func validateAlertResponseExecutionAuthorityLookup(command AlertResponseExecutionCommand, lookup AlertResponseExecutionAuthorityLookup) error {
	if lookup.EventID != command.EventID || lookup.JobID != command.JobID || lookup.TenantID != command.TenantID ||
		lookup.IdempotencyKey != command.IdempotencyKey || lookup.TraceID != command.TraceID {
		return fmt.Errorf("alert response authority identity mismatch")
	}
	if lookup.Provider == "" || lookup.CheckedAt.IsZero() {
		return fmt.Errorf("alert response authority response lacks provider identity")
	}
	if lookup.State != "receipt_found" && lookup.State != "absent" && lookup.State != "unknown" {
		return fmt.Errorf("alert response authority response has invalid state")
	}
	if lookup.State == "receipt_found" {
		if lookup.Receipt == nil {
			return fmt.Errorf("alert response authority omitted recovered receipt")
		}
		if err := validateAlertResponseExecutionReceipt(*lookup.Receipt); err != nil {
			return err
		}
		if lookup.Receipt.Provider != lookup.Provider {
			return fmt.Errorf("alert response authority provider/receipt mismatch")
		}
	} else if lookup.Receipt != nil {
		return fmt.Errorf("alert response authority supplied receipt without receipt_found state")
	}
	return nil
}

func validateAlertResponseCompensationCommand(command AlertResponseCompensationCommand) error {
	values := []string{command.RequestID, command.EventID, command.JobID, command.TenantID,
		command.AlertID, command.OriginalActionID, command.CompensationActionID,
		command.OriginalProvider, command.OriginalProviderReceiptID, command.RequestedBy,
		command.Reason, command.TraceID, command.IdempotencyKey}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("alert response compensation command is incomplete")
		}
	}
	if command.AggregateVersion <= 0 || len(command.OriginalEffectIDs) == 0 {
		return fmt.Errorf("alert response compensation command lacks durable effect authority")
	}
	seen := make(map[string]struct{}, len(command.OriginalEffectIDs))
	for _, effectID := range command.OriginalEffectIDs {
		effectID = strings.TrimSpace(effectID)
		if effectID == "" {
			return fmt.Errorf("alert response compensation command has empty effect identity")
		}
		if _, duplicate := seen[effectID]; duplicate {
			return fmt.Errorf("alert response compensation command has duplicate effect identity")
		}
		seen[effectID] = struct{}{}
	}
	return nil
}

func normalizeAlertResponseCompensationReceipt(receipt AlertResponseCompensationReceipt) AlertResponseCompensationReceipt {
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	receipt.Provider = strings.TrimSpace(receipt.Provider)
	receipt.ProviderReceiptID = strings.TrimSpace(receipt.ProviderReceiptID)
	receipt.EffectState = strings.ToLower(strings.TrimSpace(receipt.EffectState))
	receipt.ErrorCode = strings.TrimSpace(receipt.ErrorCode)
	receipt.ErrorMessage = strings.TrimSpace(receipt.ErrorMessage)
	if receipt.CompensatedEffectIDs == nil {
		receipt.CompensatedEffectIDs = []string{}
	} else {
		for index := range receipt.CompensatedEffectIDs {
			receipt.CompensatedEffectIDs[index] = strings.TrimSpace(receipt.CompensatedEffectIDs[index])
		}
	}
	if receipt.Result == nil {
		receipt.Result = map[string]interface{}{}
	}
	return receipt
}

func validateAlertResponseCompensationReceipt(command AlertResponseCompensationCommand, receipt AlertResponseCompensationReceipt) error {
	if receipt.Status != "compensated" && receipt.Status != "partial" && receipt.Status != "failed" {
		return fmt.Errorf("alert response compensation receipt has invalid status")
	}
	if receipt.Provider == "" || receipt.ProviderReceiptID == "" || receipt.CompensatedAt.IsZero() {
		return fmt.Errorf("alert response compensation receipt lacks durable identity")
	}
	if receipt.EffectState != "compensated" && receipt.EffectState != "none" && receipt.EffectState != "unknown" {
		return fmt.Errorf("alert response compensation receipt has invalid effect_state")
	}
	if receipt.Status == "compensated" && receipt.EffectState != "compensated" {
		return fmt.Errorf("completed compensation receipt lacks compensated effect authority")
	}
	if receipt.Status == "failed" && receipt.EffectState != "none" {
		return fmt.Errorf("failed compensation receipt cannot hide an uncertain inverse effect")
	}
	if receipt.Status != "compensated" && receipt.ErrorCode == "" {
		return fmt.Errorf("non-success compensation receipt lacks error_code")
	}
	if receipt.EffectState == "compensated" && !sameAlertResponseEffectSet(command.OriginalEffectIDs, receipt.CompensatedEffectIDs) {
		return fmt.Errorf("compensation receipt does not reconcile the original effect identities")
	}
	if receipt.EffectState != "compensated" && len(receipt.CompensatedEffectIDs) != 0 {
		return fmt.Errorf("compensation receipt supplies effect identities without compensated authority")
	}
	return nil
}

func sameAlertResponseEffectSet(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	seen := make(map[string]int, len(expected))
	for _, value := range expected {
		seen[strings.TrimSpace(value)]++
	}
	for _, value := range actual {
		value = strings.TrimSpace(value)
		if seen[value] != 1 {
			return false
		}
		delete(seen, value)
	}
	return len(seen) == 0
}

func normalizeAlertResponseCompensationAuthorityLookup(lookup AlertResponseCompensationAuthorityLookup) AlertResponseCompensationAuthorityLookup {
	lookup.RequestID = strings.TrimSpace(lookup.RequestID)
	lookup.EventID = strings.TrimSpace(lookup.EventID)
	lookup.JobID = strings.TrimSpace(lookup.JobID)
	lookup.TenantID = strings.TrimSpace(lookup.TenantID)
	lookup.IdempotencyKey = strings.TrimSpace(lookup.IdempotencyKey)
	lookup.TraceID = strings.TrimSpace(lookup.TraceID)
	lookup.State = strings.ToLower(strings.TrimSpace(lookup.State))
	lookup.Provider = strings.TrimSpace(lookup.Provider)
	if lookup.Receipt != nil {
		receipt := normalizeAlertResponseCompensationReceipt(*lookup.Receipt)
		lookup.Receipt = &receipt
	}
	return lookup
}

func validateAlertResponseCompensationAuthorityLookup(command AlertResponseCompensationCommand, lookup AlertResponseCompensationAuthorityLookup) error {
	if lookup.RequestID != command.RequestID || lookup.EventID != command.EventID ||
		lookup.JobID != command.JobID || lookup.TenantID != command.TenantID ||
		lookup.IdempotencyKey != command.IdempotencyKey || lookup.TraceID != command.TraceID {
		return fmt.Errorf("alert response compensation authority identity mismatch")
	}
	if lookup.Provider == "" || lookup.CheckedAt.IsZero() {
		return fmt.Errorf("alert response compensation authority lacks provider identity")
	}
	if lookup.State != "receipt_found" && lookup.State != "absent" && lookup.State != "unknown" {
		return fmt.Errorf("alert response compensation authority has invalid state")
	}
	if lookup.State == "receipt_found" {
		if lookup.Receipt == nil {
			return fmt.Errorf("alert response compensation authority omitted recovered receipt")
		}
		if err := validateAlertResponseCompensationReceipt(command, *lookup.Receipt); err != nil {
			return err
		}
		if lookup.Receipt.Provider != lookup.Provider {
			return fmt.Errorf("alert response compensation authority provider/receipt mismatch")
		}
	} else if lookup.Receipt != nil {
		return fmt.Errorf("alert response compensation authority supplied receipt without receipt_found state")
	}
	return nil
}
