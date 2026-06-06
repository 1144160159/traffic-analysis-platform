package stress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dedup"
	"go.uber.org/zap"
)

func TestStressDedup(t *testing.T) {
	cfg := dedup.DefaultDedupConfig()
	cfg.LocalCacheSize = 500000
	cfg.RedisEnabled = false
	d, err := dedup.NewDeduplicator(cfg, nil, zap.NewNop())
	if err != nil { t.Fatal(err) }

	ctx := context.Background()
	concurrency := 16
	count := 100000
	var wg sync.WaitGroup
	var dupCount int64

	start := time.Now()
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < count/concurrency; i++ {
				if d.IsDuplicate(ctx, fmt.Sprintf("event-%d-%d", base, i)) {
					atomic.AddInt64(&dupCount, 1)
				}
			}
		}(g)
	}
	wg.Wait()
	elapsed := time.Since(start)

	s := d.GetStats()
	total := s.HitLocal + s.MissTotal
	rate := float64(total) / elapsed.Seconds()
	t.Logf("Dedup: %d ops/%.2fs = %.0f ops/s | hit=%d miss=%d dup=%d",
		total, elapsed.Seconds(), rate, s.HitLocal, s.MissTotal, dupCount)
	if rate < 100000 { t.Errorf("throughput %.0f < 100K/s", rate) }
}

func TestStressErrorCreation(t *testing.T) {
	concurrency := 16
	count := 50000
	var wg sync.WaitGroup
	start := time.Now()
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < count/concurrency; i++ {
				_ = errors.Newf(errors.ErrCodeInvalidRequest, "err-%d", i)
			}
		}()
	}
	wg.Wait()
	rate := float64(count) / time.Since(start).Seconds()
	t.Logf("Error: %d errors/%.2fs = %.0f/s", count, time.Since(start).Seconds(), rate)
	if rate < 100000 { t.Errorf("throughput %.0f < 100K/s", rate) }
}
