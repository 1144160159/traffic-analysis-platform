package repository

import (
	"bytes"
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/opensearchbulk"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// OpenSearchRepository OpenSearch数据访问层
type OpenSearchRepository struct {
	client             *opensearch.Client
	readTarget         string
	writeTarget        string
	exactTarget        bool
	cursorEnabled      bool
	cursorCodec        *searchCursorCodec
	shallowResultLimit int
	maxPageSize        int
	queryTimeout       time.Duration
	cursorTTL          time.Duration
	trackTotalHitsUpTo int
	logger             *zap.Logger
}

// OpenSearchConfig OpenSearch配置
type OpenSearchConfig struct {
	Addresses          []string
	Username           string
	Password           string
	ReadTarget         string
	WriteTarget        string
	ExactTarget        bool
	CursorEnabled      bool
	CursorSigningKey   string
	ShallowResultLimit int
	MaxPageSize        int
	QueryTimeout       time.Duration
	CursorTTL          time.Duration
	TrackTotalHitsUpTo int
}

// NewOpenSearchRepository 创建OpenSearch Repository
func NewOpenSearchRepository(cfg OpenSearchConfig, logger *zap.Logger) (*OpenSearchRepository, error) {
	if cfg.ReadTarget == "" || cfg.WriteTarget == "" {
		return nil, fmt.Errorf("opensearch read and write targets are required")
	}
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create opensearch client: %w", err)
	}
	if cfg.ShallowResultLimit <= 0 {
		cfg.ShallowResultLimit = 1000
	}
	if cfg.MaxPageSize <= 0 {
		cfg.MaxPageSize = 200
	}
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 2 * time.Second
	}
	if cfg.CursorTTL <= 0 {
		cfg.CursorTTL = 2 * time.Minute
	}
	if cfg.TrackTotalHitsUpTo <= 0 {
		cfg.TrackTotalHitsUpTo = 10000
	}
	var cursorCodec *searchCursorCodec
	if cfg.CursorEnabled {
		cursorCodec, err = newSearchCursorCodec(cfg.CursorSigningKey, cfg.CursorTTL)
		if err != nil {
			return nil, fmt.Errorf("configure OpenSearch search cursor: %w", err)
		}
	}

	// This repository is used by request handlers; startup should not hang forever
	// if OpenSearch is slow after the primary persistence client has connected.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := opensearchapi.InfoRequest{}.Do(ctx, client)
	if err != nil {
		logger.Warn("OpenSearch repository ping failed, continuing with lazy checks", zap.Error(err))
	} else {
		res.Body.Close()
	}

	return &OpenSearchRepository{
		client:             client,
		readTarget:         cfg.ReadTarget,
		writeTarget:        cfg.WriteTarget,
		exactTarget:        cfg.ExactTarget,
		cursorEnabled:      cfg.CursorEnabled,
		cursorCodec:        cursorCodec,
		shallowResultLimit: cfg.ShallowResultLimit,
		maxPageSize:        cfg.MaxPageSize,
		queryTimeout:       cfg.QueryTimeout,
		cursorTTL:          cfg.CursorTTL,
		trackTotalHitsUpTo: cfg.TrackTotalHitsUpTo,
		logger:             logger,
	}, nil
}

func (r *OpenSearchRepository) targetFor(firstSeen time.Time) string {
	if r.exactTarget {
		return r.writeTarget
	}
	return fmt.Sprintf("%s-%s", r.writeTarget, firstSeen.Format("2006-01-02"))
}

// SearchQuery 搜索查询参数
type SearchQuery struct {
	TenantID     string
	Query        string   // 全文搜索词
	Severity     []string // 严重程度过滤
	Status       []string // 状态过滤
	AlertTypes   []string // 告警类型过滤
	Labels       []string // 标签过滤
	SrcIP        string
	DstIP        string
	AssetIP      string
	RuleVersion  string
	ModelVersion string
	AttackPhase  string
	MinScore     *float64
	StartTime    time.Time
	EndTime      time.Time
	From         int
	Size         int
	SortField    string
	SortOrder    string
	Cursor       string
	CursorMode   string
	// OmitAggregations is intended for bounded watermark/health reads against
	// legacy mappings whose text fields cannot be aggregated safely.
	OmitAggregations bool
	// BoundedTotalHits uses the configured total-hit ceiling and preserves the
	// OpenSearch eq/gte relation instead of forcing an unbounded exact count.
	BoundedTotalHits bool
}

const (
	SearchCursorModeLive = "live"
	SearchCursorModePIT  = "pit"
)

// SearchResult 搜索结果
type SearchResult struct {
	Alerts              []*persistence.Alert   `json:"alerts"`
	Total               int64                  `json:"total"`
	TotalRelation       string                 `json:"total_relation,omitempty"`
	Aggregations        map[string]interface{} `json:"aggregations,omitempty"`
	AggregationsOmitted bool                   `json:"aggregations_omitted,omitempty"`
	Took                int                    `json:"took"` // 耗时(ms)
	NextCursor          string                 `json:"next_cursor,omitempty"`
	HasMore             bool                   `json:"has_more"`
	CursorMode          string                 `json:"cursor_mode,omitempty"`
	SnapshotID          string                 `json:"snapshot_id,omitempty"`
	AsOf                string                 `json:"as_of,omitempty"`
	Partial             bool                   `json:"partial"`
	SourceWatermarks    map[string]string      `json:"source_watermarks,omitempty"`
}

// Search 全文搜索告警
func (r *OpenSearchRepository) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.search")
	defer span.End()
	if query == nil || query.TenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant-bound search query is required")
	}
	usesCursorContract := query.Cursor != "" || query.CursorMode != ""
	if usesCursorContract && !r.cursorEnabled {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "OpenSearch cursor contract is disabled")
	}
	if usesCursorContract {
		return r.searchWithCursor(ctx, query)
	}
	if r.cursorEnabled {
		size := query.Size
		if size <= 0 {
			size = 50
		}
		if query.From < 0 || size > r.maxPageSize || query.From+size > r.shallowResultLimit {
			return nil, errors.Newf(errors.ErrCodeInvalidParameter,
				"from/size is limited to the first %d results; use cursor_mode for deeper traversal",
				r.shallowResultLimit)
		}
	}
	return r.searchLegacy(ctx, query)
}

type openSearchResponse struct {
	Took     int    `json:"took"`
	TimedOut bool   `json:"timed_out"`
	PITID    string `json:"pit_id,omitempty"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int64  `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			Source    persistence.Alert   `json:"_source"`
			Highlight map[string][]string `json:"highlight,omitempty"`
			Sort      []json.RawMessage   `json:"sort,omitempty"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}

func (r *OpenSearchRepository) searchLegacy(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	boolQuery := buildOpenSearchBoolQuery(query)
	sortField := normalizedSearchSortField(query.SortField)
	sortOrder := normalizedSearchSortOrder(query.SortOrder)
	from := query.From
	size := query.Size
	if size <= 0 || size > 1000 {
		size = 50
	}
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{"bool": boolQuery},
		"sort":  []map[string]interface{}{{sortField: map[string]interface{}{"order": sortOrder}}},
		"from":  from,
		"size":  size,
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"alert_type": map[string]interface{}{},
				"labels":     map[string]interface{}{},
			},
		},
	}
	if !query.OmitAggregations {
		searchBody["aggs"] = defaultAlertSearchAggregations()
	}
	trackTotalHits := interface{}(true)
	if query.BoundedTotalHits {
		trackTotalHits = r.trackTotalHitsUpTo
	}
	response, err := r.executeSearch(ctx, searchBody, true, r.readTarget, 0, trackTotalHits)
	if err != nil {
		return nil, err
	}
	alerts := make([]*persistence.Alert, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		alert := hit.Source
		alerts = append(alerts, &alert)
	}
	return &SearchResult{
		Alerts: alerts, Total: response.Hits.Total.Value, TotalRelation: response.Hits.Total.Relation,
		Aggregations: response.Aggregations, AggregationsOmitted: query.OmitAggregations,
		Took: response.Took, HasMore: from+len(alerts) < int(response.Hits.Total.Value), Partial: false,
	}, nil
}

func (r *OpenSearchRepository) searchWithCursor(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	if query.From != 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "cursor traversal cannot be combined with from")
	}
	if r.queryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.queryTimeout)
		defer cancel()
	}
	mode := query.CursorMode
	size := query.Size
	if size <= 0 {
		size = 50
	}
	var claims *searchCursorClaims
	snapshotAt := time.Now().UTC()
	if query.Cursor != "" {
		decoded, err := r.cursorCodec.decode(query.Cursor, query.TenantID)
		if err != nil {
			return nil, cursorAppError(err)
		}
		claims = decoded
		if claims.Mode == SearchCursorModePIT {
			snapshotAt = time.UnixMilli(claims.SnapshotUnixMilli).UTC()
		}
		if mode != "" && mode != claims.Mode {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "cursor_mode differs from the signed cursor")
		}
		mode = claims.Mode
		if query.Size != 0 && query.Size != claims.Size {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "size differs from the signed cursor")
		}
		size = claims.Size
	}
	if mode != SearchCursorModeLive && mode != SearchCursorModePIT {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "cursor_mode must be live or pit")
	}
	if size < 1 || size > r.maxPageSize {
		return nil, errors.Newf(errors.ErrCodeInvalidParameter, "cursor size must be between 1 and %d", r.maxPageSize)
	}
	querySHA := searchQuerySHA256(query, mode, size)
	if claims != nil && !hmacEqualString(claims.QuerySHA256, querySHA) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "cursor does not match the current search query")
	}

	var physicalTargets []string
	var err error
	targetSHA256 := ""
	if claims != nil && claims.Mode == SearchCursorModePIT {
		// A PIT already freezes its physical shards. Resolving the mutable alias
		// again would incorrectly invalidate a consistent traversal after an
		// approved alias cutover.
		targetSHA256 = claims.TargetSHA256
	} else {
		physicalTargets, targetSHA256, err = r.resolveSearchTargets(ctx)
		if err != nil {
			return nil, err
		}
		if claims != nil && !hmacEqualString(claims.TargetSHA256, targetSHA256) {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "search cursor is stale because the OpenSearch read target changed")
		}
	}

	pitID := ""
	createdPIT := false
	if claims != nil {
		pitID = claims.PITID
	} else if mode == SearchCursorModePIT {
		pitID, err = r.createPIT(ctx, physicalTargets)
		if err != nil {
			return nil, err
		}
		createdPIT = true
	}

	sortField := normalizedSearchSortField(query.SortField)
	sortOrder := normalizedSearchSortOrder(query.SortOrder)
	sorts := []map[string]interface{}{{sortField: map[string]interface{}{"order": sortOrder}}}
	if sortField != "alert_id" {
		sorts = append(sorts, map[string]interface{}{"alert_id": map[string]interface{}{"order": sortOrder}})
	}
	searchBody := map[string]interface{}{
		"query":            map[string]interface{}{"bool": buildOpenSearchBoolQuery(query)},
		"sort":             sorts,
		"size":             size + 1,
		"_source":          alertSearchSourceFields,
		"track_total_hits": r.trackTotalHitsUpTo,
	}
	if claims != nil {
		searchBody["search_after"] = claims.SortValues
	}
	index := strings.Join(physicalTargets, ",")
	if mode == SearchCursorModePIT {
		index = ""
		searchBody["pit"] = map[string]interface{}{"id": pitID, "keep_alive": formatOpenSearchDuration(r.cursorTTL)}
	}
	response, err := r.executeSearch(ctx, searchBody, false, index, r.queryTimeout, r.trackTotalHitsUpTo)
	if err != nil {
		if createdPIT {
			r.bestEffortClosePIT(ctx, pitID)
		}
		return nil, err
	}
	if response.PITID != "" {
		pitID = response.PITID
	}
	hasMore := len(response.Hits.Hits) > size
	hits := response.Hits.Hits
	if hasMore {
		hits = hits[:size]
	}
	alerts := make([]*persistence.Alert, 0, len(hits))
	for _, hit := range hits {
		alert := hit.Source
		alerts = append(alerts, &alert)
	}
	nextCursor := ""
	if hasMore {
		lastSort := hits[len(hits)-1].Sort
		nextCursor, err = r.cursorCodec.encode(query.TenantID, querySHA, targetSHA256, mode, size, lastSort, pitID, snapshotAt)
		if err != nil {
			if mode == SearchCursorModePIT {
				r.bestEffortClosePIT(ctx, pitID)
			}
			return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode search cursor")
		}
	} else if mode == SearchCursorModePIT {
		r.bestEffortClosePIT(ctx, pitID)
	}
	result := &SearchResult{
		Alerts: alerts, Total: response.Hits.Total.Value, TotalRelation: response.Hits.Total.Relation,
		AggregationsOmitted: true, Took: response.Took, NextCursor: nextCursor, HasMore: hasMore,
		CursorMode: mode, Partial: false,
		AsOf: snapshotAt.Format(time.RFC3339Nano),
		SourceWatermarks: map[string]string{
			"opensearch.alerts.search":        snapshotAt.Format(time.RFC3339Nano),
			"opensearch.alerts.target_sha256": targetSHA256,
		},
	}
	if mode == SearchCursorModePIT {
		result.SnapshotID = searchSnapshotID(query.TenantID, pitID)
	}
	return result, nil
}

type resolvedOpenSearchTargets struct {
	Indices []struct {
		Name string `json:"name"`
	} `json:"indices"`
	Aliases []struct {
		Indices []string `json:"indices"`
	} `json:"aliases"`
	DataStreams []struct {
		BackingIndices []string `json:"backing_indices"`
	} `json:"data_streams"`
}

// resolveSearchTargets freezes a logical alias or wildcard to the exact
// physical index set used by one cursor page. Live continuations compare this
// set before searching; PIT continuations retain the PIT snapshot but still
// carry the same signed target identity for evidence.
func (r *OpenSearchRepository) resolveSearchTargets(ctx context.Context) ([]string, string, error) {
	res, err := r.client.Indices.ResolveIndex(
		[]string{r.readTarget},
		r.client.Indices.ResolveIndex.WithContext(ctx),
		r.client.Indices.ResolveIndex.WithExpandWildcards("open,hidden"),
	)
	if err != nil {
		return nil, "", errors.Wrap(err, errors.ErrCodeOpenSearchError, "resolve OpenSearch read target")
	}
	if res == nil || res.Body == nil {
		return nil, "", errors.New(errors.ErrCodeOpenSearchError, "OpenSearch returned no read-target resolution")
	}
	defer res.Body.Close()
	if res.IsError() {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64*1024))
		return nil, "", errors.Newf(errors.ErrCodeOpenSearchError, "resolve OpenSearch read target: %s", res.Status())
	}
	var resolved resolvedOpenSearchTargets
	if err := json.NewDecoder(io.LimitReader(res.Body, 4*1024*1024)).Decode(&resolved); err != nil {
		return nil, "", errors.Wrap(err, errors.ErrCodeSerializationError, "decode OpenSearch read-target resolution")
	}
	unique := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" {
			unique[name] = struct{}{}
		}
	}
	for _, index := range resolved.Indices {
		add(index.Name)
	}
	for _, alias := range resolved.Aliases {
		for _, index := range alias.Indices {
			add(index)
		}
	}
	for _, stream := range resolved.DataStreams {
		for _, index := range stream.BackingIndices {
			add(index)
		}
	}
	physicalTargets := make([]string, 0, len(unique))
	combinedLength := 0
	for name := range unique {
		if len(name) > 255 {
			return nil, "", errors.New(errors.ErrCodeOpenSearchError, "resolved OpenSearch index name exceeds the cursor budget")
		}
		combinedLength += len(name)
		physicalTargets = append(physicalTargets, name)
	}
	if len(physicalTargets) == 0 || len(physicalTargets) > 256 || combinedLength > 16*1024 {
		return nil, "", errors.New(errors.ErrCodeOpenSearchError, "resolved OpenSearch read target is empty or exceeds the cursor budget")
	}
	sort.Strings(physicalTargets)
	return physicalTargets, searchTargetSHA256(r.readTarget, physicalTargets), nil
}

func buildOpenSearchBoolQuery(query *SearchQuery) map[string]interface{} {
	must := make([]map[string]interface{}, 0, 1)
	filter := []map[string]interface{}{{"term": map[string]interface{}{"tenant_id": query.TenantID}}}
	if query.Query != "" {
		must = append(must, map[string]interface{}{"match": map[string]interface{}{
			"search_text": map[string]interface{}{"query": query.Query, "operator": "and"},
		}})
	}
	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			rangeQuery["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			rangeQuery["lte"] = query.EndTime.Format(time.RFC3339)
		}
		filter = append(filter, map[string]interface{}{"range": map[string]interface{}{"last_seen": rangeQuery}})
	}
	for _, item := range []struct {
		field  string
		values []string
	}{{"severity", query.Severity}, {"status", query.Status}, {"alert_type", query.AlertTypes}, {"labels", query.Labels}} {
		if len(item.values) > 0 {
			filter = append(filter, map[string]interface{}{"terms": map[string]interface{}{item.field: item.values}})
		}
	}
	if query.SrcIP != "" {
		filter = append(filter, map[string]interface{}{"term": map[string]interface{}{"src_ip": query.SrcIP}})
	}
	if query.DstIP != "" {
		filter = append(filter, map[string]interface{}{"term": map[string]interface{}{"dst_ip": query.DstIP}})
	}
	if query.AssetIP != "" {
		filter = append(filter, map[string]interface{}{"bool": map[string]interface{}{
			"should": []map[string]interface{}{
				{"term": map[string]interface{}{"src_ip": query.AssetIP}},
				{"term": map[string]interface{}{"dst_ip": query.AssetIP}},
			},
			"minimum_should_match": 1,
		}})
	}
	for _, item := range []struct {
		field string
		value string
	}{{"rule_version", query.RuleVersion}, {"model_version", query.ModelVersion}, {"attack_phase", query.AttackPhase}} {
		if item.value != "" {
			filter = append(filter, map[string]interface{}{"term": map[string]interface{}{item.field: item.value}})
		}
	}
	if query.MinScore != nil {
		filter = append(filter, map[string]interface{}{"range": map[string]interface{}{"score": map[string]interface{}{"gte": *query.MinScore}}})
	}
	result := map[string]interface{}{"filter": filter}
	if len(must) > 0 {
		result["must"] = must
	}
	return result
}

func defaultAlertSearchAggregations() map[string]interface{} {
	return map[string]interface{}{
		"severity_count":   map[string]interface{}{"terms": map[string]interface{}{"field": "severity"}},
		"status_count":     map[string]interface{}{"terms": map[string]interface{}{"field": "status"}},
		"alert_type_count": map[string]interface{}{"terms": map[string]interface{}{"field": "alert_type", "size": 10}},
	}
}

var alertSearchSourceFields = []string{
	"tenant_id", "alert_id", "fingerprint", "community_id", "session_id", "campaign_id",
	"src_ip", "dst_ip", "src_port", "dst_port", "protocol", "alert_type", "labels", "score",
	"severity", "first_seen", "last_seen", "count", "status", "assignee", "updated_at",
	"model_version", "rule_version", "feature_set_id", "evidence_ids", "event_id",
	"attack_phase", "state_version", "trace_id",
}

func (r *OpenSearchRepository) executeSearch(
	ctx context.Context,
	searchBody map[string]interface{},
	legacy bool,
	index string,
	timeout time.Duration,
	trackTotalHits interface{},
) (*openSearchResponse, error) {
	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal search query")
	}
	options := []func(*opensearchapi.SearchRequest){
		r.client.Search.WithContext(ctx),
		r.client.Search.WithBody(bytes.NewReader(body)),
		r.client.Search.WithTrackTotalHits(trackTotalHits),
	}
	if index != "" {
		options = append(options, r.client.Search.WithIndex(strings.Split(index, ",")...))
	}
	if !legacy {
		options = append(options, r.client.Search.WithAllowPartialSearchResults(false))
	}
	if timeout > 0 {
		options = append(options, r.client.Search.WithTimeout(timeout))
	}
	res, err := r.client.Search(options...)
	if err != nil {
		r.logger.Error("OpenSearch search failed", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "search failed")
	}
	defer res.Body.Close()
	if res.IsError() {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64*1024))
		r.logger.Error("OpenSearch search error", zap.String("status", res.Status()))
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "search error: %s", res.Status())
	}
	var response openSearchResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 32*1024*1024)).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode search response")
	}
	if !legacy && (response.TimedOut || response.Shards.Failed > 0) {
		return nil, errors.Newf(errors.ErrCodeOpenSearchError,
			"search did not complete on all shards (timed_out=%t failed_shards=%d)",
			response.TimedOut, response.Shards.Failed)
	}
	if response.Hits.Total.Relation == "" {
		response.Hits.Total.Relation = "eq"
	}
	return &response, nil
}

func (r *OpenSearchRepository) createPIT(ctx context.Context, physicalTargets []string) (string, error) {
	res, response, err := r.client.PointInTime.Create(
		r.client.PointInTime.Create.WithContext(ctx),
		r.client.PointInTime.Create.WithIndex(physicalTargets...),
		r.client.PointInTime.Create.WithKeepAlive(r.cursorTTL),
	)
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeOpenSearchError, "failed to create point-in-time")
	}
	if res == nil || res.IsError() || response == nil || response.PitID == "" || response.Shards.Failed > 0 {
		return "", errors.New(errors.ErrCodeOpenSearchError, "OpenSearch did not create a complete point-in-time")
	}
	return response.PitID, nil
}

func (r *OpenSearchRepository) closePIT(ctx context.Context, pitID string) error {
	res, response, err := r.client.PointInTime.Delete(
		r.client.PointInTime.Delete.WithContext(ctx),
		r.client.PointInTime.Delete.WithPitID(pitID),
	)
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "failed to close point-in-time")
	}
	if res == nil || res.IsError() || response == nil || len(response.Pits) == 0 || !response.Pits[0].Successful {
		return errors.New(errors.ErrCodeOpenSearchError, "OpenSearch did not close point-in-time")
	}
	return nil
}

func (r *OpenSearchRepository) bestEffortClosePIT(ctx context.Context, pitID string) {
	if pitID == "" {
		return
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := r.closePIT(closeCtx, pitID); err != nil {
		r.logger.Warn("Failed to close OpenSearch point-in-time; it will expire", zap.Error(err))
	}
}

// CloseSearchCursor releases a PIT carried by a tenant-bound signed cursor.
// Live cursors have no server-side resource and are accepted as a no-op.
func (r *OpenSearchRepository) CloseSearchCursor(ctx context.Context, tenantID, cursor string) error {
	if !r.cursorEnabled || r.cursorCodec == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "OpenSearch cursor contract is disabled")
	}
	claims, err := r.cursorCodec.decode(cursor, tenantID)
	if err != nil {
		return cursorAppError(err)
	}
	if claims.Mode == SearchCursorModePIT {
		return r.closePIT(ctx, claims.PITID)
	}
	return nil
}

func cursorAppError(err error) error {
	if stdErrors.Is(err, errSearchCursorExpired) {
		return errors.New(errors.ErrCodeInvalidParameter, "search cursor has expired")
	}
	return errors.New(errors.ErrCodeInvalidParameter, "search cursor is invalid")
}

func hmacEqualString(left, right string) bool {
	return len(left) == len(right) && subtleConstantTimeEqual([]byte(left), []byte(right))
}

func subtleConstantTimeEqual(left, right []byte) bool {
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func formatOpenSearchDuration(value time.Duration) string {
	if value%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(value/time.Minute))
	}
	return fmt.Sprintf("%dms", value.Milliseconds())
}

// Suggest 自动补全建议
func (r *OpenSearchRepository) Suggest(ctx context.Context, tenantID, prefix string, field string, size int) ([]string, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.suggest")
	defer span.End()

	if size <= 0 || size > 20 {
		size = 10
	}

	// 使用prefix查询实现简单补全
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"tenant_id": tenantID,
						},
					},
					{
						"prefix": map[string]interface{}{
							field: map[string]interface{}{
								"value": prefix,
							},
						},
					},
				},
			},
		},
		"aggs": map[string]interface{}{
			"suggestions": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": field,
					"size":  size,
				},
			},
		},
		"size": 0,
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal suggest query")
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(r.readTarget),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "suggest failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "suggest error: %s", res.Status())
	}

	var response struct {
		Aggregations struct {
			Suggestions struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int    `json:"doc_count"`
				} `json:"buckets"`
			} `json:"suggestions"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode response")
	}

	suggestions := make([]string, 0, len(response.Aggregations.Suggestions.Buckets))
	for _, bucket := range response.Aggregations.Suggestions.Buckets {
		suggestions = append(suggestions, bucket.Key)
	}

	return suggestions, nil
}

// AggregateQuery 聚合查询参数
type AggregateQuery struct {
	TenantID  string
	Field     string // 聚合字段
	StartTime time.Time
	EndTime   time.Time
	Size      int
}

// AggregateResult 聚合结果
type AggregateResult struct {
	Buckets []AggBucket `json:"buckets"`
}

// AggBucket 聚合桶
type AggBucket struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

// Aggregate 聚合统计
func (r *OpenSearchRepository) Aggregate(ctx context.Context, query *AggregateQuery) (*AggregateResult, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.aggregate")
	defer span.End()

	if query.Size <= 0 || query.Size > 100 {
		query.Size = 20
	}

	// 构建查询
	must := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"tenant_id": query.TenantID,
			},
		},
	}

	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			rangeQuery["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			rangeQuery["lte"] = query.EndTime.Format(time.RFC3339)
		}
		must = append(must, map[string]interface{}{
			"range": map[string]interface{}{
				"last_seen": rangeQuery,
			},
		})
	}

	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": must,
			},
		},
		"aggs": map[string]interface{}{
			"field_agg": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": query.Field,
					"size":  query.Size,
				},
			},
		},
		"size": 0,
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal aggregate query")
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(r.readTarget),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "aggregate failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "aggregate error: %s", res.Status())
	}

	var response struct {
		Aggregations struct {
			FieldAgg struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"field_agg"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode response")
	}

	buckets := make([]AggBucket, 0, len(response.Aggregations.FieldAgg.Buckets))
	for _, b := range response.Aggregations.FieldAgg.Buckets {
		buckets = append(buckets, AggBucket{
			Key:      b.Key,
			DocCount: b.DocCount,
		})
	}

	return &AggregateResult{Buckets: buckets}, nil
}

// Index 索引单个告警
func (r *OpenSearchRepository) Index(ctx context.Context, alert *persistence.Alert) error {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.index")
	defer span.End()

	indexName := r.targetFor(alert.FirstSeen)

	body, err := json.Marshal(alert)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal alert")
	}

	req := opensearchapi.IndexRequest{
		Index:      indexName,
		DocumentID: alert.AlertID,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		r.logger.Error("Failed to index alert", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "index failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Newf(errors.ErrCodeOpenSearchError, "index error: %s", res.Status())
	}

	return nil
}

// BulkIndex 批量索引告警
func (r *OpenSearchRepository) BulkIndex(ctx context.Context, alerts []*persistence.Alert) error {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.bulk_index")
	defer span.End()

	if len(alerts) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, alert := range alerts {
		indexName := r.targetFor(alert.FirstSeen)

		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    alert.AlertID,
			},
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeOpenSearchError, "marshal bulk metadata")
		}
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		docBytes, err := json.Marshal(alert)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeOpenSearchError, "marshal bulk alert")
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body:    bytes.NewReader(buf.Bytes()),
		Refresh: "false",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		r.logger.Error("Bulk index failed", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "bulk index failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		r.logger.Error("Bulk index error",
			zap.String("status", res.Status()),
			zap.ByteString("body", bodyBytes))
		return errors.Newf(errors.ErrCodeOpenSearchError, "bulk index error: %s", res.Status())
	}

	if err := opensearchbulk.DecodeSuccess(res.Body, len(alerts)); err != nil {
		r.logger.Error("Bulk index was not fully acknowledged", zap.Error(err))
		otel.RecordError(ctx, err)
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "bulk index incomplete")
	}

	return nil
}

// Ping 健康检查
func (r *OpenSearchRepository) Ping(ctx context.Context) error {
	res, err := r.client.Ping(r.client.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ping failed: %s", res.Status())
	}
	return nil
}

// Close 关闭连接（OpenSearch client不需要显式关闭）
func (r *OpenSearchRepository) Close() error {
	return nil
}
