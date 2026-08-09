package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPlaybookExecutionProviderRequiresDurableStepReceipts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute" || r.Header.Get("Idempotency-Key") != "execution-a:execute" ||
			r.Header.Get("X-Tenant-ID") != "tenant-a" || r.Header.Get("X-Playbook-Name") != "isolate-host" ||
			r.Header.Get("Authorization") != "Bearer token-a" {
			t.Fatalf("unexpected request path=%s headers=%v", r.URL.Path, r.Header)
		}
		_ = json.NewEncoder(w).Encode(PlaybookExecutionProviderReceipt{Status: "succeeded", Steps: []PlaybookStepReceipt{{
			StepIndex: 0, ActionType: "quarantine", Provider: "provider-a",
			ProviderReceiptID: "receipt-a", Status: "succeeded", ExternalEffect: true,
		}}})
	}))
	defer server.Close()
	provider, err := NewHTTPPlaybookExecutionProvider(server.URL, "token-a", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := provider.Execute(context.Background(), PlaybookExecutionProviderRequest{
		ExecutionID: "execution-a", TenantID: "tenant-a", PlaybookName: "isolate-host",
		IdempotencyKey: "execution-a:execute",
	})
	if err != nil || len(receipt.Steps) != 1 || receipt.Steps[0].ProviderReceiptID != "receipt-a" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestHTTPPlaybookExecutionProviderRejectsReceiptWithoutProviderIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(PlaybookExecutionProviderReceipt{Status: "succeeded", Steps: []PlaybookStepReceipt{{
			StepIndex: 0, ActionType: "quarantine", Status: "succeeded", ExternalEffect: true,
		}}})
	}))
	defer server.Close()
	provider, err := NewHTTPPlaybookExecutionProvider(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Execute(context.Background(), PlaybookExecutionProviderRequest{IdempotencyKey: "execution-a:execute"})
	if err == nil {
		t.Fatal("expected missing provider identity to fail")
	}
}

func TestHTTPPlaybookExecutionProviderRejectsUnsafeURL(t *testing.T) {
	if _, err := NewHTTPPlaybookExecutionProvider("https://user:secret@example.invalid", "", time.Second); err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}
}
