package risk

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// getTestDB 获取 PostgreSQL 连接 (K8s 环境自动连接)
func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	host := os.Getenv("PG_HOST")
	if host == "" {
		host = "postgres.databases.svc"
	}
	port := os.Getenv("PG_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("PG_USER")
	if user == "" {
		user = "postgres"
	}
	pass := os.Getenv("PG_PASSWORD")
	if pass == "" {
		t.Skip("PG_PASSWORD is required for PostgreSQL-backed risk tests")
	}
	dbname := os.Getenv("PG_DATABASE")
	if dbname == "" {
		dbname = "traffic_platform"
	}

	dsn := "host=" + host + " port=" + port + " user=" + user + " password=" + pass +
		" dbname=" + dbname + " sslmode=disable connect_timeout=5"
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Logf("PG unavailable (%s:%s)", host, port)
		if db != nil {
			db.Close()
		}
		return nil
	}
	t.Logf("PG connected: %s:%s/%s", host, port, dbname)
	return db
}

// getClickHouseDB 获取 ClickHouse 原生协议连接
func getClickHouseDB(t *testing.T) *sql.DB {
	t.Helper()
	host := os.Getenv("CH_HOST")
	if host == "" {
		host = "clickhouse-1.middleware.svc"
	}
	port := os.Getenv("CH_PORT")
	if port == "" {
		port = "9000"
	}

	dsn := "clickhouse://default:@" + host + ":" + port + "/traffic?dial_timeout=5s&read_timeout=10s"
	db, err := sql.Open("clickhouse", dsn)
	if err != nil || db.Ping() != nil {
		t.Logf("CH unavailable (%s:%s)", host, port)
		if db != nil {
			db.Close()
		}
		return nil
	}
	t.Logf("CH connected: %s:%s/traffic", host, port)
	return db
}

func TestRiskLevelClassification(t *testing.T) {
	tests := []struct {
		score float64
		level string
	}{
		{95, "critical"}, {80, "critical"}, {75, "high"},
		{60, "high"}, {50, "medium"}, {30, "medium"}, {20, "low"}, {0, "low"},
	}
	scorer := NewAssetRiskScorer(nil, nil, zap.NewNop())
	for _, tc := range tests {
		level := scorer.levelFromScore(tc.score)
		if level != tc.level {
			t.Errorf("score %.0f → got %s, want %s", tc.score, level, tc.level)
		}
	}
}

func TestSortByScore(t *testing.T) {
	scores := []*AssetRiskScore{
		{AssetID: "a", TotalScore: 30},
		{AssetID: "b", TotalScore: 95},
		{AssetID: "c", TotalScore: 60},
		{AssetID: "d", TotalScore: 10},
	}
	sortByScore(scores)
	if scores[0].AssetID != "b" {
		t.Errorf("top should be b(95), got %s", scores[0].AssetID)
	}
	if scores[3].AssetID != "d" {
		t.Errorf("last should be d(10), got %s", scores[3].AssetID)
	}
}

func TestScoreAssetRequiresBothDBs(t *testing.T) {
	pgDB := getTestDB(t)
	if pgDB == nil {
		t.Skip("No PostgreSQL connection available (set PG_HOST or run in K8s Pod)")
	}
	defer pgDB.Close()

	chDB := getClickHouseDB(t)
	if chDB == nil {
		// CH unavailable → MUST return error (not fake score)
		scorer := NewAssetRiskScorer(nil, pgDB, zap.NewNop())
		_, err := scorer.ScoreAsset(t.Context(), "campus-net", "10.0.0.1")
		if err == nil {
			t.Fatal("ScoreAsset with nil CH should return error, not fake score")
		}
		t.Logf("Correct: CH unavailable → error: %v", err)
		return
	}
	defer chDB.Close()

	// Both DBs available → real scoring
	scorer := NewAssetRiskScorer(chDB, pgDB, zap.NewNop())
	score, err := scorer.ScoreAsset(t.Context(), "campus-net", "10.0.0.1")
	if err != nil {
		t.Fatalf("ScoreAsset failed: %v", err)
	}
	if score.TotalScore < 0 || score.TotalScore > 100 {
		t.Errorf("TotalScore out of range: %.2f", score.TotalScore)
	}
	if score.RiskLevel == "" {
		t.Error("RiskLevel should not be empty")
	}
	t.Logf("Asset risk (CH+PG): score=%.1f, level=%s, alerts=%d, critical=%d",
		score.TotalScore, score.RiskLevel, score.ActiveAlerts, score.CriticalAlerts)
}

func TestRiskSummaryWithDB(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Skip("No PostgreSQL connection available (set PG_HOST or run in K8s Pod)")
	}
	defer db.Close()

	// 检查 assets 表是否存在
	var tableExists bool
	_ = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'assets')").Scan(&tableExists)
	if !tableExists {
		t.Skip("assets table not found in database (Asset Service schema not initialized)")
	}

	scorer := NewAssetRiskScorer(nil, db, zap.NewNop())
	summary, err := scorer.GetRiskSummary(t.Context(), "campus-net", 10)
	if err != nil {
		t.Fatalf("GetRiskSummary failed: %v", err)
	}
	t.Logf("Total assets: %d, risk distribution: %v",
		summary.TotalAssets, summary.RiskDistribution)
	for i, a := range summary.TopRiskyAssets {
		if i >= 3 {
			break
		}
		t.Logf("  %d. %s — score:%.1f, level:%s",
			i+1, a.IPAddress, a.TotalScore, a.RiskLevel)
	}
}
