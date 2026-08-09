package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

const (
	assetCursorVersion  = 1
	assetCursorMaxBytes = 8192
	assetCursorTTL      = 30 * time.Minute
	assetCursorSort     = "last_seen:desc,asset_id:desc"
)

var (
	errAssetCursorInvalid = errors.New("invalid asset cursor")
	errAssetCursorExpired = errors.New("asset cursor expired")
)

type assetCursorClaims struct {
	Version             int    `json:"v"`
	TenantID            string `json:"tenant_id"`
	FilterSHA256        string `json:"filter_sha256"`
	Limit               int    `json:"limit"`
	SnapshotUnixMicro   int64  `json:"snapshot_us"`
	SnapshotXIDs        string `json:"snapshot_xids"`
	LastSeenUnixMicro   int64  `json:"last_seen_us"`
	LastAssetID         string `json:"last_asset_id"`
	Total               int    `json:"total"`
	ExpiresAtUnixSecond int64  `json:"expires_at"`
}

type assetCursorCodec struct {
	signingKey []byte
	now        func() time.Time
}

func newAssetCursorCodec(secret string) (*assetCursorCodec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("%w: signing key is not configured", errAssetCursorInvalid)
	}
	derive := hmac.New(sha256.New, []byte(secret))
	_, _ = derive.Write([]byte("traffic.asset.cursor.v1"))
	return &assetCursorCodec{
		signingKey: derive.Sum(nil),
		now:        time.Now,
	}, nil
}

func (c *assetCursorCodec) encode(
	tenantID string,
	filter config.AssetListFilter,
	limit int,
	page *config.AssetCursorPage,
) (string, error) {
	if c == nil || len(c.signingKey) == 0 || page == nil || !page.HasMore {
		return "", fmt.Errorf("%w: incomplete cursor state", errAssetCursorInvalid)
	}
	if strings.TrimSpace(tenantID) == "" || limit < 1 || limit > 200 ||
		page.SnapshotAt.IsZero() || page.LastSeen.IsZero() || page.Total < 0 {
		return "", fmt.Errorf("%w: invalid cursor state", errAssetCursorInvalid)
	}
	if !validPGSnapshot(page.SnapshotXIDs) {
		return "", fmt.Errorf("%w: invalid MVCC snapshot", errAssetCursorInvalid)
	}
	if _, err := uuid.Parse(page.LastAssetID); err != nil {
		return "", fmt.Errorf("%w: invalid last asset id", errAssetCursorInvalid)
	}
	now := c.now().UTC()
	claims := assetCursorClaims{
		Version:             assetCursorVersion,
		TenantID:            tenantID,
		FilterSHA256:        assetCursorFilterSHA256(filter),
		Limit:               limit,
		SnapshotUnixMicro:   page.SnapshotAt.UTC().UnixMicro(),
		SnapshotXIDs:        page.SnapshotXIDs,
		LastSeenUnixMicro:   page.LastSeen.UTC().UnixMicro(),
		LastAssetID:         page.LastAssetID,
		Total:               page.Total,
		ExpiresAtUnixSecond: now.Add(assetCursorTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal asset cursor: %w", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, c.signingKey)
	_, _ = mac.Write([]byte(payloadPart))
	signaturePart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := payloadPart + "." + signaturePart
	if len(token) > assetCursorMaxBytes {
		return "", fmt.Errorf("%w: cursor is too large", errAssetCursorInvalid)
	}
	return token, nil
}

func (c *assetCursorCodec) decode(
	token string,
	tenantID string,
	filter config.AssetListFilter,
	explicitLimit *int,
) (*assetCursorClaims, error) {
	if c == nil || len(c.signingKey) == 0 || token == "" || len(token) > assetCursorMaxBytes {
		return nil, errAssetCursorInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errAssetCursorInvalid
	}
	providedMAC, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(providedMAC) != sha256.Size {
		return nil, errAssetCursorInvalid
	}
	expectedMAC := hmac.New(sha256.New, c.signingKey)
	_, _ = expectedMAC.Write([]byte(parts[0]))
	if !hmac.Equal(providedMAC, expectedMAC.Sum(nil)) {
		return nil, errAssetCursorInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > assetCursorMaxBytes {
		return nil, errAssetCursorInvalid
	}
	var claims assetCursorClaims
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, errAssetCursorInvalid
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, errAssetCursorInvalid
	}
	now := c.now().UTC()
	snapshotAt := time.UnixMicro(claims.SnapshotUnixMicro).UTC()
	lastSeen := time.UnixMicro(claims.LastSeenUnixMicro).UTC()
	if claims.Version != assetCursorVersion ||
		claims.TenantID != tenantID ||
		!hmac.Equal([]byte(claims.FilterSHA256), []byte(assetCursorFilterSHA256(filter))) ||
		claims.Limit < 1 || claims.Limit > 200 ||
		(explicitLimit != nil && *explicitLimit != claims.Limit) ||
		claims.Total < 0 ||
		!validPGSnapshot(claims.SnapshotXIDs) ||
		snapshotAt.IsZero() || snapshotAt.After(now.Add(30*time.Second)) ||
		lastSeen.IsZero() || lastSeen.After(snapshotAt.Add(30*time.Second)) {
		return nil, errAssetCursorInvalid
	}
	if _, err := uuid.Parse(claims.LastAssetID); err != nil {
		return nil, errAssetCursorInvalid
	}
	if claims.ExpiresAtUnixSecond <= now.Unix() {
		return nil, errAssetCursorExpired
	}
	return &claims, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func assetCursorFilterSHA256(filter config.AssetListFilter) string {
	normalized := struct {
		AssetType  string `json:"asset_type"`
		Status     string `json:"status"`
		Search     string `json:"search"`
		Department string `json:"department"`
		Campus     string `json:"campus"`
		IPPrefix   string `json:"ip_prefix"`
		Vendor     string `json:"vendor"`
		Sort       string `json:"sort"`
	}{
		AssetType:  strings.TrimSpace(filter.AssetType),
		Status:     strings.TrimSpace(filter.Status),
		Search:     strings.TrimSpace(filter.Search),
		Department: strings.TrimSpace(filter.Department),
		Campus:     strings.TrimSpace(filter.Campus),
		IPPrefix:   strings.TrimSpace(filter.IPPrefix),
		Vendor:     strings.TrimSpace(filter.Vendor),
		Sort:       assetCursorSort,
	}
	encoded, _ := json.Marshal(normalized)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validPGSnapshot(value string) bool {
	if value == "" || len(value) > 4096 {
		return false
	}
	colons := 0
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character == ':':
			colons++
		case character == ',':
		default:
			return false
		}
	}
	return colons == 2
}

func assetSnapshotID(tenantID, filterSHA, snapshotXIDs string, snapshotAt time.Time) string {
	sum := sha256.Sum256([]byte(
		tenantID + "\x00" + filterSHA + "\x00" + snapshotXIDs + "\x00" +
			snapshotAt.UTC().Format(time.RFC3339Nano),
	))
	return "asset-pg-" + hex.EncodeToString(sum[:16])
}
