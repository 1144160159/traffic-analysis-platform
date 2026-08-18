package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	graphprojection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
)

func TestProjectionStatusHandlerIsTenantScopedAndFailVisible(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := graphprojection.NewStatusRepository(db, true, true, 0)
	handler := &Handler{projectionStatus: repository, logger: zap.NewNop()}
	mock.ExpectQuery("SELECT count\\(\\*\\) FILTER .*projection_state='PENDING'").
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"pending", "applied", "dead", "oldest", "last"}).
			AddRow(0, 2, 1, nil, nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FILTER .*projection_kind='entity'").
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"entities", "relations", "revoked"}).
			AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT source_partition,source_offset,event_id,projection_sha256").
		WithArgs("tenant-a", graphprojection.Topic).
		WillReturnRows(sqlmock.NewRows([]string{
			"source_partition", "source_offset", "event_id", "projection_sha256", "source_timestamp_ms", "projected_at",
		}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/graph/projections/status", nil)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, "tenant-a")
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	handler.GetProjectionStatus(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"state":"failed"`) || !strings.Contains(body, `"dead_count":1`) ||
		!strings.Contains(body, `"complete":false`) {
		t.Fatalf("failed projection state was hidden: %s", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionStatusHandlerRejectsMissingTenant(t *testing.T) {
	handler := &Handler{logger: zap.NewNop()}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/graph/projections/status", nil)
	response := httptest.NewRecorder()
	handler.GetProjectionStatus(response, request)
	if response.Code == http.StatusOK {
		t.Fatalf("missing tenant unexpectedly succeeded: %s", response.Body.String())
	}
}
