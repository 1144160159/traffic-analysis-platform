package dedup

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDefaultDedupConfig(t *testing.T) {
	cfg := DefaultDedupConfig()
	if cfg.LocalCacheSize <= 0 { t.Error("LocalCacheSize must be positive") }
	if cfg.LocalTTL <= 0 { t.Error("LocalTTL must be positive") }
}

func TestNewDeduplicator(t *testing.T) {
	cfg := DefaultDedupConfig(); cfg.RedisEnabled = false
	d, err := NewDeduplicator(cfg, nil, zap.NewNop())
	if err != nil { t.Fatalf("NewDeduplicator: %v", err) }
	if d == nil { t.Fatal("nil") }
}

func TestDedupCheckThenMark(t *testing.T) {
	cfg := DefaultDedupConfig(); cfg.RedisEnabled = false
	d, _ := NewDeduplicator(cfg, nil, zap.NewNop())
	ctx := context.Background()
	if d.IsDuplicate(ctx, "event-1") { t.Error("first check: should not be dup") }
	d.MarkSeen(ctx, "event-1") // mark after processing
	if !d.IsDuplicate(ctx, "event-1") { t.Error("second check: should be dup after mark") }
}

func TestDedupExpiry(t *testing.T) {
	cfg := DefaultDedupConfig(); cfg.LocalTTL = 10 * time.Millisecond; cfg.RedisEnabled = false
	d, _ := NewDeduplicator(cfg, nil, zap.NewNop())
	ctx := context.Background()
	d.MarkSeen(ctx, "expire-me")
	time.Sleep(100 * time.Millisecond)
	if d.IsDuplicate(ctx, "expire-me") { t.Error("should expire after TTL") }
}

func TestDedupStats(t *testing.T) {
	cfg := DefaultDedupConfig(); cfg.RedisEnabled = false
	d, _ := NewDeduplicator(cfg, nil, zap.NewNop())
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		key := "key-" + string(rune('a'+i%50))
		if !d.IsDuplicate(ctx, key) { d.MarkSeen(ctx, key) }
	}
	s := d.GetStats()
	t.Logf("hit_local=%d miss=%d dup=%d rate=%.2f", s.HitLocal, s.MissTotal, s.DupDropped, s.DedupRate())
	if s.HitLocal+s.MissTotal < 100 { t.Errorf("too few ops: h=%d m=%d", s.HitLocal, s.MissTotal) }
}

func BenchmarkDedup(b *testing.B) {
	d, _ := NewDeduplicator(DefaultDedupConfig(), nil, zap.NewNop())
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "bench-" + string(rune('a'+i%1000))
		if !d.IsDuplicate(ctx, key) { d.MarkSeen(ctx, key) }
	}
}
