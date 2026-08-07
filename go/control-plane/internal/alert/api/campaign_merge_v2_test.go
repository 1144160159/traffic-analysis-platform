package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func TestCampaignMergeRejectsCrossTenantTargetBeforePersistence(t *testing.T) {
	ctx := context.WithValue(context.Background(), httpx.ContextKeyTenantID, "tenant-a")
	request := campaignActionRequest{
		ActionID: "campaign-merge", ExpectedRevision: int64Pointer(1), TargetExpectedRevision: int64Pointer(2),
	}
	handler := &SystemHandler{}
	_, err := handler.commitCampaignMergeV2Command(ctx, nil, request, campaignActionSpecs["campaign-merge"],
		campaignDTO{TenantID: "tenant-a", CampaignID: "source"},
		campaignDTO{TenantID: "tenant-b", CampaignID: "target"}, "cross-tenant-idempotency", "request-sha")
	var commandErr *campaignCommandError
	if !errors.As(err, &commandErr) || commandErr.status != http.StatusNotFound || commandErr.code != "CAMPAIGN_NOT_FOUND" {
		t.Fatalf("cross-tenant merge err=%v", err)
	}
}
