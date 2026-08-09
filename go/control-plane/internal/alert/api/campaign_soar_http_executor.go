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

const campaignSOARProviderResponseLimit = 1 << 20

// HTTPCampaignSOARExecutor is a real provider adapter. It only accepts a
// provider response that contains a durable receipt identity and explicit
// external-effect flag; HTTP 2xx by itself is never treated as completion.
type HTTPCampaignSOARExecutor struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPCampaignSOARExecutor(baseURL, token string, timeout time.Duration) (*HTTPCampaignSOARExecutor, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("campaign SOAR executor URL must be an absolute http(s) URL without embedded credentials")
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	return &HTTPCampaignSOARExecutor{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return fmt.Errorf("campaign SOAR provider redirects are disabled")
			},
		},
	}, nil
}

func (e *HTTPCampaignSOARExecutor) Execute(ctx context.Context, request CampaignSOARExecutionRequest) (CampaignSOARReceipt, error) {
	return e.send(ctx, "execute", request, nil)
}

func (e *HTTPCampaignSOARExecutor) Compensate(ctx context.Context, request CampaignSOARExecutionRequest, prior CampaignSOARReceipt) (CampaignSOARReceipt, error) {
	return e.send(ctx, "compensate", request, &prior)
}

func (e *HTTPCampaignSOARExecutor) send(ctx context.Context, phase string, request CampaignSOARExecutionRequest, prior *CampaignSOARReceipt) (CampaignSOARReceipt, error) {
	body, err := json.Marshal(map[string]interface{}{
		"schema_version": 1,
		"phase":          phase,
		"request":        request,
		"prior_receipt":  prior,
	})
	if err != nil {
		return CampaignSOARReceipt{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/"+phase, bytes.NewReader(body))
	if err != nil {
		return CampaignSOARReceipt{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Idempotency-Key", request.JobID+":"+phase)
	httpRequest.Header.Set("X-Tenant-ID", request.TenantID)
	httpRequest.Header.Set("X-Campaign-ID", request.CampaignID)
	if e.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+e.token)
	}
	response, err := e.client.Do(httpRequest)
	if err != nil {
		return CampaignSOARReceipt{}, fmt.Errorf("campaign SOAR provider request failed: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, campaignSOARProviderResponseLimit+1))
	if err != nil {
		return CampaignSOARReceipt{}, fmt.Errorf("read campaign SOAR provider response: %w", err)
	}
	if len(payload) > campaignSOARProviderResponseLimit {
		return CampaignSOARReceipt{}, fmt.Errorf("campaign SOAR provider response exceeds %d bytes", campaignSOARProviderResponseLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return CampaignSOARReceipt{}, fmt.Errorf("campaign SOAR provider returned HTTP %d", response.StatusCode)
	}
	var receipt CampaignSOARReceipt
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return CampaignSOARReceipt{}, fmt.Errorf("decode campaign SOAR provider receipt: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return CampaignSOARReceipt{}, fmt.Errorf("campaign SOAR provider returned trailing response data")
	}
	receipt, err = normalizeCampaignSOARReceipt(receipt)
	if err != nil {
		return CampaignSOARReceipt{}, err
	}
	return receipt, nil
}
