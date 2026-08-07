package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPCampaignSOARExecutorRequiresDurableExternalReceipt(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/execute" || r.Header.Get("Idempotency-Key") != "soar-job-1:execute" ||
			r.Header.Get("X-Tenant-ID") != "tenant-a" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected provider request path=%s headers=%v", r.URL.Path, r.Header)
		}
		var payload struct {
			Phase   string                       `json:"phase"`
			Request CampaignSOARExecutionRequest `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if payload.Phase != "execute" || payload.Request.CampaignID != "campaign-a" {
			t.Errorf("unexpected payload %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(CampaignSOARReceipt{
			Provider: "sentinel-soar", ProviderReceiptID: "provider-receipt-1",
			Status: "succeeded", ExternalEffect: true,
			Detail: map[string]interface{}{"rule_id": "isolation-1"},
		})
	}))
	defer server.Close()
	executor, err := NewHTTPCampaignSOARExecutor(server.URL, "test-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := executor.Execute(context.Background(), CampaignSOARExecutionRequest{
		JobID: "soar-job-1", TenantID: "tenant-a", CampaignID: "campaign-a",
		PlaybookID: "contain-host", Target: "asset-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || receipt.ProviderReceiptID != "provider-receipt-1" || !receipt.ExternalEffect {
		t.Fatalf("receipt=%+v requests=%d", receipt, requestCount)
	}
}

func TestHTTPCampaignSOARExecutorRejectsSuccessWithoutExternalEffect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(CampaignSOARReceipt{
			Provider: "sentinel-soar", ProviderReceiptID: "provider-receipt-no-effect",
			Status: "succeeded", ExternalEffect: false,
		})
	}))
	defer server.Close()
	executor, err := NewHTTPCampaignSOARExecutor(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), CampaignSOARExecutionRequest{JobID: "job-no-effect"}); err == nil {
		t.Fatal("expected receipt without external effect to be rejected")
	}
}

func TestHTTPCampaignSOARExecutorCompensationCarriesPriorReceipt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compensate" || r.Header.Get("Idempotency-Key") != "soar-job-2:compensate" {
			t.Errorf("unexpected compensation request %s %s", r.URL.Path, r.Header.Get("Idempotency-Key"))
		}
		var payload struct {
			Prior *CampaignSOARReceipt `json:"prior_receipt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode compensation request: %v", err)
		}
		if payload.Prior == nil || payload.Prior.ProviderReceiptID != "provider-receipt-2" {
			t.Errorf("missing prior receipt: %+v", payload.Prior)
		}
		_ = json.NewEncoder(w).Encode(CampaignSOARReceipt{
			Provider: "sentinel-soar", ProviderReceiptID: "provider-compensation-2",
			Status: "succeeded", ExternalEffect: true,
		})
	}))
	defer server.Close()
	executor, err := NewHTTPCampaignSOARExecutor(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := executor.Compensate(context.Background(), CampaignSOARExecutionRequest{JobID: "soar-job-2"}, CampaignSOARReceipt{
		Provider: "sentinel-soar", ProviderReceiptID: "provider-receipt-2", Status: "succeeded", ExternalEffect: true,
	})
	if err != nil || receipt.ProviderReceiptID != "provider-compensation-2" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestNewHTTPCampaignSOARExecutorRejectsUnsafeURL(t *testing.T) {
	for _, candidate := range []string{"", "ftp://soar.example", "https://user:secret@soar.example"} {
		if _, err := NewHTTPCampaignSOARExecutor(candidate, "", time.Second); err == nil {
			t.Fatalf("expected invalid URL %q to be rejected", candidate)
		}
	}
}
