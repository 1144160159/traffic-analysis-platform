package repository

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	searchCursorVersion  = 1
	searchCursorMaxBytes = 8192
	searchCursorDomain   = "traffic.alert.search.cursor.v1"
)

var (
	errSearchCursorInvalid = stdErrors.New("invalid alert search cursor")
	errSearchCursorExpired = stdErrors.New("alert search cursor expired")
)

type searchCursorClaims struct {
	Version             int               `json:"v"`
	TenantID            string            `json:"tenant_id"`
	QuerySHA256         string            `json:"query_sha256"`
	Mode                string            `json:"mode"`
	Size                int               `json:"size"`
	SortValues          []json.RawMessage `json:"sort_values"`
	PITID               string            `json:"pit_id,omitempty"`
	SnapshotUnixMilli   int64             `json:"snapshot_ms"`
	ExpiresAtUnixSecond int64             `json:"expires_at"`
}

type searchCursorCodec struct {
	signingKey []byte
	ttl        time.Duration
	now        func() time.Time
}

func newSearchCursorCodec(secret string, ttl time.Duration) (*searchCursorCodec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("%w: signing key is not configured", errSearchCursorInvalid)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("%w: ttl must be positive", errSearchCursorInvalid)
	}
	derive := hmac.New(sha256.New, []byte(secret))
	_, _ = derive.Write([]byte(searchCursorDomain))
	return &searchCursorCodec{signingKey: derive.Sum(nil), ttl: ttl, now: time.Now}, nil
}

func (c *searchCursorCodec) encode(
	tenantID, querySHA256, mode string,
	size int,
	sortValues []json.RawMessage,
	pitID string,
	snapshotAt time.Time,
) (string, error) {
	if c == nil || len(c.signingKey) == 0 || strings.TrimSpace(tenantID) == "" ||
		len(querySHA256) != sha256.Size*2 || (mode != SearchCursorModeLive && mode != SearchCursorModePIT) ||
		size < 1 || !validSearchSortValues(sortValues) ||
		snapshotAt.IsZero() || snapshotAt.After(c.now().UTC().Add(30*time.Second)) ||
		(mode == SearchCursorModeLive && pitID != "") ||
		(mode == SearchCursorModePIT && (pitID == "" || len(pitID) > 4096)) {
		return "", errSearchCursorInvalid
	}
	claims := searchCursorClaims{
		Version:             searchCursorVersion,
		TenantID:            tenantID,
		QuerySHA256:         querySHA256,
		Mode:                mode,
		Size:                size,
		SortValues:          append([]json.RawMessage(nil), sortValues...),
		PITID:               pitID,
		SnapshotUnixMilli:   snapshotAt.UTC().UnixMilli(),
		ExpiresAtUnixSecond: c.now().UTC().Add(c.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal alert search cursor: %w", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, c.signingKey)
	_, _ = mac.Write([]byte(payloadPart))
	token := payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(token) > searchCursorMaxBytes {
		return "", fmt.Errorf("%w: cursor is too large", errSearchCursorInvalid)
	}
	return token, nil
}

func (c *searchCursorCodec) decode(token, tenantID string) (*searchCursorClaims, error) {
	if c == nil || len(c.signingKey) == 0 || token == "" || len(token) > searchCursorMaxBytes {
		return nil, errSearchCursorInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errSearchCursorInvalid
	}
	providedMAC, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(providedMAC) != sha256.Size {
		return nil, errSearchCursorInvalid
	}
	expectedMAC := hmac.New(sha256.New, c.signingKey)
	_, _ = expectedMAC.Write([]byte(parts[0]))
	if !hmac.Equal(providedMAC, expectedMAC.Sum(nil)) {
		return nil, errSearchCursorInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > searchCursorMaxBytes {
		return nil, errSearchCursorInvalid
	}
	var claims searchCursorClaims
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, errSearchCursorInvalid
	}
	if err := requireSearchCursorEOF(decoder); err != nil {
		return nil, errSearchCursorInvalid
	}
	if claims.Version != searchCursorVersion ||
		!hmac.Equal([]byte(claims.TenantID), []byte(tenantID)) ||
		len(claims.QuerySHA256) != sha256.Size*2 ||
		(claims.Mode != SearchCursorModeLive && claims.Mode != SearchCursorModePIT) ||
		claims.Size < 1 || !validSearchSortValues(claims.SortValues) ||
		claims.SnapshotUnixMilli <= 0 || time.UnixMilli(claims.SnapshotUnixMilli).After(c.now().UTC().Add(30*time.Second)) ||
		(claims.Mode == SearchCursorModeLive && claims.PITID != "") ||
		(claims.Mode == SearchCursorModePIT && (claims.PITID == "" || len(claims.PITID) > 4096)) {
		return nil, errSearchCursorInvalid
	}
	if claims.ExpiresAtUnixSecond <= c.now().UTC().Unix() {
		return nil, errSearchCursorExpired
	}
	return &claims, nil
}

func searchQuerySHA256(query *SearchQuery, mode string, size int) string {
	normalized := struct {
		Query      string   `json:"query"`
		Severity   []string `json:"severity"`
		Status     []string `json:"status"`
		AlertTypes []string `json:"alert_types"`
		Labels     []string `json:"labels"`
		SrcIP      string   `json:"src_ip"`
		DstIP      string   `json:"dst_ip"`
		StartTime  int64    `json:"start_time_ms"`
		EndTime    int64    `json:"end_time_ms"`
		SortField  string   `json:"sort_field"`
		SortOrder  string   `json:"sort_order"`
		Mode       string   `json:"mode"`
		Size       int      `json:"size"`
	}{
		Query:      strings.TrimSpace(query.Query),
		Severity:   normalizedSearchValues(query.Severity),
		Status:     normalizedSearchValues(query.Status),
		AlertTypes: normalizedSearchValues(query.AlertTypes),
		Labels:     normalizedSearchValues(query.Labels),
		SrcIP:      strings.TrimSpace(query.SrcIP),
		DstIP:      strings.TrimSpace(query.DstIP),
		SortField:  normalizedSearchSortField(query.SortField),
		SortOrder:  normalizedSearchSortOrder(query.SortOrder),
		Mode:       mode,
		Size:       size,
	}
	if !query.StartTime.IsZero() {
		normalized.StartTime = query.StartTime.UTC().UnixMilli()
	}
	if !query.EndTime.IsZero() {
		normalized.EndTime = query.EndTime.UTC().UnixMilli()
	}
	encoded, _ := json.Marshal(normalized)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func normalizedSearchValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strings.TrimSpace(value))
	}
	sort.Strings(result)
	return result
}

func normalizedSearchSortField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "last_seen"
	}
	return value
}

func normalizedSearchSortOrder(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "desc"
	}
	return value
}

func validSearchSortValues(values []json.RawMessage) bool {
	if len(values) < 1 || len(values) > 2 {
		return false
	}
	for _, value := range values {
		if len(value) == 0 || len(value) > 1024 || !json.Valid(value) {
			return false
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return false
		}
		switch decoded.(type) {
		case nil, string, float64, bool:
		default:
			return false
		}
	}
	return true
}

func requireSearchCursorEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !stdErrors.Is(err, io.EOF) {
		if err == nil {
			return stdErrors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func searchSnapshotID(tenantID, pitID string) string {
	digest := sha256.Sum256([]byte(tenantID + "\x00" + pitID))
	return "alert-os-pit-" + hex.EncodeToString(digest[:16])
}
