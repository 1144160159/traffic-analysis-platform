package quota

import (
	"sync"
	"sync/atomic"
	"time"
)

type LocalTokenBucket struct {
	capacity   float64
	refillRate float64

	tokens     int64
	lastRefill int64

	mu sync.Mutex
}

const tokenScale = 1000000

func NewLocalTokenBucket(capacity, refillRate float64) *LocalTokenBucket {
	return &LocalTokenBucket{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     int64(capacity * tokenScale),
		lastRefill: time.Now().UnixNano(),
	}
}

func (b *LocalTokenBucket) Allow() bool {
	return b.AllowN(1)
}

func (b *LocalTokenBucket) AllowN(n int) bool {
	b.refill()

	requested := int64(n * tokenScale)
	for {
		current := atomic.LoadInt64(&b.tokens)
		if current < requested {
			return false
		}
		if atomic.CompareAndSwapInt64(&b.tokens, current, current-requested) {
			return true
		}
	}
}

func (b *LocalTokenBucket) refill() {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&b.lastRefill)

	elapsed := float64(now-last) / float64(time.Second)
	if elapsed <= 0 {
		return
	}

	tokensToAdd := int64(elapsed * b.refillRate * tokenScale)
	if tokensToAdd <= 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	last = atomic.LoadInt64(&b.lastRefill)
	elapsed = float64(now-last) / float64(time.Second)
	tokensToAdd = int64(elapsed * b.refillRate * tokenScale)

	if tokensToAdd > 0 {
		current := atomic.LoadInt64(&b.tokens)
		maxTokens := int64(b.capacity * tokenScale)
		newTokens := current + tokensToAdd
		if newTokens > maxTokens {
			newTokens = maxTokens
		}
		atomic.StoreInt64(&b.tokens, newTokens)
		atomic.StoreInt64(&b.lastRefill, now)
	}
}

func (b *LocalTokenBucket) Available() float64 {
	b.refill()
	return float64(atomic.LoadInt64(&b.tokens)) / tokenScale
}

func (b *LocalTokenBucket) Reset() {
	atomic.StoreInt64(&b.tokens, int64(b.capacity*tokenScale))
	atomic.StoreInt64(&b.lastRefill, time.Now().UnixNano())
}

type LocalBucketManager struct {
	buckets sync.Map

	defaultCapacity   float64
	defaultRefillRate float64
}

func NewLocalBucketManager(defaultCapacity, defaultRefillRate float64) *LocalBucketManager {
	return &LocalBucketManager{
		defaultCapacity:   defaultCapacity,
		defaultRefillRate: defaultRefillRate,
	}
}

func (m *LocalBucketManager) GetBucket(key string) *LocalTokenBucket {
	if bucket, ok := m.buckets.Load(key); ok {
		return bucket.(*LocalTokenBucket)
	}

	bucket := NewLocalTokenBucket(m.defaultCapacity, m.defaultRefillRate)
	actual, _ := m.buckets.LoadOrStore(key, bucket)
	return actual.(*LocalTokenBucket)
}

func (m *LocalBucketManager) Allow(key string) bool {
	return m.GetBucket(key).Allow()
}

func (m *LocalBucketManager) AllowN(key string, n int) bool {
	return m.GetBucket(key).AllowN(n)
}

func (m *LocalBucketManager) Reset(key string) {
	if bucket, ok := m.buckets.Load(key); ok {
		bucket.(*LocalTokenBucket).Reset()
	}
}

func (m *LocalBucketManager) Clear() {
	m.buckets.Range(func(key, value interface{}) bool {
		m.buckets.Delete(key)
		return true
	})
}

func (m *LocalBucketManager) Size() int {
	count := 0
	m.buckets.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (m *LocalBucketManager) Stats() map[string]float64 {
	stats := make(map[string]float64)
	m.buckets.Range(func(key, value interface{}) bool {
		bucket := value.(*LocalTokenBucket)
		stats[key.(string)] = bucket.Available()
		return true
	})
	return stats
}
