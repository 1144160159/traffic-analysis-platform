package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteCampaignReadErrorClassifiesClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeCampaignReadError(recorder, context.Background(), context.Canceled)
	if recorder.Code != statusClientClosedRequest {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, statusClientClosedRequest, recorder.Body.String())
	}
}

func TestWriteCampaignReadErrorPreservesServerFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "not found", err: sql.ErrNoRows, want: http.StatusNotFound},
		{name: "internal", err: errors.New("scan failed"), want: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeCampaignReadError(recorder, context.Background(), test.err)
			if recorder.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}
