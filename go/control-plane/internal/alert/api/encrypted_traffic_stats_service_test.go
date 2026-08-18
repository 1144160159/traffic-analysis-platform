package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type encryptedTrafficStatsQueryCall struct {
	query string
	args  []interface{}
}

type encryptedTrafficStatsQueryResult struct {
	row driver.Row
	err error
}

type encryptedTrafficStatsQueryerStub struct {
	calls   []encryptedTrafficStatsQueryCall
	results []encryptedTrafficStatsQueryResult
}

func (s *encryptedTrafficStatsQueryerStub) QueryRow(_ context.Context, query string, args ...interface{}) (driver.Row, error) {
	s.calls = append(s.calls, encryptedTrafficStatsQueryCall{query: query, args: args})
	if len(s.results) == 0 {
		return nil, errors.New("unexpected QueryRow call")
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result.row, result.err
}

type encryptedTrafficStatsRowStub struct {
	values []uint64
	err    error
}

func (r encryptedTrafficStatsRowStub) Err() error { return r.err }

func (r encryptedTrafficStatsRowStub) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destination count %d, want %d", len(dest), len(r.values))
	}
	for index, value := range r.values {
		target, ok := dest[index].(*uint64)
		if !ok {
			return fmt.Errorf("scan destination %d is %T, want *uint64", index, dest[index])
		}
		*target = value
	}
	return nil
}

func (r encryptedTrafficStatsRowStub) ScanStruct(interface{}) error {
	return errors.New("ScanStruct is not supported by the stats row stub")
}

func TestClickHouseEncryptedTrafficStatsServicePreservesQueriesAndAggregation(t *testing.T) {
	queryer := &encryptedTrafficStatsQueryerStub{results: []encryptedTrafficStatsQueryResult{
		{row: encryptedTrafficStatsRowStub{values: []uint64{10, 5, 2, 1}}},
		{row: encryptedTrafficStatsRowStub{values: []uint64{1}}},
		{row: encryptedTrafficStatsRowStub{values: []uint64{4, 1}}},
	}}
	service := &clickHouseEncryptedTrafficStatsService{client: queryer}
	query := encryptedTrafficStatsQuery{TenantID: "tenant-a", StartMilli: 1700000000000, EndMilli: 1700003600000}

	stats, err := service.Load(context.Background(), query)

	if err != nil {
		t.Fatalf("load stats: %v", err)
	}
	if stats != (encryptedTrafficStatsDTO{
		TotalSessions:       8,
		ObservedSessions:    10,
		EncryptedRatio:      0.8,
		TLSSessions:         5,
		QUICSessions:        2,
		JA3Fingerprints:     4,
		MaliciousJA3Matches: 1,
	}) {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(queryer.calls) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(queryer.calls))
	}
	if !strings.Contains(queryer.calls[0].query, "FROM traffic.sessions") || !strings.Contains(queryer.calls[1].query, "FROM system.tables") || !strings.Contains(queryer.calls[2].query, "FROM traffic.feature_fp") {
		t.Fatalf("unexpected query order: %#v", queryer.calls)
	}
	for _, call := range []encryptedTrafficStatsQueryCall{queryer.calls[0], queryer.calls[2]} {
		if len(call.args) != 3 || call.args[0] != query.TenantID || call.args[1] != query.StartMilli || call.args[2] != query.EndMilli {
			t.Fatalf("query lost tenant/time boundary: %#v", call.args)
		}
	}
}

func TestClickHouseEncryptedTrafficStatsServiceKeepsFingerprintEnrichmentOptional(t *testing.T) {
	queryer := &encryptedTrafficStatsQueryerStub{results: []encryptedTrafficStatsQueryResult{
		{row: encryptedTrafficStatsRowStub{values: []uint64{2, 1, 0, 0}}},
		{err: errors.New("system.tables unavailable")},
	}}
	service := &clickHouseEncryptedTrafficStatsService{client: queryer}

	stats, err := service.Load(context.Background(), encryptedTrafficStatsQuery{TenantID: "tenant-a", StartMilli: 1, EndMilli: 2})

	if err != nil {
		t.Fatalf("optional fingerprint discovery must not fail base stats: %v", err)
	}
	if stats.TotalSessions != 1 || stats.ObservedSessions != 2 || stats.EncryptedRatio != 0.5 || stats.JA3Fingerprints != 0 || stats.MaliciousJA3Matches != 0 {
		t.Fatalf("unexpected base-only stats: %+v", stats)
	}
	if len(queryer.calls) != 2 {
		t.Fatalf("expected base and table discovery queries, got %d", len(queryer.calls))
	}
}

func TestClickHouseEncryptedTrafficStatsServicePropagatesRequiredSessionReadFailure(t *testing.T) {
	queryer := &encryptedTrafficStatsQueryerStub{results: []encryptedTrafficStatsQueryResult{{err: errors.New("session query failed")}}}
	service := &clickHouseEncryptedTrafficStatsService{client: queryer}

	_, err := service.Load(context.Background(), encryptedTrafficStatsQuery{TenantID: "tenant-a", StartMilli: 1, EndMilli: 2})

	if err == nil || !strings.Contains(err.Error(), "session query failed") {
		t.Fatalf("expected required session read error, got %v", err)
	}
}
