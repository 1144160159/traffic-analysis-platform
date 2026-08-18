package api

import (
	"context"
	"errors"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type EncryptedTrafficStatsQuery struct {
	TenantID   string
	StartMilli int64
	EndMilli   int64
}

type encryptedTrafficStatsQuery = EncryptedTrafficStatsQuery

type EncryptedTrafficStatsService interface {
	Load(context.Context, EncryptedTrafficStatsQuery) (EncryptedTrafficStats, error)
}

type encryptedTrafficStatsService = EncryptedTrafficStatsService

type encryptedTrafficStatsQueryer interface {
	QueryRow(context.Context, string, ...interface{}) (driver.Row, error)
}

type clickHouseEncryptedTrafficStatsService struct {
	client encryptedTrafficStatsQueryer
}

func newClickHouseEncryptedTrafficStatsService(client *storage.ClickHouseClient) encryptedTrafficStatsService {
	return &clickHouseEncryptedTrafficStatsService{client: client}
}

func NewClickHouseEncryptedTrafficStatsService(client *storage.ClickHouseClient) EncryptedTrafficStatsService {
	return newClickHouseEncryptedTrafficStatsService(client)
}

func (h *SystemHandler) SetEncryptedTrafficStatsService(service EncryptedTrafficStatsService) {
	if service != nil {
		h.encryptedTrafficStats = service
	}
}

func (s *clickHouseEncryptedTrafficStatsService) Load(ctx context.Context, query EncryptedTrafficStatsQuery) (EncryptedTrafficStats, error) {
	if s.client == nil {
		return encryptedTrafficStatsDTO{}, errors.New("clickhouse is not configured")
	}

	var total, tls, quic, ssh uint64
	row, err := s.client.QueryRow(ctx, `
		SELECT count(),
		       countIf(protocol != 17 AND dst_port IN (443, 8443, 853, 993, 995, 465)),
		       countIf(protocol = 17 AND dst_port IN (443, 8443)),
		       countIf(dst_port = 22)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?`, query.TenantID, query.StartMilli, query.EndMilli)
	if err != nil {
		return encryptedTrafficStatsDTO{}, err
	}
	if err := row.Scan(&total, &tls, &quic, &ssh); err != nil {
		return encryptedTrafficStatsDTO{}, err
	}

	encrypted := tls + quic + ssh
	stats := encryptedTrafficStatsDTO{
		TotalSessions:    int64(encrypted),
		ObservedSessions: int64(total),
		TLSSessions:      int64(tls),
		QUICSessions:     int64(quic),
		JA3Fingerprints:  0,
	}
	if total > 0 {
		stats.EncryptedRatio = float64(encrypted) / float64(total)
	}
	if !s.fingerprintTableAvailable(ctx) {
		return stats, nil
	}

	row, queryErr := s.client.QueryRow(ctx, `
		SELECT uniqExact(ja3),
		       uniqExactIf(ja3, entropy_payload >= 7.5 OR cert_is_self_signed = 1)
		FROM traffic.feature_fp
		WHERE tenant_id=? AND is_encrypted=1 AND ja3!=''
		  AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=?`, query.TenantID, query.StartMilli, query.EndMilli)
	if queryErr == nil {
		var fingerprints, malicious uint64
		if scanErr := row.Scan(&fingerprints, &malicious); scanErr == nil {
			stats.JA3Fingerprints = int64(fingerprints)
			stats.MaliciousJA3Matches = int64(malicious)
		}
	}
	return stats, nil
}

func (s *clickHouseEncryptedTrafficStatsService) fingerprintTableAvailable(ctx context.Context) bool {
	if s.client == nil {
		return false
	}
	var count uint64
	row, err := s.client.QueryRow(ctx, `SELECT count() FROM system.tables WHERE database='traffic' AND name='feature_fp'`)
	return err == nil && row.Scan(&count) == nil && count > 0
}
