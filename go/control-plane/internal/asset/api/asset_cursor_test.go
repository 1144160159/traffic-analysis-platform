package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestAssetCursorRoundTripBindsTenantFilterSortAndLimit(t *testing.T) {
	now := time.Date(2026, 7, 31, 5, 0, 0, 123000000, time.UTC)
	codec := testAssetCursorCodec(t, now)
	filter := config.AssetListFilter{AssetType: "server", Search: "edge"}
	page := &config.AssetCursorPage{
		Total:        7,
		SnapshotAt:   now.Add(-time.Minute),
		SnapshotXIDs: "100:200:",
		LastSeen:     now.Add(-2 * time.Minute),
		LastAssetID:  uuid.NewString(),
		HasMore:      true,
	}
	token, err := codec.encode("tenant-a", filter, 50, page)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	explicitLimit := 50
	claims, err := codec.decode(token, "tenant-a", filter, &explicitLimit)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if claims.Total != page.Total || claims.LastAssetID != page.LastAssetID ||
		claims.SnapshotUnixMicro != page.SnapshotAt.UnixMicro() {
		t.Fatalf("unexpected claims: %#v", claims)
	}

	for name, testCase := range map[string]struct {
		tenant string
		filter config.AssetListFilter
		limit  int
	}{
		"cross tenant":   {tenant: "tenant-b", filter: filter, limit: 50},
		"changed filter": {tenant: "tenant-a", filter: config.AssetListFilter{AssetType: "endpoint", Search: "edge"}, limit: 50},
		"changed limit":  {tenant: "tenant-a", filter: filter, limit: 100},
	} {
		t.Run(name, func(t *testing.T) {
			gotLimit := testCase.limit
			if _, err := codec.decode(token, testCase.tenant, testCase.filter, &gotLimit); !errors.Is(err, errAssetCursorInvalid) {
				t.Fatalf("decode error = %v, want invalid cursor", err)
			}
		})
	}
}

func TestAssetCursorRejectsTamperAndExpiry(t *testing.T) {
	now := time.Date(2026, 7, 31, 6, 0, 0, 0, time.UTC)
	codec := testAssetCursorCodec(t, now)
	page := &config.AssetCursorPage{
		Total:        2,
		SnapshotAt:   now.Add(-time.Minute),
		SnapshotXIDs: "100:200:",
		LastSeen:     now.Add(-2 * time.Minute),
		LastAssetID:  uuid.NewString(),
		HasMore:      true,
	}
	token, err := codec.encode("tenant-a", config.AssetListFilter{}, 25, page)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	tampered := "A" + token[1:]
	if _, err := codec.decode(tampered, "tenant-a", config.AssetListFilter{}, nil); !errors.Is(err, errAssetCursorInvalid) {
		t.Fatalf("tampered decode error = %v, want invalid cursor", err)
	}
	codec.now = func() time.Time { return now.Add(assetCursorTTL + time.Second) }
	if _, err := codec.decode(token, "tenant-a", config.AssetListFilter{}, nil); !errors.Is(err, errAssetCursorExpired) {
		t.Fatalf("expired decode error = %v, want expired cursor", err)
	}
}

func TestAssetCursorRejectsTrailingJSONEvenWithValidSignature(t *testing.T) {
	now := time.Date(2026, 7, 31, 6, 0, 0, 0, time.UTC)
	codec := testAssetCursorCodec(t, now)
	page := &config.AssetCursorPage{
		Total:        2,
		SnapshotAt:   now.Add(-time.Minute),
		SnapshotXIDs: "100:200:",
		LastSeen:     now.Add(-2 * time.Minute),
		LastAssetID:  uuid.NewString(),
		HasMore:      true,
	}
	token, err := codec.encode("tenant-a", config.AssetListFilter{}, 25, page)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatal("expected two cursor parts")
	}
	// A token signed by this service but containing an extra JSON value still
	// fails the strict decoder.
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(append(payload, []byte(`{}`)...))
	mac := hmac.New(sha256.New, codec.signingKey)
	_, _ = mac.Write([]byte(payloadPart))
	token = payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, err := codec.decode(token, "tenant-a", config.AssetListFilter{}, nil); !errors.Is(err, errAssetCursorInvalid) {
		t.Fatalf("decode error = %v, want invalid cursor", err)
	}
}

func testAssetCursorCodec(t *testing.T, now time.Time) *assetCursorCodec {
	t.Helper()
	codec, err := newAssetCursorCodec("test-only-asset-cursor-signing-secret")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}
	codec.now = func() time.Time { return now }
	return codec
}
