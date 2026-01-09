////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/quota/local_bucket.go
////////////////////////////////////////////////////////////////////////////////

package quota

import (
	"sync"
	"sync/atomic"
	"time"
)

// LocalTokenBucket 本地令牌桶（基于 sync/atomic 实现）
// 用于 Redis 不可用时的降级限流
type LocalTokenBucket struct {
	// 令牌桶参数
	capacity   float64 // 桶容量
	refillRate float64 // 每秒补充速率

	// 状态（使用 atomic）
	tokens     int64 // 当前令牌数 * 1000000（使用整数避免浮点原子操作）
	lastRefill int64 // 上次补充时间（UnixNano）

	// 锁（用于补充操作）
	mu sync.Mutex
}

const tokenScale = 1000000 // 令牌缩放因子

// NewLocalTokenBucket 创建本地令牌桶
func NewLocalTokenBucket(capacity, refillRate float64) *LocalTokenBucket {
	return &LocalTokenBucket{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     int64(capacity * tokenScale),
		lastRefill: time.Now().UnixNano(),
	}
}

// Allow 尝试获取令牌
func (b *LocalTokenBucket) Allow() bool {
	return b.AllowN(1)
}

// AllowN 尝试获取 n 个令牌
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

// refill 补充令牌
func (b *LocalTokenBucket) refill() {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&b.lastRefill)

	// 计算经过的时间（秒）
	elapsed := float64(now-last) / float64(time.Second)
	if elapsed <= 0 {
		return
	}

	// 计算需要补充的令牌
	tokensToAdd := int64(elapsed * b.refillRate * tokenScale)
	if tokensToAdd <= 0 {
		return
	}

	// 尝试更新（乐观锁）
	b.mu.Lock()
	defer b.mu.Unlock()

	// 双重检查
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

// Available 获取当前可用令牌数
func (b *LocalTokenBucket) Available() float64 {
	b.refill()
	return float64(atomic.LoadInt64(&b.tokens)) / tokenScale
}

// Reset 重置令牌桶
func (b *LocalTokenBucket) Reset() {
	atomic.StoreInt64(&b.tokens, int64(b.capacity*tokenScale))
	atomic.StoreInt64(&b.lastRefill, time.Now().UnixNano())
}

// LocalBucketManager 本地令牌桶管理器
type LocalBucketManager struct {
	buckets sync.Map // map[string]*LocalTokenBucket

	// 默认配置
	defaultCapacity   float64
	defaultRefillRate float64
}

// NewLocalBucketManager 创建本地令牌桶管理器
func NewLocalBucketManager(defaultCapacity, defaultRefillRate float64) *LocalBucketManager {
	return &LocalBucketManager{
		defaultCapacity:   defaultCapacity,
		defaultRefillRate: defaultRefillRate,
	}
}

// GetBucket 获取或创建令牌桶
func (m *LocalBucketManager) GetBucket(key string) *LocalTokenBucket {
	if bucket, ok := m.buckets.Load(key); ok {
		return bucket.(*LocalTokenBucket)
	}

	// 创建新桶
	bucket := NewLocalTokenBucket(m.defaultCapacity, m.defaultRefillRate)
	actual, _ := m.buckets.LoadOrStore(key, bucket)
	return actual.(*LocalTokenBucket)
}

// Allow 检查是否允许
func (m *LocalBucketManager) Allow(key string) bool {
	return m.GetBucket(key).Allow()
}

// AllowN 检查是否允许 n 个请求
func (m *LocalBucketManager) AllowN(key string, n int) bool {
	return m.GetBucket(key).AllowN(n)
}

// Reset 重置指定桶
func (m *LocalBucketManager) Reset(key string) {
	if bucket, ok := m.buckets.Load(key); ok {
		bucket.(*LocalTokenBucket).Reset()
	}
}

// Clear 清空所有桶
func (m *LocalBucketManager) Clear() {
	m.buckets.Range(func(key, value interface{}) bool {
		m.buckets.Delete(key)
		return true
	})
}

// Size 获取桶数量
func (m *LocalBucketManager) Size() int {
	count := 0
	m.buckets.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Stats 获取统计信息
func (m *LocalBucketManager) Stats() map[string]float64 {
	stats := make(map[string]float64)
	m.buckets.Range(func(key, value interface{}) bool {
		bucket := value.(*LocalTokenBucket)
		stats[key.(string)] = bucket.Available()
		return true
	})
	return stats
}
