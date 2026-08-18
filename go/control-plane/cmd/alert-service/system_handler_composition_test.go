package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"go.uber.org/zap"
)

type encryptedTrafficStatsCompositionStub struct {
	called bool
	query  api.EncryptedTrafficStatsQuery
}

func (s *encryptedTrafficStatsCompositionStub) Load(_ context.Context, query api.EncryptedTrafficStatsQuery) (api.EncryptedTrafficStats, error) {
	s.called = true
	s.query = query
	return api.EncryptedTrafficStats{
		TotalSessions:    3,
		ObservedSessions: 4,
		EncryptedRatio:   0.75,
		TLSSessions:      2,
		QUICSessions:     1,
	}, nil
}

func TestNewAlertSystemHandlerWiresEncryptedTrafficStatsSlice(t *testing.T) {
	statsService := &encryptedTrafficStatsCompositionStub{}
	handler := newAlertSystemHandler(nil, nil, zap.NewNop(), statsService)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/encrypted-traffic/stats?start_time=1000&end_time=2000", nil)
	ctx := context.WithValue(req.Context(), httpx.ContextKeyTenantID, "tenant-composition")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()

	handler.GetEncryptedTrafficStats(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected wired handler status 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !statsService.called || statsService.query != (api.EncryptedTrafficStatsQuery{TenantID: "tenant-composition", StartMilli: 1000, EndMilli: 2000}) {
		t.Fatalf("composition root did not wire the encrypted stats service: called=%v query=%+v", statsService.called, statsService.query)
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TotalSessions int64 `json:"total_sessions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.TotalSessions != 3 {
		t.Fatalf("unexpected wired response: %+v body %s", response, recorder.Body.String())
	}
}
