package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestListAssetsCursorReturnsContractEnvelopeAndRejectsCrossTenantReplay(t *testing.T) {
	const signingKey = "asset-list-handler-test-signing-key"
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{
		Auth:   config.AuthConfig{JWTSigningKey: signingKey},
		Cursor: config.CursorConfig{Enabled: true},
	}, repo, zap.NewNop())
	handler := NewHTTPHandler(svc, zap.NewNop())

	snapshot := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)
	firstSeen := snapshot.Add(-24 * time.Hour)
	lastSeen := []time.Time{
		snapshot.Add(-time.Minute),
		snapshot.Add(-2 * time.Minute),
		snapshot.Add(-3 * time.Minute),
	}
	assetIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}

	mock.ExpectQuery(`SELECT clock_timestamp\(\),pg_current_snapshot\(\)::text`).
		WillReturnRows(sqlmock.NewRows([]string{"clock_timestamp", "pg_current_snapshot"}).AddRow(snapshot, "100:200:"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets WHERE tenant_id=\$1 AND updated_at<=\$2 AND pg_visible_in_snapshot\(xmin::text::xid8,\$3::pg_snapshot\)`).
		WithArgs("tenant-a", snapshot, "100:200:").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	rows := assetCursorRows()
	for index := range assetIDs {
		rows.AddRow(
			assetIDs[index], 1, "AST-"+assetIDs[index][:8], "tenant-a", "server", "active",
			"10.0.0.1", "00:11:22:33:44:55", "host", "vendor", "linux", "manual",
			"", "", "", "", "", 80, []byte(`{}`), []byte(`{}`), firstSeen, lastSeen[index],
		)
	}
	mock.ExpectQuery(`ORDER BY last_seen DESC,asset_id DESC LIMIT \$4`).
		WithArgs("tenant-a", snapshot, "100:200:", 3).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []config.AssetRecord `json:"data"`
		Meta struct {
			ContractVersion string `json:"contract_version"`
			SnapshotID      string `json:"snapshot_id"`
			Pagination      struct {
				Mode       string `json:"mode"`
				NextCursor string `json:"next_cursor"`
				HasMore    bool   `json:"has_more"`
			} `json:"pagination"`
		} `json:"meta"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data) != 2 || response.Meta.ContractVersion != "1" ||
		response.Meta.SnapshotID == "" || response.Meta.Pagination.Mode != "cursor" ||
		!response.Meta.Pagination.HasMore || response.Meta.Pagination.NextCursor == "" ||
		response.Error != nil {
		t.Fatalf("unexpected contract response: %#v", response)
	}

	// Cursor validation happens before the repository call, so a tenant-b token
	// cannot be used as a data-existence oracle.
	replay := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/assets?cursor="+response.Meta.Pagination.NextCursor,
		nil,
	)
	replay.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-b", []string{authmodel.ScopeAssetRead}))
	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusBadRequest {
		t.Fatalf("cross-tenant replay status=%d body=%s", replayRecorder.Code, replayRecorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListAssetsOffsetCompatibilityEmitsDeprecationAndRejectsAmbiguousInputs(t *testing.T) {
	const signingKey = "asset-list-offset-test-signing-key"
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{
		Auth:   config.AuthConfig{JWTSigningKey: signingKey},
		Cursor: config.CursorConfig{Enabled: true},
	}, repo, zap.NewNop())
	handler := NewHTTPHandler(svc, zap.NewNop())

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets WHERE tenant_id=\$1`).
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`ORDER BY last_seen DESC,asset_id DESC LIMIT \$2 OFFSET \$3`).
		WithArgs("tenant-a", 50, 0).
		WillReturnRows(assetCursorRows())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Meta struct {
			Deprecation struct {
				Offset      bool   `json:"offset"`
				Replacement string `json:"replacement"`
			} `json:"deprecation"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Meta.Deprecation.Offset || response.Meta.Deprecation.Replacement != "cursor" {
		t.Fatalf("missing offset deprecation metadata: %s", recorder.Body.String())
	}

	for _, path := range []string{
		"/api/v1/assets?cursor=opaque&offset=0",
		"/api/v1/assets?limit=not-an-int",
		"/api/v1/assets?offset=-1",
	} {
		bad := httptest.NewRequest(http.MethodGet, path, nil)
		bad.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
		badRecorder := httptest.NewRecorder()
		handler.ServeHTTP(badRecorder, bad)
		if badRecorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", path, badRecorder.Code, badRecorder.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListAssetsCursorRolloutFlagFailsClosed(t *testing.T) {
	const signingKey = "asset-list-disabled-cursor-test-key"
	codec, err := newAssetCursorCodec(signingKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token, err := codec.encode("tenant-a", config.AssetListFilter{}, 50, &config.AssetCursorPage{
		Total:        2,
		SnapshotAt:   now.Add(-time.Minute),
		SnapshotXIDs: "100:200:",
		LastSeen:     now.Add(-2 * time.Minute),
		LastAssetID:  uuid.NewString(),
		HasMore:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?cursor="+token, nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	recorder := httptest.NewRecorder()
	(&HTTPHandler{jwtSigningKey: signingKey, logger: zap.NewNop()}).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGRPCListAssetsUsesSignedPageTokenAndBindsProtoFilters(t *testing.T) {
	const signingKey = "asset-grpc-cursor-test-signing-key"
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{
		Auth:   config.AuthConfig{JWTSigningKey: signingKey},
		Cursor: config.CursorConfig{Enabled: true},
	}, repo, zap.NewNop())
	handler := NewAssetHandler(svc, repo, zap.NewNop())
	snapshot := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	firstSeen := snapshot.Add(-24 * time.Hour)
	firstAssetID := uuid.NewString()
	secondAssetID := uuid.NewString()

	mock.ExpectQuery(`SELECT clock_timestamp\(\),pg_current_snapshot\(\)::text`).
		WillReturnRows(sqlmock.NewRows([]string{"clock_timestamp", "pg_current_snapshot"}).AddRow(snapshot, "100:200:"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets WHERE tenant_id=\$1 AND ip_address LIKE \$2 AND vendor ILIKE \$3 AND updated_at<=\$4 AND pg_visible_in_snapshot\(xmin::text::xid8,\$5::pg_snapshot\)`).
		WithArgs("tenant-a", "10.20.%", "Acme", snapshot, "100:200:").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	rows := assetCursorRows().
		AddRow(firstAssetID, 1, "GRPC-1", "tenant-a", "server", "active",
			"10.20.0.1", "02:00:00:00:00:01", "host-1", "Acme", "linux", "manual",
			"", "", "", "", "", 50, []byte(`{}`), []byte(`{}`), firstSeen, snapshot.Add(-time.Minute)).
		AddRow(secondAssetID, 1, "GRPC-2", "tenant-a", "server", "active",
			"10.20.0.2", "02:00:00:00:00:02", "host-2", "Acme", "linux", "manual",
			"", "", "", "", "", 50, []byte(`{}`), []byte(`{}`), firstSeen, snapshot.Add(-2*time.Minute))
	mock.ExpectQuery(`ORDER BY last_seen DESC,asset_id DESC LIMIT \$6`).
		WithArgs("tenant-a", "10.20.%", "Acme", snapshot, "100:200:", 2).
		WillReturnRows(rows)

	response, err := handler.ListAssets(context.Background(), &pb.ListAssetsRequest{
		TenantId:     "tenant-a",
		PageSize:     1,
		IpPrefix:     "10.20.",
		VendorFilter: "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Assets) != 1 || response.TotalCount != 2 || response.NextPageToken == "" {
		t.Fatalf("unexpected gRPC page: %#v", response)
	}
	_, err = handler.ListAssets(context.Background(), &pb.ListAssetsRequest{
		TenantId:     "tenant-b",
		PageSize:     1,
		PageToken:    response.NextPageToken,
		IpPrefix:     "10.20.",
		VendorFilter: "Acme",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("cross-tenant page token error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func assetCursorRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"asset_id", "revision", "display_code", "tenant_id", "asset_type", "status",
		"ip_address", "mac_address", "hostname", "vendor", "os_type", "source",
		"vlan_id", "switch_port", "department", "campus", "owner", "criticality",
		"tags", "metadata", "first_seen", "last_seen",
	})
}
