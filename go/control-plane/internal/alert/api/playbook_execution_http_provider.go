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

const playbookProviderResponseLimit = 1 << 20

type HTTPPlaybookExecutionProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPPlaybookExecutionProvider(baseURL, token string, timeout time.Duration) (*HTTPPlaybookExecutionProvider, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("playbook execution provider URL must be an absolute http(s) URL without embedded credentials")
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	return &HTTPPlaybookExecutionProvider{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		client: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("playbook execution provider redirects are disabled")
		}},
	}, nil
}

func (provider *HTTPPlaybookExecutionProvider) Execute(ctx context.Context, request PlaybookExecutionProviderRequest) (PlaybookExecutionProviderReceipt, error) {
	return provider.send(ctx, "execute", request, nil)
}

func (provider *HTTPPlaybookExecutionProvider) Compensate(ctx context.Context, request PlaybookExecutionProviderRequest, prior PlaybookExecutionProviderReceipt) (PlaybookExecutionProviderReceipt, error) {
	return provider.send(ctx, "compensate", request, &prior)
}

func (provider *HTTPPlaybookExecutionProvider) send(ctx context.Context, phase string, request PlaybookExecutionProviderRequest, prior *PlaybookExecutionProviderReceipt) (PlaybookExecutionProviderReceipt, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"schema_version": 2, "phase": phase, "request": request, "prior_receipt": prior,
	})
	if err != nil {
		return PlaybookExecutionProviderReceipt{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/"+phase, bytes.NewReader(payload))
	if err != nil {
		return PlaybookExecutionProviderReceipt{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Idempotency-Key", request.IdempotencyKey)
	httpRequest.Header.Set("X-Tenant-ID", request.TenantID)
	httpRequest.Header.Set("X-Playbook-Name", request.PlaybookName)
	if provider.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+provider.token)
	}
	response, err := provider.client.Do(httpRequest)
	if err != nil {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("playbook execution provider request failed: %w", err)
	}
	defer response.Body.Close()
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, playbookProviderResponseLimit+1))
	if err != nil {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("read playbook execution provider response: %w", err)
	}
	if len(responsePayload) > playbookProviderResponseLimit {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("playbook execution provider response exceeds %d bytes", playbookProviderResponseLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("playbook execution provider returned HTTP %d", response.StatusCode)
	}
	var receipt PlaybookExecutionProviderReceipt
	decoder := json.NewDecoder(bytes.NewReader(responsePayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("decode playbook execution provider receipt: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return PlaybookExecutionProviderReceipt{}, fmt.Errorf("playbook execution provider returned trailing response data")
	}
	if err := validatePlaybookProviderTransportReceipt(receipt); err != nil {
		return PlaybookExecutionProviderReceipt{}, err
	}
	return receipt, nil
}

func validatePlaybookProviderTransportReceipt(receipt PlaybookExecutionProviderReceipt) error {
	status := strings.ToLower(strings.TrimSpace(receipt.Status))
	if status != "succeeded" && status != "partial" && status != "failed" {
		return fmt.Errorf("playbook execution provider receipt has an invalid status")
	}
	if len(receipt.Steps) == 0 {
		return fmt.Errorf("playbook execution provider receipt has no step receipts")
	}
	for _, step := range receipt.Steps {
		if step.StepIndex < 0 || strings.TrimSpace(step.ActionType) == "" ||
			strings.TrimSpace(step.Provider) == "" || strings.TrimSpace(step.ProviderReceiptID) == "" {
			return fmt.Errorf("playbook execution provider receipt lacks a durable step identity")
		}
	}
	return nil
}
