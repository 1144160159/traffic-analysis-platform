package dlq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

const defaultReplayIdempotencyPrefix = "dlq:replay:idempotency:"

type replayRedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type RedisReplayIdempotencyStore struct {
	client replayRedisClient
	prefix string
	ttl    time.Duration
	logger *zap.Logger
}

func NewRedisReplayIdempotencyStore(client redis.UniversalClient, prefix string, ttl time.Duration, logger *zap.Logger) *RedisReplayIdempotencyStore {
	return newRedisReplayIdempotencyStore(client, prefix, ttl, logger)
}

func newRedisReplayIdempotencyStore(client replayRedisClient, prefix string, ttl time.Duration, logger *zap.Logger) *RedisReplayIdempotencyStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	if prefix == "" {
		prefix = defaultReplayIdempotencyPrefix
	}
	if ttl <= 0 {
		ttl = config.DefaultDLQReplayIdempotencyTTL
	}
	return &RedisReplayIdempotencyStore{
		client: client,
		prefix: prefix,
		ttl:    ttl,
		logger: logger,
	}
}

func (s *RedisReplayIdempotencyStore) Get(ctx context.Context, key string) (ReplayResult, bool, error) {
	if s == nil || s.client == nil {
		return ReplayResult{}, false, fmt.Errorf("redis replay idempotency store is not configured")
	}

	raw, err := s.client.Get(ctx, s.redisKey(key)).Result()
	if err == redis.Nil {
		return ReplayResult{}, false, nil
	}
	if err != nil {
		return ReplayResult{}, false, err
	}

	var result ReplayResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ReplayResult{}, false, fmt.Errorf("decode replay idempotency record: %w", err)
	}
	return result, true, nil
}

func (s *RedisReplayIdempotencyStore) Put(ctx context.Context, key string, result ReplayResult) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis replay idempotency store is not configured")
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode replay idempotency record: %w", err)
	}
	if err := s.client.Set(ctx, s.redisKey(key), payload, s.ttl).Err(); err != nil {
		return err
	}
	s.logger.Debug("Stored DLQ replay idempotency record",
		zap.String("replay_id", result.ReplayID),
		zap.Duration("ttl", s.ttl))
	return nil
}

func (s *RedisReplayIdempotencyStore) redisKey(key string) string {
	key = strings.TrimSpace(key)
	sum := sha256.Sum256([]byte(key))
	return s.prefix + hex.EncodeToString(sum[:])
}
